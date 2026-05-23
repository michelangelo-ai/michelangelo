"""HiveSink: writes a DatasetVariable to a Hive table via Spark."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

from michelangelo.workflow.schema.sinks.result import SinkResult
from michelangelo.workflow.tasks.functions.sinks.base import DataSink

if TYPE_CHECKING:
    from michelangelo.workflow.schema.sinks.hive import HiveSinkConfig
    from michelangelo.workflow.variables import DatasetVariable

_logger = logging.getLogger(__name__)


class HiveSink(DataSink):
    """Sink that writes a dataset to an Apache Hive table via Spark.

    Accesses ``variable.value`` as a native Spark DataFrame — no ``toPandas()``
    collection to the driver. Use this for large-scale datasets in Spark
    environments (Hive, Delta Lake, Iceberg).

    Requires pyspark. A Spark session must be active when ``write()`` is called.

    Args:
        config: Typed configuration for this sink. Validated at construction time.

    Example::

        from michelangelo.workflow.schema.sinks import HiveSinkConfig
        from michelangelo.workflow.tasks.functions.sinks import HiveSink

        sink = HiveSink(HiveSinkConfig(database="ml", table="training_features"))
        result = sink.write(variable)
        # result.uri == "hive://ml.training_features"
    """

    def __init__(self, config: HiveSinkConfig) -> None:
        """Initialise with the typed Hive sink config."""
        self._config = config

    def write(self, artifact: DatasetVariable) -> SinkResult:
        """Write the artifact to Hive as a Spark saveAsTable operation.

        Args:
            artifact: Dataset variable. Must hold a ``pyspark.sql.DataFrame``
                in ``artifact.value``.

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
        qualified = f"{self._config.database}.{self._config.table}"
        spark_df = artifact.value
        num_records = spark_df.count()
        spark_df.write.mode(self._config.mode).saveAsTable(qualified)
        _logger.info("HiveSink: wrote %d records to '%s'.", num_records, qualified)
        return SinkResult(uri=f"hive://{qualified}", num_records=num_records)
