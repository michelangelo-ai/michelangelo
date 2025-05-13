import os
import tempfile
import numpy as np
from unittest import TestCase
from michelangelo.lib.model_manager.constants import StorageType
from michelangelo.lib.model_manager.schema import ModelSchema, ModelSchemaItem, DataType
from michelangelo.lib.model_manager.packager.python_triton import PythonTritonPackager
from michelangelo.lib.model_manager._private.packager.python_triton.tests.fixtures.model_for_validation import Predict


class ValidationTest(TestCase):
    def setUp(self):
        self.schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="input", data_type=DataType.STRING, shape=[1]),
            ],
            output_schema=[
                ModelSchemaItem(name="response", data_type=DataType.STRING, shape=[1]),
            ],
        )
        self.sample_data = [{"input": np.array(["test_input"])}]

    def generate_package(self, model_path: str, model_class: str, dest_model_path: str):
        packager = PythonTritonPackager()
        return packager.create_raw_model_package(
            model_path=model_path,
            model_class=model_class,
            model_schema=self.schema,
            sample_data=self.sample_data,
            model_path_source_type=StorageType.LOCAL,
            dest_model_path=dest_model_path,
        )

    def test_validate_raw_model_package(self):
        predict = Predict("test_content")
        model_class = "michelangelo.lib.model_manager._private.packager.python_triton.tests.fixtures.model_for_validation.Predict"
        with tempfile.TemporaryDirectory() as temp_dir:
            src_model_path = os.path.join(temp_dir, "model")
            dest_model_path = os.path.join(temp_dir, "model_package")
            os.makedirs(src_model_path)
            predict.save(src_model_path)
            self.generate_package(src_model_path, model_class, dest_model_path)

    def test_validate_raw_model_package_predict_error(self):
        predict = Predict("test_content")
        model_class = "michelangelo.lib.model_manager._private.packager.python_triton.tests.fixtures.model_for_validation.ModelWithPredictError"
        with tempfile.TemporaryDirectory() as temp_dir:
            src_model_path = os.path.join(temp_dir, "model")
            dest_model_path = os.path.join(temp_dir, "model_package")
            os.makedirs(src_model_path)
            predict.save(src_model_path)
            with self.assertRaisesRegex(RuntimeError, "Error when validating the raw model package. Error when test prediction with the model"):
                self.generate_package(src_model_path, model_class, dest_model_path)

    def test_validate_raw_model_package_invalid_output(self):
        predict = Predict("test_content")
        model_class = "michelangelo.lib.model_manager._private.packager.python_triton.tests.fixtures.model_for_validation.ModelWithInvalidOutput"
        with tempfile.TemporaryDirectory() as temp_dir:
            src_model_path = os.path.join(temp_dir, "model")
            dest_model_path = os.path.join(temp_dir, "model_package")
            os.makedirs(src_model_path)
            predict.save(src_model_path)
            with self.assertRaisesRegex(RuntimeError, "Error when validating the raw model package. Error validating model output data"):
                self.generate_package(src_model_path, model_class, dest_model_path)

    def test_validate_raw_model_package_output_not_matching_schema(self):
        predict = Predict("test_content")
        model_class = (
            "michelangelo.lib.model_manager._private.packager.python_triton.tests.fixtures.model_for_validation.ModelWithOutputNotMatchingSchema"
        )
        with tempfile.TemporaryDirectory() as temp_dir:
            src_model_path = os.path.join(temp_dir, "model")
            dest_model_path = os.path.join(temp_dir, "model_package")
            os.makedirs(src_model_path)
            predict.save(src_model_path)
            with self.assertRaisesRegex(
                RuntimeError, "Error when validating the raw model package. Error validating model output data. Data fields do not match schema"
            ):
                self.generate_package(src_model_path, model_class, dest_model_path)

    def test_validate_raw_model_package_with_save_error(self):
        predict = Predict("test_content")
        model_class = "michelangelo.lib.model_manager._private.packager.python_triton.tests.fixtures.model_for_validation.ModelWithSaveError"
        with tempfile.TemporaryDirectory() as temp_dir:
            src_model_path = os.path.join(temp_dir, "model")
            dest_model_path = os.path.join(temp_dir, "model_package")
            os.makedirs(src_model_path)
            predict.save(src_model_path)
            with self.assertRaisesRegex(RuntimeError, "Error when validating the raw model package. Error when test saving the model"):
                self.generate_package(src_model_path, model_class, dest_model_path)

    def test_validate_raw_model_package_with_reload_error(self):
        predict = Predict("test_content")
        model_class = "michelangelo.lib.model_manager._private.packager.python_triton.tests.fixtures.model_for_validation.ModelWithReloadError"
        with tempfile.TemporaryDirectory() as temp_dir:
            src_model_path = os.path.join(temp_dir, "model")
            dest_model_path = os.path.join(temp_dir, "model_package")
            os.makedirs(src_model_path)
            predict.save(src_model_path)
            with self.assertRaisesRegex(
                RuntimeError, "Error when validating the raw model package. Error when test reloading the saved model, please double check"
            ):
                self.generate_package(src_model_path, model_class, dest_model_path)

    def test_validate_raw_model_package_with_invalid_model_class_after_reload(self):
        predict = Predict("test_content")
        model_class = "michelangelo.lib.model_manager._private.packager.python_triton.tests.fixtures.model_for_validation.ModelWithMismatchingLoad"
        with tempfile.TemporaryDirectory() as temp_dir:
            src_model_path = os.path.join(temp_dir, "model")
            dest_model_path = os.path.join(temp_dir, "model_package")
            os.makedirs(src_model_path)
            predict.save(src_model_path)
            with self.assertRaisesRegex(RuntimeError, "Error when validating the raw model package. The loaded model is not an instance of"):
                self.generate_package(src_model_path, model_class, dest_model_path)
