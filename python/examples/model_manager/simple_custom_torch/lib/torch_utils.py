"""Torch/NumPy conversion helpers for the simple_custom_torch example."""

from __future__ import annotations

import numpy as np
import torch


def numpy_f32_to_tensor(x: np.ndarray) -> torch.Tensor:
    return torch.from_numpy(x.astype(np.float32))


def tensor_to_numpy_f32(x: torch.Tensor) -> np.ndarray:
    return x.detach().cpu().numpy().astype(np.float32)


