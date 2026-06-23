"""Helpers for inspecting PyTorch models and mapping their dtypes."""

from __future__ import annotations

from typing import Any

import torch

from michelangelo.lib.model_manager.schema import DataType


def is_state_dict(model: Any) -> bool:
    """Check whether an object is a PyTorch state dict.

    Args:
        model: The object to check.

    Returns:
        True if the object is a dict whose values are all tensors, False
        otherwise.
    """
    return isinstance(model, dict) and all(
        isinstance(value, torch.Tensor) for value in model.values()
    )


def torch_dtype_to_data_type(dtype: torch.dtype) -> DataType:
    """Map a ``torch.dtype`` to a ModelSchema ``DataType``.

    Note:
        float16 (half) and bfloat16 are not yet supported -- ModelSchema has
        no corresponding DataType for reduced-precision floats. Add support
        here once DataType gains a FLOAT16 / BFLOAT16 variant.

    Args:
        dtype: The torch dtype to convert.

    Returns:
        The corresponding ModelSchema DataType.

    Raises:
        ValueError: If the dtype has no corresponding DataType.
    """
    if dtype == torch.float32:
        return DataType.FLOAT
    if dtype == torch.float64:
        return DataType.DOUBLE
    if dtype == torch.int32:
        return DataType.INT
    if dtype == torch.int16:
        return DataType.SHORT
    if dtype == torch.int8:
        return DataType.BYTE
    if dtype == torch.int64:
        return DataType.LONG
    if dtype == torch.bool:
        return DataType.BOOLEAN
    raise ValueError(
        f"Cannot convert torch.dtype {dtype} to DataType. "
        f"float16 and bfloat16 are not yet supported."
    )
