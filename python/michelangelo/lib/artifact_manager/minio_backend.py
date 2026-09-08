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
import tempfile

from michelangelo.lib.artifact_manager.storage_backend import StorageBackend
from michelangelo.lib.exceptions import ConfigurationError

_logger = logging.getLogger(__name__)

__all__ = ["MinioStorageBackend"]


class MinioStorageBackend(StorageBackend):
    """StorageBackend backed by MinIO or any S3-compatible object store.

    Artifacts are stored as objects under the configured bucket — callers
    interact with local filesystem paths on both sides.

    **Directory vs. file:** A directory artifact is uploaded as one object per
    file, each stored under ``destination_key`` as a prefix (e.g. a local
    directory containing ``weights.bin`` uploaded to ``"models/clf/v1/raw"``
    stores an object at ``"models/clf/v1/raw/weights.bin"``), preserving any
    nested subdirectory structure. A plain file is stored at
    ``destination_key`` directly. ``download()`` distinguishes the two by
    checking whether an object exists at the exact key: if so, it's a file;
    otherwise every object under ``key + "/"`` is downloaded, reconstructing
    the original directory structure.

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

        A directory is uploaded as one object per file, each stored under
        ``destination_key`` as a prefix, preserving nested subdirectory
        structure. A plain file is stored at ``destination_key`` as-is.

        Args:
            local_path: Absolute path to the local file or directory to upload.
            destination_key: Object key (for a file) or key prefix (for a
                directory) within the bucket
                (e.g. ``"models/my-clf/a1b2c3d4e5f6a7b8/raw"``).

        Returns:
            URI in the form ``s3://{bucket}/{destination_key}``.

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
            self._upload_directory(local_path, destination_key)
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

        Checks whether an object exists at the exact key first: if so, it's
        downloaded as a plain file. Otherwise every object under
        ``key + "/"`` is downloaded, reconstructing the original directory
        structure under ``local_path``.

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
            OSError: If the download fails, or neither a file nor a directory
                exists at ``key``.
        """
        bucket, key = self._parse_uri(uri)
        try:
            self._client.stat_object(bucket, key)
        except self._S3Error as exc:
            if exc.code != "NoSuchKey":
                raise OSError(f"MinIO download failed for {uri!r}: {exc}") from exc
            self._download_directory(bucket, key, local_path)
            return

        _logger.debug("Downloading s3://%s/%s to '%s'.", bucket, key, local_path)
        tmp_fd, tmp_path = tempfile.mkstemp()
        os.close(tmp_fd)
        try:
            try:
                self._client.fget_object(bucket, key, tmp_path)
            except self._S3Error as exc:
                raise OSError(f"MinIO download failed for {uri!r}: {exc}") from exc
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
        """Upload every file under ``local_path`` to a matching object key.

        Each file is stored under the ``destination_key`` prefix, preserving
        nested subdirectory structure.
        """
        for root, _dirs, files in os.walk(local_path):
            for filename in files:
                file_path = os.path.join(root, filename)
                rel_path = os.path.relpath(file_path, local_path).replace(os.sep, "/")
                object_key = f"{destination_key}/{rel_path}"
                _logger.debug(
                    "Uploading file '%s' to s3://%s/%s.",
                    file_path,
                    self._bucket,
                    object_key,
                )
                try:
                    self._client.fput_object(self._bucket, object_key, file_path)
                except self._S3Error as exc:
                    raise OSError(
                        f"MinIO upload failed for key {object_key!r}: {exc}"
                    ) from exc

    def _download_directory(self, bucket: str, key: str, local_path: str) -> None:
        """Download every object under the ``key`` prefix into ``local_path``.

        Reconstructs the original directory structure.
        """
        prefix = key + "/"
        objects = list(self._client.list_objects(bucket, prefix=prefix, recursive=True))
        if not objects:
            raise OSError(f"No object or directory found at s3://{bucket}/{key}.")
        os.makedirs(local_path, exist_ok=True)
        for obj in objects:
            rel_path = obj.object_name[len(prefix) :]
            dest_path = os.path.join(local_path, *rel_path.split("/"))
            os.makedirs(os.path.dirname(dest_path), exist_ok=True)
            try:
                self._client.fget_object(bucket, obj.object_name, dest_path)
            except self._S3Error as exc:
                raise OSError(
                    f"MinIO download failed for s3://{bucket}/{obj.object_name}: {exc}"
                ) from exc

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
