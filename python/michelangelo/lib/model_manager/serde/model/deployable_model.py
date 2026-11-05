"""Deployable model loader."""

from __future__ import annotations

import os
from typing import TYPE_CHECKING

from michelangelo.lib.model_manager._private.serde.model import (
    get_deployable_backend_type,
    load_onnx_deployable_model,
    load_python_deployable_model,
    load_torch_deployable_model,
)
from michelangelo.lib.model_manager.constants import TritonBackendType

if TYPE_CHECKING:
    import onnxruntime as ort
    import torch

    from michelangelo.lib.model_manager.interface.custom_model import Model


def load_deployable_model(
    model_path: str,
) -> torch.jit.ScriptModule | torch.nn.Module | ort.InferenceSession | Model:
    """Load the model from a deployable (Triton) model package.

    The backend named in the package's ``config.pbtxt`` decides how the model
    is loaded. PyTorch models are loaded on the CPU.

    Args:
        model_path: The deployable model package path.

    Returns:
        For a ``pytorch`` package, a ``torch.jit.ScriptModule``. For an
        ``onnxruntime`` package, an ``onnxruntime.InferenceSession``. For a
        ``python`` package, a ``torch.nn.Module`` or a custom ``Model``,
        depending on the packaged model class.

    Raises:
        FileNotFoundError: If ``model_path`` or the package's model file does
            not exist.
        ValueError: If the package has no ``config.pbtxt``, or names no
            supported backend.
        NotImplementedError: If the backend has no loader.
    """
    if not os.path.exists(model_path):
        raise FileNotFoundError(f"Model path not found: {model_path}")

    if not os.path.exists(os.path.join(model_path, "config.pbtxt")):
        raise ValueError(
            "Not a Triton deployable model package. Missing config.pbtxt in "
            f"{model_path}"
        )

    backend_type = get_deployable_backend_type(model_path)

    if backend_type == TritonBackendType.TORCH:
        return load_torch_deployable_model(model_path)

    if backend_type == TritonBackendType.PYTHON:
        return load_python_deployable_model(model_path)

    if backend_type == TritonBackendType.ONNX:
        return load_onnx_deployable_model(model_path)

    raise NotImplementedError(
        f"The loader for {backend_type} backend is not supported yet. Only "
        "'pytorch', 'python', and 'onnxruntime' backends are currently supported "
        "for deployable models."
    )
