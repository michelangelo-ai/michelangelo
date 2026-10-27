"""Collate helpers for Ray Data / PyTorch training.

This module exposes small building blocks so callers can compose custom collate functions:

- `DEFAULT_COLLATE_NUMPY_DTYPE` / `DEFAULT_COLLATE_TORCH_DTYPE` — default dtypes
  (`float32` unless overridden via function or `LiteralEvalFloat32Collate` kwargs).
- `pad_ragged_lists` — pad nested Python lists to a dense array of *numpy_dtype*.
- `cell_is_nested_subsequence` / `row_is_list_of_nested_cells` — structure checks.
- `collate_value_to_float32_numpy` — one feature column → `numpy.ndarray`.
- `collate_value_to_float32_tensor` — one feature column → `torch.Tensor`.
- `collate_batch_to_float32_tensors` — full batch dict → tensors.

The default `literal_eval_data_collate_function` is implemented on top of these.
`LiteralEvalFloat32Collate` wraps the same behavior for subclassing (custom device, hooks).

`pinned_literal_eval_data_collate_function` runs the default collate and additionally pins the
output tensors in page-locked host memory, which can make host-to-GPU copies asynchronous. Use it as
`LightningTrainer.data_collate_fn` or as `BatchIterConfig.collate_fn`.

Example:
Using the default collate with a PyTorch `DataLoader`:

```python
from torch.utils.data import DataLoader
from michelangelo.lib.trainer.torch.data_collate_functions import (
    literal_eval_data_collate_function,
)

loader = DataLoader(
    dataset,
    batch_size=32,
    collate_fn=literal_eval_data_collate_function,
)
```
"""

from __future__ import annotations

import ast
import contextlib

import numpy as np
import torch

from michelangelo.lib._internal.utils.numpy_utils import (
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
    "pinned_literal_eval_data_collate_function",
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

    Uses every cell, not only `flat0[0]`, so a leading scalar with later list cells still
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
    """Apply `ast.literal_eval` to `str` cells in object-dtype structures."""
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
    """Pad nested lists to a rectangular array of *numpy_dtype* (default: `DEFAULT_COLLATE_NUMPY_DTYPE`)."""
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
    """Convert a single batch column value to a `numpy.ndarray` of *numpy_dtype*."""
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
    """Convert one column value to `torch.Tensor` on *device* (see `collate_value_to_float32_numpy`)."""
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
    """Default collate with `ast.literal_eval` for stringified arrays."""

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
            reshape_1d_features: If True, scalar features are reshaped to `(N, 1)`.
            parse_string_with_literal_eval: If True, string-encoded arrays are decoded
                via `ast.literal_eval`.
            numpy_dtype: Optional numpy dtype to cast numeric values to before tensor
                conversion; defaults to `DEFAULT_COLLATE_NUMPY_DTYPE`.
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
        """Convert one column value to `numpy.ndarray` (override in subclasses)."""
        return collate_value_to_float32_numpy(
            value,
            reshape_1d_features=self.reshape_1d_features,
            parse_string_with_literal_eval=self.parse_string_with_literal_eval,
            numpy_dtype=self.numpy_dtype,
        )

    def collate_value_to_tensor(self, value) -> torch.Tensor:
        """Convert one column value to `torch.Tensor` on `device`."""
        arr = self.collate_value_to_numpy(value)
        torch_dtype = _torch_dtype_for_numpy_dtype(self.numpy_dtype)
        return torch.tensor(arr, dtype=torch_dtype).to(self.device)

    def collate_batch(self, batch_data: dict) -> dict[str, torch.Tensor]:
        """Map a batch dict to tensors (override for per-key routing)."""
        return {k: self.collate_value_to_tensor(v) for k, v in batch_data.items()}

    def __call__(self, batch_data: dict) -> dict[str, torch.Tensor]:
        """Delegate to `collate_batch`."""
        return self.collate_batch(batch_data)


_DEFAULT_LITERAL_EVAL_COLLATE = LiteralEvalFloat32Collate()


def literal_eval_data_collate_function(batch_data: dict) -> dict[str, torch.Tensor]:
    """Convert processed batch data to tensors (default training collate)."""
    return _DEFAULT_LITERAL_EVAL_COLLATE(batch_data)


def pinned_literal_eval_data_collate_function(
    batch_data: dict,
) -> dict[str, torch.Tensor]:
    """Collate like `literal_eval_data_collate_function`, with tensors in pinned memory.

    Pinning tensors in page-locked host memory can make host-to-GPU copies asynchronous when they are issued
    with `non_blocking=True`, and such copies can transfer at higher bandwidth than copies from pageable
    memory. Whether this speeds up training depends on the downstream transfers actually being
    non-blocking, so measure before relying on it. Ray Data's built-in `pin_memory` option only applies
    to its default collate function, so a custom collate has to pin on its own.

    To use it, pass the function as `LightningTrainer.data_collate_fn`, or set
    `BatchIterConfig.collate_fn` to its dotted import path, for example
    `"michelangelo.lib.trainer.torch.data_collate_functions.pinned_literal_eval_data_collate_function"`.

    When CUDA is not available (for example when the collate runs on a CPU-only driver to produce a sample
    row), pinning is skipped and the tensors are returned unpinned, since `Tensor.pin_memory` requires a
    CUDA runtime. The function assumes the collated tensors live on the CPU, as the default collate
    produces; tensors already on a GPU cannot be pinned.

    Args:
        batch_data: Mapping from feature name to the batch column, as accepted by
            `literal_eval_data_collate_function`.

    Returns:
        Mapping from feature name to a `float32` tensor. Tensors are pinned when CUDA is available.

    Raises:
        RuntimeError: If CUDA is available but pinned host memory cannot be allocated.
    """
    batch = literal_eval_data_collate_function(batch_data)
    if not torch.cuda.is_available():
        return batch
    return {name: tensor.pin_memory() for name, tensor in batch.items()}
