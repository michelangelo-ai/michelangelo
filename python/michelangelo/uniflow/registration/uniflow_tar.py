"""
Michelangelo-specific wrapper for uniflow tar building with S3 defaults.

This module provides backward compatibility for Michelangelo internal usage
while delegating to the storage-agnostic implementation in uniflow_tar_impl.py.
"""

import logging
import os
from typing import Callable, Optional

# Import the storage-agnostic implementation
from .uniflow_tar_impl import UniflowTarBuilderImpl

# Check for s3fs availability
try:
    import s3fs
except ImportError:
    s3fs = None

_logger = logging.getLogger(__name__)
_UNIFLOW_TAR_PATH_FILENAME = "uniflow_tar_path.txt"
_DEFAULT_S3_PATH = "s3://default/uniflow"


class UniflowTarBuilder:
    """
    Michelangelo-specific UniflowTarBuilder with S3 defaults.

    This class maintains compatibility with existing usage patterns
    while delegating to the storage-agnostic UniflowTarBuilderImpl.
    Uses S3 with MinIO defaults for Michelangelo environments.
    """

    def __init__(
        self,
        project_name: str,
        pipeline_name: str,
        workflow_function: str,
        workflow_function_obj: Optional[Callable] = None,
        storage_base_url: Optional[str] = None,
        output_filename: Optional[str] = None,
    ):
        """
        Initialize the Michelangelo Uniflow tar builder.

        Args:
            project_name: Name of the project
            pipeline_name: Name of the pipeline
            workflow_function: Fully qualified function name
            workflow_function_obj: Optional callable workflow function object
            storage_base_url: Optional storage URL. Defaults to s3://default/uniflow
            output_filename: Name of output file. Defaults to uniflow_tar_path.txt
        """
        # Use S3 with MinIO defaults for Michelangelo
        if storage_base_url is None:
            storage_base_url = _DEFAULT_S3_PATH
            _logger.debug("Using default S3 storage: %s", storage_base_url)

        # Use Michelangelo-specific filename by default
        if output_filename is None:
            output_filename = _UNIFLOW_TAR_PATH_FILENAME

        # Check s3fs availability if using S3
        if storage_base_url.startswith("s3://") and s3fs is None:
            _logger.warning(
                "s3fs not available. Install with: pip install s3fs"
                " or add s3fs to your dependencies."
            )

        # Delegate to storage-agnostic implementation
        self._impl = UniflowTarBuilderImpl(
            project_name=project_name,
            pipeline_name=pipeline_name,
            workflow_function=workflow_function,
            workflow_function_obj=workflow_function_obj,
            storage_base_path=storage_base_url,
            output_filename=output_filename,
        )

    def get_random_tar_name(self) -> str:
        """Delegate to underlying implementation."""
        return self._impl.get_random_tar_name()

    def get_tar_name(self) -> str:
        """Delegate to underlying implementation."""
        return self._impl.get_tar_name()

    def get_remote_tar_path(self) -> str:
        """Delegate to underlying implementation."""
        return self._impl.get_remote_tar_path()

    def build_and_upload_tarball(self) -> str:
        """Delegate to underlying implementation."""
        return self._impl.build_and_upload_tarball()


def prepare_uniflow_tar(
    project_name: str,
    pipeline_name: str,
    output_dir: str,
    workflow_function: str,
    workflow_function_obj: Optional[Callable] = None,
    storage_base_url: Optional[str] = None,
    output_filename: Optional[str] = None,
):
    """
    Prepare and upload a Uniflow tarball for Michelangelo pipeline registration.

    This function builds a Uniflow package, uploads it to storage, and writes
    the storage path to a file for mactl consumption.

    Args:
        project_name: Name of the project
        pipeline_name: Name of the pipeline
        output_dir: Directory to write the tar path file
        workflow_function: Fully qualified function name (e.g., "module.function")
        workflow_function_obj: Optional callable workflow function object
        storage_base_url: Optional storage URL. Defaults to s3://default/uniflow
        output_filename: Name of output file. Defaults to uniflow_tar_path.txt

    Returns:
        str: The remote path where the tarball was uploaded

    Raises:
        ValueError: If workflow_function_obj is not provided
        Exception: If build, upload, or file write fails
    """
    _logger.info(
        "Preparing uniflow tar for project=%s, pipeline=%s",
        project_name,
        pipeline_name,
    )

    # Create tar builder with Michelangelo-specific defaults
    tar_builder = UniflowTarBuilder(
        project_name=project_name,
        pipeline_name=pipeline_name,
        workflow_function=workflow_function,
        workflow_function_obj=workflow_function_obj,
        storage_base_url=storage_base_url,
        output_filename=output_filename,
    )

    # Build and upload the tarball
    remote_path = tar_builder.build_and_upload_tarball()

    # Ensure output directory exists
    os.makedirs(output_dir, exist_ok=True)

    # Use the filename from the builder (either provided or default)
    actual_filename = (
        output_filename if output_filename is not None else _UNIFLOW_TAR_PATH_FILENAME
    )
    output_path = os.path.join(output_dir, actual_filename)

    # Write the remote path to file for mactl consumption
    with open(output_path, "w") as f:
        f.write(remote_path)

    _logger.info("Wrote tar path to: %s", output_path)
    _logger.info("Remote tarball location: %s", remote_path)

    return remote_path
