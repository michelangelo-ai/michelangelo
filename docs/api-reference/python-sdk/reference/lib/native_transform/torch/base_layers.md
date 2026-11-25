---
sidebar_label: base_layers
title: lib.native_transform.torch.base_layers
---

PyTorch native transform layers.

TorchScript- and ONNX-exportable ``nn.Module`` transform layers that operate on a
``dict[str, torch.Tensor]`` in/out contract so the exact same transform runs at
train and serve time. Every layer subclasses `TorchTransformBaseLayer` and
uses `format_inputs` /
`format_outputs` to map its
declared input/output columns to and from a single stacked tensor.

This module provides the foundation (stateless, elementwise) layers, plus
structural layers (``Tile``, ``PadOrCrop1D``, ``Clip``, ``Scale``, ``CaseWhen``,
``Compare``, ``TensorColFillNone``, etc.) and the ``IDHashTokenizer`` wrapper
layer. Fitted-statistics layers (``MinMax``, ``Normalization``,
``Bucketization``) live in
`michelangelo.lib.native_transform.torch.stats_layers`.

## TorchTransformBaseLayer Objects

```python
class TorchTransformBaseLayer(torch.nn.Module, abc.ABC)
```

Abstract base for native PyTorch transform layers.

All layers consume and produce ``dict[str, torch.Tensor]`` so they compose
into a single TorchScript-exportable graph. Subclasses select their inputs by
``input_cols`` and write their results under ``output_cols``.

**Arguments**:

- `input_cols` - Column names of the input tensors.
- `output_cols` - Column names of the output tensors.
- `**kwargs` - Additional options. ``name`` (str) sets the layer name, which
  must be unique within a model. When omitted, a unique name is
  generated automatically from the layer's class name (e.g.
  ``"stack_A1B2C3D4E5"``).

#### \_\_init\_\_

```python
def __init__(input_cols: list[str], output_cols: list[str], **kwargs) -> None
```

Initialize the base layer.

**Arguments**:

- `input_cols` - Column names of the input tensors.
- `output_cols` - Column names of the output tensors.
- `**kwargs` - Additional options; ``name`` (str) sets the layer name. When
  omitted, a unique name is generated from the class name.

#### forward

```python
@abc.abstractmethod
def forward(inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]
```

Apply the transform.

**Arguments**:

- `inputs` - Mapping from column name to tensor for at least every column
  in ``input_cols``.
  

**Returns**:

  A mapping from each column in ``output_cols`` to its result tensor.
  

**Raises**:

- `NotImplementedError` - If a subclass does not override this method.

## Concatenate Objects

```python
class Concatenate(TorchTransformBaseLayer)
```

Concatenate input tensors along the last dimension.

When ``dtype`` is ``None`` (default) the output dtype follows torch's
standard type-promotion rules (e.g. ``int32`` + ``float64`` -> ``float64``).
When ``dtype`` is given, the output is explicitly cast to it.

**Arguments**:

- `input_cols` - Column names of the input tensors.
- `output_cols` - Single-element list naming the concatenated output column.
- `dtype` - Optional output dtype. When ``None``, the input dtype is
  preserved via type promotion.
- `**kwargs` - Additional base-layer options (e.g. ``name``).

#### \_\_init\_\_

```python
def __init__(input_cols: list[str],
             output_cols: list[str],
             dtype: torch.dtype | str | None = None,
             **kwargs) -> None
```

Initialize the Concatenate layer.

**Arguments**:

- `input_cols` - Column names of the input tensors.
- `output_cols` - Single-element list naming the concatenated output.
- `dtype` - Optional output dtype; when ``None``, preserves input dtype.
- `**kwargs` - Additional base-layer options (e.g. ``name``).

#### forward

```python
def forward(inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]
```

Concatenate the input columns along the last dimension.

**Arguments**:

- `inputs` - Mapping from column name to tensor.
  

**Returns**:

  A single-entry mapping from the output column to the concatenated
  tensor, cast to ``dtype`` when one was provided.

## Stack Objects

```python
class Stack(TorchTransformBaseLayer)
```

Stack input tensors along a new dimension.

Inputs are cast to ``float32`` before stacking. For ``N`` input tensors each
of shape ``(B, L)``, the output has shape ``(B, L, N)`` when ``dim=-1`` or
``(B, N, L)`` when ``dim=1``.

**Arguments**:

- `input_cols` - Column names of the input tensors.
- `output_cols` - Single-element list naming the stacked output column.
- `dim` - The dimension along which to stack (default ``-1``).
- `**kwargs` - Additional base-layer options (e.g. ``name``).

#### \_\_init\_\_

```python
def __init__(input_cols: list[str],
             output_cols: list[str],
             dim: int = -1,
             **kwargs) -> None
```

Initialize the Stack layer.

**Arguments**:

- `input_cols` - Column names of the input tensors.
- `output_cols` - Single-element list naming the stacked output column.
- `dim` - The new dimension along which to stack (default ``-1``).
- `**kwargs` - Additional base-layer options (e.g. ``name``).

#### forward

```python
def forward(inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]
```

Stack the input columns along ``dim``.

**Arguments**:

- `inputs` - Mapping from column name to tensor.
  

**Returns**:

  A single-entry mapping from the output column to the stacked tensor.

## Cast Objects

```python
class Cast(TorchTransformBaseLayer)
```

Cast input tensors to a target dtype.

**Arguments**:

- `input_cols` - Column names of the input tensors.
- `output_cols` - Column names of the output tensors; must match the length
  of ``input_cols``.
- `dtype` - Target dtype to cast to. May be a ``torch.dtype`` or a string
  alias (e.g. ``"float32"`` or ``"torch.float32"``). Defaults to
  ``torch.int64`` when ``None``. An unrecognized string alias raises
  ``ValueError``.
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` and ``output_cols`` differ in length, or if
  ``dtype`` is a string that names no recognized dtype.

#### \_\_init\_\_

```python
def __init__(input_cols: list[str],
             output_cols: list[str],
             dtype: torch.dtype | str | None = None,
             **kwargs) -> None
```

Initialize the Cast layer.

**Arguments**:

- `input_cols` - Column names of the input tensors.
- `output_cols` - Column names of the output tensors; must match the
  length of ``input_cols``.
- `dtype` - Target dtype (``torch.dtype`` or string alias); defaults to
  ``torch.int64`` when ``None``.
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` and ``output_cols`` differ in length,
  or if ``dtype`` is a string that names no recognized dtype.

#### forward

```python
def forward(inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]
```

Cast each input column to ``dtype``.

**Arguments**:

- `inputs` - Mapping from column name to tensor.
  

**Returns**:

  A mapping from each output column to its cast tensor.

## Constant Objects

```python
class Constant(TorchTransformBaseLayer)
```

Produce a constant tensor shaped like the input.

Useful for migrating conditional expressions (``if (cond) {...} else {...}``)
whose branches return constants: the constant is materialized as a tensor
matching the reference input's shape.

**Arguments**:

- `input_cols` - Column names of the input tensors, used only for shape
  reference; must match the length of ``output_cols``.
- `output_cols` - Column names of the output tensors.
- `constant` - The value to fill the output tensor with.
- `dtype` - Output dtype. When ``None``, it is inferred from ``constant``.
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` and ``output_cols`` differ in length, or if
  ``input_cols`` is empty (no shape reference available).

#### \_\_init\_\_

```python
def __init__(input_cols: list[str],
             output_cols: list[str],
             constant: int | float | bool,
             dtype: torch.dtype | str | None = None,
             **kwargs) -> None
```

Initialize the Constant layer.

**Arguments**:

- `input_cols` - Column names used for shape reference; must match the
  length of ``output_cols`` and be non-empty.
- `output_cols` - Column names of the output tensors.
- `constant` - The value to fill the output tensor with.
- `dtype` - Output dtype; inferred from ``constant`` when ``None``.
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` and ``output_cols`` differ in length,
  or if ``input_cols`` is empty.

#### forward

```python
def forward(inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]
```

Create a constant tensor matching the input's shape.

**Arguments**:

- `inputs` - Mapping from column name to tensor.
  

**Returns**:

  A mapping from each output column to a constant-filled tensor.

## Divide Objects

```python
class Divide(TorchTransformBaseLayer)
```

Divide input columns pairwise, element-wise, with zero-safe handling.

Input columns are read in ``(numerator, denominator)`` pairs (even indices
are numerators, odd indices denominators), so ``len(input_cols)`` must be
even and ``output_cols`` half its length. Both operands are upcast to
``float64`` before division. A zero denominator is replaced with ``eps`` to
avoid division by zero; when both operands are zero the result is forced to
``0``.

**Arguments**:

- `input_cols` - Column names as ``(numerator, denominator)`` pairs.
- `output_cols` - Column names of the quotient outputs.
- `add_constant_to_divisor` - Constant added to every denominator before
  division.
- `eps` - Small value substituted for a zero denominator to avoid division by
  zero (default
  `DEFAULT_EPSILON`).
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` is not even, or ``output_cols`` is not half
  its length.

#### \_\_init\_\_

```python
def __init__(input_cols: list[str],
             output_cols: list[str],
             add_constant_to_divisor: float = 0.0,
             eps: float = DEFAULT_EPSILON,
             **kwargs) -> None
```

Initialize the Divide layer.

**Arguments**:

- `input_cols` - Column names as ``(numerator, denominator)`` pairs.
- `output_cols` - Column names of the quotient outputs.
- `add_constant_to_divisor` - Constant added to every denominator.
- `eps` - Small value substituted for a zero denominator (default
  `DEFAULT_EPSILON`).
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` is not even, or ``output_cols`` is not
  half its length.

#### forward

```python
def forward(inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]
```

Divide numerators by denominators, pairwise and zero-safe.

**Arguments**:

- `inputs` - Mapping from column name to tensor.
  

**Returns**:

  A mapping from each output column to its quotient tensor.

## LogTransform Objects

```python
class LogTransform(TorchTransformBaseLayer)
```

Apply a logarithmic transform with an offset and input clamping.

Computes ``log(clamp(x + add_constant, min=1.0, max=1e20))``, which is
equivalent to ``log(max(x + add_constant, 1))`` with an upper cap.

**Notes**:

  The input to the logarithm is clamped to ``[1.0, 1e20]``, so the output
  lies in ``[0.0, ~46.05]`` (``log(1e20)``). Any finite or infinite
  element with ``x + add_constant <= 1`` maps to ``0.0``, and values with
  ``x + add_constant`` above ``1`` keep their true logarithm. NaN inputs
  (or a NaN ``add_constant``) propagate to NaN output.
  

**Arguments**:

- `input_cols` - Column names of the input tensors.
- `output_cols` - Column names of the output tensors; must match the length of
  ``input_cols``.
- `add_constant` - Value added before the logarithm to avoid ``log(0)``
  (default ``1.0``).
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` and ``output_cols`` differ in length.

#### \_\_init\_\_

```python
def __init__(input_cols: list[str],
             output_cols: list[str],
             add_constant: float = 1.0,
             **kwargs) -> None
```

Initialize the LogTransform layer.

**Arguments**:

- `input_cols` - Column names of the input tensors.
- `output_cols` - Column names of the output tensors; must match the
  length of ``input_cols``.
- `add_constant` - Value added before the logarithm (default ``1.0``).
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` and ``output_cols`` differ in length.

#### forward

```python
def forward(inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]
```

Apply the log transform to each input column.

**Arguments**:

- `inputs` - Mapping from column name to tensor.
  

**Returns**:

  A mapping from each output column to its transformed tensor, with
  values in ``[0.0, log(1e20)]``.

## Subtract Objects

```python
class Subtract(TorchTransformBaseLayer)
```

Subtract input columns pairwise, element-wise.

Input columns are read in ``(minuend, subtrahend)`` pairs (even indices are
minuends, odd indices subtrahends), so ``len(input_cols)`` must be even and
``output_cols`` half its length. Both operands are upcast to ``float64``
before subtraction.

**Arguments**:

- `input_cols` - Column names as ``(minuend, subtrahend)`` pairs.
- `output_cols` - Column names of the difference outputs.
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` is not even, or ``output_cols`` is not half
  its length.

#### \_\_init\_\_

```python
def __init__(input_cols: list[str], output_cols: list[str], **kwargs) -> None
```

Initialize the Subtract layer.

**Arguments**:

- `input_cols` - Column names as ``(minuend, subtrahend)`` pairs.
- `output_cols` - Column names of the difference outputs.
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` is not even, or ``output_cols`` is not
  half its length.

#### forward

```python
def forward(inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]
```

Subtract subtrahends from minuends, pairwise.

**Arguments**:

- `inputs` - Mapping from column name to tensor.
  

**Returns**:

  A mapping from each output column to its difference tensor.

## Floor Objects

```python
class Floor(TorchTransformBaseLayer)
```

Apply an element-wise floor to input columns.

**Arguments**:

- `input_cols` - Column names of the input tensors.
- `output_cols` - Column names of the output tensors; must match the length of
  ``input_cols``.
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` and ``output_cols`` differ in length.

#### \_\_init\_\_

```python
def __init__(input_cols: list[str], output_cols: list[str], **kwargs) -> None
```

Initialize the Floor layer.

**Arguments**:

- `input_cols` - Column names of the input tensors.
- `output_cols` - Column names of the output tensors; must match the
  length of ``input_cols``.
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` and ``output_cols`` differ in length.

#### forward

```python
def forward(inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]
```

Apply floor to each input column.

**Arguments**:

- `inputs` - Mapping from column name to tensor.
  

**Returns**:

  A mapping from each output column to its floored tensor.

## Ceil Objects

```python
class Ceil(TorchTransformBaseLayer)
```

Apply an element-wise ceiling to input columns.

**Arguments**:

- `input_cols` - Column names of the input tensors.
- `output_cols` - Column names of the output tensors; must match the length of
  ``input_cols``.
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` and ``output_cols`` differ in length.

#### \_\_init\_\_

```python
def __init__(input_cols: list[str], output_cols: list[str], **kwargs) -> None
```

Initialize the Ceil layer.

**Arguments**:

- `input_cols` - Column names of the input tensors.
- `output_cols` - Column names of the output tensors; must match the
  length of ``input_cols``.
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` and ``output_cols`` differ in length.

#### forward

```python
def forward(inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]
```

Apply ceiling to each input column.

**Arguments**:

- `inputs` - Mapping from column name to tensor.
  

**Returns**:

  A mapping from each output column to its ceiled tensor.

## IdentityTransform Objects

```python
class IdentityTransform(TorchTransformBaseLayer)
```

Pass input tensors through unchanged.

Explicitly includes fields in a native transform's input schema without
modifying them — useful for bypass fields that downstream model assembly
needs available.

**Arguments**:

- `input_cols` - Column names of the input tensors.
- `output_cols` - Column names of the output tensors; must match the length of
  ``input_cols``.
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` and ``output_cols`` differ in length.

#### \_\_init\_\_

```python
def __init__(input_cols: list[str], output_cols: list[str], **kwargs) -> None
```

Initialize the IdentityTransform layer.

**Arguments**:

- `input_cols` - Column names of the input tensors.
- `output_cols` - Column names of the output tensors; must match the
  length of ``input_cols``.
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` and ``output_cols`` differ in length.

#### forward

```python
def forward(inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]
```

Pass each input column through unchanged.

**Arguments**:

- `inputs` - Mapping from column name to tensor.
  

**Returns**:

  A mapping from each output column to the corresponding input tensor.

## TensorColFillNone Objects

```python
class TensorColFillNone(TorchTransformBaseLayer)
```

Replace missing (``None``) positions in each input column with a default.

Missing values are detected from the runtime tensor dtype rather than a
passed-in flag: ``NaN`` marks missing values in floating-point tensors, and
the dtype's minimum value marks them in ``int32``/``int64`` tensors (the
convention used when ingesting nullable integer columns).

**Arguments**:

- `input_cols` - Column names of the input tensors.
- `output_cols` - Column names of the output tensors; must match the length of
  ``input_cols``.
- `default_value` - Value substituted for every detected missing position.
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` and ``output_cols`` differ in length.

#### \_\_init\_\_

```python
def __init__(input_cols: list[str], output_cols: list[str],
             default_value: int | float, **kwargs) -> None
```

Initialize the TensorColFillNone layer.

**Arguments**:

- `input_cols` - Column names of the input tensors.
- `output_cols` - Column names of the output tensors; must match the
  length of ``input_cols``.
- `default_value` - Value substituted for every detected missing position.
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` and ``output_cols`` differ in length.

#### forward

```python
def forward(inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]
```

Fill missing positions in each input column.

**Arguments**:

- `inputs` - Mapping from column name to tensor.
  

**Returns**:

  A mapping from each output column to its filled tensor.

## CaseWhen Objects

```python
class CaseWhen(TorchTransformBaseLayer)
```

Select values by condition, like a SQL ``CASE WHEN`` expression.

Input columns are read as ``(condition, value)`` pairs, so
``len(input_cols)`` must be even. For each element, the value of the first
pair whose condition is ``True`` is returned; if no condition matches,
``default_value`` is used. Later pairs take lower priority than earlier ones.

**Arguments**:

- `input_cols` - Column names ordered as ``condition1, value1, condition2,
  value2, ...``.
- `output_cols` - Single-element list naming the result column.
- `default_value` - Value used where no condition matches. A scalar
  (``int``/``float``/``bool``) is broadcast to the value shape; a list
  or tensor is used as-is.
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` does not contain an even number of columns.

#### \_\_init\_\_

```python
def __init__(input_cols: list[str], output_cols: list[str],
             default_value: int | float | bool | list | torch.Tensor,
             **kwargs) -> None
```

Initialize the CaseWhen layer.

**Arguments**:

- `input_cols` - Column names ordered as ``condition1, value1,
  condition2, value2, ...``.
- `output_cols` - Single-element list naming the result column.
- `default_value` - Value used where no condition matches. A scalar is
  broadcast to the value shape; a list or tensor is used as-is.
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` does not contain an even number of
  columns.

#### forward

```python
def forward(inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]
```

Resolve each element to the first matching value, else the default.

**Arguments**:

- `inputs` - Mapping from column name to tensor.
  

**Returns**:

  A single-entry mapping from the output column to the resolved tensor.

## Compare Objects

```python
class Compare(TorchTransformBaseLayer)
```

Compare input columns pairwise with a named comparison operator.

Input columns are read in ``(left, right)`` pairs (even indices are left
operands, odd indices right operands), so ``len(input_cols)`` must be even
and ``output_cols`` half its length. Each pair is compared element-wise and
the boolean result is written to the corresponding output column.

**Arguments**:

- `input_cols` - Column names as ``(left, right)`` pairs.
- `output_cols` - Column names of the boolean outputs.
- `compare_op` - One of ``"equal"``, ``"greater"``, ``"less"``,
  ``"greater_equal"``, ``"less_equal"``, or ``"not_equal"``.
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` is not even or ``output_cols`` is not half
  its length, or if ``compare_op`` is not a supported operator.

#### \_\_init\_\_

```python
def __init__(input_cols: list[str], output_cols: list[str], compare_op: str,
             **kwargs) -> None
```

Initialize the Compare layer.

**Arguments**:

- `input_cols` - Column names as ``(left, right)`` pairs.
- `output_cols` - Column names of the boolean outputs.
- `compare_op` - One of ``"equal"``, ``"greater"``, ``"less"``,
  ``"greater_equal"``, ``"less_equal"``, or ``"not_equal"``.
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` is not even or ``output_cols`` is not
  half its length, or if ``compare_op`` is unsupported.

#### forward

```python
def forward(inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]
```

Compare each ``(left, right)`` pair element-wise.

**Arguments**:

- `inputs` - Mapping from column name to tensor.
  

**Returns**:

  A mapping from each output column to its boolean result tensor.

## Tile Objects

```python
class Tile(TorchTransformBaseLayer)
```

Repeat each input tensor along an axis a fixed or inferred number of times.

The repeat count is either given explicitly via ``count`` or inferred from a
target tensor's size along ``axis``. When inferred, the target tensor is the
last input column and the source tensors are the remaining columns.

As a convenience, when ``axis == 1`` and every source tensor is 1D, sources
are unsqueezed to 2D first so tiling produces a ``(batch, count)`` result
rather than a flat ``(batch * count,)`` tensor.

**Arguments**:

- `input_cols` - Column names of the source tensors. When a target tensor is
  used to infer the count, it is the last column and the sources are
  the columns before it.
- `output_cols` - Column names of the tiled outputs.
- `axis` - Axis along which to tile (default ``0``). Negative values index
  from the end.
- `count` - Explicit number of repetitions. Takes precedence over
  ``target_tensor_provided`` when set.
- `target_tensor_provided` - When ``True`` and ``count`` is ``None``, infer
  the count from the last input column's size along ``axis``.
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If neither ``count`` is set nor ``target_tensor_provided``
  is ``True``.

#### \_\_init\_\_

```python
def __init__(input_cols: list[str],
             output_cols: list[str],
             axis: int = 0,
             count: int | None = None,
             target_tensor_provided: bool = False,
             **kwargs) -> None
```

Initialize the Tile layer.

**Arguments**:

- `input_cols` - Column names of the source tensors (and, when inferring
  the count, a trailing target column).
- `output_cols` - Column names of the tiled outputs.
- `axis` - Axis along which to tile (default ``0``).
- `count` - Explicit number of repetitions; takes precedence over
  ``target_tensor_provided`` when set.
- `target_tensor_provided` - When ``True`` and ``count`` is ``None``,
  infer the count from the last input column's size along ``axis``.
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If neither ``count`` is set nor
  ``target_tensor_provided`` is ``True``.

#### forward

```python
def forward(inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]
```

Tile each source column along ``axis``.

**Arguments**:

- `inputs` - Mapping from column name to tensor.
  

**Returns**:

  A mapping from each output column to its tiled tensor.
  

**Raises**:

- `ValueError` - If neither ``count`` is set nor
  ``target_tensor_provided`` is ``True``.

## PadOrCrop1D Objects

```python
class PadOrCrop1D(TorchTransformBaseLayer)
```

Pad or crop each 1D input column to a fixed length.

Each input column is normalized to exactly ``max_length`` along its last
dimension: shorter sequences are padded with ``pad_value`` and longer ones
are cropped. ``align`` controls which end is kept and padded.

Sentinel positions from upstream ragged-batch collation (``NaN`` for float
dtypes, ``INT32_SENTINEL`` for integer dtypes) are automatically replaced
with ``pad_value`` before the pad/crop logic runs.

**Arguments**:

- `input_cols` - Column names of the input tensors.
- `output_cols` - Column names of the output tensors; must match the length of
  ``input_cols``.
- `max_length` - The fixed target length; must be positive.
- `dtype` - Optional output dtype. When ``None``, the input dtype is
  preserved.
- `pad_value` - The value used for padding (default ``0``).
- `align` - ``"left"`` (default) pads on the right and keeps the first
  ``max_length`` elements; ``"right"`` pads on the left and keeps the
  last ``max_length`` elements of the real content, i.e. sentinel
  positions trailing the data are stripped before the crop rather
  than being cropped to. Retained elements keep their original order.
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` and ``output_cols`` differ in length, if
  ``align`` is not ``"left"`` or ``"right"``, or if ``max_length`` is
  not positive.

#### \_\_init\_\_

```python
def __init__(input_cols: list[str],
             output_cols: list[str],
             max_length: int,
             dtype: torch.dtype | str | None = None,
             pad_value: int | float = 0,
             align: str = "left",
             **kwargs) -> None
```

Initialize the PadOrCrop1D layer.

**Arguments**:

- `input_cols` - Column names of the input tensors.
- `output_cols` - Column names of the output tensors; must match the
  length of ``input_cols``.
- `max_length` - The fixed target length; must be positive.
- `dtype` - Optional output dtype; preserves input dtype when ``None``.
- `pad_value` - The value used for padding (default ``0``).
- `align` - ``"left"`` pads/crops on the right; ``"right"`` pads/crops on
  the left.
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` and ``output_cols`` differ in length,
  if ``align`` is invalid, or if ``max_length`` is not positive.

#### forward

```python
def forward(inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]
```

Normalize each input column to ``max_length``.

**Arguments**:

- `inputs` - Mapping from column name to tensor.
  

**Returns**:

  A mapping from each output column to its padded/cropped tensor.

## Scale Objects

```python
class Scale(TorchTransformBaseLayer)
```

Multiply each input column by a scalar factor.

**Arguments**:

- `input_cols` - Column names of the input tensors.
- `output_cols` - Column names of the output tensors; must match the length of
  ``input_cols``.
- `factor` - The scalar multiplier.
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` and ``output_cols`` differ in length.

#### \_\_init\_\_

```python
def __init__(input_cols: list[str], output_cols: list[str], factor: float,
             **kwargs) -> None
```

Initialize the Scale layer.

**Arguments**:

- `input_cols` - Column names of the input tensors.
- `output_cols` - Column names of the output tensors; must match the
  length of ``input_cols``.
- `factor` - The scalar multiplier.
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` and ``output_cols`` differ in length.

#### forward

```python
def forward(inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]
```

Scale each input column by ``factor``.

**Arguments**:

- `inputs` - Mapping from column name to tensor.
  

**Returns**:

  A mapping from each output column to its scaled tensor.

## Clip Objects

```python
class Clip(TorchTransformBaseLayer)
```

Clamp each input column to a range, optionally exempting one value.

Values are clamped to ``[min_value, max_value]``; a bound left as ``None`` is
not enforced on that side. When ``ignore_value`` is set, positions equal to
it are passed through unchanged even if they fall outside the range (useful
for preserving a padding value such as ``-1.0``).

**Arguments**:

- `input_cols` - Column names of the input tensors.
- `output_cols` - Column names of the output tensors; must match the length of
  ``input_cols``.
- `min_value` - Lower bound, or ``None`` for no lower bound.
- `max_value` - Upper bound, or ``None`` for no upper bound.
- `ignore_value` - Optional value that is preserved unchanged rather than
  clamped.
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` and ``output_cols`` differ in length, or if
  both ``min_value`` and ``max_value`` are ``None``.

#### \_\_init\_\_

```python
def __init__(input_cols: list[str],
             output_cols: list[str],
             min_value: float | None = None,
             max_value: float | None = None,
             ignore_value: float | None = None,
             **kwargs) -> None
```

Initialize the Clip layer.

**Arguments**:

- `input_cols` - Column names of the input tensors.
- `output_cols` - Column names of the output tensors; must match the
  length of ``input_cols``.
- `min_value` - Lower bound, or ``None`` for no lower bound.
- `max_value` - Upper bound, or ``None`` for no upper bound.
- `ignore_value` - Optional value preserved unchanged rather than clamped.
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` and ``output_cols`` differ in length,
  or if both ``min_value`` and ``max_value`` are ``None``.

#### forward

```python
def forward(inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]
```

Clamp each input column, preserving ``ignore_value`` positions.

**Arguments**:

- `inputs` - Mapping from column name to tensor.
  

**Returns**:

  A mapping from each output column to its clamped tensor.

## IDHashTokenizer Objects

```python
class IDHashTokenizer(TorchTransformBaseLayer)
```

Map integer IDs to contiguous vocabulary indices.

TODO(https://github.com/michelangelo-ai/michelangelo/issues/1699): the
core this wraps is colocated under ``native_transform`` only because it's
currently the sole consumer; move both into a standalone reusable-layers
package if that changes.

Composes the vocabulary-lookup core in
`michelangelo.lib.native_transform.torch.id_hash_tokenizer.IDHashTokenizer`
into the ``dict[str, torch.Tensor]`` layer contract. Sentinel positions
from upstream ragged-batch collation (``NaN`` for float inputs,
``INT32_SENTINEL`` for integer inputs) are automatically detected and
preserved through the lookup: the sentinel mask is captured before the
float-to-int32 cast the core requires (``NaN`` does not survive that
cast), and restored as ``INT32_SENTINEL`` in the output regardless of the
input's original dtype.

**Notes**:

  This is a different class from the one previously exported
  as ``michelangelo.lib.native_transform.torch.IDHashTokenizer``: prior to
  this layer's addition, that top-level name referred to the bare
  lookup core above (a plain ``nn.Module`` taking and returning a
  single ``torch.Tensor``). That core is still available under its
  original name at
  `michelangelo.lib.native_transform.torch.id_hash_tokenizer`; the
  top-level package name now refers to this layer instead, which
  composes the core into the dict-of-tensors transform-layer contract
  used throughout this package.
  
  This naming collision is specific to this repo's layout: internally,
  the lookup core lives in a separate package entirely (outside
  ``native_transform``), so its wrapper layer and the core it composes
  never shared a name or a module. Here, both live under
  ``native_transform.torch``, which is what forces this repoint.
  

**Arguments**:

- `input_cols` - Column names of the input tensors.
- `output_cols` - Column names of the output tensors; must match the
  length of ``input_cols``.
- `vocabulary` - List of integer values to map to contiguous indices,
  forwarded to the wrapped core tokenizer.
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` and ``output_cols`` differ in length.

#### \_\_init\_\_

```python
def __init__(input_cols: list[str], output_cols: list[str],
             vocabulary: list[int], **kwargs) -> None
```

Initialize the IDHashTokenizer layer.

**Arguments**:

- `input_cols` - Column names of the input tensors.
- `output_cols` - Column names of the output tensors; must match the
  length of ``input_cols``.
- `vocabulary` - List of integer values to map to contiguous indices,
  forwarded to the wrapped core tokenizer.
- `**kwargs` - Additional base-layer options (e.g. ``name``).
  

**Raises**:

- `ValueError` - If ``input_cols`` and ``output_cols`` differ in
  length.

#### forward

```python
def forward(inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]
```

Map the stacked input columns to vocabulary indices.

**Arguments**:

- `inputs` - Mapping from column name to tensor.
  

**Returns**:

  A mapping from each output column to its mapped-index tensor.

