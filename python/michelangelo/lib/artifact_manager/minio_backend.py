"""MinIO / S3-compatible storage backend for Michelangelo artifact storage.

Implements :class:`~michelangelo.lib.artifact_manager.storage_backend.StorageBackend`
using the MinIO Python SDK. Suitable for local sandbox MinIO servers and any
S3-compatible object store (AWS S3, GCS via S3 interop, DigitalOcean Spaces).

Requires the ``minio`` package::

    pip install 'michelangelo[minio]'

Typical usage::

    from michelangelo.lib.artifact_manager.minio_backend import MinioStorageBackend

    # Production — TLS enabled, bucket pre-created by infra
    backend = MinioStorageBackend(
        endpoint="minio.prod.example.com:443",
        bucket="michelangelo-models",
        access_key=os.environ["MINIO_ACCESS_KEY"],
        secret_key=os.environ["MINIO_SECRET_KEY"],
    )

    # Local sandbox — plaintext, auto-create bucket
    backend = MinioStorageBackend(
        endpoint="localhost:9000",
        bucket="michelangelo-models",
        access_key="minioadmin",
        secret_key="minioadmin",
        secure=False,            # local dev only — do not use in production
        create_bucket_if_missing=True,
    )

    uri = backend.upload("/tmp/my-model", "models/clf/v1/raw")
    backend.download(uri, "/tmp/retrieved")
"""

from __future__ import annotations

import logging
import os
import shutil
import sys
import tarfile
import tempfile

from michelangelo.lib.artifact_manager.storage_backend import StorageBackend
from michelangelo.lib.exceptions import ConfigurationError

_logger = logging.getLogger(__name__)

__all__ = ["MinioStorageBackend"]

_DIR_TAR_SUFFIX = "/__dir__.tar"


def _safe_extractall(tar: tarfile.TarFile, dest: str) -> None:
    """Extract a tar archive into ``dest`` without path-traversal risk.

    Uses ``filter="data"`` on Python 3.12+ (PEP 706), which strips absolute
    paths, ``..`` traversal components, and unsafe symlinks. On older runtimes
    a manual member check provides the same protection.

    Raises:
        ValueError: If a member's resolved path escapes ``dest`` (Python < 3.12).
    """
    if sys.version_info >= (3, 12):
        tar.extractall(dest, filter="data")
    else:
        real_dest = os.path.realpath(dest)
        safe_members = []
        for member in tar.getmembers():
            if not member.name:  # skip the empty-name root entry from arcname=""
                continue
            member_path = os.path.realpath(os.path.join(dest, member.name))
            if not member_path.startswith(real_dest + os.sep):
                raise ValueError(
                    f"Refusing to extract '{member.name}': path would escape "
                    f"the destination directory '{dest}'."
                )
            safe_members.append(member)
        tar.extractall(dest, members=safe_members)


class MinioStorageBackend(StorageBackend):
    """StorageBackend backed by MinIO or any S3-compatible object store.

    Artifacts are stored as objects under the configured bucket. Directory
    artifacts are transparently tar-archived before upload and extracted on
    download — callers interact with local filesystem paths on both sides.

    **Directory vs. file sentinel:** When ``upload()`` is called on a directory,
    the archive is stored with the key suffix ``/__dir__.tar``
    (e.g. ``"models/clf/v1/raw/__dir__.tar"``). The returned URI encodes this
    suffix; ``download()`` reads it to decide whether to extract or copy. A
    plain ``.tar`` file uploaded directly (not a directory) is stored as-is and
    delivered as-is on download.

    **Bucket creation:** By default the backend does *not* attempt to create the
    bucket. Set ``create_bucket_if_missing=True`` for local sandbox environments
    where the bucket may not exist yet. Most production IAM policies do not grant
    ``s3:CreateBucket``, so the default avoids a confusing permission error on
    startup.

    The ``minio`` package is imported lazily so the rest of the library
    remains usable without it. If ``minio`` is not installed, instantiating
    this class raises :class:`ImportError` with an actionable install hint.

    Args:
        endpoint: MinIO or S3-compatible server address without the scheme
            (e.g. ``"localhost:9000"`` or ``"s3.amazonaws.com"``).
        bucket: Target bucket name.
        access_key: Access key ID (MinIO root user or AWS IAM key ID).
        secret_key: Secret access key matching ``access_key``.
        secure: Use TLS for the connection. Defaults to ``True`` (matches the
            MinIO SDK default). Set ``False`` only for a local sandbox server
            where TLS is not configured.
        region: AWS region string (e.g. ``"us-east-1"``). Required when
            connecting to AWS S3. Leave ``None`` for plain MinIO installations.
        create_bucket_if_missing: When ``True``, calls ``make_bucket`` on
            initialisation if the configured bucket does not exist. Defaults
            to ``False`` — useful when the IAM policy does not grant
            ``s3:CreateBucket`` (most production environments).

    Raises:
        ImportError: If the ``minio`` package is not installed.
        ConfigurationError: If ``endpoint`` or ``bucket`` is empty.

    Example::

        from michelangelo.lib.artifact_manager.minio_backend import MinioStorageBackend

        backend = MinioStorageBackend(
            endpoint="localhost:9000",
            bucket="my-bucket",
            access_key="minioadmin",
            secret_key="minioadmin",
            secure=False,            # local dev only
            create_bucket_if_missing=True,
        )
        uri = backend.upload("/tmp/weights.pt", "models/clf/abc123/raw")
        # uri == "s3://my-bucket/models/clf/abc123/raw"
        backend.download(uri, "/tmp/retrieved.pt")
    """

    def __init__(
        self,
        endpoint: str,
        bucket: str,
        access_key: str,
        secret_key: str,
        secure: bool = True,
        region: str | None = None,
        create_bucket_if_missing: bool = False,
    ) -> None:
        """Initialize the backend and optionally ensure the target bucket exists.

        Args:
            endpoint: Server address without scheme (e.g. ``"localhost:9000"``).
            bucket: Target bucket name.
            access_key: Access key ID.
            secret_key: Secret access key.
            secure: Use TLS. Defaults to ``True``.
            region: AWS region, or ``None`` for plain MinIO.
            create_bucket_if_missing: Create the bucket if absent. Defaults to
                ``False``.

        Raises:
            ConfigurationError: If ``endpoint`` or ``bucket`` is empty.
            ImportError: If ``minio`` is not installed.
        """
        if not endpoint:
            raise ConfigurationError(
                "MinioStorageBackend endpoint must be non-empty. "
                "Provide the server address, e.g. 'localhost:9000'."
            )
        if not bucket:
            raise ConfigurationError(
                "MinioStorageBackend bucket must be non-empty. "
                "Provide the target bucket name."
            )
        try:
            from minio import Minio
            from minio.error import S3Error
        except ImportError as exc:
            raise ImportError(
                "MinioStorageBackend requires the 'minio' package. "
                "Install it with: pip install 'michelangelo[minio]'"
            ) from exc
        self._bucket = bucket
        self._S3Error = S3Error
        self._client = Minio(
            endpoint,
            access_key=access_key,
            secret_key=secret_key,
            secure=secure,
            region=region,
        )
        if create_bucket_if_missing:
            self._ensure_bucket()

    def upload(self, local_path: str, destination_key: str) -> str:
        """Upload a local file or directory to the configured MinIO bucket.

        Directory artifacts are tar-archived and stored with the key suffix
        ``/__dir__.tar`` (e.g. ``destination_key + "/__dir__.tar"``); the
        returned URI encodes this suffix so ``download()`` can extract them
        correctly. Plain files are stored at ``destination_key`` as-is.

        Args:
            local_path: Absolute path to the local file or directory to upload.
            destination_key: Object key within the bucket
                (e.g. ``"models/my-clf/a1b2c3d4e5f6a7b8/raw"``).

        Returns:
            URI in the form ``s3://{bucket}/{key}`` where ``key`` is
            ``destination_key`` for files or ``destination_key/__dir__.tar``
            for directories.

        Raises:
            ValueError: If ``destination_key`` is empty.
            OSError: If the local path does not exist or the upload fails.
        """
        if not destination_key:
            raise ValueError(
                "destination_key must be non-empty. "
                "Provide a key such as 'models/classifier/v1'."
            )
        if os.path.isdir(local_path):
            dir_key = destination_key + _DIR_TAR_SUFFIX
            self._upload_directory(local_path, dir_key)
            return f"s3://{self._bucket}/{dir_key}"
        else:
            _logger.debug(
                "Uploading file '%s' to s3://%s/%s.",
                local_path,
                self._bucket,
                destination_key,
            )
            try:
                self._client.fput_object(self._bucket, destination_key, local_path)
            except self._S3Error as exc:
                raise OSError(
                    f"MinIO upload failed for key {destination_key!r}: {exc}"
                ) from exc
        return f"s3://{self._bucket}/{destination_key}"

    def download(self, uri: str, local_path: str) -> None:
        """Download an artifact from MinIO to a local path.

        If the URI ends with ``/__dir__.tar`` (produced by uploading a
        directory), the archive is extracted to ``local_path`` as a directory.
        Otherwise the object is copied as a plain file.

        Args:
            uri: URI returned by a previous :meth:`upload` call on any
                ``MinioStorageBackend`` pointing at the same bucket
                (``s3://{bucket}/{key}``).
            local_path: Destination file or directory path. For file artifacts
                the parent directory must exist. For directory artifacts the
                destination is created if absent.

        Raises:
            ValueError: If ``uri`` is not a valid ``s3://`` URI with a
                non-empty bucket and key.
            OSError: If the download or extraction fails.
        """
        bucket, key = self._parse_uri(uri)
        is_directory = key.endswith(_DIR_TAR_SUFFIX)
        tmp_fd, tmp_path = tempfile.mkstemp()
        os.close(tmp_fd)
        try:
            _logger.debug("Downloading s3://%s/%s to '%s'.", bucket, key, local_path)
            try:
                self._client.fget_object(bucket, key, tmp_path)
            except self._S3Error as exc:
                raise OSError(f"MinIO download failed for {uri!r}: {exc}") from exc
            if is_directory:
                os.makedirs(local_path, exist_ok=True)
                with tarfile.open(tmp_path, "r") as tar:
                    _safe_extractall(tar, local_path)
            else:
                shutil.copy2(tmp_path, local_path)
        finally:
            if os.path.exists(tmp_path):
                os.unlink(tmp_path)

    # ── Public helpers ───────────────────────────────────────────────────────

    def get_storage_location(self) -> str:
        """Return the ``s3://bucket`` URI for this backend.

        Returns:
            URI in the form ``s3://{bucket}`` identifying the root of the
            configured bucket. Useful for logging and debugging.
        """
        return f"s3://{self._bucket}"

    # ── Private helpers ──────────────────────────────────────────────────────

    def _upload_directory(self, local_path: str, destination_key: str) -> None:
        """Tar ``local_path`` and upload the archive as ``destination_key``."""
        tmp_fd, tmp_path = tempfile.mkstemp(suffix=".tar")
        os.close(tmp_fd)
        try:
            _logger.debug(
                "Archiving directory '%s' before upload to s3://%s/%s.",
                local_path,
                self._bucket,
                destination_key,
            )
            with tarfile.open(tmp_path, "w") as tar:
                for entry in os.scandir(local_path):
                    tar.add(entry.path, arcname=entry.name)
            try:
                self._client.fput_object(self._bucket, destination_key, tmp_path)
            except self._S3Error as exc:
                raise OSError(
                    f"MinIO upload failed for key {destination_key!r}: {exc}"
                ) from exc
        finally:
            if os.path.exists(tmp_path):
                os.unlink(tmp_path)

    def _ensure_bucket(self) -> None:
        """Create the target bucket if it does not already exist.

        Handles the ``BucketAlreadyOwnedByYou`` race that can occur when two
        workers start simultaneously and both observe the bucket as absent.
        """
        if not self._client.bucket_exists(self._bucket):
            try:
                _logger.info("Creating bucket '%s'.", self._bucket)
                self._client.make_bucket(self._bucket)
            except self._S3Error as exc:
                if exc.code != "BucketAlreadyOwnedByYou":
                    raise
                _logger.info(
                    "Bucket '%s' already exists (created concurrently).",
                    self._bucket,
                )

    def _parse_uri(self, uri: str) -> tuple[str, str]:
        """Parse ``s3://{bucket}/{key}`` into ``(bucket, key)``.

        Args:
            uri: An S3-style URI produced by :meth:`upload`.

        Returns:
            Tuple of ``(bucket, key)``.

        Raises:
            ValueError: If ``uri`` does not start with ``s3://``, contains no
                bucket, or contains no object key.
        """
        if not uri.startswith("s3://"):
            raise ValueError(
                f"URI '{uri}' is not a MinIO/S3 URI. "
                "Expected a URI in the form 's3://{bucket}/{key}'."
            )
        rest = uri[5:]  # strip "s3://"
        bucket, sep, key = rest.partition("/")
        if not bucket:
            raise ValueError(f"URI contains no bucket: {uri!r}")
        if not sep or not key:
            raise ValueError(f"URI contains no object key: {uri!r}")
        return bucket, key
