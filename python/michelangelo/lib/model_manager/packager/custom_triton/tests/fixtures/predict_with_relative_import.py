# flake8: noqa:TID252
import numpy as np
from michelangelo.lib.model_manager.interface.custom_model import Model
from ....._private.utils.module_finder.tests.fixtures.simple_module import module_attr
from ....._private.utils.module_finder.tests.fixtures.folder.fn1 import fn1
from ....._private.utils.module_finder.tests.fixtures.folder.fn2 import fn2
from ....._private.utils.module_finder.tests.fixtures.package import fn1 as pfn1


class Predict(Model):
    def save(self, path: str):
        assert path

    @classmethod
    def load(cls, path: str) -> "Predict":
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
        return inputs