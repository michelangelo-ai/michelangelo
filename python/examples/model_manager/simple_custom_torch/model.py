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
from examples.model_manager.simple_custom_torch.lib.regular_pkg.nested import init_linear
from examples.model_manager.simple_custom_torch.lib.utils import load_state_dict, save_state_dict


class TorchLinearModel(Model):
    """Toy torch model: a single Linear layer.

    - **Inputs**:
      - x: required float32 [1, 4]
      - y: optional float32 [1, 4]
      - scale: optional float32 [1]
    - **Outputs**:
      - response: float32 [1, 2]
      - response_alt: float32 [1, 2]
      - sum: float32 [1]
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
        # Required input
        x = numpy_f32_to_tensor(inputs["x"])
        # Optional inputs (defaults)
        y_np = inputs.get("y")
        y = numpy_f32_to_tensor(y_np) if y_np is not None else (0.0 * x)

        scale_np = inputs.get("scale")
        scale = float(scale_np[0]) if scale_np is not None else 1.0

        x_eff = x + (scale * y)
        with torch.no_grad():
            out = self.net(x_eff)
            out_alt = self.net(2.0 * x_eff)

        out_np = tensor_to_numpy_f32(out)
        out_alt_np = tensor_to_numpy_f32(out_alt)

        return {
            "response": out_np,
            "response_alt": out_alt_np,
            "sum": out_np.sum(axis=1).astype(np.float32),
        }


