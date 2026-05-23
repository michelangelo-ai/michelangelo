"""Built-in DataSink implementations for DatasetPusherPlugin.

Each sink accepts a typed config dataclass from
``michelangelo.workflow.schema.sinks`` — validated at pipeline-definition time:

    from michelangelo.workflow.schema.sinks import HiveSinkConfig, LocalFileSinkConfig
    from michelangelo.workflow.tasks.functions.sinks import (
        DataSink, HiveSink, LocalFileSink, InMemorySink
    )

    sinks = [
        LocalFileSink(LocalFileSinkConfig("/tmp/out")),
        HiveSink(HiveSinkConfig(database="ml", table="predictions")),
    ]

Provider layers extend this by subclassing ``DataSink`` and registering their
own config dataclasses in ``schema/sinks/``.
"""

from michelangelo.workflow.tasks.functions.sinks.base import DataSink
from michelangelo.workflow.tasks.functions.sinks.hive import HiveSink
from michelangelo.workflow.tasks.functions.sinks.local import LocalFileSink
from michelangelo.workflow.tasks.functions.sinks.memory import InMemorySink

__all__ = ["DataSink", "HiveSink", "InMemorySink", "LocalFileSink"]
