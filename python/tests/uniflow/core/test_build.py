import json
import unittest

from michelangelo.uniflow.core.build import build


class Test(unittest.TestCase):
    def test_demo_app(self):
        from tests.uniflow.core.demo_app.demo_app import main
        package = build(main)

        # Find and assert the main file
        main_file = [
            p for p in package.files.keys() if p.endswith("/tests/uniflow/core/demo_app/demo_app.py")
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
