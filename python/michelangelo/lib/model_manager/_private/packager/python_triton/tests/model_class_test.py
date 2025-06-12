from unittest import TestCase
import os
import tempfile
from pathlib import Path
from michelangelo.lib.model_manager._private.packager.python_triton import serialize_model_class

# enable metabuild to build bazel dependencies
import michelangelo.lib.model_manager._private.packager.python_triton.tests.fixtures.predict  # noqa:F401


class ModelClassTest(TestCase):
    def test_serialize_model_class(self):
        model_class = "michelangelo.lib.model_manager._private.packager.python_triton.tests.fixtures.predict.Predict"

        with tempfile.TemporaryDirectory() as temp_dir:
            target_dir = os.path.join(temp_dir, "package")
            serialize_model_class(model_class, target_dir, "model_class.txt", include_import_prefixes=["michelangelo"])

            # test we can load the model file
            with open(os.path.join(target_dir, "model_class.txt")) as f:
                content = f.read()
                self.assertEqual(content, model_class)

            # test the dependencies are serialized
            with open(
                os.path.join(
                    target_dir,
                    "michelangelo",
                    "lib",
                    "model_manager",
                    "_private",
                    "packager",
                    "python_triton",
                    "tests",
                    "fixtures",
                    "predict.py",
                ),
            ) as f:
                content = f.read()
                self.assertIn("class Predict(Model):", content)

            files = sorted(
                [
                    str(
                        Path(os.path.join(dirpath, file)).relative_to(target_dir),
                    )
                    for dirpath, _, filenames in os.walk(target_dir)
                    for file in filenames
                ],
            )

            self.assertEqual(
                files,
                [
                    "michelangelo/lib/model_manager/_private/packager/python_triton/tests/fixtures/predict.py",
                    "michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn1.py",
                    "michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn2.py",
                    "michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn3.py",
                    "michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn4.py",
                    "michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/__init__.py",
                    "michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/fn1.py",
                    "michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/fn2.py",
                    "michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/simple_module.py",
                    "michelangelo/lib/model_manager/interface/custom_model.py",
                    "model_class.txt",
                ],
            )
