"""Four-task pipeline for verifying manual-retry cache behavior.

(task_a, task_b) -> (task_c, task_d), covering both the directly-retried
concurrent group and a downstream concurrent group forced to replay by the
same retry.
"""

from __future__ import annotations

import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.core.lib.concurrent import run as concurrent_run
from michelangelo.uniflow.plugins.spark import SparkTask


@uniflow.task(
    config=SparkTask(
        driver_cpu=1,
        driver_memory="512M",
        executor_cpu=1,
        executor_memory="512M",
        executor_instances=1,
    ),
)
def task_a(sleep_seconds: int, fail: bool = False) -> str:
    """Sleep for sleep_seconds, then succeed or raise if fail is set."""
    import time

    time.sleep(sleep_seconds)
    if fail:
        raise RuntimeError("forced failure for task-a")
    return "task-a-done"


@uniflow.task(
    config=SparkTask(
        driver_cpu=1,
        driver_memory="512M",
        executor_cpu=1,
        executor_memory="512M",
        executor_instances=1,
    ),
)
def task_b(sleep_seconds: int, fail: bool = False) -> str:
    """Sleep for sleep_seconds, then succeed or raise if fail is set."""
    import time

    time.sleep(sleep_seconds)
    if fail:
        raise RuntimeError("forced failure for task-b")
    return "task-b-done"


@uniflow.task(
    config=SparkTask(
        driver_cpu=1,
        driver_memory="512M",
        executor_cpu=1,
        executor_memory="512M",
        executor_instances=1,
    ),
)
def task_c(sleep_seconds: int, fail: bool = False) -> str:
    """Sleep for sleep_seconds, then succeed or raise if fail is set."""
    import time

    time.sleep(sleep_seconds)
    if fail:
        raise RuntimeError("forced failure for task-c")
    return "task-c-done"


@uniflow.task(
    config=SparkTask(
        driver_cpu=1,
        driver_memory="512M",
        executor_cpu=1,
        executor_memory="512M",
        executor_instances=1,
    ),
)
def task_d(sleep_seconds: int, fail: bool = False) -> str:
    """Sleep for sleep_seconds, then succeed or raise if fail is set."""
    import time

    time.sleep(sleep_seconds)
    if fail:
        raise RuntimeError("forced failure for task-d")
    return "task-d-done"


@uniflow.workflow()
def retry_cache_workflow(fail_b: bool = False):
    """Run (task_a, task_b) concurrently, then (task_c, task_d) concurrently."""
    future_a = concurrent_run(task_a, 10, False)
    future_b = concurrent_run(task_b, 60, fail_b)
    result_a = future_a.result()
    result_b = future_b.result()

    future_c = concurrent_run(task_c, 10, False)
    future_d = concurrent_run(task_d, 30, False)
    result_c = future_c.result()
    result_d = future_d.result()

    return {"a": result_a, "b": result_b, "c": result_c, "d": result_d}


if __name__ == "__main__":
    ctx = uniflow.create_context()
    ctx.run(retry_cache_workflow, fail_b=False)
