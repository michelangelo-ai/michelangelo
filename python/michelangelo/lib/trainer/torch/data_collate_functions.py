"""Collate helpers for Ray Data / PyTorch training.

This module exposes small building blocks so callers can compose custom collate functions:

- :data:`DEFAULT_COLLATE_NUMPY_DTYPE` / :data:`DEFAULT_COLLATE_TORCH_DTYPE` — default dtypes
  (``float32`` unless overridden via function or :class:`LiteralEvalFloat32Collate` kwargs).
- :func:`pad_ragged_lists` — pad nested Python lists to a dense array of *numpy_dtype*.
- :func:`cell_is_nested_subsequence` / :func:`row_is_list_of_nested_cells` — structure checks.
- :func:`collate_value_to_float32_numpy` — one feature column → :class:`numpy.ndarray`.
- :func:`collate_value_to_float32_tensor` — one feature column → :class:`torch.Tensor`.
- :func:`collate_batch_to_float32_tensors` — full batch dict → tensors.

The default :func:`literal_eval_data_collate_function` is implemented on top of these.
:class:`LiteralEvalFloat32Collate` wraps the same behavior for subclassing (custom device, hooks).
"""

from __future__ import annotations

import ast
import contextlib

import numpy as np
import torch

from michelangelo.lib._internal.numpy_utils import (
    pad_ragged_tensor,
    sentinel_for_numpy_dtype,
)

# Default dtypes for all collate paths (subclass / kwargs may override per call).
DEFAULT_COLLATE_NUMPY_DTYPE: np.dtype = np.dtype(np.float32)
DEFAULT_COLLATE_TORCH_DTYPE: torch.dtype = torch.float32


def _torch_dtype_for_numpy_dtype(numpy_dtype: np.dtype) -> torch.dtype:
    """Map a NumPy dtype to the closest standard torch.dtype for tensor construction."""
    kind = numpy_dtype.kind
    if kind == "f":
        if numpy_dtype == np.dtype(np.float64):
            return torch.float64
        return torch.float32
    if kind in "iu":
        if numpy_dtype == np.dtype(np.int64):
            return torch.int64
        if numpy_dtype == np.dtype(np.int32):
            return torch.int32
    return DEFAULT_COLLATE_TORCH_DTYPE


__all__ = [
    "DEFAULT_COLLATE_NUMPY_DTYPE",
    "DEFAULT_COLLATE_TORCH_DTYPE",
    "LiteralEvalFloat32Collate",
    "cell_is_nested_subsequence",
    "collate_batch_to_float32_tensors",
    "collate_value_to_float32_numpy",
    "collate_value_to_float32_tensor",
    "literal_eval_data_collate_function",
    "pad_ragged_lists",
    "row_is_list_of_nested_cells",
]


def cell_is_nested_subsequence(cell) -> bool:
    """Return True if *cell* is a vector-valued slot (list/tuple or ndarray with ndim >= 1).

    Scalars and 0-D ndarrays are leaves for the 2-D-ragged path (one flat vector per row).
    """
    if isinstance(cell, (list, tuple)):
        return True
    if isinstance(cell, np.ndarray):
        return cell.ndim >= 1
    return False


def row_is_list_of_nested_cells(flat0: list | np.ndarray) -> bool:
    """Return True when *flat0* is a row of cells where at least one cell is a sub-sequence (3-D path).

    Uses every cell, not only ``flat0[0]``, so a leading scalar with later list cells still
    selects the 3-D normalization branch.
    """
    if len(flat0) == 0:
        return False
    return any(cell_is_nested_subsequence(c) for c in flat0)


def _batch_rows_are_one_scalar_each(items: list) -> bool:
    """True when each batch row is a single scalar value (one number per row)."""
    flat0 = items[0]
    if isinstance(flat0, np.ndarray) and flat0.ndim == 0:
        return True
    return not isinstance(flat0, (list, tuple, np.ndarray))


def _literal_eval_str_cells_in_object_array(obj: object) -> object:
    """Apply :func:`ast.literal_eval` to ``str`` cells in object-dtype structures."""
    if isinstance(obj, str):
        return ast.literal_eval(obj)
    if isinstance(obj, np.ndarray) and obj.dtype == np.dtype(object):
        if obj.ndim == 0:
            return _literal_eval_str_cells_in_object_array(obj.item())
        return [
            _literal_eval_str_cells_in_object_array(obj[i]) for i in range(obj.shape[0])
        ]
    return obj


def pad_ragged_lists(
    items: list,
    pad_value: float | None = None,
    *,
    numpy_dtype: np.dtype | None = None,
) -> np.ndarray:
    """Pad nested lists to a rectangular array of *numpy_dtype* (default: :data:`DEFAULT_COLLATE_NUMPY_DTYPE`)."""
    target = (
        np.dtype(numpy_dtype)
        if numpy_dtype is not None
        else DEFAULT_COLLATE_NUMPY_DTYPE
    )

    if not items:
        return np.array([], dtype=target)

    if _batch_rows_are_one_scalar_each(items):
        return np.array(items, dtype=target)

    try:
        arr = np.array(items, dtype=target)
        if arr.dtype == target:
            return arr
    except (ValueError, TypeError):
        pass

    flat0 = items[0]
    if row_is_list_of_nested_cells(flat0):
        normalized = [
            [np.asarray(sub, dtype=target).ravel() for sub in row] for row in items
        ]
    else:
        normalized = [np.asarray(seq, dtype=target).ravel() for seq in items]

    obj = np.asarray(normalized, dtype=object)
    effective_pad = (
        pad_value if pad_value is not None else sentinel_for_numpy_dtype(target)
    )
    padded = pad_ragged_tensor(obj, effective_pad)
    if padded.dtype == np.object_:
        return np.array(padded.tolist(), dtype=target)
    return padded.astype(target, copy=False)


def collate_value_to_float32_numpy(
    value,
    *,
    reshape_1d_features: bool = True,
    parse_string_with_literal_eval: bool = True,
    numpy_dtype: np.dtype | None = None,
) -> np.ndarray:
    """Convert a single batch column value to a :class:`numpy.ndarray` of *numpy_dtype*."""
    target = (
        np.dtype(numpy_dtype)
        if numpy_dtype is not None
        else DEFAULT_COLLATE_NUMPY_DTYPE
    )

    if parse_string_with_literal_eval and isinstance(value, str):
        with contextlib.suppress(ValueError, SyntaxError):
            value = ast.literal_eval(value)

    if not isinstance(value, np.ndarray):
        value = np.array(value)

    if value.dtype == np.object_:
        parsed_items = _literal_eval_str_cells_in_object_array(value)
        if not isinstance(parsed_items, list):
            parsed_items = [parsed_items]
        value = pad_ragged_lists(parsed_items, numpy_dtype=target)
    else:
        value = value.astype(target)

    if reshape_1d_features and isinstance(value, np.ndarray) and value.ndim == 1:
        value = value.reshape(-1, 1)

    return value


def collate_value_to_float32_tensor(
    value,
    *,
    device: str | torch.device = "cpu",
    reshape_1d_features: bool = True,
    parse_string_with_literal_eval: bool = True,
    numpy_dtype: np.dtype | None = None,
) -> torch.Tensor:
    """Convert one column value to :class:`torch.Tensor` on *device* (see :func:`collate_value_to_float32_numpy`)."""
    target = (
        np.dtype(numpy_dtype)
        if numpy_dtype is not None
        else DEFAULT_COLLATE_NUMPY_DTYPE
    )
    arr = collate_value_to_float32_numpy(
        value,
        reshape_1d_features=reshape_1d_features,
        parse_string_with_literal_eval=parse_string_with_literal_eval,
        numpy_dtype=target,
    )
    torch_dtype = _torch_dtype_for_numpy_dtype(target)
    return torch.tensor(arr, dtype=torch_dtype).to(device)


def collate_batch_to_float32_tensors(
    batch_data: dict,
    *,
    device: str | torch.device = "cpu",
    reshape_1d_features: bool = True,
    parse_string_with_literal_eval: bool = True,
    numpy_dtype: np.dtype | None = None,
) -> dict[str, torch.Tensor]:
    """Map a batch dict of Python / NumPy values to tensors (default element dtype: float32)."""
    target = (
        np.dtype(numpy_dtype)
        if numpy_dtype is not None
        else DEFAULT_COLLATE_NUMPY_DTYPE
    )
    return {
        k: collate_value_to_float32_tensor(
            v,
            device=device,
            reshape_1d_features=reshape_1d_features,
            parse_string_with_literal_eval=parse_string_with_literal_eval,
            numpy_dtype=target,
        )
        for k, v in batch_data.items()
    }


class LiteralEvalFloat32Collate:
    """Default collate with :func:`ast.literal_eval` for stringified arrays."""

    def __init__(
        self,
        *,
        device: str | torch.device = "cpu",
        reshape_1d_features: bool = True,
        parse_string_with_literal_eval: bool = True,
        numpy_dtype: np.dtype | None = None,
    ) -> None:
        """Initialize the collate.

        Args:
            device: Target device for emitted tensors.
            reshape_1d_features: If True, scalar features are reshaped to ``(N, 1)``.
            parse_string_with_literal_eval: If True, string-encoded arrays are decoded
                via :func:`ast.literal_eval`.
            numpy_dtype: Optional numpy dtype to cast numeric values to before tensor
                conversion; defaults to :data:`DEFAULT_COLLATE_NUMPY_DTYPE`.
        """
        self.device = device
        self.reshape_1d_features = reshape_1d_features
        self.parse_string_with_literal_eval = parse_string_with_literal_eval
        self.numpy_dtype = (
            np.dtype(numpy_dtype)
            if numpy_dtype is not None
            else DEFAULT_COLLATE_NUMPY_DTYPE
        )

    def collate_value_to_numpy(self, value) -> np.ndarray:
        """Convert one column value to :class:`~numpy.ndarray` (override in subclasses)."""
        return collate_value_to_float32_numpy(
            value,
            reshape_1d_features=self.reshape_1d_features,
            parse_string_with_literal_eval=self.parse_string_with_literal_eval,
            numpy_dtype=self.numpy_dtype,
        )

    def collate_value_to_tensor(self, value) -> torch.Tensor:
        """Convert one column value to :class:`torch.Tensor` on :attr:`device`."""
        arr = self.collate_value_to_numpy(value)
        torch_dtype = _torch_dtype_for_numpy_dtype(self.numpy_dtype)
        return torch.tensor(arr, dtype=torch_dtype).to(self.device)

    def collate_batch(self, batch_data: dict) -> dict[str, torch.Tensor]:
        """Map a batch dict to tensors (override for per-key routing)."""
        return {k: self.collate_value_to_tensor(v) for k, v in batch_data.items()}

    def __call__(self, batch_data: dict) -> dict[str, torch.Tensor]:
        """Delegate to :meth:`collate_batch`."""
        return self.collate_batch(batch_data)


_DEFAULT_LITERAL_EVAL_COLLATE = LiteralEvalFloat32Collate()


def literal_eval_data_collate_function(batch_data: dict) -> dict[str, torch.Tensor]:
    """Convert processed batch data to tensors (default training collate)."""
    return _DEFAULT_LITERAL_EVAL_COLLATE(batch_data)
