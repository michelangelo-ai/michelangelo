from dataclasses import dataclass
from pathlib import Path

from michelangelo.uniflow.core.task_config import TaskConfig, TaskBinding

_binding = TaskBinding(
    star_file=Path(__file__).resolve().parent / "task_b.star",
    function="task_b",
    export="__task_b",
)


@dataclass
class TaskB(TaskConfig):
    def get_binding(self) -> TaskBinding:
        return _binding

    def pre_run(self):
        pass

    def post_run(self):
        pass
