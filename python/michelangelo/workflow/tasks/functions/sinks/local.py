"""LocalFileSink: writes a DatasetVariable to a local file."""

from __future__ import annotations

import logging
import os
from typing import TYPE_CHECKING

from michelangelo.workflow.schema.pusher import DatasetFormat
from michelangelo.workflow.schema.sinks.result import SinkResult
from michelangelo.workflow.tasks.functions.sinks.base import DataSink

if TYPE_CHECKING:
    from michelangelo.workflow.schema.sinks.local import LocalFileSinkConfig
    from michelangelo.workflow.variables import DatasetVariable

_logger = logging.getLogger(__name__)


class LocalFileSink(DataSink):
    """Sink that writes a dataset to a local file in CSV, Parquet, or JSON Lines.

    Suitable for development, testing, and single-machine workflows.
    Not appropriate for large-scale Spark datasets; use ``HiveSink`` for those.

    .. note::
        Parquet output is a **single** ``data.parquet`` file written via
        ``pandas.DataFrame.to_parquet()``. This differs from
        ``michelangelo.uniflow.plugins.pandas.PandasIO``, which produces a
        directory of ``part-*.parquet`` files.

    Args:
        config: Typed configuration for this sink. Validated at construction time.

    Example::

        from michelangelo.workflow.schema.sinks import LocalFileSinkConfig
        from michelangelo.workflow.tasks.functions.sinks import LocalFileSink

        sink = LocalFileSink(
            LocalFileSinkConfig("/tmp/eval_data", format=DatasetFormat.CSV)
        )
        result = sink.write(variable)
        # result.uri == "/tmp/eval_data/data.csv"
    """

    def __init__(self, config: LocalFileSinkConfig) -> None:
        """Initialise with the typed local file sink config."""
        self._config = config

    def write(self, artifact: DatasetVariable) -> SinkResult:
        """Write the artifact as a local file.

        Accepts pandas DataFrames only. For Spark DataFrames use ``HiveSink``.

        Args:
            artifact: Dataset variable. ``artifact.value`` must be a
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
        fmt = self._config.format
        df = artifact.value
        os.makedirs(self._config.destination_path, exist_ok=True)
        output_path = os.path.join(self._config.destination_path, f"data.{fmt.value}")

        if fmt == DatasetFormat.CSV:
            df.to_csv(output_path, index=False)
        elif fmt == DatasetFormat.PARQUET:
            df.to_parquet(output_path, index=False)
        elif fmt == DatasetFormat.JSON:
            df.to_json(output_path, orient="records", lines=True)
        else:
            raise ValueError(f"Unsupported DatasetFormat: {fmt!r}")

        num_records = len(df)
        _logger.info(
            "LocalFileSink: wrote %d records to '%s'.", num_records, output_path
        )
        return SinkResult(uri=output_path, num_records=num_records)
