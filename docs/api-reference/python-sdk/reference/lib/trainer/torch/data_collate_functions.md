---
sidebar_label: data_collate_functions
title: lib.trainer.torch.data_collate_functions
---

Collate helpers for Ray Data / PyTorch training.

This module exposes small building blocks so callers can compose custom collate functions:

- :data:`DEFAULT_COLLATE_NUMPY_DTYPE` / :data:`DEFAULT_COLLATE_TORCH_DTYPE` — default dtypes
  (``float32`` unless overridden via function or :class:`LiteralEvalFloat32Collate` kwargs).
- :func:`pad_ragged_lists` — pad nested Python lists to a dense array of *numpy_dtype*.
- :func:`cell_is_nested_subsequence` / :func:`row_is_list_of_nested_cells` — structure checks.
- :func:`collate_value_to_float32_numpy` — one feature column → :class:`numpy.ndarray`.
- :func:`DEFAULT_COLLATE_TORCH_DTYPE`0 — one feature column → :class:`DEFAULT_COLLATE_TORCH_DTYPE`1.
- :func:`DEFAULT_COLLATE_TORCH_DTYPE`2 — full batch dict → tensors.

The default :func:`DEFAULT_COLLATE_TORCH_DTYPE`3 is implemented on top of these.
:class:`LiteralEvalFloat32Collate` wraps the same behavior for subclassing (custom device, hooks).

#### cell\_is\_nested\_subsequence

```python
def cell_is_nested_subsequence(cell) -> bool
```

Return True if *cell* is a vector-valued slot (list/tuple or ndarray with ndim &gt;= 1).

Scalars and 0-D ndarrays are leaves for the 2-D-ragged path (one flat vector per row).

#### row\_is\_list\_of\_nested\_cells

```python
def row_is_list_of_nested_cells(flat0: list | np.ndarray) -> bool
```

Return True when *flat0* is a row of cells where at least one cell is a sub-sequence (3-D path).

Uses every cell, not only ``flat0[0]``, so a leading scalar with later list cells still
selects the 3-D normalization branch.

#### pad\_ragged\_lists

```python
def pad_ragged_lists(items: list,
                     pad_value: float | None = None,
                     *,
                     numpy_dtype: np.dtype | None = None) -> np.ndarray
```

Pad nested lists to a rectangular array of *numpy_dtype* (default: :data:`DEFAULT_COLLATE_NUMPY_DTYPE`).

#### collate\_value\_to\_float32\_numpy

```python
def collate_value_to_float32_numpy(
        value,
        *,
        reshape_1d_features: bool = True,
        parse_string_with_literal_eval: bool = True,
        numpy_dtype: np.dtype | None = None) -> np.ndarray
```

Convert a single batch column value to a :class:`numpy.ndarray` of *numpy_dtype*.

#### collate\_value\_to\_float32\_tensor

```python
def collate_value_to_float32_tensor(
        value,
        *,
        device: str | torch.device = "cpu",
        reshape_1d_features: bool = True,
        parse_string_with_literal_eval: bool = True,
        numpy_dtype: np.dtype | None = None) -> torch.Tensor
```

Convert one column value to :class:`torch.Tensor` on *device* (see :func:`collate_value_to_float32_numpy`).

#### collate\_batch\_to\_float32\_tensors

```python
def collate_batch_to_float32_tensors(
        batch_data: dict,
        *,
        device: str | torch.device = "cpu",
        reshape_1d_features: bool = True,
        parse_string_with_literal_eval: bool = True,
        numpy_dtype: np.dtype | None = None) -> dict[str, torch.Tensor]
```

Map a batch dict of Python / NumPy values to tensors (default element dtype: float32).

## LiteralEvalFloat32Collate Objects

```python
class LiteralEvalFloat32Collate()
```

Default collate with :func:`ast.literal_eval` for stringified arrays.

#### \_\_init\_\_

```python
def __init__(*,
             device: str | torch.device = "cpu",
             reshape_1d_features: bool = True,
             parse_string_with_literal_eval: bool = True,
             numpy_dtype: np.dtype | None = None) -> None
```

Initialize the collate.

**Arguments**:

- `device` - Target device for emitted tensors.
- `reshape_1d_features` - If True, scalar features are reshaped to ``(N, 1)``.
- `parse_string_with_literal_eval` - If True, string-encoded arrays are decoded
  via :func:`ast.literal_eval`.
- `numpy_dtype` - Optional numpy dtype to cast numeric values to before tensor
  conversion; defaults to :data:`DEFAULT_COLLATE_NUMPY_DTYPE`.

#### collate\_value\_to\_numpy

```python
def collate_value_to_numpy(value) -> np.ndarray
```

Convert one column value to :class:`~numpy.ndarray` (override in subclasses).

#### collate\_value\_to\_tensor

```python
def collate_value_to_tensor(value) -> torch.Tensor
```

Convert one column value to :class:`torch.Tensor` on :attr:`device`.

#### collate\_batch

```python
def collate_batch(batch_data: dict) -> dict[str, torch.Tensor]
```

Map a batch dict to tensors (override for per-key routing).

#### \_\_call\_\_

```python
def __call__(batch_data: dict) -> dict[str, torch.Tensor]
```

Delegate to :meth:`collate_batch`.

#### literal\_eval\_data\_collate\_function

```python
def literal_eval_data_collate_function(
        batch_data: dict) -> dict[str, torch.Tensor]
```

Convert processed batch data to tensors (default training collate).

