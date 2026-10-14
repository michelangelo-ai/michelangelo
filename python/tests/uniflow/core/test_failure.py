"""Tests for local workflow failure callbacks and remote packaging."""

import json
import unittest

from michelangelo.uniflow.core.build import build
from michelangelo.uniflow.core.decorator import workflow
from michelangelo.uniflow.core.failure import FailureEntrypoint, WorkflowFailure


def report_workflow_failure(failure: WorkflowFailure) -> str:
    """Return a field supported by both local and remote failure values."""
    return failure.reason


@workflow(on_failure=report_workflow_failure)
def handled_workflow():
    """Provide a workflow whose failure handler can be packaged."""
    return "ok"


def lower_failure(workflow_file, workflow_function, handler_file, handler_function):
    """Generate a minimal backend entrypoint for builder tests."""
    return FailureEntrypoint(
        main_file="__failure_entrypoint.star",
        main_function="__failure_entrypoint",
        source=(
            f'load("{workflow_file}", original = "{workflow_function}")\n'
            f'load("{handler_file}", handler = "{handler_function}")\n'
            "def __failure_entrypoint():\n"
            "    return original()\n"
        ),
    )


class TestWorkflowFailure(unittest.TestCase):
    """Verify local reporting and backend packaging behavior."""

    def test_handled_workflow_requires_backend(self):
        """Remote builds must reject handlers without a backend."""
        with self.assertRaisesRegex(RuntimeError, "on_failure requires a remote"):
            build(handled_workflow)

    def test_handled_workflow_packages_callback_and_entrypoint(self):
        """The callback and chosen entrypoint must both be packaged."""
        package = build(handled_workflow, failure_lowering=lower_failure)

        self.assertEqual(package.main_file, "__failure_entrypoint.star")
        self.assertEqual(package.main_function, "__failure_entrypoint")
        self.assertIn(
            b"def report_workflow_failure(failure):", b"".join(package.files.values())
        )
        self.assertNotIn(b"WorkflowFailure", b"".join(package.files.values()))
        self.assertIn(b"return failure.reason", b"".join(package.files.values()))
        self.assertIn(b"return original()", package.files[package.main_file])
        self.assertEqual(
            json.loads(package.files["meta.json"]),
            {"main_file": package.main_file, "main_function": package.main_function},
        )

    def test_handled_workflow_uses_decorator_backend(self):
        """A decorator-provided backend also reaches direct build calls."""
        handled_workflow._uf_failure_lowering = lower_failure
        try:
            package = build(handled_workflow)
            self.assertEqual(package.main_function, "__failure_entrypoint")
        finally:
            handled_workflow._uf_failure_lowering = None

    def test_on_failure_receives_classified_failure_and_reraises(self):
        """Local handlers receive classification without swallowing the error."""
        failures = []

        @workflow(on_failure=failures.append)
        def failing_workflow():
            raise ValueError("source failed")

        with self.assertRaisesRegex(ValueError, "source failed"):
            failing_workflow()

        self.assertEqual(len(failures), 1)
        self.assertEqual(failures[0].reason, "ValueError")
        self.assertEqual(failures[0].details, {"exception_type": "ValueError"})
        self.assertIn("source failed", failures[0].backtrace)

    def test_on_failure_preserves_original_if_handler_fails(self):
        """A failed reporter must not replace the workflow error."""

        def failing_handler(_failure):
            raise RuntimeError("handler failed")

        @workflow(on_failure=failing_handler)
        def failing_workflow():
            raise ValueError("source failed")

        with (
            self.assertLogs("michelangelo.uniflow.core.decorator", level="ERROR"),
            self.assertRaisesRegex(ValueError, "source failed"),
        ):
            failing_workflow()

    def test_on_failure_is_not_called_after_success(self):
        """Successful workflows do not invoke the failure handler."""
        failures = []

        @workflow(on_failure=failures.append)
        def successful_workflow():
            return "ok"

        self.assertEqual(successful_workflow(), "ok")
        self.assertEqual(failures, [])
