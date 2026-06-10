"""S3Sink: uploads a DatasetVariable to any S3-compatible object store."""

from __future__ import annotations

import logging
import os
import tempfile
from typing import TYPE_CHECKING

from michelangelo.workflow.schema.pusher import DatasetFormat
from michelangelo.workflow.schema.sinks.result import SinkResult
from michelangelo.workflow.tasks.functions.sinks.base import DataSink

if TYPE_CHECKING:
    from michelangelo.workflow.schema.sinks.s3 import S3SinkConfig
    from michelangelo.workflow.variables import DatasetVariable

_logger = logging.getLogger(__name__)


class S3Sink(DataSink):
    """Sink that uploads a dataset artifact to any S3-compatible object store.

    Serialises the artifact's pandas DataFrame to a temporary local file and
    uploads it via the ``StorageBackend`` in ``config.storage_backend``. The
    returned ``SinkResult.uri`` is the ``s3://`` URI produced by the backend's
    ``upload()`` method.

    Works with any ``StorageBackend`` subclass — ``MinioStorageBackend`` for
    MinIO and S3-compatible endpoints, or a custom implementation for GCS,
    Azure Blob, HDFS, etc.

    The backend is provided via ``S3SinkConfig.storage_backend``, keeping
    ``S3Sink`` consistent with the single-argument constructor pattern of
    ``LocalFileSink`` and ``HiveSink``. Construct the backend inside the
    ``@uniflow.task`` body — stateful objects cannot cross the UniFlow codec
    boundary.

    The uploaded object key is ``config.destination_key + "/data.<ext>"``,
    where ``<ext>`` matches the configured ``DatasetFormat``.

    Args:
        config: ``S3SinkConfig`` carrying the destination key, format, and
            an initialised storage backend.

    Raises:
        TypeError: If ``artifact.value`` is not a pandas DataFrame.
        ValueError: If the configured ``DatasetFormat`` is not supported.
        OSError: If the upload fails (propagated from the backend).

    Example::

        from michelangelo.lib.artifact_manager.minio_backend import MinioStorageBackend
        from michelangelo.workflow.schema.sinks.s3 import S3SinkConfig
        from michelangelo.workflow.tasks.functions.sinks import S3Sink

        backend = MinioStorageBackend(
            endpoint="localhost:9000",
            bucket="my-bucket",
            access_key="minioadmin",
            secret_key="minioadmin",
            secure=False,
            create_bucket_if_missing=True,
        )
        sink = S3Sink(S3SinkConfig("datasets/california/v1", storage_backend=backend))
        result = sink.write(variable)
        # result.uri == "s3://my-bucket/datasets/california/v1/data.parquet"
    """

    def __init__(self, config: S3SinkConfig) -> None:
        """Initialise with a typed config (includes the storage backend)."""
        self._config = config
        self._backend = config.storage_backend

    def write(self, artifact: DatasetVariable) -> SinkResult:
        """Serialise the artifact and upload it to the configured S3 bucket.

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
            OSError: Propagated from the storage backend's ``upload()`` on failure.
        """
        import pandas as _pd

        if not isinstance(artifact.value, _pd.DataFrame):
            raise TypeError(
                f"S3Sink requires artifact.value to be a pandas.DataFrame, "
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
        _logger.info("S3Sink: uploaded %d records to '%s'.", num_records, uri)
        return SinkResult(uri=uri, num_records=num_records)
