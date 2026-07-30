"""Tests for pipeline_task (michelangelo.canvas.pipeline.task)."""

import unittest

from michelangelo.canvas.pipeline.task import pipeline_task
from michelangelo.canvas.schema.v2alpha1.config import TaskConfig as EnvelopeTaskConfig
from tests.uniflow.core.test_task_config import TaskA


class PipelineTaskTest(unittest.TestCase):
    """Tests for the pipeline_task decorator."""

    def test_unwraps_envelope_config_kwarg(self):
        """The envelope's inner config is passed to the wrapped function via kwarg."""
        received = {}

        @pipeline_task(config=TaskA())
        def my_task(config):
            received["config"] = config
            return config

        envelope = EnvelopeTaskConfig(
            task_function="", config={"lr": 0.1}, job_specs=None
        )
        result = my_task(config=envelope)

        self.assertEqual(received["config"], {"lr": 0.1})
        self.assertEqual(result, {"lr": 0.1})

    def test_unwraps_envelope_config_positional(self):
        """The envelope's inner config is passed positionally to the wrapped fn."""
        received = {}

        @pipeline_task(config=TaskA())
        def my_task(config):
            received["config"] = config
            return config

        envelope = EnvelopeTaskConfig(task_function="", config="value", job_specs=None)
        result = my_task(envelope)

        self.assertEqual(received["config"], "value")
        self.assertEqual(result, "value")

    def test_hooks_are_invoked_in_order(self):
        """pre_hook, the task body, and post_hook run in that order."""
        calls = []

        @pipeline_task(
            config=TaskA(),
            pre_hook=lambda: calls.append("pre"),
            post_hook=lambda result: calls.append(("post", result)),
        )
        def my_task(config):
            calls.append("run")
            return "ok"

        envelope = EnvelopeTaskConfig(task_function="", config=None, job_specs=None)
        my_task(config=envelope)

        self.assertEqual(calls, ["pre", "run", ("post", "ok")])

    def test_on_error_is_invoked_and_exception_reraised(self):
        """on_error is called with the exception, which is still re-raised."""
        errors = []

        @pipeline_task(config=TaskA(), on_error=lambda e: errors.append(e))
        def my_task(config):
            raise ValueError("boom")

        envelope = EnvelopeTaskConfig(task_function="", config=None, job_specs=None)
        with self.assertRaises(ValueError):
            my_task(config=envelope)

        self.assertEqual(len(errors), 1)
        self.assertIsInstance(errors[0], ValueError)


if __name__ == "__main__":
    unittest.main()
