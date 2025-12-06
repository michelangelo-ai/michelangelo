"""Tests for raw model package generation."""
import re
import tempfile
import numpy as np
from unittest import TestCase
from michelangelo.lib.model_manager.schema import ModelSchema
from michelangelo.lib.model_manager._private.packager.custom_triton import generate_raw_model_package_content


class RawModelPackageTest(TestCase):
    """Tests for raw model package generation."""

    def test_generate_raw_model_package_content(self):
        """It generates the raw model package content."""
        with tempfile.TemporaryDirectory() as temp_dir:
            content = generate_raw_model_package_content(
                temp_dir,
                "michelangelo.lib.model_manager._private.packager.custom_triton.tests.fixtures.predict.Predict",
                ModelSchema(),
                [{"input": np.array([1, 2])}],
                include_import_prefixes=["michelangelo"],
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

    def test_generate_raw_model_package_content_with_batch_inference(self):
        """It generates the raw model package content with batch inference."""
        with tempfile.TemporaryDirectory() as temp_dir:
            content = generate_raw_model_package_content(
                temp_dir,
                "michelangelo.lib.model_manager._private.packager.custom_triton.tests.fixtures.predict.Predict",
                ModelSchema(),
                [{"input": np.array([1, 2])}],
                include_import_prefixes=["michelangelo"],
                batch_inference=True,
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
