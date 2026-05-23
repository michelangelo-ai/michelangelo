"""DataSink: abstract base class for all dataset sink implementations."""

from __future__ import annotations

from abc import ABC, abstractmethod
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from michelangelo.workflow.schema.sinks.result import SinkResult
    from michelangelo.workflow.variables import DatasetVariable


class DataSink(ABC):
    """Abstract base for all dataset sinks.

    A ``DataSink`` receives a ``DatasetVariable`` and writes the data to a
    destination in the most efficient format for that sink:

    - Sinks requiring a pandas DataFrame check
      ``isinstance(artifact.value, pd.DataFrame)``.
    - Sinks requiring a native Spark DataFrame access ``artifact.value`` directly —
      avoiding ``toPandas()`` which would collect all data to the driver.
    - Sinks requiring a Ray Dataset access ``artifact.value`` directly.

    Each sink class accepts a typed config dataclass from
    ``michelangelo.workflow.schema.sinks`` — validated at pipeline-definition
    time before any I/O occurs.

    Example::

        from michelangelo.workflow.schema.sinks import LocalFileSinkConfig, SinkResult
        from michelangelo.workflow.tasks.functions.sinks import DataSink, LocalFileSink

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
