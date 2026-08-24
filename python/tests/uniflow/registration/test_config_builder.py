import unittest

import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.core.lib.concurrent import batch_run, new_callable
from michelangelo.uniflow.core.lib.concurrent import run as concurrent_run
from michelangelo.uniflow.registration.config_builder import ConfigBuilder


def task_a(*args):
    return "a"


def task_b(*args):
    return "b"


def task_c(*args):
    return "c"


def task_d(*args):
    return "d"


def simple_concurrent_wave():
    future_a = concurrent_run(task_a, 1)
    future_b = concurrent_run(task_b, 2)
    return {"a": future_a.result(), "b": future_b.result()}


def batch_run_wave():
    callables = [new_callable(task_a, 1), new_callable(task_b, 2)]
    batch = batch_run(callables)
    return batch.get()


def sequential_then_concurrent_pairs():
    future_a = concurrent_run(task_a, 1)
    future_b = concurrent_run(task_b, 2)
    result_a = future_a.result()
    result_b = future_b.result()

    future_c = concurrent_run(task_c, result_a)
    future_d = concurrent_run(task_d, result_b)
    result_c = future_c.result()
    result_d = future_d.result()
    return {"c": result_c, "d": result_d}


def purely_sequential():
    result_a = task_a(1)
    result_b = task_b(result_a)
    return {"a": result_a, "b": result_b}


@uniflow.workflow()
def decorated_concurrent_wave(fail_b: bool = False):
    # Mirrors python/examples/pipelines/retry_cache_test/retry_cache_test.py: a
    # real @uniflow.workflow()-decorated function is the wrapper closure the
    # decorator returns, whose __globals__ belong to the decorator's own module,
    # not this one - resolving concurrent_run's binding must unwrap back to the
    # original function first.
    future_a = concurrent_run(task_a, 10, False)
    future_b = concurrent_run(task_b, 60, fail_b)
    return {"a": future_a.result(), "b": future_b.result()}


def batch_run_with_dynamic_callables():
    callables = []
    for fn in (task_a, task_b):
        callables.append(new_callable(fn))
    batch = batch_run(callables)
    return batch.get()


class TestGetWorkflowConcurrentGroups(unittest.TestCase):
    def test_simple_concurrent_wave(self):
        groups = ConfigBuilder(simple_concurrent_wave).get_workflow_concurrent_groups()
        self.assertEqual([["task_a", "task_b"]], groups)

    def test_batch_run_wave(self):
        groups = ConfigBuilder(batch_run_wave).get_workflow_concurrent_groups()
        self.assertEqual([["task_a", "task_b"]], groups)

    def test_sequential_then_concurrent_pairs(self):
        # (a,b) -> (c,d): two separate waves, both should be reported so caching
        # auto-enables for every task forced to replay by a retry within its own batch.
        groups = ConfigBuilder(
            sequential_then_concurrent_pairs
        ).get_workflow_concurrent_groups()
        self.assertEqual([["task_a", "task_b"], ["task_c", "task_d"]], groups)

    def test_decorated_workflow_wrapper_resolves_globals_correctly(self):
        groups = ConfigBuilder(
            decorated_concurrent_wave
        ).get_workflow_concurrent_groups()
        self.assertEqual([["task_a", "task_b"]], groups)

    def test_purely_sequential_yields_no_groups(self):
        groups = ConfigBuilder(purely_sequential).get_workflow_concurrent_groups()
        self.assertEqual([], groups)

    def test_batch_run_dynamic_callables_falls_back_to_no_group(self):
        # callables list built via a loop can't be statically resolved - safe default
        # is to skip emitting a group rather than guessing wrong.
        groups = ConfigBuilder(
            batch_run_with_dynamic_callables
        ).get_workflow_concurrent_groups()
        self.assertEqual([], groups)


if __name__ == "__main__":
    unittest.main()
