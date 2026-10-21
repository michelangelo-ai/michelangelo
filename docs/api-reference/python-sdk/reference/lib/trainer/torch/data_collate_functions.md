---
sidebar_label: data_collate_functions
title: lib.trainer.torch.data_collate_functions
---

Collate helpers for Ray Data / PyTorch training.

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

**Example**:

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

#### cell\_is\_nested\_subsequence

```python
def cell_is_nested_subsequence(cell) -> bool
```

Return True if *cell* is a vector-valued slot (list/tuple or ndarray with ndim >= 1).

Scalars and 0-D ndarrays are leaves for the 2-D-ragged path (one flat vector per row).

#### row\_is\_list\_of\_nested\_cells

```python
def row_is_list_of_nested_cells(flat0: list | np.ndarray) -> bool
```

Return True when *flat0* is a row of cells where at least one cell is a sub-sequence (3-D path).

Uses every cell, not only `flat0[0]`, so a leading scalar with later list cells still
selects the 3-D normalization branch.

#### pad\_ragged\_lists

```python
def pad_ragged_lists(items: list,
                     pad_value: float | None = None,
                     *,
                     numpy_dtype: np.dtype | None = None) -> np.ndarray
```

Pad nested lists to a rectangular array of *numpy_dtype* (default: `DEFAULT_COLLATE_NUMPY_DTYPE`).

#### collate\_value\_to\_float32\_numpy

```python
def collate_value_to_float32_numpy(
        value,
        *,
        reshape_1d_features: bool = True,
        parse_string_with_literal_eval: bool = True,
        numpy_dtype: np.dtype | None = None) -> np.ndarray
```

Convert a single batch column value to a `numpy.ndarray` of *numpy_dtype*.

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

Convert one column value to `torch.Tensor` on *device* (see `collate_value_to_float32_numpy`).

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

Default collate with `ast.literal_eval` for stringified arrays.

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
- `reshape_1d_features` - If True, scalar features are reshaped to `(N, 1)`.
- `parse_string_with_literal_eval` - If True, string-encoded arrays are decoded
  via `ast.literal_eval`.
- `numpy_dtype` - Optional numpy dtype to cast numeric values to before tensor
  conversion; defaults to `DEFAULT_COLLATE_NUMPY_DTYPE`.

#### collate\_value\_to\_numpy

```python
def collate_value_to_numpy(value) -> np.ndarray
```

Convert one column value to `numpy.ndarray` (override in subclasses).

#### collate\_value\_to\_tensor

```python
def collate_value_to_tensor(value) -> torch.Tensor
```

Convert one column value to `torch.Tensor` on `device`.

#### collate\_batch

```python
def collate_batch(batch_data: dict) -> dict[str, torch.Tensor]
```

Map a batch dict to tensors (override for per-key routing).

#### \_\_call\_\_

```python
def __call__(batch_data: dict) -> dict[str, torch.Tensor]
```

Delegate to `collate_batch`.

#### literal\_eval\_data\_collate\_function

```python
def literal_eval_data_collate_function(
        batch_data: dict) -> dict[str, torch.Tensor]
```

Convert processed batch data to tensors (default training collate).

#### pinned\_literal\_eval\_data\_collate\_function

```python
def pinned_literal_eval_data_collate_function(
        batch_data: dict) -> dict[str, torch.Tensor]
```

Collate like `literal_eval_data_collate_function`, with tensors in pinned memory.

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

**Arguments**:

- `batch_data` - Mapping from feature name to the batch column, as accepted by
  `literal_eval_data_collate_function`.
  

**Returns**:

  Mapping from feature name to a `float32` tensor. Tensors are pinned when CUDA is available.
  

**Raises**:

- `RuntimeError` - If CUDA is available but pinned host memory cannot be allocated.

