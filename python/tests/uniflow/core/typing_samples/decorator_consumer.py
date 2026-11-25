"""Sample consumer of the uniflow decorators, type-checked by mypy.

This module is data for ``test_decorator_typing.py``: it is passed to mypy
and never imported or collected by pytest.
"""

from typing_extensions import assert_type

from michelangelo.uniflow.core import star_plugin, task, workflow
from michelangelo.uniflow.core.decorator import TaskFunction
from michelangelo.uniflow.core.task_config import TaskConfig


def make_config() -> TaskConfig:
    """Return a task config; never called, only type-checked."""
    raise NotImplementedError


@task(config=make_config())
def add(x: int, y: int = 1) -> int:
    """Add two integers."""
    return x + y


@workflow()
def pipeline(n: int) -> int:
    """Run the sample workflow."""
    return add(n, y=2)


@star_plugin(binding="sample.upper")
def upper(s: str) -> str:
    """Upper-case a string."""
    return s.upper()


as_task: TaskFunction[[int, int], int] = add
assert_type(add(1), int)
overridden: TaskFunction[[int, int], int] = add.with_overrides(alias="add_v2")
assert_type(add.__name__, str)
assert_type(pipeline(1), int)
assert_type(upper("a"), str)
