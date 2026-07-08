"""Typed sink contracts: SinkResult and config dataclasses.

Import what you need, then pass configs to the matching sink:

    from michelangelo.workflow.schema.sinks import HiveSinkConfig, SinkResult
    from michelangelo.workflow.tasks.functions.sinks import HiveSink

    sink = HiveSink(HiveSinkConfig(database="ml", table="predictions"))
    result: SinkResult = sink.write(variable)

Provider layers add their own config dataclasses in the same pattern.
"""

from michelangelo.workflow.schema.sinks.hive import HiveSinkConfig
from michelangelo.workflow.schema.sinks.local import LocalFileSinkConfig
from michelangelo.workflow.schema.sinks.memory import InMemorySinkConfig
from michelangelo.workflow.schema.sinks.result import SinkResult
from michelangelo.workflow.schema.sinks.s3 import S3SinkConfig

__all__ = [
    "HiveSinkConfig",
    "InMemorySinkConfig",
    "LocalFileSinkConfig",
    "S3SinkConfig",
    "SinkResult",
]
