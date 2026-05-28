"""DatasetPusherPlugin — dispatches a DatasetVariable to one or more DataSinks.

Consumes a ``DatasetVariable`` (wrapping pandas, Spark, or Ray data) and
routes it to each configured ``DataSink``. The sink, not the plugin, is
responsible for extracting the data in its most efficient format:

- ``LocalFileSink.write(variable)`` → ``variable.value`` as pandas DataFrame
- ``HiveSink.write(variable)`` → ``variable.value`` as native Spark DataFrame
- ``S3Sink.write(variable)`` → native Ray/Spark write or pandas fallback

This design means ``DatasetPusherPlugin`` has zero knowledge of storage
technology — adding a new sink requires no changes to the plugin.
"""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING, Any

from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.tasks.pusher.plugins.base import PusherPluginBase

if TYPE_CHECKING:
    from michelangelo.workflow.schema.pusher import DatasetPluginConfig
    from michelangelo.workflow.variables import DatasetVariable

_logger = logging.getLogger(__name__)

__all__ = ["DatasetPusherPlugin"]


class DatasetPusherPlugin(PusherPluginBase):
    """Plugin that dispatches a dataset artifact to one or more configured sinks.

    Consumes a ``DatasetVariable`` and calls ``sink.write(artifact)`` on each
    sink in ``config.sinks``. All sinks receive the same artifact; each sink
    extracts data in the format most efficient for its target backend.

    Configure sinks explicitly or via the ``destination_path`` shorthand:

    - **Shorthand** (auto-creates ``LocalFileSink``):
      ``DatasetPluginConfig(destination_path="/tmp/out")``
    - **Explicit** (preferred, supports multi-sink and large-scale targets):
      ``DatasetPluginConfig(sinks=[LocalFileSink("/tmp/out"), HiveSink("db", "tbl")])``

    Args:
        config: ``DatasetPluginConfig`` containing at least one sink.
        artifact: A ``DatasetVariable`` wrapping the dataset to write.
        storage_backend: Unused by the built-in sinks. Available for custom
            sink implementations that compose with a ``StorageBackend``.
        registry_client: Unused by this plugin.

    Raises:
        ConfigurationError: If ``config.sinks`` is empty after ``__post_init__``
            resolution (neither ``sinks`` nor ``destination_path`` was set).

    Example::

        from michelangelo.workflow.tasks.functions.sinks import LocalFileSink
        from michelangelo.workflow.schema.pusher import (
            DatasetFormat, DatasetPluginConfig,
        )
        from michelangelo.workflow.variables import DatasetVariable
        import pandas as pd

        artifact = DatasetVariable(value=pd.DataFrame([{"x": 1}]))
        plugin = DatasetPusherPlugin(
            config=DatasetPluginConfig(
                destination_path="/tmp/out",
                format=DatasetFormat.PARQUET,
            ),
            artifact=artifact,
        )
        result = plugin.execute()
        print(result["destination_path"], result["num_records"])
    """

    def __init__(
        self,
        config: DatasetPluginConfig,
        artifact: DatasetVariable | None = None,
        storage_backend: Any = None,
        registry_client: Any = None,
    ) -> None:
        """Validate that at least one sink is configured and artifact is present."""
        super().__init__(config, artifact, storage_backend, registry_client)
        if artifact is None:
            raise ConfigurationError(
                "DatasetPusherPlugin requires a DatasetVariable. "
                "Pass the artifact via the artifact= argument."
            )
        if not config.sinks:
            raise ConfigurationError(
                "DatasetPusherPlugin requires at least one sink. "
                "Set DatasetPluginConfig(destination_path=...) or pass an "
                "explicit sinks=[LocalFileSink(...)] list."
            )

    def execute(self) -> dict[str, Any]:
        """Dispatch the artifact to each configured sink.

        Calls ``sink.write(self._artifact)`` on each sink in order. Each sink
        is responsible for extracting the data in the format it needs — no
        ``to_pandas()`` conversion at the plugin level.

        Returns:
            A dict with:

            - ``"sinks"``: list of per-sink result dicts, each with ``uri``
              and ``num_records`` keys.
            - ``"num_records"``: record count from the first sink.
            - ``"destination_path"``: first sink's URI (backwards-compat alias
              for callers that read this key from the old single-file API).

        Raises:
            IOError: Propagated from any sink's ``write()`` on failure.
            TypeError: If the artifact cannot be converted to the format a
                sink requires.
        """
        sink_results = []
        for sink in self._config.sinks:
            result = sink.write(self._artifact)
            sink_results.append(
                {"uri": result.uri, "num_records": result.num_records, **result.extra}
            )
            _logger.info(
                "DatasetPusherPlugin: %s wrote %d records to '%s'.",
                type(sink).__name__,
                result.num_records,
                result.uri,
            )

        return {
            "sinks": sink_results,
            "num_records": sink_results[0]["num_records"] if sink_results else 0,
            "destination_path": sink_results[0]["uri"] if sink_results else None,
        }
