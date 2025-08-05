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

from .uniflow_tar import prepare_uniflow_tar

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
    args: Optional[tuple],
    kwargs: Optional[dict],
    environ: Optional[dict],
    output_dir: str,
):
    """
    Prepare uniflow input file for workflow registration and write it to the output directory.

    This creates a JSON file containing the arguments, keyword arguments, and environment
    variables that will be passed to the workflow function during execution.

    Args:
        args: Positional arguments to pass to the function
        kwargs: Keyword arguments to pass to the function
        environ: Environment variables to set during execution
        output_dir: Directory to write the input file

    Returns:
        str: Path to the created input file
    """
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


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format=LOGGING_FORMAT)
    sys.exit(main())
