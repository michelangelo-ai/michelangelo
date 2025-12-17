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

from examples.model_manager.simple_custom.lib.artifacts import write_text_artifact
from examples.model_manager.simple_custom.lib.constants import ARTIFACT_FILENAME, ARTIFACT_PREFIX
from examples.model_manager.simple_custom.lib.ns_pkg.echo import echo_int32
from examples.model_manager.simple_custom.lib.preprocess import ensure_int32
from examples.model_manager.simple_custom.lib.utils import build_artifact_content


class DummyEchoModel(Model):
    """Dummy model: returns the input unchanged (echo).

    - **Input**: {"input": np.ndarray[int32] shape [1]}
    - **Output**: {"response": np.ndarray[int32] shape [1]}
    """

    def save(self, path: str):
        # Write a tiny artifact via example lib/ code (dependency extraction test).
        content = build_artifact_content(prefix=ARTIFACT_PREFIX, model_name="DummyEchoModel")
        write_text_artifact(path, ARTIFACT_FILENAME, content)

    @classmethod
    def load(cls, path: str) -> "DummyEchoModel":
        # We don't need any state; just validate the artifact exists.
        _ = open(os.path.join(path, "artifact.txt"), encoding="utf-8").read().strip()
        return cls()

    def predict(self, inputs: dict[str, np.ndarray]) -> dict[str, np.ndarray]:
        # Use lib/ helper to ensure it is included in the packaged deps.
        x = ensure_int32(inputs["input"])
        # Use a namespace package helper too.
        x = echo_int32(x)
        return {"response": x}


