"""Ray task configuration and execution for Uniflow workflows.

This module provides task configuration for executing Uniflow workflows on Ray clusters.
It handles Ray cluster initialization, resource allocation, and lifecycle management
for distributed task execution.

Ray tasks support configurable resources for both head and worker nodes, including
CPU, memory, disk, and GPU allocations. The execution model initializes a Ray cluster
before running the task and properly shuts it down afterward.
"""

import logging
import os
from dataclasses import dataclass
from pathlib import Path
from typing import Optional

import ray
from michelangelo.uniflow.core.io_registry import io_registry
from michelangelo.uniflow.core.task_config import TaskBinding, TaskConfig
from michelangelo.uniflow.plugins.ray.io import RayDatasetIO
from ray.data import Dataset

log = logging.getLogger(__name__)

io_registry()[Dataset] = RayDatasetIO

_binding = TaskBinding(
    star_file=Path(__file__).resolve().parent / "task.star",
    function="task",
    export="__ray_task",
)

_config_binding = TaskBinding(
    star_file=Path(__file__).resolve().parent / "task.star",
    function="ray_config",
    export="__ray_config",
)


@dataclass
class RayTask(TaskConfig):
    """Configuration for Ray-based task execution in Uniflow workflows.

    This class defines resource specifications and runtime configuration for executing
    tasks on Ray clusters. It manages the lifecycle of Ray cluster initialization and
    shutdown through pre_run and post_run hooks.

    Unlike SparkTask which uses driver/executor terminology, RayTask uses head/worker
    terminology to describe the cluster nodes. The head node coordinates execution
    while worker nodes perform distributed computation.

    Attributes:
        head_cpu: Number of CPUs allocated to the head node.
        head_memory: Memory allocation for the head node (e.g., "4G", "512M").
        head_disk: Disk space allocation for the head node (e.g., "10G").
        head_gpu: Number of GPUs allocated to the head node.
        head_object_store_memory: Object store memory for the head node in bytes.
        worker_cpu: Number of CPUs allocated per worker node.
        worker_memory: Memory allocation per worker node (e.g., "4G", "512M").
        worker_disk: Disk space allocation per worker node (e.g., "10G").
        worker_gpu: Number of GPUs allocated per worker node.
        worker_object_store_memory: Object store memory per worker node in bytes.
        worker_instances: Number of worker instances to launch.
        breakpoint: If True, enables breakpoint debugging for the task.
        runtime_env: Runtime environment configuration dict for Ray
            (packages, env vars, etc.).
    """

    head_cpu: Optional[int] = None
    head_memory: Optional[str] = None
    head_disk: Optional[str] = None
    head_gpu: Optional[int] = None
    head_object_store_memory: Optional[int] = None
    worker_cpu: Optional[int] = None
    worker_memory: Optional[str] = None
    worker_disk: Optional[str] = None
    worker_gpu: Optional[int] = None
    worker_object_store_memory: Optional[int] = None
    worker_instances: Optional[int] = None
    breakpoint: Optional[bool] = None
    runtime_env: Optional[dict] = None

    def get_binding(self) -> TaskBinding:
        """Return the TaskBinding linking this config to its Starlark function.

        Returns:
            TaskBinding that specifies the Starlark file and function for
            Ray task execution.
        """
        return _binding

    @classmethod
    def get_config_binding(cls) -> TaskBinding:
        """Return the TaskBinding for Ray configuration.

        Returns:
            TaskBinding that specifies the Starlark file and function for
            Ray configuration.
        """
        return _config_binding

    def pre_run(self):
        """Initialize the Ray cluster before task execution.

        Reads Ray initialization parameters from the _RAY_INIT_KWARGS
        environment variable and initializes the Ray runtime with those
        parameters.
        """
        ray_init_kwargs = eval(os.environ.get("_RAY_INIT_KWARGS", "{}"))
        log.info(f"_RAY_INIT_KWARGS: {ray_init_kwargs}")
        ray.init(**ray_init_kwargs)

    def post_run(self):
        """Shut down the Ray cluster after task execution.

        Ensures proper cleanup of Ray resources by shutting down the Ray runtime.
        """
        ray.shutdown()
