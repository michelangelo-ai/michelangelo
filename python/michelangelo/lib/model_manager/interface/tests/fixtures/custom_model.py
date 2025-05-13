import numpy as np
from uber.ai.michelangelo.sdk.model_manager.interface.custom_model import Model


class CustomModel(Model):
    def __init__(self, content: str):
        self.content = content

    def predict(
        self,
        inputs: dict[str, np.ndarray],
    ):
        return {
            "response": inputs["feature"],
            "content": np.array([self.content]),
        }

    def save(self, path: str):
        with open(path, "w") as f:
            f.write(self.content)

    @classmethod
    def load(cls, path) -> "CustomModel":
        with open(path) as f:
            content = f.read()

        return CustomModel(content)
