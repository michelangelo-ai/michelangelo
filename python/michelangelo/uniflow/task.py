from typing import Callable, TypeVar
from typing_extensions import ParamSpec

P = ParamSpec("P")
R = TypeVar("R")


def task(*, config):
    assert config

    def decorator(fn: Callable[P, R]) -> Callable[P, R]:
        def wrapper(*args, **kwargs) -> R:
            return fn(*args, **kwargs)

        return wrapper

    return decorator
