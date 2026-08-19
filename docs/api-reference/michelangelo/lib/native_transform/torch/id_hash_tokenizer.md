---
sidebar_label: id_hash_tokenizer
title: michelangelo.lib.native_transform.torch.id_hash_tokenizer
---

ID-hash tokenizer layer for native feature transforms.

Maps arbitrary integer input values to contiguous, zero-based indices based on a
provided vocabulary, mapping out-of-vocabulary values to a dedicated unknown
index. The layer is TorchScript- and ONNX-exportable so it can be embedded in a
model graph and run identically at train and serve time.

## IDHashTokenizer Objects

```python
class IDHashTokenizer(nn.Module)
```

Map integer IDs to contiguous vocabulary indices.

Maps arbitrary input integer values to new, contiguous integer indices based
on a provided vocabulary. Values not found in the vocabulary are mapped to an
unknown index, which is set to the size of the (deduplicated) vocabulary.

The input ``vocabulary`` may be unsorted. The mapping from an original
vocabulary value to its new index is based on its position in the *provided*
vocabulary list (i.e. ``vocabulary[i]`` maps to ``i``). Internally the values
are sorted for an efficient :func:`torch.bucketize` lookup, then remapped back
to their original positions, so ordering of the provided list is preserved in
the output indices.

The layer is compatible with both TorchScript and ONNX export.

Despite the name &quot;Hash&quot;, this performs an exact vocabulary lookup via
:func:`torch.bucketize` (not a hash); the name is kept for parity with the
internal SDK layer it was migrated from.

**Arguments**:

- `vocabulary` - List of integer values to map to contiguous indices. Duplicate
  values are removed, preserving the index of their first occurrence.
  

**Raises**:

- `TypeError` - If ``vocabulary`` is not a list of integers.
- ``2 - If ``vocabulary`` is empty.
  

**Example**:

  &gt;&gt;&gt; tokenizer = IDHashTokenizer(vocabulary=[-10, -3, 0, 2, 4, 6])
  &gt;&gt;&gt; tokenizer(torch.tensor([-10, 0, 5], dtype=torch.long))
  tensor([0, 2, 6])

#### \_\_init\_\_

```python
def __init__(vocabulary: list[int]) -> None
```

Initialize the tokenizer from a vocabulary of integer values.

**Arguments**:

- `vocabulary` - List of integer values to map to contiguous indices.
  Duplicate values are removed, preserving the index of their first
  occurrence.
  

**Raises**:

- `TypeError` - If ``vocabulary`` is not a list of integers.
- `ValueError` - If ``vocabulary`` is empty.

#### forward

```python
def forward(input_ids: torch.Tensor) -> torch.Tensor
```

Map input integer IDs to contiguous vocabulary indices.

Values not found in the vocabulary are mapped to :attr:`unk_index`.

**Arguments**:

- `input_ids` - Tensor of integer IDs of any shape (e.g.
  ``(batch_size, sequence_length)``). Must have dtype
  ``torch.int32`` or ``torch.long``.
  

**Returns**:

  Tensor of mapped indices with the same shape and dtype as
  ``input_ids``.
  

**Raises**:

- `input_ids`0 - If ``input_ids`` is not of integer type (``torch.int32`` or
  ``torch.long``).

