from michelangelo.uniflow.core import task, workflow
from michelangelo.uniflow.core.lib.time import sleep
from tests.uniflow.core.demo_platform.test_conf.task_a import TaskA

GREETINGS_TEMPLATE = "Hello {}!"


@task(config=TaskA())
def _greetings_task(_greetings):
    return {"status": "ok"}


@workflow()
def greetings(user_name="Jane"):
    g = "Hello {}!".format(user_name)  # noqa: UP032
    sleep(seconds=1)
    return _greetings_task(g)


@workflow()
def echo(*args):
    return args
