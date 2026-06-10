"""Config dataclass for MinioSink."""

from __future__ import annotations

from dataclasses import dataclass

from michelangelo.workflow.schema.pusher import DatasetFormat


@dataclass
class MinioSinkConfig:
    """Typed configuration for ``MinioSink``.

    Carries the semantic sink parameters (object key prefix and serialisation
    format). Connection details (endpoint, bucket, credentials) belong on the
    ``MinioStorageBackend`` passed to ``MinioSink.__init__()``.

    Attributes:
        destination_key: Object key prefix within the configured bucket
            (e.g. ``"datasets/california-housing/v1"``). The actual uploaded
            object key becomes ``destination_key/data.<ext>`` where ``<ext>``
            is determined by ``format``. Must be non-empty and must not start
            with ``/``. Trailing slashes are stripped automatically.
        format: Serialisation format — Parquet, CSV, or JSON Lines.
            Defaults to ``DatasetFormat.PARQUET``.

    Raises:
        ValueError: If ``destination_key`` is empty or starts with ``/``.

    Example::

        from michelangelo.workflow.schema.sinks.minio import MinioSinkConfig
        from michelangelo.workflow.schema.pusher import DatasetFormat

        cfg = MinioSinkConfig("datasets/california-housing/v1")
        cfg.format   # DatasetFormat.PARQUET

        cfg_csv = MinioSinkConfig(
            "datasets/california-housing/v1",
            format=DatasetFormat.CSV,
        )
    """

    destination_key: str
    format: DatasetFormat = DatasetFormat.PARQUET

    def __post_init__(self) -> None:
        """Validate and normalise ``destination_key``."""
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
