import json
import unittest

import tests.uniflow.core.demo_app.demo_app as demo_app
import tests.uniflow.core.demo_platform.workflows as demo_platform_workflows
from michelangelo.uniflow.core.build import TranspilerCallback, build
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


class Test(unittest.TestCase):
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

    def test_demo_app(self):
        package = build(demo_app.main)

        # Find and assert the main file
        main_file = [
            p
            for p in package.files.keys()
            if p.endswith("/tests/uniflow/core/demo_app/demo_app.py")
        ]
        self.assertEqual(1, len(main_file))
        self.assertEqual(main_file[0], package.main_file)
        self.assertEqual("main", package.main_function)

        # Assert file paths in the package
        path_set = set()
        for path, ast_module in package.files.items():
            self.assertIsInstance(ast_module, bytes)
            if path != "meta.json":
                _, path = path.split("/tests/uniflow/core/")
            path_set.add(path)

        expected_path_set = {
            "demo_app/demo_app.py",
            "demo_platform/workflows.py",
            "demo_platform/test_conf/task_a.star",
            "demo_platform/test_conf/task_b.star",
            "demo_platform/test_conf/commons.star",
            "meta.json",
        }
        self.assertSetEqual(expected_path_set, path_set)

        meta = json.loads(package.files["meta.json"])
        expected_meta = {
            "main_file": package.main_file,
            "main_function": package.main_function,
        }
        self.assertEqual(expected_meta, meta)

    def test_build_with_transpiler_callback(self):
        class MyTranspilerCallback(TranspilerCallback):
            def __init__(self):
                self.task_functions = []

            def on_task_function(self, task_fn):
                self.task_functions.append(task_fn)

        transpiler_callback = MyTranspilerCallback()
        package0 = build(demo_app.main, transpiler_callback=transpiler_callback)
        self.assertIsNotNone(package0)

        task_functions = transpiler_callback.task_functions

        self.assertEqual(len(task_functions), 3)

        t = task_functions[0]
        self.assertEqual(t, demo_app.task_1)
        self.assertEqual(t.image_spec.container_image, "test_image:test")

        t = task_functions[1]
        self.assertEqual(t, demo_app.task_wrapped)
        self.assertIsNone(t.image_spec)

        t = task_functions[2]
        self.assertEqual(t, demo_platform_workflows._greetings_task)

        package1 = build(demo_app.main)
        self.assertEqual(package0, package1)
