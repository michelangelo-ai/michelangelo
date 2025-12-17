"""A minimal torch-backed custom model for Model Manager + CustomTritonPackager.

The packager expects numpy ndarray inputs/outputs, but your model can use torch
internally (convert to/from numpy).
"""

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

try:
    import torch
except ModuleNotFoundError as e:  # pragma: no cover
    raise ModuleNotFoundError(
        "This example requires torch. Install an environment that includes torch, "
        "then run from the `python/` directory with `poetry run ...`."
    ) from e

from michelangelo.lib.model_manager.interface.custom_model import Model

from examples.model_manager.simple_custom_torch.lib.ns_pkg.conversions import (
    numpy_f32_to_tensor,
    tensor_to_numpy_f32,
)
from examples.model_manager.simple_custom_torch.lib.regular_pkg.nested.init import init_linear
from examples.model_manager.simple_custom_torch.lib.utils import load_state_dict, save_state_dict


class TorchLinearModel(Model):
    """Toy torch model: a single Linear layer.

    - **Input**: {"input": np.ndarray[float32] shape [1, 4]}
    - **Output**: {"response": np.ndarray[float32] shape [1, 2]}
    """

    def __init__(self):
        self.net = torch.nn.Linear(4, 2)
        # Use lib/ init helper to exercise dependency extraction.
        init_linear(self.net, weight=0.1, bias=0.2)

    def save(self, path: str):
        save_state_dict(path, self.net.state_dict())

    @classmethod
    def load(cls, path: str) -> "TorchLinearModel":
        obj = cls()
        state = load_state_dict(path)
        obj.net.load_state_dict(state)
        obj.net.eval()
        return obj

    def predict(self, inputs: dict[str, np.ndarray]) -> dict[str, np.ndarray]:
        # Use lib/ conversion helpers to exercise dependency extraction.
        x = numpy_f32_to_tensor(inputs["input"])
        with torch.no_grad():
            y = self.net(x)
        return {"response": tensor_to_numpy_f32(y)}


