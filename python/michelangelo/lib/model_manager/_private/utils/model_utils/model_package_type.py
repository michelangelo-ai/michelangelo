"""Infer what a deployable model package contains."""

from __future__ import annotations

import os

from michelangelo._internal.utils.reflection_utils import get_module_attr
from michelangelo.lib.model_manager.constants import RawModelType, TritonBackendType
from michelangelo.lib.model_manager.interface.custom_model import Model


def infer_deployable_model_package_type(model_path: str) -> str | None:
    """Infer the training framework from a deployable model package.

    For most backends this returns a ``RawModelType`` value (the training
    framework that produced the deployable). The onnxruntime backend is an
    exception: it has no corresponding raw model type, so the Triton backend
    string (``TritonBackendType.ONNX``) is returned and callers branch on it
    directly.

    Args:
        model_path: Path to the deployable model package directory.

    Returns:
        A ``RawModelType`` value, ``TritonBackendType.ONNX`` for ONNX packages,
        or ``None`` if the backend is unrecognized.
    """
    # Imported here because the serde package imports torch eagerly.
    from michelangelo.lib.model_manager._private.serde.model.triton_backend_type import (  # noqa: E501
        get_deployable_backend_type,
    )

    try:
        backend = get_deployable_backend_type(model_path)
    except (FileNotFoundError, ValueError):
        return None

    if backend == TritonBackendType.TORCH:
        return RawModelType.TORCH

    if backend == TritonBackendType.ONNX:
        return TritonBackendType.ONNX

    if backend == TritonBackendType.PYTHON:
        # The model class distinguishes a torch module from a custom Python model.
        model_class_path = os.path.join(model_path, "0", "model_class.txt")
        if not os.path.isfile(model_class_path):
            return None

        with open(model_class_path) as f:
            model_class_str = f.read().strip()
        if not model_class_str:
            return None

        try:
            model_cls = get_module_attr(model_class_str)
        except Exception:
            return None

        if issubclass(model_cls, Model):
            return RawModelType.CUSTOM_PYTHON

        import torch.nn as nn

        if issubclass(model_cls, nn.Module):
            return RawModelType.TORCH

    return None
