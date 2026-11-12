"""Conversions between ONNX Runtime tensor types and numpy dtypes."""

from __future__ import annotations

import numpy as np

# Map ONNX Runtime tensor-type strings (e.g. "tensor(float)") to numpy dtypes.
# Used to cast input numpy arrays before InferenceSession.run so they match the
# model graph's declared input types.
ONNX_TENSOR_TYPE_TO_NP_DTYPE: dict[str, np.dtype] = {
    "tensor(float)": np.dtype(np.float32),
    "tensor(float16)": np.dtype(np.float16),
    "tensor(double)": np.dtype(np.float64),
    "tensor(int8)": np.dtype(np.int8),
    "tensor(int16)": np.dtype(np.int16),
    "tensor(int32)": np.dtype(np.int32),
    "tensor(int64)": np.dtype(np.int64),
    "tensor(uint8)": np.dtype(np.uint8),
    "tensor(uint16)": np.dtype(np.uint16),
    "tensor(uint32)": np.dtype(np.uint32),
    "tensor(uint64)": np.dtype(np.uint64),
    "tensor(bool)": np.dtype(np.bool_),
}


def onnx_tensor_type_to_np_dtype(onnx_type: str) -> np.dtype | None:
    """Map an ONNX Runtime tensor-type string to its numpy dtype.

    Args:
        onnx_type: The tensor type string, for example ``"tensor(float)"``.

    Returns:
        The numpy dtype, or ``None`` for unmapped types so callers can decide
        whether to skip casting (preserving the input array's dtype) or raise.
    """
    return ONNX_TENSOR_TYPE_TO_NP_DTYPE.get(onnx_type)
