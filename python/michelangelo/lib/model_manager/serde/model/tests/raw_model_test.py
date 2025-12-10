import os
import tempfile
import numpy as np
import torch
import yaml
from unittest import TestCase
from michelangelo.lib.model_manager.constants import StorageType
from michelangelo.lib.model_manager.schema import ModelSchema, ModelSchemaItem, DataType
from michelangelo.lib.model_manager.packager.custom_triton import CustomTritonPackager
from michelangelo.lib.model_manager.serde.model import load_raw_model
from michelangelo.lib.model_manager.constants import RawModelType
from michelangelo.lib.model_manager.packager.custom_triton.tests.fixtures.predict import (
    Predict,
)


class RawModelTest(TestCase):
    def setUp(self):
        self.model_schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(
                    name="input",
                    data_type=DataType.INT,
                    shape=[1],
                ),
            ],
            output_schema=[
                ModelSchemaItem(
                    name="response",
                    data_type=DataType.INT,
                    shape=[1],
                ),
            ],
        )
        self.sample_data = [{"input": np.array([1])}]

    def create_model_package(self, directory: str):
        model_class = "michelangelo.lib.model_manager.packager.custom_triton.tests.fixtures.predict.Predict"
        src_model_path = os.path.join(directory, "model")
        dest_model_path = os.path.join(directory, "model_package")
        os.makedirs(src_model_path)
        os.makedirs(dest_model_path)

        with open(os.path.join(src_model_path, "test_file.txt"), "w") as f:
            f.write("test_content")

        packager = CustomTritonPackager()

        model_package = packager.create_raw_model_package(
            model_path=src_model_path,
            model_class=model_class,
            model_schema=self.model_schema,
            sample_data=self.sample_data,
            dest_model_path=dest_model_path,
            include_import_prefixes=["michelangelo"],
        )

        return model_package

    def test_load_raw_model(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            model_package = self.create_model_package(temp_dir)

            model = load_raw_model(model_package)

            self.assertIsInstance(model, Predict)

    def test_load_raw_model_loader_not_implemented(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            model_package = self.create_model_package(temp_dir)

            with open(os.path.join(model_package, "metadata", "type.yaml"), "w") as f:
                f.write(f"type: {RawModelType.HUGGINGFACE}")

            with self.assertRaises(NotImplementedError):
                load_raw_model(model_package)
