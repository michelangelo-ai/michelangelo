"""Demo app with a module-level with_overrides() copy of a task."""

from michelangelo.uniflow.core import task, workflow
from tests.uniflow.core.demo_platform.test_conf.task_b import TaskB


@task(config=TaskB(), alias="task_slow")
def task_slow(msg):
    """Task used both as is and through with_overrides()."""
    return {"status": "ok"}


task_fast = task_slow.with_overrides(alias="task_fast", retry_attempts=3)


@workflow()
def main(msg="hi"):
    """Call the original task and its module-level override."""
    task_slow(msg)
    return task_fast(msg)
