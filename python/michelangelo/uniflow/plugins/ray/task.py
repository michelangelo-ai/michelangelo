from dataclasses import dataclass
import logging
from pathlib import Path
import os
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

_config_binding = TaskBinding(
    star_file=Path(__file__).resolve().parent / "task.star",
    function="ray_config",
    export="__ray_config",
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
    runtime_env: Optional[dict] = None

    def get_binding(self) -> TaskBinding:
        return _binding

    @classmethod
    def get_config_binding(cls) -> TaskBinding:
        return _config_binding

    def pre_run(self):
        ray_init_kwargs = eval(os.environ.get("_RAY_INIT_KWARGS", "{}"))
        log.info(f"_RAY_INIT_KWARGS: {ray_init_kwargs}")
        ray.init(**ray_init_kwargs)

    def post_run(self):
        ray.shutdown()
