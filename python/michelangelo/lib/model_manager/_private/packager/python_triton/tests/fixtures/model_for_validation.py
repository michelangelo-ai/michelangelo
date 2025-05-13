import os
import numpy as np
from uber.ai.michelangelo.sdk.model_manager.interface.custom_model import Model
from uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder.tests.fixtures.simple_module import module_attr
from uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder.tests.fixtures.folder.fn1 import fn1
from uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder.tests.fixtures.folder.fn2 import fn2
from uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder.tests.fixtures.package import fn1 as pfn1


class Predict(Model):
    def __init__(self, content: str):
        self.content = content

    def save(self, path: str):
        with open(os.path.join(path, "file.txt"), "w") as f:
            f.write(self.content)

    @classmethod
    def load(cls, path) -> "Predict":
        with open(os.path.join(path, "file.txt")) as f:
            content = f.read()

        return Predict(content)

    def predict(
        self,
        inputs: dict[str, np.ndarray],
    ) -> dict[str, np.ndarray]:
        fn1()
        fn2()
        pfn1()
        module_attr()
        ipt = inputs.get("input")[0]
        return {"response": np.array([ipt + " " + self.content])}


class ModelWithPredictError(Model):
    def __init__(self, content: str):
        self.content = content

    def save(self, path: str):
        with open(os.path.join(path, "file.txt"), "w") as f:
            f.write(self.content)

    @classmethod
    def load(cls, path) -> "Predict":
        with open(os.path.join(path, "file.txt")) as f:
            content = f.read()

        return ModelWithPredictError(content)

    def predict(
        self,
        inputs: dict[str, np.ndarray],  # noqa: ARG002
    ) -> dict[str, np.ndarray]:
        raise RuntimeError("error")


class ModelWithInvalidOutput(Model):
    def __init__(self, content: str):
        self.content = content

    def save(self, path: str):
        with open(os.path.join(path, "file.txt"), "w") as f:
            f.write(self.content)

    @classmethod
    def load(cls, path) -> "ModelWithInvalidOutput":
        with open(os.path.join(path, "file.txt")) as f:
            content = f.read()

        return ModelWithInvalidOutput(content)

    def predict(
        self,
        inputs: dict[str, np.ndarray],  # noqa: ARG002
    ) -> dict[str, np.ndarray]:
        return {"response": "invalid_output"}


class ModelWithOutputNotMatchingSchema(Model):
    def __init__(self, content: str):
        self.content = content

    def save(self, path: str):
        with open(os.path.join(path, "file.txt"), "w") as f:
            f.write(self.content)

    @classmethod
    def load(cls, path) -> "ModelWithOutputNotMatchingSchema":
        with open(os.path.join(path, "file.txt")) as f:
            content = f.read()

        return ModelWithOutputNotMatchingSchema(content)

    def predict(
        self,
        inputs: dict[str, np.ndarray],  # noqa: ARG002
    ) -> dict[str, np.ndarray]:
        return {"output": np.array(["output"])}


class ModelWithSaveError(Model):
    def __init__(self, content: str):
        self.content = content

    def save(self, path: str):  # noqa: ARG002
        raise RuntimeError("error")

    @classmethod
    def load(cls, path) -> "ModelWithSaveError":
        with open(os.path.join(path, "file.txt")) as f:
            content = f.read()

        return ModelWithSaveError(content)

    def predict(
        self,
        inputs: dict[str, np.ndarray],  # noqa: ARG002
    ) -> dict[str, np.ndarray]:
        return {"response": np.array(["output"])}


class ModelWithReloadError(Model):
    def __init__(self, content: str):
        self.content = content

    def save(self, path: str):
        assert path

    @classmethod
    def load(cls, path) -> "ModelWithReloadError":
        with open(os.path.join(path, "file.txt")) as f:
            content = f.read()

        return ModelWithReloadError(content)

    def predict(
        self,
        inputs: dict[str, np.ndarray],  # noqa: ARG002
    ) -> dict[str, np.ndarray]:
        return {"response": np.array(["output"])}


class ModelWithMismatchingLoad(Model):
    def __init__(self, content: str):
        self.content = content

    def save(self, path: str):
        with open(os.path.join(path, "file.txt"), "w") as f:
            f.write(self.content)

    @classmethod
    def load(cls, path) -> "ModelWithMismatchingLoad":
        with open(os.path.join(path, "file.txt")) as f:
            content = f.read()

        return Predict(content)

    def predict(
        self,
        inputs: dict[str, np.ndarray],
    ) -> dict[str, np.ndarray]:
        fn1()
        fn2()
        pfn1()
        module_attr()
        ipt = inputs.get("input")[0]
        return {"response": np.array([ipt + " " + self.content])}
