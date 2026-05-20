"""Storage backend abstractions for artifact upload and download."""

from __future__ import annotations

import os
import shutil
from abc import ABC, abstractmethod


class StorageBackend(ABC):
    """Abstract base class for artifact storage backends.

    Implementations are infrastructure-specific (local filesystem, S3, GCS,
    object store). After a successful ``upload()``, the returned URI must be
    passable to ``download()`` to retrieve the same artifact.

    Example implementation::

        class S3StorageBackend(StorageBackend):
            def __init__(self, bucket: str) -> None:
                self._bucket = bucket

            def upload(self, local_path: str, destination_key: str) -> str:
                uri = f"s3://{self._bucket}/{destination_key}"
                # boto3 upload logic here
                return uri

            def download(self, uri: str, local_path: str) -> None:
                # boto3 download logic here
                pass
    """

    @abstractmethod
    def upload(self, local_path: str, destination_key: str) -> str:
        """Upload a local file or directory to the storage backend.

        Args:
            local_path: Absolute path to the local file or directory to upload.
            destination_key: Logical key identifying the artifact within this
                backend (e.g. ``"models/my-classifier/v3"``). Parent
                directories are created automatically.

        Returns:
            A URI string uniquely identifying the uploaded artifact. This URI
            must be accepted by ``download()`` on the same backend instance.

        Raises:
            IOError: If the upload fails due to a filesystem or network error.
        """

    @abstractmethod
    def download(self, uri: str, local_path: str) -> None:
        """Download an artifact from the storage backend to a local path.

        Args:
            uri: The URI returned by a previous ``upload()`` call on this
                backend.
            local_path: Absolute destination path. For file artifacts the
                parent directory must exist. For directory artifacts the
                destination is created if absent.

        Raises:
            IOError: If the download fails due to a filesystem or network error.
            ValueError: If the URI format is not recognised by this backend.
        """


class LocalStorageBackend(StorageBackend):
    """StorageBackend backed by the local filesystem.

    Intended for development, testing, and single-machine workflows. Artifacts
    are stored under ``base_dir`` using ``destination_key`` as a relative path.
    The returned URI is the absolute path of the stored artifact.

    Args:
        base_dir: Root directory for artifact storage. The directory must exist
            before any ``upload()`` call. Use ``tempfile.mkdtemp()`` for
            ephemeral storage.

    Example:
        >>> import tempfile
        >>> backend = LocalStorageBackend(base_dir=tempfile.mkdtemp())
        >>> uri = backend.upload("/tmp/model", "models/classifier/v1")
        >>> backend.download(uri, "/tmp/retrieved")
    """

    def __init__(self, base_dir: str) -> None:
        """Initialize with the root directory for artifact storage."""
        self._base_dir = base_dir

    def upload(self, local_path: str, destination_key: str) -> str:
        """Upload a local file or directory under ``base_dir``.

        Args:
            local_path: Absolute path to the local file or directory.
            destination_key: Relative path under ``base_dir`` where the
                artifact will be stored. Parent directories are created
                automatically.

        Returns:
            Absolute path of the stored artifact, usable as the URI argument
            to ``download()``.

        Raises:
            IOError: If the copy operation fails.
        """
        dest = os.path.join(self._base_dir, destination_key)
        os.makedirs(os.path.dirname(dest), exist_ok=True)
        if os.path.isdir(local_path):
            if os.path.exists(dest):
                shutil.rmtree(dest)
            shutil.copytree(local_path, dest)
        else:
            shutil.copy2(local_path, dest)
        return dest

    def download(self, uri: str, local_path: str) -> None:
        """Download an artifact from ``base_dir`` to a local path.

        Args:
            uri: Absolute path returned by a previous ``upload()`` call on
                this backend instance.
            local_path: Absolute destination path for the downloaded artifact.

        Raises:
            ValueError: If ``uri`` does not start with this backend's
                ``base_dir``, indicating it was not produced by this instance.
            IOError: If the copy operation fails.
        """
        if not uri.startswith(self._base_dir):
            raise ValueError(
                f"URI '{uri}' is not managed by this LocalStorageBackend "
                f"(base_dir='{self._base_dir}'). Pass a URI returned by "
                "upload() on this instance."
            )
        if os.path.isdir(uri):
            shutil.copytree(uri, local_path, dirs_exist_ok=True)
        else:
            shutil.copy2(uri, local_path)
