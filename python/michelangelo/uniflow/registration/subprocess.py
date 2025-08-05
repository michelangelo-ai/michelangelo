"""
Subprocess registration module for MaCTL isolation.

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

import yaml

from michelangelo.uniflow.core.utils import LOGGING_FORMAT
from michelangelo.uniflow.registration import register
from michelangelo.uniflow.registration.config_builder import ConfigBuilder

_logger = logging.getLogger(__name__)


def discover_workflow_from_config(config_file_path: str):
    """
    Discover and import workflow function from pipeline configuration.

    This function reads the pipeline YAML configuration, extracts the manifest.path,
    and discovers @workflow decorated functions in that module.

    Args:
        config_file_path: Path to pipeline YAML configuration file

    Returns:
        Callable: The discovered workflow function

    Raises:
        ValueError: If no workflow function found or multiple found
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
        # Use importlib to import the module directly
        import importlib

        module = importlib.import_module(manifest_path)
        _logger.info("Successfully imported module: %s", manifest_path)
    except ImportError as e:
        _logger.error("Failed to import module %s: %s", manifest_path, e)
        raise

    # Find @workflow decorated functions
    workflow_functions = []

    for name, obj in inspect.getmembers(module):
        if inspect.isfunction(obj):
            # Check if function has @workflow decorator
            if hasattr(obj, "__wrapped__") or hasattr(obj, "_uniflow_workflow"):
                workflow_functions.append((name, obj))
                _logger.info("Found workflow function: %s", name)

    # Validate results
    if not workflow_functions:
        raise ValueError(
            f"No @workflow decorated functions found in module {manifest_path}"
        )

    if len(workflow_functions) > 1:
        func_names = [name for name, _ in workflow_functions]
        _logger.warning(
            "Multiple workflow functions found: %s. Using first one.", func_names
        )

    selected_name, selected_func = workflow_functions[0]
    _logger.info("Selected workflow function: %s", selected_name)

    return selected_func


def main():
    """
    Entry point for subprocess registration.

    This function runs in the user's Python environment and handles
    workflow registration independently from the main MaCTL process.
    """
    parser = create_argument_parser()
    args = parser.parse_args()

    # Set up logging
    logging.basicConfig(level=logging.INFO, format=LOGGING_FORMAT)

    try:
        # Read pipeline configuration to discover workflow function
        _logger.info("Reading pipeline configuration: %s", args.config_file)
        workflow_fn = discover_workflow_from_config(args.config_file)

        # Create ConfigBuilder to extract workflow arguments
        _logger.info("Analyzing workflow function for argument extraction")
        config_builder = ConfigBuilder(workflow_fn)

        # Get workflow arguments from function analysis
        workflow_args = config_builder.get_workflow_args()
        workflow_kwargs = config_builder.get_workflow_kwargs()
        workflow_environ = config_builder.get_workflow_environ()

        # Override with any provided arguments (command line takes precedence)
        final_environ = json.loads(args.environ) if args.environ else workflow_environ
        final_args = json.loads(args.args) if args.args else workflow_args
        final_kwargs = json.loads(args.kwargs) if args.kwargs else workflow_kwargs

        _logger.info("Starting registration process with extracted arguments")
        _logger.info("Final args: %s", final_args)
        _logger.info("Final kwargs: %s", final_kwargs)
        _logger.info("Final environ: %s", final_environ)

        # Execute registration in user's environment
        remote_path = register(
            fn=workflow_fn,
            project=args.project,
            pipeline=args.pipeline,
            output_dir=args.output_dir,
            storage_url=args.storage_url,
            output_filename=args.output_filename,
            environ=final_environ,
            args=final_args,
            kwargs=final_kwargs,
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
    """
    Create argument parser for subprocess registration.

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
