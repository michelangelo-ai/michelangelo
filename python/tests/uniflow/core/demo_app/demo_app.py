from functools import wraps
import logging
from typing import Callable

from michelangelo.uniflow.core import workflow, task
from michelangelo.uniflow.core.lib.time import sleep
from michelangelo.uniflow.core.utils import LOGGING_FORMAT
from tests.uniflow.core.demo_platform.test_conf.task_b import TaskB
from tests.uniflow.core.demo_platform.workflows import greetings
from tests.uniflow.core.demo_platform.workflows import greetings as greetings_renamed

GREETINGS_TEMPLATE = "Hello {}!"

log = logging.getLogger(__name__)


def test_wrapper(fn: Callable):
    @wraps(fn)
    def wrapper(*args, **kwargs):
        return fn(*args, **kwargs)

    return wrapper


@task(config=TaskB())
def task_1(msg):
    log.info("task_1: msg: %r", msg)
    return {"status": "ok"}


@task(config=TaskB())
@test_wrapper
def task_wrapped(msg):
    log.info("task_wrapped: msg: %r", msg)
    return {"status": "ok"}


@workflow()
def main(user_name="Anon"):
    sleep(seconds=1)
    task_1("test " + user_name)
    task_wrapped("test " + user_name)
    print(_sum(5, 5))
    result_1 = greetings(user_name)
    result_2 = greetings_renamed(user_name)
    return {
        "result_1": result_1,
        "result_2": result_2,
    }


@workflow()
def _sum(a, b):
    sleep(seconds=2)
    return a + b


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format=LOGGING_FORMAT)
    main(user_name="Jane")
