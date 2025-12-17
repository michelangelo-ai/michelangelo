"""A minimal custom model for Model Manager + CustomTritonPackager."""

from __future__ import annotations

import os

try:
    import numpy as np
except ModuleNotFoundError as e:  # pragma: no cover
    raise ModuleNotFoundError(
        "This example requires numpy. Run it from the `python/` directory with "
        "`poetry run ...` after installing example deps, e.g. "
        "`poetry install -E example`."
    ) from e

from michelangelo.lib.model_manager.interface.custom_model import Model


class DummyEchoModel(Model):
    """Dummy model: returns the input unchanged (echo).

    - **Input**: {"input": np.ndarray[int32] shape [1]}
    - **Output**: {"response": np.ndarray[int32] shape [1]}
    """

    def save(self, path: str):
        os.makedirs(path, exist_ok=True)
        # Write a tiny artifact to prove packaging copies model artifacts.
        with open(os.path.join(path, "artifact.txt"), "w", encoding="utf-8") as f:
            f.write("dummy-echo")

    @classmethod
    def load(cls, path: str) -> "DummyEchoModel":
        # We don't need any state; just validate the artifact exists.
        _ = open(os.path.join(path, "artifact.txt"), encoding="utf-8").read().strip()
        return cls()

    def predict(self, inputs: dict[str, np.ndarray]) -> dict[str, np.ndarray]:
        x = inputs["input"].astype(np.int32)
        return {"response": x}


