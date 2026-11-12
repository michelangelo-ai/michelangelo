"""Create an ONNX Runtime session for an ``onnxruntime`` deployable package."""

from __future__ import annotations

import logging
import os
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    import onnxruntime as ort

_logger = logging.getLogger(__name__)

_ONNX_DEPLOYABLE_MODEL_FILE = "model.onnx"


def load_onnx_deployable_model(model_path: str) -> ort.InferenceSession:
    """Create an ONNX Runtime session from an ``onnxruntime`` deployable package.

    The CUDA execution provider is requested only when PyTorch sees a GPU.

    Args:
        model_path: The deployable model package path.

    Returns:
        The inference session.

    Raises:
        FileNotFoundError: If the package has no ``0/model.onnx``.
    """
    import onnxruntime as ort

    onnx_file = os.path.join(model_path, "0", _ONNX_DEPLOYABLE_MODEL_FILE)
    if not os.path.exists(onnx_file):
        raise FileNotFoundError(
            f"ONNX model file not found at {onnx_file}. Deployable ONNX models "
            f"should be in 0/{_ONNX_DEPLOYABLE_MODEL_FILE}"
        )

    available = set(ort.get_available_providers())
    # A GPU build of onnxruntime lists the CUDA provider even on a machine with
    # no GPU, and requesting it there can crash the process, so gate on a real
    # device rather than on the provider being compiled in.
    providers = ["CPUExecutionProvider"]
    if "CUDAExecutionProvider" in available and _cuda_available():
        providers.insert(0, "CUDAExecutionProvider")

    # ONNX Runtime loads models with external data (over 2 GiB) directly: the
    # data files are memory-mapped relative to the .onnx path.
    session = ort.InferenceSession(onnx_file, providers=providers)
    _logger.info(
        "ONNX Runtime session created. Requested providers=%s, actual "
        "providers=%s, available=%s",
        providers,
        session.get_providers(),
        sorted(available),
    )
    return session


def _cuda_available() -> bool:
    """Whether PyTorch is installed and sees a GPU."""
    try:
        import torch
    except ImportError:
        return False
    return torch.cuda.is_available()
