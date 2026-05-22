"""Typed configuration dataclasses for built-in DataSink implementations.

Import the config you need, instantiate it, then pass it to the matching sink:

    from michelangelo.workflow.schema.sinks import HiveSinkConfig
    from michelangelo.workflow.sinks import HiveSink

    sink = HiveSink(HiveSinkConfig(database="ml", table="predictions"))

Provider layers add their own config dataclasses in the same pattern.
"""

from michelangelo.workflow.schema.sinks.hive import HiveSinkConfig
from michelangelo.workflow.schema.sinks.local import LocalFileSinkConfig
from michelangelo.workflow.schema.sinks.memory import InMemorySinkConfig

__all__ = ["HiveSinkConfig", "InMemorySinkConfig", "LocalFileSinkConfig"]
