"""Pluggable dataset sink abstraction for DatasetPusherPlugin.

``DataSink`` decouples ``DatasetPusherPlugin`` from any specific storage
technology. Built-in sinks (``LocalFileSink``, ``InMemorySink``) are provided
for development and testing. Provider layers extend this:

    # Provider layer example (Hive/Spark):
    class HiveProviderSink(DataSink):
        def write(self, artifact: DatasetVariable) -> SinkResult:
            spark_df = artifact.value          # native Spark — no toPandas()
            spark_df.write.mode("overwrite").saveAsTable(f"{db}.{table}")
            return SinkResult(uri=f"hive://{db}.{table}",
                              num_records=spark_df.count())

    # Community (Phase 7 optional extras):
    class S3ParquetSink(DataSink):             # requires boto3
        def write(self, artifact: DatasetVariable) -> SinkResult: ...
"""

from __future__ import annotations

import logging
import os
from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from typing import TYPE_CHECKING, Any

from michelangelo.workflow.schema.pusher import DatasetFormat

if TYPE_CHECKING:
    from michelangelo.workflow.variables import DatasetVariable

_logger = logging.getLogger(__name__)

__all__ = [
    "DataSink",
    "HiveSink",
    "InMemorySink",
    "LocalFileSink",
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
      calling ``artifact.load_pandas_dataframe()`` (or check ``artifact.backend``).
    - Sinks that require a native Spark DataFrame access ``artifact.value``
      directly — avoiding ``toPandas()`` which would collect all data to the
      driver and cause OOM on large datasets.
    - Sinks that require Ray Dataset access ``artifact.value`` directly.

    Each sink calls ``sink.write(variable)`` — the plugin dispatches without
    knowledge of storage technology.

    Example::

        class S3ParquetSink(DataSink):
            def __init__(self, bucket: str, key: str) -> None:
                self._bucket = bucket
                self._key = key

            def write(self, artifact: DatasetVariable) -> SinkResult:
                import boto3
                import pandas as pd
                df = artifact.value
                if not isinstance(df, pd.DataFrame):
                    raise TypeError("S3ParquetSink requires a pandas DataFrame.")
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
        """Write the dataset artifact to this sink's target.

        Args:
            artifact: The dataset artifact produced by the assembler or trainer
                task. Its ``.value`` may be a ``pandas.DataFrame``, a
                ``pyspark.sql.DataFrame``, or a ``ray.data.Dataset`` depending
                on the runtime environment.

        Returns:
            A ``SinkResult`` describing what was written and where.

        Raises:
            IOError: On write failure.
            ImportError: If a required optional dependency is not installed.
        """


class LocalFileSink(DataSink):
    """Sink that writes a dataset to a local file in CSV, Parquet, or JSON Lines.

    Accesses ``artifact.value`` directly — suitable for development,
    testing, and single-machine workflows. Not appropriate for large-scale
    Spark datasets; use ``HiveSink`` or a custom ``DataSink`` for those.

    .. note::
        Parquet output is a **single** ``data.parquet`` file written via
        ``pandas.DataFrame.to_parquet()``. This differs from
        ``michelangelo.uniflow.plugins.pandas.PandasIO``, which produces a
        directory of ``part-*.parquet`` files. Do not mix the two paths.

    Args:
        destination_path: Directory where the output file is written.
            Created automatically if absent.
        format: Output format. Defaults to ``DatasetFormat.PARQUET``.
        partition_by: Column names to use for directory partitioning.
            Reserved for provider subclasses; unused by the built-in writer.

    Example::

        sink = LocalFileSink("/tmp/eval_data", format=DatasetFormat.CSV)
        result = sink.write(artifact)
        # result.uri == "/tmp/eval_data/data.csv"
    """

    def __init__(
        self,
        destination_path: str,
        format: DatasetFormat = DatasetFormat.PARQUET,
        partition_by: list[str] | None = None,
    ) -> None:
        """Initialise with the output directory path and format."""
        self._destination_path = destination_path
        self._format = format
        self._partition_by = partition_by or []

    def write(self, artifact: DatasetVariable) -> SinkResult:
        """Write the artifact as a local file.

        Accepts pandas DataFrames only. For Spark DataFrames use ``HiveSink``.

        Args:
            artifact: Dataset artifact. ``artifact.value`` must be a
                ``pandas.DataFrame``.

        Returns:
            A ``SinkResult`` with the absolute file path as ``uri``.

        Raises:
            TypeError: If ``artifact.value`` is not a pandas DataFrame.
            ImportError: If pyarrow is not installed (Parquet only).
            IOError: If the destination directory is not writable.
        """
        import pandas as _pd

        if not isinstance(artifact.value, _pd.DataFrame):
            raise TypeError(
                f"LocalFileSink requires artifact.value to be a pandas.DataFrame, "
                f"got {type(artifact.value).__name__}. "
                "For Spark DataFrames use HiveSink; for Ray Datasets use a "
                "custom DataSink."
            )
        df = artifact.value
        os.makedirs(self._destination_path, exist_ok=True)
        output_path = os.path.join(self._destination_path, f"data.{self._format.value}")

        if self._format == DatasetFormat.CSV:
            df.to_csv(output_path, index=False)
        elif self._format == DatasetFormat.PARQUET:
            df.to_parquet(output_path, index=False)
        elif self._format == DatasetFormat.JSON:
            df.to_json(output_path, orient="records", lines=True)
        else:
            raise ValueError(f"Unsupported DatasetFormat: {self._format!r}")

        num_records = len(df)
        _logger.info(
            "LocalFileSink: wrote %d records to '%s'.", num_records, output_path
        )
        return SinkResult(uri=output_path, num_records=num_records)


class InMemorySink(DataSink):
    """Sink that accumulates records in memory without any I/O.

    Intended exclusively for testing. Written records are accessible via
    the ``records`` property after ``write()`` is called.

    Example::

        sink = InMemorySink()
        result = sink.write(artifact)
        assert len(sink.records) == result.num_records
    """

    def __init__(self) -> None:
        """Initialise with an empty record store."""
        self._df: Any = None

    def write(self, artifact: DatasetVariable) -> SinkResult:
        """Store the artifact's data in memory.

        Accepts pandas DataFrames only.

        Args:
            artifact: Dataset artifact. ``artifact.value`` must be a
                ``pandas.DataFrame``.

        Returns:
            A ``SinkResult`` with ``uri="memory://in-memory-sink"``.

        Raises:
            TypeError: If ``artifact.value`` is not a pandas DataFrame.
        """
        import pandas as _pd

        if not isinstance(artifact.value, _pd.DataFrame):
            raise TypeError(
                f"InMemorySink requires artifact.value to be a pandas.DataFrame, "
                f"got {type(artifact.value).__name__}."
            )
        self._df = artifact.value
        return SinkResult(uri="memory://in-memory-sink", num_records=len(self._df))

    @property
    def records(self) -> list[dict[str, Any]]:
        """Records from the most recent ``write()`` call as a list of dicts."""
        if self._df is None:
            return []
        return self._df.to_dict(orient="records")


class HiveSink(DataSink):
    """Sink that writes a dataset to an Apache Hive table via Spark.

    Accesses ``artifact.value`` directly as a native Spark DataFrame — no
    ``toPandas()`` collection to the driver. This is the recommended sink for
    large-scale datasets in Spark environments (Hive, Delta Lake, Iceberg).

    Requires pyspark. A Spark session must be active when ``write()`` is called.

    Args:
        database: Hive database name (e.g. ``"ml_features"``).
        table: Hive table name (e.g. ``"training_data"``).
        mode: Spark write mode — ``"overwrite"`` (default) or ``"append"``.

    Example::

        sink = HiveSink(database="ml", table="training_features")
        result = sink.write(artifact)
        # result.uri == "hive://ml.training_features"

    Provider subclasses can override ``write()`` to add partition config,
    table properties, or custom write paths.
    """

    def __init__(
        self,
        database: str,
        table: str,
        mode: str = "overwrite",
    ) -> None:
        """Initialise with Hive database, table, and write mode."""
        _valid_modes = {"overwrite", "append", "ignore", "error"}
        if mode not in _valid_modes:
            raise ValueError(
                f"Invalid mode {mode!r}. Expected one of: {sorted(_valid_modes)}."
            )
        self._database = database
        self._table = table
        self._mode = mode

    def write(self, artifact: DatasetVariable) -> SinkResult:
        """Write the artifact to Hive as a Spark saveAsTable operation.

        Args:
            artifact: Dataset artifact. Must hold a ``pyspark.sql.DataFrame``
                in ``artifact.value`` (e.g. ``DatasetVariable(value=spark_df)``).

        Returns:
            A ``SinkResult`` with a ``hive://`` URI and the row count.

        Raises:
            ImportError: If pyspark is not installed.
            TypeError: If ``artifact.value`` is not a Spark DataFrame.
        """
        try:
            import pyspark.sql as _ps
        except ImportError as e:
            raise ImportError(
                "pyspark is required for HiveSink: pip install pyspark"
            ) from e
        if not isinstance(artifact.value, _ps.DataFrame):
            raise TypeError(
                f"HiveSink requires artifact.value to be a pyspark.sql.DataFrame, "
                f"got {type(artifact.value).__name__}. "
                "Use DatasetVariable(value=spark_df) to pass a Spark DataFrame."
            )
        qualified = f"{self._database}.{self._table}"
        spark_df = artifact.value  # native — no toPandas()
        num_records = spark_df.count()  # count before write to avoid a second scan
        spark_df.write.mode(self._mode).saveAsTable(qualified)
        _logger.info("HiveSink: wrote %d records to '%s'.", num_records, qualified)
        return SinkResult(uri=f"hive://{qualified}", num_records=num_records)
