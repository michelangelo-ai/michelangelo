import tempfile
import numpy as np
from unittest import TestCase
from uber.ai.michelangelo.sdk.model_manager.interface.custom_model import Model
from uber.ai.michelangelo.sdk.model_manager.interface.tests.fixtures.custom_model import CustomModel


class IncompleteModel(Model):
    def save(self, path: str):
        pass

    def predict(
        self,
        inputs: dict[str, np.ndarray],
    ):
        pass


class CustomModelTest(TestCase):
    def test_model(self):
        model = CustomModel("content")

        with tempfile.NamedTemporaryFile() as f:
            model.save(f.name)
            loaded_model = CustomModel.load(f.name)
            result = loaded_model.predict(inputs={"feature": np.array([1, 2])})
            self.assertEqual(result["response"].tolist(), [1, 2])
            self.assertEqual(result["content"].tolist(), ["content"])

    def test_model_load_not_implemented(self):
        with self.assertRaises(TypeError):
            IncompleteModel()

        with self.assertRaises(NotImplementedError):
            IncompleteModel.load("path")
