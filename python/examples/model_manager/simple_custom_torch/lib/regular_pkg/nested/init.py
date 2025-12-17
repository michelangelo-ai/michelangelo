"""Nested init helpers (regular package)."""

from __future__ import annotations

import torch


def init_linear(
    linear: torch.nn.Linear, weight: float = 0.1, bias: float = 0.2
) -> None:
    """Deterministically initialize a Linear layer (exercise nested package import)."""
    torch.manual_seed(0)
    with torch.no_grad():
        linear.weight.fill_(float(weight))
        linear.bias.fill_(float(bias))
