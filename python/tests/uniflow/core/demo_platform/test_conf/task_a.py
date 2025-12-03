from dataclasses import dataclass
from pathlib import Path

from michelangelo.uniflow.core.task_config import TaskBinding, TaskConfig

_binding = TaskBinding(
    star_file=Path(__file__).resolve().parent / "task_a.star",
    function="task_a",
    export="__task_a",
)


@dataclass
class TaskA(TaskConfig):  # noqa: D101
    def get_binding(self) -> TaskBinding:  # noqa: D102
        return _binding

    @classmethod
    def get_config_binding(cls) -> TaskBinding:  # noqa: D102
        return _binding

    def pre_run(self):  # noqa: D102
        pass

    def post_run(self):  # noqa: D102
        pass
