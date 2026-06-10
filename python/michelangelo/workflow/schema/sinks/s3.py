"""Config dataclass for S3Sink."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import TYPE_CHECKING

from michelangelo.workflow.schema.pusher import DatasetFormat

if TYPE_CHECKING:
    from michelangelo.lib.artifact_manager.storage_backend import StorageBackend


@dataclass
class S3SinkConfig:
    """Typed configuration for ``S3Sink``.

    Bundles the object key prefix, serialisation format, and the storage
    backend in a single dataclass so that ``S3Sink`` can follow the same
    single-argument constructor pattern as ``LocalFileSink`` and ``HiveSink``.

    The ``storage_backend`` is required and must be an initialised
    ``StorageBackend`` instance (e.g. ``MinioStorageBackend``). Credentials
    and connection details live on the backend, not in this config — keeping
    the config free of secrets.

    Construct the backend inside a ``@uniflow.task`` body: stateful objects
    (network connections, credentials) cannot be serialised across the
    UniFlow codec boundary.

    Attributes:
        destination_key: Object key prefix within the configured bucket
            (e.g. ``"datasets/california-housing/v1"``). The uploaded object
            key becomes ``destination_key/data.<ext>`` where ``<ext>`` is
            determined by ``format``. Must be non-empty and must not start
            with ``/``. Trailing slashes are stripped automatically.
        storage_backend: Initialised ``StorageBackend`` (e.g.
            ``MinioStorageBackend``) used for the upload. Required — there
            is no default.
        format: Serialisation format — Parquet, CSV, or JSON Lines.
            Defaults to ``DatasetFormat.PARQUET``.

    Raises:
        ValueError: If ``destination_key`` is empty, starts with ``/``, or
            if ``storage_backend`` is ``None``.

    Example::

        from michelangelo.lib.artifact_manager.minio_backend import MinioStorageBackend
        from michelangelo.workflow.schema.sinks.s3 import S3SinkConfig
        from michelangelo.workflow.schema.pusher import DatasetFormat

        backend = MinioStorageBackend(
            endpoint="localhost:9000",
            bucket="my-bucket",
            access_key="minioadmin",
            secret_key="minioadmin",
            secure=False,
            create_bucket_if_missing=True,
        )
        cfg = S3SinkConfig(
            destination_key="datasets/california-housing/v1",
            storage_backend=backend,
        )
        cfg.format   # DatasetFormat.PARQUET
    """

    destination_key: str
    storage_backend: StorageBackend = field(default=None, repr=False)  # type: ignore[assignment]
    format: DatasetFormat = DatasetFormat.PARQUET

    def __post_init__(self) -> None:
        """Validate and normalise fields."""
        if not self.destination_key or not self.destination_key.strip():
            raise ValueError(
                "destination_key must be a non-empty string. "
                "Use a relative path such as 'datasets/california-housing/v1'."
            )
        if self.destination_key.startswith("/"):
            raise ValueError(
                f"destination_key must not start with '/': {self.destination_key!r}. "
                "Use a relative path such as 'datasets/california-housing/v1'."
            )
        self.destination_key = self.destination_key.rstrip("/")
        if self.storage_backend is None:
            raise ValueError(
                "storage_backend is required. Pass an initialised StorageBackend "
                "instance (e.g. MinioStorageBackend) constructed inside the task body."
            )
