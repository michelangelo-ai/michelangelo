"""Spark task configuration and execution for Uniflow workflows.

This module provides task configuration for executing Uniflow workflows on Spark clusters.
It handles Spark session initialization, resource allocation, and lifecycle management
for distributed task execution.

Spark tasks support configurable resources for both driver and executor nodes, including
CPU, memory, disk, and GPU allocations. The execution model initializes a Spark session
with Hive support before running the task and properly stops it afterward.
"""

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
    """Configuration for Spark-based task execution in Uniflow workflows.

    This class defines resource specifications and runtime configuration for executing
    tasks on Spark clusters. It manages the lifecycle of Spark session initialization
    and shutdown through pre_run and post_run hooks.

    Unlike RayTask which uses head/worker terminology, SparkTask uses driver/executor
    terminology to describe the cluster nodes. The driver coordinates execution while
    executors perform distributed computation.

    The class intentionally avoids defining default values for its properties. Instead,
    defaults should be provided through the keyword arguments of the associated Starlark
    function. This approach facilitates more flexible configuration management, allowing
    runtime overrides of the default settings.

    Attributes:
        driver_cpu: Number of CPUs allocated to the driver node.
        driver_memory: Memory allocation for the driver node (e.g., "4G", "512M").
        driver_disk: Disk space allocation for the driver node (e.g., "10G").
        driver_gpu: Number of GPUs allocated to the driver node.
        executor_cpu: Number of CPUs allocated per executor.
        executor_memory: Memory allocation per executor (e.g., "4G", "512M").
        executor_disk: Disk space allocation per executor (e.g., "10G").
        executor_gpu: Number of GPUs allocated per executor.
        executor_instances: Number of executor instances to launch.
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
        """Return the TaskBinding linking this config to its Starlark execution function.

        Returns:
            TaskBinding that specifies the Starlark file and function for Spark task execution.
        """
        return _binding

    @classmethod
    def get_config_binding(self) -> TaskBinding:
        """Return the TaskBinding for Spark configuration.

        Returns:
            TaskBinding that specifies the Starlark file and function for Spark configuration.
        """
        return _config_binding

    def pre_run(self):
        """Initialize the Spark session before task execution.

        Creates a Spark session with Hive support enabled. Additional Spark properties
        can be specified via the _SPARK_PROPERTIES environment variable as comma-separated
        key=value pairs.
        """
        sb = SparkSession.builder.enableHiveSupport()

        if props := os.environ.get("_SPARK_PROPERTIES"):
            for kv in props.split(","):
                k, v = kv.split("=", 1)
                sb = sb.config(k, v)

        spark = sb.getOrCreate()
        assert spark

    def post_run(self):
        """Stop the Spark session after task execution.

        Ensures proper cleanup of Spark resources by stopping the active session if one exists.
        """
        spark = SparkSession.getActiveSession()
        if spark:
            spark.stop()
