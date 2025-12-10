from __future__ import annotations
import os
import numpy as np
from michelangelo.lib.model_manager.interface.custom_model import Model
from uber.ai.michelangelo.experimental.model_manager_playground.python_triton_model.package import (
    fn1,
)
from uber.ai.michelangelo.experimental.model_manager_playground.python_triton_model.folder.fn2 import (
    fn2,
)


class Predict(Model):
    def __init__(self, content: str):
        self.content = content

    def save(self, path: str):
        with open(os.path.join(path, "test_file.txt"), "w") as f:
            f.write(self.content)

    @classmethod
    def load(cls, path) -> Predict:
        model_file = os.path.join(path, "test_file.txt")
        content = ""

        with open(model_file) as f:
            content = f.read()

        return Predict(content)

    def predict(
        self,
        inputs: dict[str, np.ndarray],
    ) -> dict[str, np.ndarray]:
        feature = inputs.get("feature")[0]
        response = f"feature: {feature} and content: {self.content} and deps: {fn1()} and deps: {fn2()}"
        array = np.array([response])
        return {"response": array}
