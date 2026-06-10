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
            is determined by ``format``.
        format: Serialisation format — Parquet, CSV, or JSON Lines.
            Defaults to ``DatasetFormat.PARQUET``.

    Example:
        >>> cfg = MinioSinkConfig("datasets/california-housing/v1")
        >>> cfg.format
        <DatasetFormat.PARQUET: 'parquet'>
    """

    destination_key: str
    format: DatasetFormat = DatasetFormat.PARQUET
