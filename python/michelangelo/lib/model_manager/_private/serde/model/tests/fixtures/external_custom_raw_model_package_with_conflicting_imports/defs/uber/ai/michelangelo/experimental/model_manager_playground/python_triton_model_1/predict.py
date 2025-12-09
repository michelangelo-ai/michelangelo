from __future__ import annotations
import os
import numpy as np
import uber.ai.michelangelo.sdk.model_manager._private.serde.model.dummy_module as dummy_module
from uber.ai.michelangelo.sdk.model_manager.interface.custom_model import Model
from uber.ai.michelangelo.sdk.model_manager._private.serde.model.raw_model_type import dummy_type
from uber.ai.michelangelo.sdk.model_manager._private.serde.model.tests.custom_raw_model_test import foo
from uber.ai.michelangelo.experimental.model_manager_playground.python_triton_model_1.package import fn1
from uber.ai.michelangelo.experimental.model_manager_playground.python_triton_model_1.folder.fn2 import fn2


class Predict(Model):
    def __init__(self, content: str):
        self.content = content
        self.foo = foo()
        self.dummy_f = dummy_module.dummy_function()
        self.dummy_t = dummy_type()

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
        response = (
            f"feature: {feature} and content: {self.content} and deps: {fn1()} and deps: {fn2()} and {self.dummy_f} and {self.dummy_t} and {self.foo}"
        )
        array = np.array([response])
        return {"response": array}
