---
sidebar_label: utils
title: michelangelo.lib.native_transform.torch.utils
---

Utility helpers for PyTorch native transform layers.

Helpers for dtype resolution, layer-name generation, and the
dict-of-tensors input/output contract shared by every transform layer. The
``format_inputs`` / ``format_outputs`` pair defines the package&#x27;s I/O convention:
layers receive and return ``dict[str, torch.Tensor]`` (TorchScript-friendly),
stacking the selected columns into a single tensor for vectorized computation.

#### sentinel\_for\_torch\_dtype

```python
def sentinel_for_torch_dtype(dtype: torch.dtype) -> float | int
```

Return the type-native sentinel value for a torch dtype.

**Arguments**:

- `dtype` - The torch dtype to look up a sentinel for.
  

**Returns**:

  ``FLOAT_SENTINEL`` (NaN) for floating-point dtypes and ``INT32_SENTINEL``
  for integer dtypes.
  

**Raises**:

- `ValueError` - If no sentinel is defined for ``dtype``.

#### id\_generator

```python
def id_generator(size: int = 10,
                 chars: str = string.ascii_uppercase + string.digits) -> str
```

Generate a random identifier string.

**Arguments**:

- `size` - Number of characters in the generated string.
- `chars` - Character set to sample from. Defaults to uppercase ASCII letters
  and digits.
  

**Returns**:

  A random string of length ``size`` drawn from ``chars``.

#### to\_snake\_case

```python
def to_snake_case(name: str) -> str
```

Convert a class-style name to snake_case.

Adapted from the Keras backend helper. Names that would begin with an
underscore (i.e. from private class names) are prefixed with ``&quot;private&quot;``,
since a leading underscore is not a valid TorchScript scope name.

**Arguments**:

- `name` - The name to convert (e.g. a class name in ``CamelCase``).
  

**Returns**:

  The snake_case form of ``name``.

#### generate\_layer\_name

```python
def generate_layer_name(layer_name: str) -> str
```

Generate a unique snake_case layer name.

**Arguments**:

- `layer_name` - The base name to derive from (typically a layer class name).
  

**Returns**:

  The snake_case form of ``layer_name`` suffixed with a random identifier,
  e.g. ``&quot;concatenate_A1B2C3D4E5&quot;``.

#### resolve\_torch\_dtype

```python
def resolve_torch_dtype(dtype_spec: torch.dtype | str) -> torch.dtype | str
```

Resolve a dtype spec to a concrete torch dtype.

**Arguments**:

- `dtype_spec` - Either a ``torch.dtype`` or a string alias. Recognized
  strings include the ``&quot;torch.&quot;``-prefixed class names (e.g.
  ``&quot;torch.float32&quot;``) and the bare aliases (e.g. ``&quot;float32&quot;``). The
  special value ``&quot;string&quot;`` resolves to itself.
  

**Returns**:

  The resolved ``torch.dtype`` (or ``&quot;string&quot;`` for the string alias).
  

**Raises**:

- ``5 - If ``dtype_spec`` cannot be resolved.

#### initialize\_dtype

```python
def initialize_dtype(
        raw_dtype: torch.dtype | str | None,
        default_dtype: torch.dtype | None) -> torch.dtype | str | None
```

Resolve a layer&#x27;s dtype argument, falling back to a default.

String inputs are resolved through :func:`resolve_torch_dtype`, so the two
functions agree on every string: both the ``&quot;torch.&quot;``-prefixed class names
(e.g. ``&quot;torch.float32&quot;``) and the bare aliases (e.g. ``&quot;float32&quot;``) are
recognized, and an unrecognized string raises ``ValueError`` rather than
silently resolving to ``None``.

**Arguments**:

- ``1 - The dtype value from a layer spec. May be a ``torch.dtype``, a
  string alias (e.g. ``&quot;float32&quot;`` or ``&quot;torch.float32&quot;``), or
  ``None``.
- ``0 - The dtype to return when ``raw_dtype`` is neither a
  ``torch.dtype`` nor a string (e.g. ``None``).
  

**Returns**:

  The resolved ``torch.dtype`` for a dtype or recognized string, ``&quot;string&quot;``
  for the string-type alias, or ``default_dtype`` when ``raw_dtype`` is
  neither a ``torch.dtype`` nor a string.
  

**Raises**:

- ``7 - If ``raw_dtype`` is a string that names no recognized dtype.

#### format\_inputs

```python
def format_inputs(input_columns: list[str],
                  inputs: dict[str, torch.Tensor]) -> torch.Tensor
```

Stack selected input columns into a single tensor.

**Arguments**:

- `input_columns` - The column names to select, in order.
- `inputs` - Mapping from column name to tensor.
  

**Returns**:

  A tensor stacking ``inputs[col]`` for each column in ``input_columns``
  along a new leading dimension.

#### format\_outputs

```python
def format_outputs(output_columns: list[str],
                   outputs: torch.Tensor) -> dict[str, torch.Tensor]
```

Split a stacked output tensor into a column-keyed dictionary.

Inverse of :func:`format_inputs`: unbinds ``outputs`` along its leading
dimension and maps each slice to the corresponding output column name.

**Arguments**:

- `output_columns` - The output column names, in order.
- `outputs` - The stacked output tensor to split.
  

**Returns**:

  A mapping from each output column name to its tensor slice.

