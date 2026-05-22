"""DataSink abstract base class and SinkResult for DatasetPusherPlugin.

``DataSink`` decouples ``DatasetPusherPlugin`` from any specific storage
technology. Built-in sinks live in ``michelangelo.workflow.sinks``; their typed
config dataclasses live in ``michelangelo.workflow.schema.sinks``.

Provider layers extend this by subclassing ``DataSink``:

    from michelangelo.workflow.schema.data_sink import DataSink, SinkResult

    class S3ParquetSink(DataSink):
        def __init__(self, bucket: str, key: str) -> None:
            self._bucket = bucket
            self._key = key

        def write(self, artifact: DatasetVariable) -> SinkResult:
            import boto3
            df = artifact.value
            buf = df.to_parquet(index=False)
            boto3.client("s3").put_object(
                Bucket=self._bucket, Key=self._key, Body=buf
            )
            return SinkResult(
                uri=f"s3://{self._bucket}/{self._key}",
                num_records=len(df),
            )
"""

from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from michelangelo.workflow.variables import DatasetVariable

__all__ = [
    "DataSink",
    "SinkResult",
]


@dataclass(frozen=True)
class SinkResult:
    """Structured result returned by ``DataSink.write()``.

    Attributes:
        uri: Canonical location of the written data. An absolute path for
            local sinks; a URI (``s3://...``, ``hive://...``) for remote sinks.
        num_records: Number of rows written.
        extra: Optional sink-specific metadata (partition paths, byte count,
            table name, etc.). Included verbatim in the plugin's result dict.

    Example:
        >>> result = SinkResult(uri="/tmp/data/data.parquet", num_records=3)
        >>> result.num_records
        3
    """

    uri: str
    num_records: int
    extra: dict[str, Any] = field(default_factory=dict)


class DataSink(ABC):
    """Abstract base for all dataset sinks.

    A ``DataSink`` receives a ``DatasetVariable`` and writes the data to a
    destination in the most efficient format for that sink:

    - Sinks that require a pandas DataFrame access ``artifact.value`` after
      loading via ``artifact.load_pandas_dataframe()``.
    - Sinks that require a native Spark DataFrame access ``artifact.value``
      directly — avoiding ``toPandas()`` which would collect all data to the
      driver and cause OOM on large datasets.
    - Sinks that require Ray Dataset access ``artifact.value`` directly.

    Each sink class accepts a typed config dataclass from
    ``michelangelo.workflow.schema.sinks`` — validated at pipeline-definition
    time before any I/O occurs.

    Example::

        from michelangelo.workflow.schema.sinks import LocalFileSinkConfig
        from michelangelo.workflow.sinks import LocalFileSink

        sink = LocalFileSink(LocalFileSinkConfig("/tmp/out"))
        result = sink.write(variable)
    """

    @abstractmethod
    def write(self, artifact: DatasetVariable) -> SinkResult:
        """Write the dataset variable to this sink's target.

        Args:
            artifact: The dataset variable produced by the assembler or trainer
                task. Its ``.value`` may be a ``pandas.DataFrame``, a
                ``pyspark.sql.DataFrame``, or a ``ray.data.Dataset`` depending
                on the runtime environment.

        Returns:
            A ``SinkResult`` describing what was written and where.

        Raises:
            IOError: On write failure.
            ImportError: If a required optional dependency is not installed.
        """
