"""MinioSink: uploads a DatasetVariable to a MinIO / S3-compatible object store."""

from __future__ import annotations

import logging
import os
import tempfile
from typing import TYPE_CHECKING

from michelangelo.workflow.schema.pusher import DatasetFormat
from michelangelo.workflow.schema.sinks.result import SinkResult
from michelangelo.workflow.tasks.functions.sinks.base import DataSink

if TYPE_CHECKING:
    from michelangelo.lib.artifact_manager.minio_backend import MinioStorageBackend
    from michelangelo.workflow.schema.sinks.minio import MinioSinkConfig
    from michelangelo.workflow.variables import DatasetVariable

_logger = logging.getLogger(__name__)


class MinioSink(DataSink):
    """Sink that uploads a dataset artifact to a MinIO / S3-compatible object store.

    Serialises the artifact's pandas DataFrame to a temporary local file and
    uploads it via the supplied ``MinioStorageBackend``. The returned
    ``SinkResult.uri`` is the ``s3://`` URI produced by the backend's
    ``upload()`` method.

    The ``MinioStorageBackend`` is constructed by the caller (typically inside
    a ``@uniflow.task`` body) so that credentials are not serialised into the
    config dataclass:

    .. code-block:: python

        from michelangelo.lib.artifact_manager.minio_backend import MinioStorageBackend
        from michelangelo.workflow.schema.sinks.minio import MinioSinkConfig
        from michelangelo.workflow.tasks.functions.sinks import MinioSink

        backend = MinioStorageBackend(
            endpoint="localhost:9000",
            bucket="my-bucket",
            access_key="minioadmin",
            secret_key="minioadmin",
            secure=False,
            create_bucket_if_missing=True,
        )
        sink = MinioSink(
            MinioSinkConfig("datasets/california/v1", format=DatasetFormat.PARQUET),
            storage_backend=backend,
        )
        result = sink.write(variable)
        # result.uri == "s3://my-bucket/datasets/california/v1/data.parquet"

    The uploaded object key is ``config.destination_key + "/data.<ext>"``, where
    ``<ext>`` matches the configured ``DatasetFormat``.

    Args:
        config: Sink configuration carrying the destination key prefix and format.
        storage_backend: Initialised ``MinioStorageBackend`` (or any
            ``StorageBackend`` subclass) used for the upload.

    Raises:
        TypeError: If ``artifact.value`` is not a pandas DataFrame.
        ValueError: If the configured ``DatasetFormat`` is not supported.
        OSError: If the upload fails (propagated from the backend).
    """

    def __init__(
        self,
        config: MinioSinkConfig,
        storage_backend: MinioStorageBackend,
    ) -> None:
        """Initialise with a typed config and a pre-built storage backend."""
        self._config = config
        self._backend = storage_backend

    def write(self, artifact: DatasetVariable) -> SinkResult:
        """Serialise the artifact and upload it to the configured MinIO bucket.

        The DataFrame is written to a temporary file, uploaded via the backend,
        and the temp file is removed after the upload completes (or fails).

        Args:
            artifact: Dataset variable. ``artifact.value`` must be a
                ``pandas.DataFrame``.

        Returns:
            A ``SinkResult`` with the ``s3://`` URI of the uploaded object and
            the number of records written.

        Raises:
            TypeError: If ``artifact.value`` is not a pandas DataFrame.
            ValueError: If the configured format is not supported.
            OSError: Propagated from ``MinioStorageBackend.upload()`` on failure.
        """
        import pandas as _pd

        if not isinstance(artifact.value, _pd.DataFrame):
            raise TypeError(
                f"MinioSink requires artifact.value to be a pandas.DataFrame, "
                f"got {type(artifact.value).__name__}. "
                "For Spark DataFrames use HiveSink; for Ray Datasets use a "
                "custom DataSink."
            )

        fmt = self._config.format
        df = artifact.value
        filename = f"data.{fmt.value}"
        object_key = f"{self._config.destination_key}/{filename}"

        tmp_fd, tmp_path = tempfile.mkstemp(suffix=f".{fmt.value}")
        os.close(tmp_fd)
        try:
            if fmt == DatasetFormat.CSV:
                df.to_csv(tmp_path, index=False)
            elif fmt == DatasetFormat.PARQUET:
                df.to_parquet(tmp_path, index=False)
            elif fmt == DatasetFormat.JSON:
                df.to_json(tmp_path, orient="records", lines=True)
            else:
                raise ValueError(f"Unsupported DatasetFormat: {fmt!r}")

            uri = self._backend.upload(tmp_path, object_key)
        finally:
            if os.path.exists(tmp_path):
                os.unlink(tmp_path)

        num_records = len(df)
        _logger.info(
            "MinioSink: uploaded %d records to '%s'.", num_records, uri
        )
        return SinkResult(uri=uri, num_records=num_records)
