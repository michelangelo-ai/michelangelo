"""
CLI program for registering Uniflow workflows with Michelangelo.

This module provides the main registration interface that builds, uploads,
and prepares Uniflow workflows for pipeline creation via mactl.
"""

import argparse
import json
import logging
import os
import sys
from typing import Callable, Optional

from michelangelo.uniflow.core.codec import encoder
from michelangelo.uniflow.core.utils import LOGGING_FORMAT, import_attribute

from michelangelo.uniflow.registration.uniflow_tar import prepare_uniflow_tar
from michelangelo.uniflow.registration.config_builder import ConfigBuilder

_logger = logging.getLogger(__name__)


def main(args=None):
    """
    CLI program to register the given workflow.
    
    Usage:
        python -m michelangelo.uniflow.registration.register \
            --project my-project \
            --pipeline my-pipeline \
            --output-dir /tmp/pipeline-artifacts \
            my_module.my_workflow_function
    """
    p = register_argument_parser()
    ns = p.parse_args(args=args).__dict__

    # Extract and import the function
    fn_path = ns.pop("fn")
    fn = import_attribute(fn_path)

    register(fn=fn, **ns)


def prepare_uniflow_input(
    config_builder_or_args,
    kwargs_or_output_dir=None,
    environ=None,
    output_dir=None,
):
    """
    Prepare uniflow input file for workflow registration.

    This function supports two calling patterns:
    1. New pattern: prepare_uniflow_input(config_builder, output_dir)
    2. Legacy pattern: prepare_uniflow_input(args, kwargs, environ, output_dir)

    Args:
        config_builder_or_args: Either ConfigBuilder instance or positional args (legacy)
        kwargs_or_output_dir: Either output directory (new) or kwargs dict (legacy)
        environ: Environment variables (legacy only)
        output_dir: Output directory (legacy only)

    Returns:
        str: Path to the created input file
    """
    # Detect calling pattern
    if isinstance(config_builder_or_args, ConfigBuilder):
        # New pattern: prepare_uniflow_input(config_builder, output_dir)
        config_builder = config_builder_or_args
        output_dir = kwargs_or_output_dir

        _logger.info("Preparing uniflow input JSON using ConfigBuilder")

        # Get workflow configuration as JSON
        workflow_config_json = config_builder.get_workflow_config_in_json()

        # Write to uniflow input file
        inputs_path = os.path.join(output_dir, "uniflow_input.txt")
        with open(inputs_path, "w") as f:
            f.write(workflow_config_json)

        _logger.info("Wrote uniflow input to: %s", inputs_path)
        return inputs_path

    else:
        # Legacy pattern: prepare_uniflow_input(args, kwargs, environ, output_dir)
        args = config_builder_or_args
        kwargs = kwargs_or_output_dir

        _logger.info("Preparing uniflow input JSON using legacy pattern")

        _kwargs = list(kwargs.items()) if kwargs else []
        inputs = {"args": args or (), "kwargs": _kwargs, "environ": environ or {}}

        inputs_path = os.path.join(output_dir, "uniflow_input.txt")
        with open(inputs_path, "w") as f:
            json.dump(inputs, f, default=encoder.default, indent=2)

        _logger.info("Wrote uniflow input to: %s", inputs_path)
        return inputs_path


def register_argument_parser():
    """
    Creates an argument parser for the Pipeline Registration CLI.

    Returns:
        argparse.ArgumentParser: Configured argument parser
    """
    p = argparse.ArgumentParser(
        description="Register a Uniflow workflow with Michelangelo",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Register a workflow function
  python -m michelangelo.uniflow.registration.register \\
    --project my-project \\
    --pipeline my-pipeline \\
    --output-dir /tmp/artifacts \\
    my_module.train_model

  # Register with custom storage
  python -m michelangelo.uniflow.registration.register \\
    --project my-project \\
    --pipeline my-pipeline \\
    --output-dir /tmp/artifacts \\
    --storage-url s3://my-bucket/uniflow \\
    my_module.train_model

After registration, use mactl to create the pipeline:
  mactl pipeline create pipeline.yaml
        """,
    )

    p.add_argument(
        "fn",
        type=str,
        help="Fully qualified function name (e.g., 'my_module.my_function')",
    )
    p.add_argument("--project", required=True, help="The project name")
    p.add_argument("--pipeline", required=True, help="The pipeline name")
    p.add_argument(
        "--output-dir",
        required=True,
        help="Directory to write pipeline artifacts for mactl consumption",
    )
    p.add_argument(
        "--storage-url",
        help="Storage URL for uploading tarballs (default: s3://default/uniflow)",
    )
    p.add_argument(
        "--output-filename",
        help="Name of the tar path output file (default: uniflow_tar_path.txt)",
    )
    p.add_argument(
        "--environ", type=json.loads, help="Environment variables as JSON string"
    )
    p.add_argument("--args", type=json.loads, help="Positional arguments as JSON array")
    p.add_argument("--kwargs", type=json.loads, help="Keyword arguments as JSON object")

    return p


def register(
    *,
    fn: Callable,
    project: str,
    pipeline: str,
    output_dir: str,
    storage_url: Optional[str] = None,
    output_filename: Optional[str] = None,
    environ: Optional[dict] = None,
    args: Optional[tuple] = None,
    kwargs: Optional[dict] = None,
):
    """
    Register the workflow function in the specified context.

    This function builds a Uniflow package from the workflow function,
    uploads it to storage, and creates the necessary files for mactl
    to create a pipeline.

    Args:
        fn: The workflow function to register
        project: The project name
        pipeline: The pipeline name
        output_dir: The output directory for artifacts
        storage_url: Optional storage URL (defaults to s3://default/uniflow)
        output_filename: Optional output filename (defaults to uniflow_tar_path.txt)
        environ: Environment variables to pass to the workflow
        args: Positional arguments to pass to the workflow function
        kwargs: Keyword arguments to pass to the workflow function

    Returns:
        str: The remote path where the tarball was uploaded

    Raises:
        ValueError: If required parameters are missing
        Exception: If registration fails
    """
    if not fn:
        raise ValueError("Function (fn) is required")

    # Set defaults
    environ = environ or {}
    args = args or ()
    kwargs = kwargs or {}

    _logger.info(
        "Registering workflow: %s.%s for project=%s, pipeline=%s",
        fn.__module__,
        fn.__name__,
        project,
        pipeline,
    )

    # Build the workflow function identifier
    workflow_function = f"{fn.__module__}.{fn.__name__}"

    try:
        # Prepare and upload the uniflow tarball
        remote_path = prepare_uniflow_tar(
            project_name=project,
            pipeline_name=pipeline,
            output_dir=output_dir,
            workflow_function=workflow_function,
            workflow_function_obj=fn,
            storage_base_url=storage_url,
            output_filename=output_filename,
        )

        # Prepare the input configuration
        prepare_uniflow_input(args, kwargs, environ, output_dir)

        _logger.info("Registration completed successfully")
        _logger.info("Tarball uploaded to: %s", remote_path)
        _logger.info("Artifacts written to: %s", output_dir)
        _logger.info(
            "Next step: Use mactl to create the pipeline with the generated artifacts"
        )

        return remote_path

    except Exception as e:
        _logger.error("Registration failed: %s", e)
        raise


def register_pipeline(
    project: str, pipeline: str, output_dir: str, workflow_config: str
):
    """
    Main registration function following the specification pattern.

    This function implements the complete registration process:
    1. Create and upload workflow tarball
    2. Generate workflow config JSON
    3. Generate task image metadata (skipped for now)

    Args:
        project: Project name
        pipeline: Pipeline name
        output_dir: Directory to write artifacts
        workflow_config: Path to workflow configuration file

    Returns:
        str: Remote path to uploaded tarball
    """
    _logger.info(
        "Starting pipeline registration for project=%s, pipeline=%s", project, pipeline
    )

    with ConfigBuilder.from_config_file(workflow_config) as wf_conf_builder:
        # 1. Create and upload workflow tarball
        remote_path = prepare_uniflow_tar(
            project_name=project,
            pipeline_name=pipeline,
            output_dir=output_dir,
            workflow_function=wf_conf_builder.workflow_function,
            workflow_function_obj=wf_conf_builder.get_workflow_func_with_task_override(),
            storage_base_url="s3://default/uniflow",  # Hard-coded for now
        )

        # 2. Generate workflow config JSON
        prepare_uniflow_input(wf_conf_builder, output_dir)

        # 3. Generate task image metadata (skipped as requested)
        # prepare_task_images(wf_conf_builder, output_dir)

        _logger.info("Pipeline registration completed successfully")
        _logger.info("Tarball uploaded to: %s", remote_path)

        return remote_path


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format=LOGGING_FORMAT)
    sys.exit(main())
