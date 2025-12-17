"""Torch/NumPy conversion helpers for the simple_custom_torch example.

Kept as a thin wrapper around the namespace package implementation.
"""

from __future__ import annotations

import numpy as np
import torch

from examples.model_manager.simple_custom_torch.lib.ns_pkg.conversions import (
    numpy_f32_to_tensor,
    tensor_to_numpy_f32,
)


__all__ = ["numpy_f32_to_tensor", "tensor_to_numpy_f32"]


