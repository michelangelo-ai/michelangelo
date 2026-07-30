"""Tests for model package generation."""

import json
import os
import re
import tempfile
from unittest import TestCase

import numpy as np

from michelangelo.lib.model_manager._private.packager.custom_triton import (
    generate_model_package_content,
)
from michelangelo.lib.model_manager._private.packager.template_renderer import (
    TritonTemplateRenderer,
)
from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem


def _model_schema() -> ModelSchema:
    return ModelSchema(
        input_schema=[ModelSchemaItem(name="input", data_type=DataType.INT, shape=[1])],
        output_schema=[
            ModelSchemaItem(name="response", data_type=DataType.INT, shape=[1])
        ],
    )


class ModelPackageTest(TestCase):
    """Tests for model package generation."""

    def test_generate_model_package_content(self):
        """It generates the model package content."""
        with tempfile.TemporaryDirectory() as temp_dir:
            gen = TritonTemplateRenderer()
            input_schema = {
                "input": {
                    "type": "int32",
                    "shape": "[ 1 ]",
                },
            }
            output_schema = {
                "response": {
                    "type": "int32",
                    "shape": "[ 1 ]",
                },
            }

            content = generate_model_package_content(
                gen,
                temp_dir,
                "test_model_name",
                "test_model_revision",
                "michelangelo.lib.model_manager._private.packager.custom_triton.tests.fixtures.predict.Predict",
                _model_schema(),
                input_schema,
                output_schema,
                include_import_prefixes=["michelangelo"],
            )

            self.assertIsNotNone(content)
            self.assertIn("config.pbtxt", content)
            self.assertIn("0", content)
            self.assertIn("model.py", content["0"])
            self.assertIn("user_model.py", content["0"])
            predict = content["0"]["model_class.txt"]
            self.assertIsNotNone(
                re.fullmatch(r"file://(?:/.+)*/model_class.txt", predict)
            )
            model = content["0"]["model"]
            self.assertIsNotNone(re.fullmatch(r"dir://(?:/.+)*/model", model))
            self.assertIn("schema.yaml", content["metadata"])
            self.assertIn("name: input", content["metadata"]["schema.yaml"])

    def test_generate_model_package_content_with_sample_data(self):
        """It writes sample_data.json with an added batch dimension."""
        with tempfile.TemporaryDirectory() as temp_dir:
            gen = TritonTemplateRenderer()
            input_schema = {"input": {"type": "int32", "shape": "[ 1 ]"}}
            output_schema = {"response": {"type": "int32", "shape": "[ 1 ]"}}
            sample_data = [{"input": np.array([1])}]

            content = generate_model_package_content(
                gen,
                temp_dir,
                "test_model_name",
                "test_model_revision",
                "michelangelo.lib.model_manager._private.packager.custom_triton.tests.fixtures.predict.Predict",
                _model_schema(),
                input_schema,
                output_schema,
                include_import_prefixes=["michelangelo"],
                sample_data=sample_data,
            )

            self.assertIn("metadata", content)
            sample_data_json = content["metadata"]["sample_data.json"]
            loaded = json.loads(sample_data_json)
            self.assertEqual(loaded[0]["input"], [[1]])

    def test_generate_model_package_content_with_sample_data_custom_batch(self):
        """It does not add a batch dimension when batching is manual."""
        with tempfile.TemporaryDirectory() as temp_dir:
            gen = TritonTemplateRenderer()
            input_schema = {"input": {"type": "int32", "shape": "[ 1 ]"}}
            output_schema = {"response": {"type": "int32", "shape": "[ 1 ]"}}
            sample_data = [{"input": np.array([1])}]

            content = generate_model_package_content(
                gen,
                temp_dir,
                "test_model_name",
                "test_model_revision",
                "michelangelo.lib.model_manager._private.packager.custom_triton.tests.fixtures.predict.Predict",
                _model_schema(),
                input_schema,
                output_schema,
                include_import_prefixes=["michelangelo"],
                custom_batch_processing=True,
                sample_data=sample_data,
            )

            sample_data_json = content["metadata"]["sample_data.json"]
            loaded = json.loads(sample_data_json)
            self.assertEqual(loaded[0]["input"], [1])

    def test_generate_model_package_content_without_sample_data(self):
        """It writes schema.yaml but skips sample_data.json without sample_data."""
        with tempfile.TemporaryDirectory() as temp_dir:
            gen = TritonTemplateRenderer()
            input_schema = {"input": {"type": "int32", "shape": "[ 1 ]"}}
            output_schema = {"response": {"type": "int32", "shape": "[ 1 ]"}}

            content = generate_model_package_content(
                gen,
                temp_dir,
                "test_model_name",
                "test_model_revision",
                "michelangelo.lib.model_manager._private.packager.custom_triton.tests.fixtures.predict.Predict",
                _model_schema(),
                input_schema,
                output_schema,
                include_import_prefixes=["michelangelo"],
            )

            self.assertIn("schema.yaml", content["metadata"])
            self.assertNotIn("sample_data.json", content["metadata"])

    def test_generate_model_package_content_with_triton_parameters(self):
        """It threads triton_parameters into the generated config.pbtxt."""
        with tempfile.TemporaryDirectory() as temp_dir:
            gen = TritonTemplateRenderer()
            input_schema = {"input": {"type": "int32", "shape": "[ 1 ]"}}
            output_schema = {"response": {"type": "int32", "shape": "[ 1 ]"}}

            content = generate_model_package_content(
                gen,
                temp_dir,
                "test_model_name",
                "test_model_revision",
                "michelangelo.lib.model_manager._private.packager.custom_triton.tests.fixtures.predict.Predict",
                _model_schema(),
                input_schema,
                output_schema,
                include_import_prefixes=["michelangelo"],
                triton_parameters={"MY_CUSTOM_PARAM": "16"},
            )

            self.assertIn('key: "MY_CUSTOM_PARAM"', content["config.pbtxt"])

    def test_generate_model_package_content_with_additional_import_prefixes(self):
        """It serializes additional dynamically-imported module prefixes."""
        with tempfile.TemporaryDirectory() as temp_dir:
            gen = TritonTemplateRenderer()
            input_schema = {"input": {"type": "int32", "shape": "[ 1 ]"}}
            output_schema = {"response": {"type": "int32", "shape": "[ 1 ]"}}

            content = generate_model_package_content(
                gen,
                temp_dir,
                "test_model_name",
                "test_model_revision",
                "michelangelo.lib.model_manager._private.packager.custom_triton.tests.fixtures.predict.Predict",
                _model_schema(),
                input_schema,
                output_schema,
                include_import_prefixes=["michelangelo"],
                additional_import_prefixes=[
                    "michelangelo.lib.model_manager._private.utils.module_finder."
                    "tests.fixtures.simple_module"
                ],
            )

            model_class_uri = content["0"]["model_class.txt"]
            model_0_dir = os.path.dirname(model_class_uri.removeprefix("file://"))
            expected_file = os.path.join(
                model_0_dir,
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
            self.assertIn("model", content["0"])

