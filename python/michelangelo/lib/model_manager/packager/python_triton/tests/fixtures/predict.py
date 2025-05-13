import numpy as np
from uber.ai.michelangelo.sdk.model_manager.interface.custom_model import Model
from uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder.tests.fixtures.simple_module import module_attr
from uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder.tests.fixtures.folder.fn1 import fn1
from uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder.tests.fixtures.folder.fn2 import fn2
from uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder.tests.fixtures.package import fn1 as pfn1


class Predict(Model):
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
        fn1()
        fn2()
        pfn1()
        module_attr()
        return {"response": inputs.get("input")}
