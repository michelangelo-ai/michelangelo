"""Deployable model loader."""

from __future__ import annotations

import os
from typing import Any

from michelangelo.lib.model_manager._private.serde.model.triton_backend_type import (
    get_deployable_backend_type,
)
from michelangelo.lib.model_manager.constants.triton_backend_type import (
    TritonBackendType,
)

_VERSION_DIR = "0"
_TORCHSCRIPT_FILE = "model.pt"
_ONNX_FILE = "model.onnx"
_MODEL_CLASS_FILE = "model_class.txt"


def load_deployable_model(model_path: str, device: str = "cpu") -> Any:
    """Load the model from a deployable (Triton) model package.

    The backend named in ``config.pbtxt`` decides how the model is loaded.

    Args:
        model_path: The deployable model package path.
        device: Device to load a PyTorch model on, such as ``"cpu"`` or
            ``"cuda"``. For an ONNX model, ``"cuda"`` selects the CUDA
            execution provider when onnxruntime has one.

    Returns:
        For ``pytorch``, a ``torch.jit.ScriptModule`` in eval mode. For
        ``onnxruntime``, an ``onnxruntime.InferenceSession``. For ``python``,
        the custom ``Model`` instance, or a PyTorch ``nn.Module`` in eval mode
        when the packaged class is a module.

    Raises:
        FileNotFoundError: If the package or its model file is missing.
        ValueError: If ``config.pbtxt`` names no backend, or the
            ``model_class.txt`` of a ``python`` package is missing or empty.
        NotImplementedError: If the backend has no loader.
    """
    if not os.path.isdir(model_path):
        raise FileNotFoundError(f"Model package not found: {model_path}")

    backend = get_deployable_backend_type(model_path)
    version_dir = os.path.join(model_path, _VERSION_DIR)

    if backend == TritonBackendType.TORCH:
        return _load_torchscript(os.path.join(version_dir, _TORCHSCRIPT_FILE), device)
    if backend == TritonBackendType.ONNX:
        return _load_onnx(os.path.join(version_dir, _ONNX_FILE), device)
    if backend == TritonBackendType.PYTHON:
        return _load_python(model_path, version_dir, device)
    raise NotImplementedError(
        f"Deployable packages with the {backend!r} backend cannot be loaded. "
        f"Supported backends: {TritonBackendType.TORCH}, {TritonBackendType.ONNX}, "
        f"{TritonBackendType.PYTHON}."
    )


def _require_file(path: str) -> None:
    """Raise ``FileNotFoundError`` if ``path`` is not a file."""
    if not os.path.isfile(path):
        raise FileNotFoundError(f"Deployable model file not found: {path}")


def _load_torchscript(path: str, device: str) -> Any:
    """Load a TorchScript module."""
    import torch

    _require_file(path)
    return torch.jit.load(path, map_location=device).eval()


def _load_onnx(path: str, device: str) -> Any:
    """Create an ONNX Runtime session."""
    import onnxruntime as ort

    _require_file(path)
    # A GPU build of onnxruntime lists the CUDA provider even on a machine with
    # no GPU, so it is only requested when the caller asked for CUDA.
    providers = ["CPUExecutionProvider"]
    if device.startswith("cuda") and (
        "CUDAExecutionProvider" in ort.get_available_providers()
    ):
        providers.insert(0, "CUDAExecutionProvider")
    return ort.InferenceSession(path, providers=providers)


def _load_python(model_path: str, version_dir: str, device: str) -> Any:
    """Load the model class of a ``python`` package and its weights."""
    from michelangelo.lib.model_manager._private.serde.loader.custom_model_loader import (  # noqa: E501
        load_custom_model,
    )
    from michelangelo.lib.model_manager._private.serde.loader.torch_deployable_model_loader import (  # noqa: E501
        _load_torch_python_deployable_model,
    )
    from michelangelo.lib.model_manager._private.utils.loader_utils import (
        import_model_class,
    )
    from michelangelo.lib.model_manager.interface.custom_model import Model

    class_path = os.path.join(version_dir, _MODEL_CLASS_FILE)
    if not os.path.isfile(class_path):
        raise ValueError(f"Missing {_MODEL_CLASS_FILE} in {version_dir}.")
    with open(class_path) as f:
        class_name = f.read().strip()
    if not class_name:
        raise ValueError(f"{_MODEL_CLASS_FILE} is empty in {version_dir}.")

    model_class = import_model_class(version_dir, class_name)
    if issubclass(model_class, Model):
        return load_custom_model(
            os.path.join(version_dir, "model"), model_class, version_dir
        )
    return _load_torch_python_deployable_model(model_path, device)
