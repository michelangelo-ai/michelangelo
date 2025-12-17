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


class TorchLinearModel(Model):
    """Toy torch model: a single Linear layer.

    - **Input**: {"input": np.ndarray[float32] shape [1, 4]}
    - **Output**: {"response": np.ndarray[float32] shape [1, 2]}
    """

    def __init__(self):
        self.net = torch.nn.Linear(4, 2)
        torch.manual_seed(0)
        with torch.no_grad():
            self.net.weight.fill_(0.1)
            self.net.bias.fill_(0.2)

    def save(self, path: str):
        os.makedirs(path, exist_ok=True)
        torch.save(self.net.state_dict(), os.path.join(path, "state_dict.pt"))

    @classmethod
    def load(cls, path: str) -> "TorchLinearModel":
        obj = cls()
        state = torch.load(os.path.join(path, "state_dict.pt"), map_location="cpu")
        obj.net.load_state_dict(state)
        obj.net.eval()
        return obj

    def predict(self, inputs: dict[str, np.ndarray]) -> dict[str, np.ndarray]:
        x_np = inputs["input"].astype(np.float32)
        x = torch.from_numpy(x_np)
        with torch.no_grad():
            y = self.net(x)
        return {"response": y.cpu().numpy().astype(np.float32)}


