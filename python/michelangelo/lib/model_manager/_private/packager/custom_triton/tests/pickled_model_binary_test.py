"""Tests for the pickle model binary serialization."""

import os
import pickle
import tempfile
from pathlib import Path
from unittest import TestCase
from unittest.mock import patch

import numpy as np

from michelangelo.lib.model_manager._private.packager.custom_triton import (
    serialize_pickle_dependencies,
    serialize_pickled_file_dependencies,
)
from michelangelo.lib.model_manager._private.packager.custom_triton.tests.fixtures.invalid_model import (
    Model,
)
from michelangelo.lib.model_manager._private.packager.custom_triton.tests.fixtures.predict import (
    Predict,
)


class PickledModelBinaryTest(TestCase):
    """Tests for the pickle model binary serialization."""

    def test_serialize_pickle_dependencies(self):
        """Tests that the pickle dependencies are serialized."""
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            sub_model_path = os.path.join(model_path, "sub_model")
            os.makedirs(sub_model_path)
            fn1 = os.path.join(model_path, "test1.pkl")
            fn2 = os.path.join(sub_model_path, "test2.pkl")
            fn3 = os.path.join(model_path, "test3.txt")
            target_dir = os.path.join(temp_dir, "target")

            with open(fn1, "wb") as f:
                pickle.dump(Predict(), f)

            with open(fn2, "wb") as f:
                pickle.dump(Model(), f)

            with open(fn3, "w") as f:
                f.write("not a pickle")

            serialize_pickle_dependencies(
                model_path, target_dir, include_import_prefixes=["michelangelo"]
            )

            files = sorted(
                str(Path(os.path.join(dirpath, file)).relative_to(target_dir))
                for dirpath, _, filenames in os.walk(target_dir)
                for file in filenames
            )

            prefix = "michelangelo/lib/model_manager/"
            self.assertEqual(
                files,
                [
                    f"{prefix}_private/packager/custom_triton/tests/fixtures/invalid_model.py",
                    f"{prefix}_private/packager/custom_triton/tests/fixtures/predict.py",
                    f"{prefix}_private/utils/module_finder/tests/fixtures/folder/fn1.py",
                    f"{prefix}_private/utils/module_finder/tests/fixtures/folder/fn2.py",
                    f"{prefix}_private/utils/module_finder/tests/fixtures/folder/fn3.py",
                    f"{prefix}_private/utils/module_finder/tests/fixtures/folder/fn4.py",
                    f"{prefix}_private/utils/module_finder/tests/fixtures/package/__init__.py",
                    f"{prefix}_private/utils/module_finder/tests/fixtures/package/fn1.py",
                    f"{prefix}_private/utils/module_finder/tests/fixtures/package/fn2.py",
                    f"{prefix}_private/utils/module_finder/tests/fixtures/simple_module.py",
                    f"{prefix}interface/custom_model.py",
                ],
            )

    @patch(
        "michelangelo.lib.model_manager._private.packager.custom_triton.pickled_model_binary.find_pickle_definitions"
    )
    def test_serialize_pickle_dependencies_with_main(
        self, mock_find_pickle_definitions
    ):
        """Tests that the main module is serialized if it is a pickle dependency."""
        mock_find_pickle_definitions.return_value = ["__main__.test"]
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            os.makedirs(model_path)
            target_dir = os.path.join(temp_dir, "target")
            with open(os.path.join(model_path, "test.pkl"), "wb") as f:
                pickle.dump({}, f)

            serialize_pickle_dependencies(
                model_path, target_dir, include_import_prefixes=["michelangelo"]
            )
            self.assertTrue(len(os.listdir(target_dir)) > 0)

    def test_serialize_pickled_file_dependencies(self):
        """Tests that the pickle file dependencies are serialized."""
        with tempfile.TemporaryDirectory() as temp_dir:
            fn = os.path.join(temp_dir, "test.pkl")
            target_dir = os.path.join(temp_dir, "target")

            with open(fn, "wb") as f:
                pickle.dump(Predict(), f)

            serialize_pickled_file_dependencies(
                fn, target_dir, include_import_prefixes=["michelangelo"]
            )

            files = sorted(
                str(Path(os.path.join(dirpath, file)).relative_to(target_dir))
                for dirpath, _, filenames in os.walk(target_dir)
                for file in filenames
            )

            prefix = "michelangelo/lib/model_manager/"
            self.assertEqual(
                files,
                [
                    f"{prefix}_private/packager/custom_triton/tests/fixtures/predict.py",
                    f"{prefix}_private/utils/module_finder/tests/fixtures/folder/fn1.py",
                    f"{prefix}_private/utils/module_finder/tests/fixtures/folder/fn2.py",
                    f"{prefix}_private/utils/module_finder/tests/fixtures/folder/fn3.py",
                    f"{prefix}_private/utils/module_finder/tests/fixtures/folder/fn4.py",
                    f"{prefix}_private/utils/module_finder/tests/fixtures/package/__init__.py",
                    f"{prefix}_private/utils/module_finder/tests/fixtures/package/fn1.py",
                    f"{prefix}_private/utils/module_finder/tests/fixtures/package/fn2.py",
                    f"{prefix}_private/utils/module_finder/tests/fixtures/simple_module.py",
                    f"{prefix}interface/custom_model.py",
                ],
            )

    def test_serialize_pickled_file_dependencies_skip(self):
        """Tests that the pickle file dependencies are not serialized if they are not a pickle file."""
        with tempfile.TemporaryDirectory() as temp_dir:
            fn = os.path.join(temp_dir, "test.pkl")
            target_dir = os.path.join(temp_dir, "target")

            with open(fn, "wb") as f:
                pickle.dump(np.array([]), f)

            serialize_pickled_file_dependencies(
                fn, target_dir, include_import_prefixes=["michelangelo"]
            )

            files = sorted(
                str(Path(os.path.join(dirpath, file)).relative_to(target_dir))
                for dirpath, _, filenames in os.walk(target_dir)
                for file in filenames
            )

            self.assertEqual(files, [])
