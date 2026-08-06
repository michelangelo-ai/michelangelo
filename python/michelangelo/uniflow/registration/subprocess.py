"""Subprocess registration module for MaCTL isolation.

This module runs in the user's Python environment as a subprocess,
allowing MaCTL to remain isolated while accessing user dependencies.
Communication occurs via command-line arguments and file I/O.
"""

import argparse
import inspect
import json
import logging
import sys
from pathlib import Path
from typing import Optional, Union

import yaml

from michelangelo.uniflow.core.utils import LOGGING_FORMAT
from michelangelo.uniflow.registration import register
from michelangelo.uniflow.registration.config_builder import ConfigBuilder
from michelangelo.uniflow.registration.register import prepare_uniflow_input

_logger = logging.getLogger(__name__)


def find_pipeline_conf(config_file_path: str) -> Optional[Path]:
    """Locate the ``pipeline_conf.yaml`` behind this registration, if any.

    Standard (YAML-authored) pipelines are registered from a
    ``pipeline_conf.yaml`` (top-level ``workflow_function``/``task_configs``
    keys — see ``michelangelo.canvas.pipeline.config_loader``). Two accepted
    shapes:

    - ``config_file_path`` is itself a ``pipeline_conf.yaml``.
    - ``config_file_path`` is a Pipeline CRD YAML whose
      ``spec.manifest.filePath`` (or ``path``) names a ``.yaml``/``.yml``
      file, resolved relative to the CRD file's directory and then the
      current directory.

    Args:
        config_file_path: Path to the configuration file mactl handed us.

    Returns:
        Path to the ``pipeline_conf.yaml`` to register from, or None when
        this registration should use the module-import discovery flow
        (custom Python workflows) instead.
    """
    with open(config_file_path) as f:
        raw = yaml.safe_load(f) or {}
    if not isinstance(raw, dict):
        return None
    if "workflow_function" in raw:
        return Path(config_file_path)

    manifest = (raw.get("spec") or {}).get("manifest") or {}
    manifest_path = manifest.get("filePath") or manifest.get("path")
    if not manifest_path or not str(manifest_path).endswith((".yaml", ".yml")):
        return None

    path = Path(manifest_path)
    if path.is_absolute():
        candidates = [path]
    else:
        candidates = [
            Path(config_file_path).resolve().parent / path,
            Path.cwd() / path,
        ]
    for candidate in candidates:
        if not candidate.exists():
            continue
        with open(candidate) as f:
            conf_raw = yaml.safe_load(f) or {}
        if isinstance(conf_raw, dict) and "workflow_function" in conf_raw:
            return candidate
        return None
    return None


def register_yaml_pipeline(
    pipeline_conf_path: Union[str, Path], args: argparse.Namespace
) -> tuple[str, str]:
    """Register a ``pipeline_conf.yaml`` pipeline and write mactl artifacts.

    Builds and uploads the workflow tarball from the resolved workflow
    function, then writes ``uniflow_input.txt`` in the YAML-pipeline manifest
    shape: top-level ``task_configs`` (plus ``workflow_config`` when the
    workflow takes one) and ``environ``. The pipeline-run controller keys on
    ``task_configs`` to pass these to the workflow as positional args
    (``extractWorkflowInputs`` in
    ``go/components/pipelinerun/actors/executeworkflow.go``, which already
    labels this the "Yaml based pipeline" case), so this shape — not the
    ``args``/``kwargs`` shape of custom workflows — is the wire contract for
    YAML pipelines.

    Args:
        pipeline_conf_path: Path to the ``pipeline_conf.yaml`` file.
        args: Parsed CLI namespace (project, pipeline, output_dir,
            storage_url, output_filename, environ).

    Returns:
        Tuple of (remote tarball path, workflow function name).
    """
    from michelangelo.canvas.pipeline.config_loader import load_pipeline_config
    from michelangelo.canvas.pipeline.register import resolve_workflow_call
    from michelangelo.uniflow.core.codec import encoder
    from michelangelo.uniflow.registration.uniflow_tar import prepare_uniflow_tar

    pipeline_config = load_pipeline_config(pipeline_conf_path)
    # resolve_workflow_call binds task functions into the workflow module's
    # globals; the kwargs it returns are keyed by the workflow's own parameter
    # names, but the manifest content must use the fixed task_configs /
    # workflow_config keys, so those are taken from the WorkflowConfig below.
    fn, _ = resolve_workflow_call(pipeline_config)

    remote_path = prepare_uniflow_tar(
        project_name=args.project,
        pipeline_name=args.pipeline,
        output_dir=args.output_dir,
        workflow_function=f"{fn.__module__}.{fn.__name__}",
        workflow_function_obj=fn,
        storage_base_url=args.storage_url,
        output_filename=args.output_filename,
    )

    workflow_config = pipeline_config.workflow_config
    payload = {
        "task_configs": workflow_config.task_configs,
        "environ": json.loads(args.environ) if args.environ else {},
    }
    if workflow_config.workflow_config is not None:
        payload["workflow_config"] = workflow_config.workflow_config

    inputs_path = Path(args.output_dir) / "uniflow_input.txt"
    with open(inputs_path, "w") as f:
        json.dump(payload, f, default=encoder.default, indent=2)
    _logger.info("Wrote YAML-pipeline uniflow input to: %s", inputs_path)

    return remote_path, fn.__name__


def discover_workflow_from_config(config_file_path: str):
    """Discover and import workflow function from pipeline configuration.

    This function reads the pipeline YAML configuration, extracts the manifest.path,
    and discovers the workflow function from ctx.run() calls in that module.

    Args:
        config_file_path: Path to pipeline YAML configuration file

    Returns:
        Callable: The discovered workflow function

    Raises:
        ValueError: If no workflow function found
        ImportError: If module cannot be imported
    """
    _logger.info("Discovering workflow function from config: %s", config_file_path)

    # Read YAML configuration
    with open(config_file_path, "r") as f:
        config = yaml.safe_load(f)

    # Extract manifest path
    manifest_data = config.get("spec", {}).get("manifest", {})
    manifest_path = manifest_data.get("filePath") or manifest_data.get("path")

    if not manifest_path:
        raise ValueError(
            "No manifest.filePath or manifest.path found in pipeline configuration"
        )
    _logger.info("Found manifest path: %s", manifest_path)

    # Import the module
    try:
        import importlib

        module = importlib.import_module(manifest_path)
        _logger.info("Successfully imported module: %s", manifest_path)
    except ImportError as e:
        _logger.error("Failed to import module %s: %s", manifest_path, e)
        raise

    # Find workflow function from ctx.run() calls using AST parsing
    workflow_function_name = None

    try:
        import ast
        import os

        # Get the module file path
        module_file = module.__file__
        if module_file and os.path.exists(module_file):
            with open(module_file, "r") as f:
                source = f.read()

            # Parse the AST to find ctx.run calls
            tree = ast.parse(source)

            for node in ast.walk(tree):
                # Look for calls like: ctx.run(workflow_function, ...)
                if (
                    isinstance(node, ast.Call)
                    and isinstance(node.func, ast.Attribute)
                    and isinstance(node.func.value, ast.Name)
                    and node.func.value.id == "ctx"
                    and node.func.attr == "run"
                ):
                    # Extract the first argument (workflow function name)
                    if len(node.args) > 0 and isinstance(node.args[0], ast.Name):
                        workflow_function_name = node.args[0].id
                        _logger.info(
                            "Found ctx.run() call with function: %s",
                            workflow_function_name,
                        )
                        break  # Only one function expected

    except Exception as e:
        _logger.warning("Failed to parse AST for ctx.run() calls: %s", e)

    # If AST parsing failed, fall back to @workflow decorator discovery
    if not workflow_function_name:
        _logger.info("Falling back to @workflow decorator discovery")

        for name, obj in inspect.getmembers(module):
            if inspect.isfunction(obj):
                # Check if function has @workflow decorator
                if hasattr(obj, "__wrapped__") or hasattr(obj, "_uniflow_workflow"):
                    workflow_function_name = name
                    _logger.info("Found @workflow decorated function: %s", name)
                    break  # Use first one found

    # Validate result
    if not workflow_function_name:
        raise ValueError(
            f"No workflow function found in ctx.run() calls or @workflow decorators in module {manifest_path}"
        )

    # Get the actual function object
    try:
        selected_func = getattr(module, workflow_function_name)
        if not inspect.isfunction(selected_func):
            raise ValueError(f"{workflow_function_name} is not a function")

        _logger.info("Selected workflow function: %s", workflow_function_name)
        return selected_func

    except AttributeError:
        raise ValueError(
            f"Function {workflow_function_name} not found in module {manifest_path}"
        )


def main():
    """Entry point for subprocess registration.

    This function runs in the user's Python environment and handles
    workflow registration independently from the main MaCTL process.
    """
    parser = create_argument_parser()
    args = parser.parse_args()

    # Set up logging
    logging.basicConfig(level=logging.INFO, format=LOGGING_FORMAT)

    try:
        # Standard YAML pipelines (pipeline_conf.yaml) register through the
        # canvas config loader; custom Python workflows keep the
        # module-import discovery flow below.
        pipeline_conf_path = find_pipeline_conf(args.config_file)
        if pipeline_conf_path is not None:
            _logger.info("Detected YAML pipeline configuration: %s", pipeline_conf_path)
            remote_path, workflow_function_name = register_yaml_pipeline(
                pipeline_conf_path, args
            )

            _logger.info("Registration completed successfully")
            _logger.info("Remote tarball path: %s", remote_path)

            success_file = Path(args.output_dir) / "registration_success.txt"
            success_file.write_text(f"SUCCESS: {remote_path}")

            function_name_file = Path(args.output_dir) / "workflow_function_name.txt"
            function_name_file.write_text(workflow_function_name)
            _logger.info("Wrote workflow function name: %s", workflow_function_name)
            return

        # Read pipeline configuration to discover workflow function
        _logger.info("Reading pipeline configuration: %s", args.config_file)
        workflow_fn = discover_workflow_from_config(args.config_file)

        # Create ConfigBuilder to extract workflow configuration
        _logger.info("Creating ConfigBuilder for workflow configuration")
        with ConfigBuilder.from_config_file(args.config_file) as config_builder:
            # Execute registration in user's environment
            remote_path = register(
                fn=workflow_fn,
                project=args.project,
                pipeline=args.pipeline,
                output_dir=args.output_dir,
                storage_url=args.storage_url,
                output_filename=args.output_filename,
                environ=json.loads(args.environ) if args.environ else {},
                args=json.loads(args.args) if args.args else [],
                kwargs=json.loads(args.kwargs) if args.kwargs else {},
            )

            # Generate workflow config JSON for manifest content using ConfigBuilder
            workflow_config = config_builder.get_workflow_config_as_manifest_content()

            # Override with any provided arguments (command line takes precedence)
            final_args = json.loads(args.args) if args.args else workflow_config["args"]

            # Handle kwargs: prepare_uniflow_input expects dict in legacy mode
            if args.kwargs:
                # Command line kwargs are provided as dict
                final_kwargs_dict = json.loads(args.kwargs)
            else:
                # Convert workflow config kwargs from list format [[k,v], [k,v]] to dict
                final_kwargs_dict = dict(workflow_config["kwargs"])

            # Use environment variables from workflow config (extracted from actual workflow)
            final_environ = workflow_config["environ"].copy()

            # Override with any provided environ (command line takes precedence)
            if args.environ:
                final_environ.update(json.loads(args.environ))

            prepare_uniflow_input(
                final_args, final_kwargs_dict, final_environ, args.output_dir
            )

        _logger.info("Registration completed successfully")
        _logger.info("Remote tarball path: %s", remote_path)

        # Write success indicator
        success_file = Path(args.output_dir) / "registration_success.txt"
        success_file.write_text(f"SUCCESS: {remote_path}")

        # Write workflow function name for main process
        function_name_file = Path(args.output_dir) / "workflow_function_name.txt"
        function_name_file.write_text(workflow_fn.__name__)
        _logger.info("Wrote workflow function name: %s", workflow_fn.__name__)

    except ImportError as e:
        error_msg = (
            f"Could not import workflow function from config '{args.config_file}': {e}"
        )
        _logger.error(error_msg)

        # Write failure indicator with specific error type
        error_file = Path(args.output_dir) / "registration_error.txt"
        error_file.write_text(f"ERROR: ImportError - {error_msg}")

        # Exit with specific error code for import failures
        sys.exit(2)
    except ModuleNotFoundError as e:
        error_msg = f"Module not found for workflow function from config '{args.config_file}': {e}"
        _logger.error(error_msg)

        # Write failure indicator
        error_file = Path(args.output_dir) / "registration_error.txt"
        error_file.write_text(f"ERROR: ModuleNotFoundError - {error_msg}")

        # Exit with specific error code for module not found
        sys.exit(3)
    except Exception as e:
        error_msg = f"Registration failed: {e}"
        _logger.error(error_msg)

        # Write failure indicator
        error_file = Path(args.output_dir) / "registration_error.txt"
        error_file.write_text(f"ERROR: {error_msg}")

        # Exit with general error code
        sys.exit(1)


def create_argument_parser() -> argparse.ArgumentParser:
    """Create argument parser for subprocess registration.

    Returns:
        Configured argument parser
    """
    parser = argparse.ArgumentParser(
        description="Subprocess registration for MaCTL pipeline workflows",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
This module is designed to run in the user's Python environment
as a subprocess spawned by MaCTL. It handles workflow registration
while maintaining environment isolation.

Communication Protocol:
- Input: Command-line arguments
- Output: Files in specified output directory
- Success: registration_success.txt + standard output files
- Failure: registration_error.txt + exit code 1
        """,
    )

    # Required arguments
    parser.add_argument(
        "--project", required=True, help="Project name for the pipeline"
    )
    parser.add_argument("--pipeline", required=True, help="Pipeline name")
    parser.add_argument(
        "--config-file", required=True, help="Path to pipeline configuration YAML file"
    )
    parser.add_argument(
        "--output-dir",
        required=True,
        help="Directory to write registration output files",
    )

    # Optional arguments
    parser.add_argument(
        "--storage-url",
        help="Storage URL for uploading tarballs (default: s3://default/uniflow)",
    )
    parser.add_argument(
        "--output-filename",
        help="Name of the tar path output file (default: uniflow_tar_path.txt)",
    )
    parser.add_argument("--environ", help="Environment variables as JSON string")
    parser.add_argument("--args", help="Positional arguments as JSON array")
    parser.add_argument("--kwargs", help="Keyword arguments as JSON object")

    return parser


if __name__ == "__main__":
    main()
