from dataclasses import dataclass
import logging
from pathlib import Path

from typing import Optional
import ray
from ray.data import Dataset

from michelangelo.uniflow.core.io_registry import io_registry
from michelangelo.uniflow.core.task_config import TaskConfig, TaskBinding
from michelangelo.uniflow.plugins.ray.io import RayDatasetIO

log = logging.getLogger(__name__)

io_registry()[Dataset] = RayDatasetIO

_binding = TaskBinding(
    star_file=Path(__file__).resolve().parent / "task.star",
    function="task",
    export="__ray_task",
)


@dataclass
class RayTask(TaskConfig):
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

    def get_binding(self) -> TaskBinding:
        return _binding

    def pre_run(self):
        ray.init()

    def post_run(self):
        ray.shutdown()
