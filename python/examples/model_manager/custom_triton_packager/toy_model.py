"""A minimal custom model compatible with Model Manager + Triton packaging.

This model is intentionally tiny: it adds a learned integer bias to an integer input.
It implements the Model interface required by `CustomTritonPackager`.
"""

from __future__ import annotations

import os

import numpy as np

from michelangelo.lib.model_manager.interface.custom_model import Model


class ToyAddBiasModel(Model):
    """Toy model: y = x + bias.

    - **Input**: {"input": np.ndarray[int32] shape [1]}
    - **Output**: {"response": np.ndarray[int32] shape [1]}
    """

    def __init__(self, bias: int = 3):
        self.bias = int(bias)

    def save(self, path: str):
        os.makedirs(path, exist_ok=True)
        with open(os.path.join(path, "bias.txt"), "w", encoding="utf-8") as f:
            f.write(str(self.bias))

    @classmethod
    def load(cls, path: str) -> ToyAddBiasModel:
        with open(os.path.join(path, "bias.txt"), encoding="utf-8") as f:
            bias = int(f.read().strip())
        return cls(bias=bias)

    def predict(self, inputs: dict[str, np.ndarray]) -> dict[str, np.ndarray]:
        x = inputs["input"].astype(np.int32)
        return {"response": (x + np.int32(self.bias)).astype(np.int32)}
