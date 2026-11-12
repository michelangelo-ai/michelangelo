import numpy as np

from michelangelo.lib.model_manager.interface.custom_model import Model


class Predict(Model):
    """Custom model that returns its ``input`` column as ``response``."""

    def save(self, path: str):
        assert path

    @classmethod
    def load(cls, path) -> "Predict":
        assert path
        return Predict()

    def predict(
        self,
        inputs: dict[str, np.ndarray],
    ) -> dict[str, np.ndarray]:
        return {"response": inputs.get("input")}
