"""I/O handlers for Ray datasets in Uniflow workflows.

This module provides I/O functionality for reading and writing Ray datasets in Uniflow
workflows. It supports both fsspec and PyArrow filesystem backends for flexible
storage access across different environments (local, S3, etc.).
"""

import os
from typing import Any, Optional

import ray
from michelangelo.uniflow.core.io_registry import IO
from ray.data import Dataset

UF_PLUGIN_RAY_USE_FSSPEC = "UF_PLUGIN_RAY_USE_FSSPEC"
"""
UF_PLUGIN_RAY_USE_FSSPEC is an environment variable that controls whether the
Ray Plugin uses fsspec instead of Ray's default filesystem - pyarrow.
Possible values:
  - 1 to use fsspec
  - 0 to use Ray's default filesystem.

Default is 0.
"""

_DEFAULT_UF_PLUGIN_RAY_USE_FSSPEC = "0"


class RayDatasetIO(IO[Dataset]):
    """I/O handler for Ray Dataset objects.

    This class provides read and write operations for Ray datasets, storing them
    in Parquet format. It supports multiple filesystem backends including local,
    S3, and other storage systems via fsspec or PyArrow.

    The filesystem backend is selected based on the UF_PLUGIN_RAY_USE_FSSPEC
    environment variable.
    """

    def write(self, url: str, ds: Dataset) -> Optional[Any]:
        """Write a Ray dataset to the specified URL in Parquet format.

        Args:
            url: Target URL where the dataset should be written. Supports local paths
                and remote URLs (e.g., s3://bucket/path).
            ds: The Ray dataset to write.

        Returns:
            None. This implementation does not return metadata.
        """
        fs, path = _fs_path(url)
        ds.write_parquet(path, filesystem=fs)
        metadata = None
        return metadata

    def read(self, url: str, metadata: Optional[Any]) -> Dataset:
        """Read a Ray dataset from the specified URL.

        Args:
            url: Source URL from which to read the dataset. Supports local paths
                and remote URLs (e.g., s3://bucket/path).
            metadata: Optional metadata from write operation. Currently unused
                and expected to be None.

        Returns:
            The loaded Ray dataset.
        """
        assert metadata is None
        fs, path = _fs_path(url)
        return ray.data.read_parquet(path, filesystem=fs, file_extensions=["parquet"])


def _fs_path(url: str) -> tuple[Any, str]:
    if os.environ.get(UF_PLUGIN_RAY_USE_FSSPEC, "0") == "1":
        import fsspec

        return fsspec.core.url_to_fs(url)

    return resolve_fs(url.split("://")[0]), url


def resolve_fs(protocol):
    """Resolve filesystem handler for a given protocol.

    Args:
        protocol: The URL protocol (e.g., "s3", "file").

    Returns:
        A filesystem object for the specified protocol, or None if the protocol
        doesn't require special handling.
    """
    if protocol == "s3":
        import pyarrow.fs

        # Configure PyArrow's S3FileSystem for MinIO
        return pyarrow.fs.S3FileSystem(
            access_key=os.getenv("AWS_ACCESS_KEY_ID"),
            secret_key=os.getenv("AWS_SECRET_ACCESS_KEY"),
            endpoint_override=os.getenv("AWS_ENDPOINT_URL"),
            allow_bucket_creation=os.getenv("S3_ALLOW_BUCKET_CREATION"),
        )
    return None
