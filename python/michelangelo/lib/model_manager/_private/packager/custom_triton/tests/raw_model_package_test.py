"""Tests for raw model package generation."""

import os
import re
import tempfile
from unittest import TestCase

import numpy as np

from michelangelo.lib.model_manager._private.packager.custom_triton import (
    generate_raw_model_package_content,
)
from michelangelo.lib.model_manager.schema import ModelSchema


class RawModelPackageTest(TestCase):
    """Tests for raw model package generation."""

    def test_generate_raw_model_package_content(self):
        """It generates the raw model package content."""
        for batch_inference in [False, True]:
            with self.subTest(batch_inference=batch_inference):
                with tempfile.TemporaryDirectory() as temp_dir:
                    content = generate_raw_model_package_content(
                        temp_dir,
                        "michelangelo.lib.model_manager._private.packager.custom_triton.tests.fixtures.predict.Predict",
                        ModelSchema(),
                        [{"input": np.array([1, 2])}],
                        include_import_prefixes=["michelangelo"],
                        batch_inference=batch_inference,
                    )

                self.assertIsNotNone(content)
                self.assertIn("metadata", content)
                self.assertIn("type.yaml", content["metadata"])
                self.assertIn("schema.yaml", content["metadata"])
                self.assertIn("sample_data.json", content["metadata"])
                self.assertIn("model", content)
                self.assertIn("defs", content)
                model = content["model"]
                self.assertIsNotNone(re.fullmatch(r"dir://(?:.+)/model", model))
                defs = content["defs"]
                self.assertIsNotNone(re.fullmatch(r"dir://(?:.+)/defs", defs))
                self.assertNotIn("dependencies", content)

    def test_generate_raw_model_package_content_with_requirements(self):
        """It generates the raw model package content with requirements."""
        with tempfile.TemporaryDirectory() as temp_dir:
            content = generate_raw_model_package_content(
                temp_dir,
                "michelangelo.lib.model_manager._private.packager.custom_triton.tests.fixtures.predict.Predict",
                ModelSchema(),
                [{"input": np.array([1, 2])}],
                requirements=["numpy", "torch"],
                include_import_prefixes=["michelangelo"],
            )

        self.assertIsNotNone(content)
        self.assertIn("metadata", content)
        self.assertIn("type.yaml", content["metadata"])
        self.assertIn("schema.yaml", content["metadata"])
        self.assertIn("sample_data.json", content["metadata"])
        self.assertIn("model", content)
        self.assertIn("defs", content)
        self.assertIn("dependencies", content)
        self.assertIn("requirements.txt", content["dependencies"])
        requirements = content["dependencies"]["requirements.txt"]
        self.assertEqual(requirements, "numpy\ntorch")

    def test_generate_raw_model_package_content_with_additional_import_prefixes(self):
        """It serializes additional dynamically-imported module prefixes."""
        with tempfile.TemporaryDirectory() as temp_dir:
            content = generate_raw_model_package_content(
                temp_dir,
                "michelangelo.lib.model_manager._private.packager.custom_triton.tests.fixtures.predict.Predict",
                ModelSchema(),
                [{"input": np.array([1, 2])}],
                include_import_prefixes=["michelangelo"],
                additional_import_prefixes=[
                    "michelangelo.lib.model_manager._private.utils.module_finder."
                    "tests.fixtures.simple_module"
                ],
            )

        defs_dir = content["defs"].removeprefix("dir://")
        expected_file = os.path.join(
            defs_dir,
            "michelangelo",
            "lib",
            "model_manager",
            "_private",
            "utils",
            "module_finder",
            "tests",
            "fixtures",
            "simple_module.py",
        )
        self.assertTrue(os.path.exists(expected_file))
