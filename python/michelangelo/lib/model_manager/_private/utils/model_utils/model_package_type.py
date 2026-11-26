"""Infer what a deployable model package contains."""

from __future__ import annotations

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
        # The model class distinguishes a torch module from a custom Python
        # model. It is imported the way the loader imports it, so a class that
        # ships only inside the package is found.
        from michelangelo.lib.model_manager._private.serde.model.python_deployable_model import (  # noqa: E501
            _import_packaged_model_class,
        )

        try:
            model_cls = _import_packaged_model_class(model_path)
        except Exception:
            return None
        if not isinstance(model_cls, type):
            return None

        if issubclass(model_cls, Model):
            return RawModelType.CUSTOM_PYTHON

        import torch.nn as nn

        if issubclass(model_cls, nn.Module):
            return RawModelType.TORCH

    return None
