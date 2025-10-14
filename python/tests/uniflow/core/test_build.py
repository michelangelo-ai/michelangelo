import json
import unittest

from michelangelo.uniflow.core.build import build, TranspilerCallback
import tests.uniflow.core.demo_app.demo_app as demo_app
import tests.uniflow.core.demo_platform.workflows as demo_platform_workflows


class Test(unittest.TestCase):
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
