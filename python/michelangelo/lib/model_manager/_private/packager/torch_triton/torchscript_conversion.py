"""Convert PyTorch model files to TorchScript for deployable packages."""

from __future__ import annotations

import os

try:
    import pytorch_lightning as pl

    _PL_MODULE = pl.LightningModule
except ImportError:
    _PL_MODULE = None
import torch

from michelangelo.lib.model_manager._private.utils.torch_utils import (
    is_state_dict,
    load_model_from_state_dict,
)


def _convert_to_torchscript(
    model_path: str,
    model_class: str | None = None,
    hyperparameters: dict | None = None,
) -> None:
    """Convert a PyTorch model file to TorchScript format in place.

    If the file is already TorchScript, it is left unchanged. Otherwise the
    artifact is loaded as a full nn.Module or reconstructed from a state_dict
    (using model_class and hyperparameters), scripted, and saved back to
    model_path.

    Args:
        model_path: Path to the .pt artifact to convert in place.
        model_class: Import path of the nn.Module subclass, required when the
            artifact is a state_dict.
        hyperparameters: Constructor kwargs used to rebuild the model from a
            state_dict.

    Raises:
        FileNotFoundError: If model_path does not exist.
        TypeError: If the file does not contain a convertible model.
    """
    if not os.path.exists(model_path):
        raise FileNotFoundError(f"File does not exist: {model_path}")

    # Already TorchScript: nothing to convert.
    try:
        torch.jit.load(model_path, map_location="cpu")
        return
    except Exception:
        pass

    loaded_model = torch.load(model_path, map_location="cpu", weights_only=False)

    if is_state_dict(loaded_model):
        if not model_class:
            raise ValueError(
                "model_class is required when the artifact is a state dict."
            )
        model = load_model_from_state_dict(loaded_model, model_class, hyperparameters)
    else:
        model = loaded_model
    model.eval()

    try:
        if _PL_MODULE is not None and isinstance(model, _PL_MODULE):
            scripted = model.to_torchscript(method="script")
            torch.jit.save(scripted, model_path)
        else:
            torch.jit.save(torch.jit.script(model), model_path)
    except Exception as e:
        raise TypeError(
            f"File does not contain a convertible model: {model_path}"
        ) from e
