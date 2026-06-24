"""Helpers for inspecting PyTorch models and mapping their dtypes."""

from __future__ import annotations

from typing import Any

import torch

from michelangelo._internal.utils.reflection_utils import get_module_attr
from michelangelo.lib.model_manager.schema import DataType


def is_state_dict(model: Any) -> bool:
    """Check whether an object is a PyTorch state dict.

    Args:
        model: The object to check.

    Returns:
        True if the object is a dict whose values are all tensors, False
        otherwise.
    """
    return (
        isinstance(model, dict)
        and len(model) > 0
        and all(isinstance(value, torch.Tensor) for value in model.values())
    )


def load_model_from_state_dict(
    state_dict: dict[str, torch.Tensor],
    model_class: str,
    hyperparameters: dict | None = None,
) -> torch.nn.Module:
    """Instantiate a model class and load a state_dict into it.

    Args:
        state_dict: The state dict to load.
        model_class: Import path of the nn.Module subclass.
        hyperparameters: Constructor kwargs passed to model_class.

    Returns:
        The model with state_dict loaded, still in training mode.
    """
    model_fn = get_module_attr(model_class)
    model = model_fn(**(hyperparameters or {}))
    model.load_state_dict(state_dict)
    return model


def tensor_to_numpy(value: Any) -> Any:
    """Convert a tensor to a numpy array; pass non-tensors through unchanged."""
    return value.detach().cpu().numpy() if hasattr(value, "detach") else value


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
