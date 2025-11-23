import logging
import os
from dataclasses import dataclass
from pathlib import Path
from typing import Optional

from pyspark.sql import DataFrame, SparkSession

from michelangelo.uniflow.core.io_registry import io_registry
from michelangelo.uniflow.core.task_config import TaskBinding, TaskConfig
from michelangelo.uniflow.plugins.spark.io import SparkIO

log = logging.getLogger(__name__)

io_registry()[DataFrame] = SparkIO

_binding = TaskBinding(
    star_file=Path(__file__).resolve().parent / "task.star",
    function="spark_task",
    export="__spark_task",
)

_config_binding = TaskBinding(
    star_file=Path(__file__).resolve().parent / "task.star",
    function="spark_config",
    export="__spark_config",
)


@dataclass
class SparkTask(TaskConfig):
    """This class encapsulates the configuration properties required to orchestrate a Spark task, including resource
    specifications. It is designed to interface with a Starlark function that executes a Spark job according to these
    configurations.

    The class intentionally avoids defining default values for its properties. Instead, defaults should be provided
    through the keyword arguments of the associated Starlark function. This approach facilitates more flexible
    configuration management, allowing runtime overrides of the default settings.
    """

    driver_cpu: Optional[int] = None
    driver_memory: Optional[str] = None
    driver_disk: Optional[str] = None
    driver_gpu: Optional[int] = None
    executor_cpu: Optional[int] = None
    executor_memory: Optional[str] = None
    executor_disk: Optional[str] = None
    executor_gpu: Optional[int] = None
    executor_instances: Optional[int] = None

    def get_binding(self) -> TaskBinding:
        return _binding

    @classmethod
    def get_config_binding(self) -> TaskBinding:
        return _config_binding

    def pre_run(self):
        sb = SparkSession.builder.enableHiveSupport()

        if props := os.environ.get("_SPARK_PROPERTIES"):
            for kv in props.split(","):
                k, v = kv.split("=", 1)
                sb = sb.config(k, v)

        spark = sb.getOrCreate()
        assert spark

    def post_run(self):
        spark = SparkSession.getActiveSession()
        if spark:
            spark.stop()
