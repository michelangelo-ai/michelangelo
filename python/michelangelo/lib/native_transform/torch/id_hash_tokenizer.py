"""ID-hash tokenizer layer for native feature transforms.

Maps arbitrary integer input values to contiguous, zero-based indices based on a
provided vocabulary, mapping out-of-vocabulary values to a dedicated unknown
index. The layer is TorchScript- and ONNX-exportable so it can be embedded in a
model graph and run identically at train and serve time.
"""

from __future__ import annotations

import warnings

import torch
from torch import nn

__all__ = ["IDHashTokenizer"]


class IDHashTokenizer(nn.Module):
    """Map integer IDs to contiguous vocabulary indices.

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

    Despite the name "Hash", this performs an exact vocabulary lookup via
    :func:`torch.bucketize` (not a hash); the name is kept for parity with the
    internal SDK layer it was migrated from.

    Args:
        vocabulary: List of integer values to map to contiguous indices. Duplicate
            values are removed, preserving the index of their first occurrence.

    Raises:
        TypeError: If ``vocabulary`` is not a list of integers.
        ValueError: If ``vocabulary`` is empty.

    Example:
        >>> tokenizer = IDHashTokenizer(vocabulary=[-10, -3, 0, 2, 4, 6])
        >>> tokenizer(torch.tensor([-10, 0, 5], dtype=torch.long))
        tensor([0, 2, 6])
    """

    def __init__(self, vocabulary: list[int]) -> None:
        """Initialize the tokenizer from a vocabulary of integer values.

        Args:
            vocabulary: List of integer values to map to contiguous indices.
                Duplicate values are removed, preserving the index of their first
                occurrence.

        Raises:
            TypeError: If ``vocabulary`` is not a list of integers.
            ValueError: If ``vocabulary`` is empty.
        """
        super().__init__()
        # Validate input vocabulary type.
        if not isinstance(vocabulary, list) or not all(
            isinstance(v, int) for v in vocabulary
        ):
            raise TypeError("Vocabulary must be a list of integers.")

        # An empty vocabulary leaves output_vocab_size == 0, which would collapse
        # every lookup index to -1 in forward() and only fail there (potentially
        # at serve time). Reject it up front with a clear message.
        if not vocabulary:
            raise ValueError("vocabulary must be non-empty.")

        # Deduplicate while preserving original order. If duplicates exist (e.g.
        # [10, 20, 10]), only the first occurrence is kept and its original index
        # is used for the mapping.
        unique_vocab_values: list[int] = []
        seen_values: set[int] = set()
        for v in vocabulary:
            if v not in seen_values:
                unique_vocab_values.append(v)
                seen_values.add(v)

        if len(unique_vocab_values) != len(vocabulary):
            warnings.warn(
                "Duplicate values found in vocabulary. Only unique values will be "
                "used, preserving the index of their first occurrence. Original "
                f"size: {len(vocabulary)}, effective unique size: "
                f"{len(unique_vocab_values)}.",
                stacklevel=2,
            )

        # Store the effective unique vocabulary (preserving original order).
        self.vocabulary = unique_vocab_values

        # The unknown index is one past the last valid mapped index.
        self.unk_index = len(self.vocabulary)
        self.output_vocab_size = len(self.vocabulary)

        # Map each unique vocabulary value to its original desired output index
        # (its position in ``self.vocabulary``).
        value_to_original_idx_map = {
            val: idx for idx, val in enumerate(self.vocabulary)
        }

        # Sorted unique values used by torch.bucketize for efficient lookup.
        sorted_unique_values = sorted(self.vocabulary)
        self.register_buffer(
            "_sorted_unique_values_tensor",
            torch.tensor(sorted_unique_values, dtype=torch.long),
            persistent=True,  # Saved with the model and moved by ``.to(device)``.
        )

        # Map from an index in ``_sorted_unique_values_tensor`` back to the value's
        # *original* index in ``self.vocabulary``, preserving the caller's desired
        # mapping order. Example: vocabulary=[0, -10] -> sorted=[-10, 0]; -10
        # (sorted index 0) was at original index 1 and 0 (sorted index 1) was at
        # original index 0, so this mapping is [1, 0].
        sorted_to_original_idx_mapping = [
            value_to_original_idx_map[val] for val in sorted_unique_values
        ]
        self.register_buffer(
            "_sorted_to_original_idx_mapping_tensor",
            torch.tensor(sorted_to_original_idx_mapping, dtype=torch.long),
            persistent=True,  # Saved with the model and moved by ``.to(device)``.
        )

    def forward(self, input_ids: torch.Tensor) -> torch.Tensor:
        """Map input integer IDs to contiguous vocabulary indices.

        Values not found in the vocabulary are mapped to :attr:`unk_index`.

        Args:
            input_ids: Tensor of integer IDs of any shape (e.g.
                ``(batch_size, sequence_length)``). Must have dtype
                ``torch.int32`` or ``torch.long``.

        Returns:
            Tensor of mapped indices with the same shape and dtype as
            ``input_ids``.

        Raises:
            TypeError: If ``input_ids`` is not of integer type (``torch.int32`` or
                ``torch.long``).
        """
        # Ensure input tensor is of integer type.
        if input_ids.dtype != torch.int32 and input_ids.dtype != torch.long:
            raise TypeError(
                "Input tensor must be of integer type (torch.int32 or "
                f"torch.long), but got {input_ids.dtype}."
            )

        input_dtype = input_ids.dtype

        # torch.bucketize and tensor indexing work best with long.
        input_ids_long = input_ids.to(torch.long)

        # Step 1: Find potential mapped indices within the *sorted* vocabulary.
        # torch.bucketize returns indices in [0, len(sorted)]; a value equal to
        # len(sorted) means the input is > the last sorted vocabulary element.
        potential_sorted_indices = torch.bucketize(
            input_ids_long, self._sorted_unique_values_tensor
        )

        # Step 2: Clamp to valid tensor bounds [0, size-1] to prevent IndexError
        # on the lookups below. ``is_known_mask`` re-identifies clamped
        # out-of-range values as unknown.
        clamped_sorted_indices = torch.clamp(
            potential_sorted_indices, 0, self.output_vocab_size - 1
        )

        # Step 3: Retrieve the vocabulary value at each clamped index, then compare
        # it to the original input to confirm the value is actually present.
        retrieved_value_from_sorted_vocab = self._sorted_unique_values_tensor[
            clamped_sorted_indices
        ]

        # Step 4: A value is "known" if its ID matches the retrieved value AND the
        # (unclamped) bucketize result was within the vocabulary bounds.
        is_known_mask = (input_ids_long == retrieved_value_from_sorted_vocab) & (
            potential_sorted_indices < self.output_vocab_size
        )

        # Step 5: For known values, remap the sorted index back to the value's
        # original position in the provided vocabulary; for unknown values, assign
        # ``unk_index``. ``new_full`` keeps the constant on the input's device
        # (avoids baking a literal device into a TorchScript trace).
        mapped_to_original_indices = self._sorted_to_original_idx_mapping_tensor[
            clamped_sorted_indices
        ]
        unk = input_ids_long.new_full((), self.unk_index)
        output_ids = torch.where(is_known_mask, mapped_to_original_indices, unk)

        # Match the input dtype on the way out.
        return output_ids.to(input_dtype)
