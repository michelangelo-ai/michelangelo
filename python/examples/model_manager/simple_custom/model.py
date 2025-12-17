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

from examples.model_manager.simple_custom.lib.artifacts import write_text_artifact
from examples.model_manager.simple_custom.lib.constants import (
    ARTIFACT_FILENAME,
    ARTIFACT_PREFIX,
)
from examples.model_manager.simple_custom.lib.ns_pkg.echo import echo_int32
from examples.model_manager.simple_custom.lib.preprocess import ensure_int32
from examples.model_manager.simple_custom.lib.utils import build_artifact_content
from michelangelo.lib.model_manager.interface.custom_model import Model


class DummyEchoModel(Model):
    """Dummy model: returns the input unchanged (echo).

    - **Inputs**:
      - a: required int32 [1]
      - b: optional int32 [1]
    - **Outputs**:
      - response: int32 [1]
      - response2: int32 [1]
    """

    def save(self, path: str):
        # Write a tiny artifact via example lib/ code (dependency extraction test).
        content = build_artifact_content(
            prefix=ARTIFACT_PREFIX, model_name="DummyEchoModel"
        )
        write_text_artifact(path, ARTIFACT_FILENAME, content)

    @classmethod
    def load(cls, path: str) -> DummyEchoModel:
        # We don't need any state; just validate the artifact exists.
        _ = open(os.path.join(path, "artifact.txt"), encoding="utf-8").read().strip()
        return cls()

    def predict(self, inputs: dict[str, np.ndarray]) -> dict[str, np.ndarray]:
        a = echo_int32(ensure_int32(inputs["a"]))
        b_raw = inputs.get("b")
        b = echo_int32(ensure_int32(b_raw)) if b_raw is not None else np.int32(0)

        out = (a + b).astype(np.int32)
        out2 = (2 * a).astype(np.int32)

        return {"response": out, "response2": out2}
