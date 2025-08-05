"""
Storage-agnostic implementation for building and uploading Uniflow tarballs.

This module provides the core functionality for packaging Uniflow workflows
into tarballs and uploading them to various storage backends using fsspec.
"""

import logging
import uuid
from typing import Callable, Optional

import fsspec

from michelangelo.uniflow.core.build import build

_logger = logging.getLogger(__name__)


class UniflowTarBuilderImpl:
    """
    Storage-agnostic Uniflow tar builder implementation.

    This class handles the core functionality of building Uniflow packages
    and uploading them to any fsspec-compatible storage backend.
    """

    def __init__(
        self,
        project_name: str,
        pipeline_name: str,
        workflow_function: str,
        workflow_function_obj: Optional[Callable] = None,
        storage_base_path: str = "s3://default/uniflow",
        output_filename: str = "uniflow_tar_path.txt",
    ):
        """
        Initialize the Uniflow tar builder.

        Args:
            project_name: Name of the project
            pipeline_name: Name of the pipeline
            workflow_function: Fully qualified function name (e.g., "module.function")
            workflow_function_obj: Optional callable workflow function object
            storage_base_path: Base path for storing tarballs
            output_filename: Name of the output file containing the tar path
        """
        self.project_name = project_name
        self.pipeline_name = pipeline_name
        self.workflow_function = workflow_function
        self.workflow_function_obj = workflow_function_obj
        self.storage_base_path = storage_base_path.rstrip("/")
        self.output_filename = output_filename
        self._tar_name = None

    def get_random_tar_name(self) -> str:
        """Generate a unique tar filename for this pipeline."""
        if self._tar_name is None:
            random_id = str(uuid.uuid4())[:8]
            self._tar_name = (
                f"{self.project_name}_{self.pipeline_name}_{random_id}.tar.gz"
            )
        return self._tar_name

    def get_tar_name(self) -> str:
        """Get the tar filename (alias for get_random_tar_name for compatibility)."""
        return self.get_random_tar_name()

    def get_remote_tar_path(self) -> str:
        """Get the full remote storage path for the tarball."""
        return f"{self.storage_base_path}/{self.get_random_tar_name()}"

    def build_and_upload_tarball(self) -> str:
        """
        Build the Uniflow package and upload it to storage.

        Returns:
            str: The remote path where the tarball was uploaded

        Raises:
            ValueError: If workflow_function_obj is not provided
            Exception: If build or upload fails
        """
        if not self.workflow_function_obj:
            raise ValueError(
                "workflow_function_obj is required for building. "
                "Please provide the actual function object."
            )

        _logger.info(
            "Building uniflow package for %s.%s",
            self.project_name,
            self.pipeline_name,
        )

        try:
            # Use existing build system from michelangelo.uniflow.core.build
            package = build(self.workflow_function_obj)
            tarball_bytes = package.to_tarball_bytes()

            _logger.info(
                "Built tarball with %d bytes, main function: %s",
                len(tarball_bytes),
                package.main_function,
            )

            # Upload to storage using fsspec
            remote_path = self.get_remote_tar_path()
            _logger.info("Uploading to: %s", remote_path)

            with fsspec.open(remote_path, "wb") as f:
                f.write(tarball_bytes)

            _logger.info("Successfully uploaded tarball to: %s", remote_path)
            return remote_path

        except Exception as e:
            _logger.error("Failed to build and upload tarball: %s", e)
            raise
