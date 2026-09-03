"""Four-task pipeline for verifying manual-retry cache behavior.

(task_a, task_b) -> (task_c, task_d). task_c and task_d take task_a's result
as an argument, so a retry of task_a that produces a new result naturally
invalidates task_c/task_d's cache (their input_hash changes) while task_b -
an independent sibling of task_a with no data dependency on it - keeps its
cache hit.
"""

from __future__ import annotations

import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.core.lib.concurrent import run as concurrent_run
from michelangelo.uniflow.plugins.ray import RayTask


@uniflow.task(
    config=RayTask(
        head_cpu=1,
        head_memory="512M",
    ),
)
def task_a(sleep_seconds: int, fail: bool = False) -> str:
    """Sleep for sleep_seconds, then succeed or raise if fail is set.

    Returns a fresh, unique result every genuine execution (not a static
    string), so a retry that actually re-runs task_a can be distinguished
    from a retry that silently reused a stale cached result.
    """
    import time
    import uuid

    time.sleep(sleep_seconds)
    if fail:
        raise RuntimeError("forced failure for task-a")
    return "task-a-done-" + uuid.uuid4().hex


@uniflow.task(
    config=RayTask(
        head_cpu=1,
        head_memory="512M",
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
    config=RayTask(
        head_cpu=1,
        head_memory="512M",
    ),
)
def task_c(sleep_seconds: int, upstream_result: str, fail: bool = False) -> str:
    """Sleep for sleep_seconds, then succeed or raise if fail is set.

    upstream_result (task_a's output) is part of the input args, so a change
    to task_a's result changes task_c's cache key too.
    """
    import time

    time.sleep(sleep_seconds)
    if fail:
        raise RuntimeError("forced failure for task-c")
    return "task-c-done:" + upstream_result


@uniflow.task(
    config=RayTask(
        head_cpu=1,
        head_memory="512M",
    ),
)
def task_d(sleep_seconds: int, upstream_result: str, fail: bool = False) -> str:
    """Sleep for sleep_seconds, then succeed or raise if fail is set.

    upstream_result (task_a's output) is part of the input args, so a change
    to task_a's result changes task_d's cache key too.
    """
    import time

    time.sleep(sleep_seconds)
    if fail:
        raise RuntimeError("forced failure for task-d")
    return "task-d-done:" + upstream_result


@uniflow.workflow()
def retry_cache_workflow(fail_b: bool = False):
    """Run (task_a, task_b) concurrently, then (task_c, task_d) concurrently.

    task_c/task_d take result_a as an argument (a genuine data dependency on
    task_a); task_b takes no argument from task_a (an independent sibling).
    """
    future_a = concurrent_run(task_a, 10, False)
    future_b = concurrent_run(task_b, 60, fail_b)
    result_a = future_a.result()
    result_b = future_b.result()

    future_c = concurrent_run(task_c, 10, result_a, False)
    future_d = concurrent_run(task_d, 30, result_a, False)
    result_c = future_c.result()
    result_d = future_d.result()

    return {"a": result_a, "b": result_b, "c": result_c, "d": result_d}


if __name__ == "__main__":
    ctx = uniflow.create_context()
    ctx.run(retry_cache_workflow, fail_b=False)
