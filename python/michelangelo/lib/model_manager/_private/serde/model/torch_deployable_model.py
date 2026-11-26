"""Load the TorchScript module of a ``pytorch`` deployable package."""

from __future__ import annotations

import os
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    import torch


def load_torch_deployable_model(model_path: str) -> torch.jit.ScriptModule:
    """Load the TorchScript module from a ``pytorch`` deployable package.

    The module is loaded on the CPU; the caller moves it to a device.

    Args:
        model_path: The deployable model package path.

    Returns:
        The TorchScript module, in eval mode.

    Raises:
        FileNotFoundError: If the package has no ``0/model.pt``.
        RuntimeError: If ``0/model.pt`` cannot be loaded as TorchScript.
    """
    import torch

    model_file_path = os.path.join(model_path, "0", "model.pt")
    if not os.path.exists(model_file_path):
        raise FileNotFoundError(
            f"Model file not found at {model_file_path}. Deployable torch models "
            "should be in 0/model.pt"
        )

    try:
        model = torch.jit.load(model_file_path, map_location="cpu")
    except Exception as e:
        raise RuntimeError(
            f"Failed to load TorchScript model from {model_file_path}: {e}"
        ) from e

    model.eval()
    return model
