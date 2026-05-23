"""InMemorySink: accumulates records in memory for testing."""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

from michelangelo.workflow.schema.sinks.result import SinkResult
from michelangelo.workflow.tasks.functions.sinks.base import DataSink

if TYPE_CHECKING:
    from michelangelo.workflow.schema.sinks.memory import InMemorySinkConfig
    from michelangelo.workflow.variables import DatasetVariable


class InMemorySink(DataSink):
    """Sink that accumulates records in memory without any I/O.

    Intended exclusively for testing. Written records are accessible via
    the ``records`` property after ``write()`` is called.

    Args:
        config: Typed configuration (stateless — carries no fields but keeps
            the config-first pattern consistent across all sinks).

    Example::

        from michelangelo.workflow.schema.sinks import InMemorySinkConfig
        from michelangelo.workflow.tasks.functions.sinks import InMemorySink

        sink = InMemorySink(InMemorySinkConfig())
        result = sink.write(variable)
        assert len(sink.records) == result.num_records
    """

    def __init__(self, config: InMemorySinkConfig | None = None) -> None:
        """Initialise with an optional config (defaults to InMemorySinkConfig())."""
        from michelangelo.workflow.schema.sinks.memory import InMemorySinkConfig as _Cfg

        self._config = config or _Cfg()
        self._df: Any = None

    def write(self, artifact: DatasetVariable) -> SinkResult:
        """Store the artifact's data in memory.

        Accepts pandas DataFrames only.

        Args:
            artifact: Dataset variable. ``artifact.value`` must be a
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
