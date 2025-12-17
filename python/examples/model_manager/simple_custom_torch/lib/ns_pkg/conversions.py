"""Namespace package module (no __init__.py in ns_pkg/)."""

from __future__ import annotations

from typing import TYPE_CHECKING

import torch

if TYPE_CHECKING:
    import numpy as np

from examples.model_manager.simple_custom_torch.lib.ns_pkg.nested_ns.ops import (
    as_float32,
)


def numpy_f32_to_tensor(x: np.ndarray) -> torch.Tensor:
    """Convert a numpy float32 ndarray to a torch Tensor."""
    return torch.from_numpy(as_float32(x))


def tensor_to_numpy_f32(x: torch.Tensor) -> np.ndarray:
    """Convert a torch Tensor to a numpy float32 ndarray."""
    return as_float32(x.detach().cpu().numpy())
