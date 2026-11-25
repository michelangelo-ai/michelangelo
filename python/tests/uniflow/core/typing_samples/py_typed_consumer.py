"""Sample consumer exercising the symbols annotated for the uniflow/ py.typed marker.

This module is data for ``test_py_typed.py``: it is passed to mypy and never
imported or collected by pytest. Unlike ``decorator_consumer.py``, this one is
checked through the package's normal import resolution (no MYPYPATH source-tree
override), the same way a real downstream consumer would see it once the
package is installed with the py.typed marker in place.
"""

from dataclasses import dataclass
from pathlib import Path

from typing_extensions import assert_type

from michelangelo.uniflow.core.context import Context, create_context
from michelangelo.uniflow.core.lib.concurrent import run
from michelangelo.uniflow.core.task_config import TaskBinding, TaskConfig

_binding = TaskBinding(star_file=Path(__file__), function="noop", export="__noop")


@dataclass
class _SampleTaskConfig(TaskConfig):
    def get_binding(self) -> TaskBinding:
        return _binding

    @classmethod
    def get_config_binding(cls) -> TaskBinding:
        return _binding

    def pre_run(self) -> None:
        pass

    def post_run(self) -> None:
        pass


def _sample_fn() -> int:
    return 1


ctx: Context = create_context()
is_local: bool = ctx.is_local_run()
ctx.run(_sample_fn)
assert_type(is_local, bool)

future = run(_sample_fn)
result = future.result()
assert_type(future.done(), bool)

config = _SampleTaskConfig()
config.pre_run()
config.post_run()
