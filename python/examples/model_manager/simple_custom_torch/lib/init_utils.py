"""Model initialization helpers for the simple_custom_torch example.

Kept as a thin wrapper around the more complex nested package implementation.
"""

from __future__ import annotations

import torch

from examples.model_manager.simple_custom_torch.lib.regular_pkg.nested.init import init_linear


__all__ = ["init_linear"]


