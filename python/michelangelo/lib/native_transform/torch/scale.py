"""Clip-and-scale transform layer.

A single-layer alternative to composing
:class:`~michelangelo.lib.native_transform.torch.base_layers.Clip` and
:class:`~michelangelo.lib.native_transform.torch.base_layers.Scale`, used by
the imperative helpers in
:mod:`~michelangelo.lib.native_transform.torch.transform_utils`.
"""

from __future__ import annotations

import torch

from michelangelo.lib.native_transform.torch.base_layers import TorchTransformBaseLayer
from michelangelo.lib.native_transform.torch.utils import resolve_torch_dtype

__all__ = ["ClipAndScale"]


class ClipAndScale(TorchTransformBaseLayer):
    """Clip input columns to a range, then scale and optionally cast the result.

    Every input column is stacked into a single tensor (1D columns are treated
    as a single feature and unsqueezed to a column vector first), clamped to
    ``[min_value, max_value]``, multiplied by ``scale_factor``, and optionally
    cast to ``output_type``.

    Args:
        input_cols: Column names of the input tensors to clip and scale.
        output_cols: Single-element list naming the output column.
        min_value: Lower bound of the clip range.
        max_value: Upper bound of the clip range.
        scale_factor: Factor the clipped tensor is multiplied by.
        output_type: Output dtype, resolved via
            :func:`~michelangelo.lib.native_transform.torch.utils.resolve_torch_dtype`.
            When ``None``, the output is cast to ``torch.float32``.
        **kwargs: Additional base-layer options (e.g. ``name``).
    """

    def __init__(
        self,
        input_cols: list[str],
        output_cols: list[str],
        min_value: float,
        max_value: float,
        scale_factor: float,
        output_type: torch.dtype | str | None,
        **kwargs,
    ) -> None:
        """Initialize the ClipAndScale layer.

        Args:
            input_cols: Column names of the input tensors to clip and scale.
            output_cols: Single-element list naming the output column.
            min_value: Lower bound of the clip range.
            max_value: Upper bound of the clip range.
            scale_factor: Factor the clipped tensor is multiplied by.
            output_type: Output dtype, resolved via
                :func:`~michelangelo.lib.native_transform.torch.utils.resolve_torch_dtype`.
                When ``None``, the output is cast to ``torch.float32``.
            **kwargs: Additional base-layer options (e.g. ``name``).
        """
        super().__init__(input_cols, output_cols, **kwargs)
        self.min_value = min_value
        self.max_value = max_value
        self.scale_factor = scale_factor
        self.output_type = output_type

    def forward(self, inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]:
        """Clip, scale, and cast the concatenated input columns.

        Args:
            inputs: Mapping from column name to tensor for at least every
                column in ``input_cols``.

        Returns:
            A single-key mapping from ``output_cols[0]`` to the clipped,
            scaled, and cast result tensor.
        """
        dtype = (
            resolve_torch_dtype(self.output_type)
            if self.output_type is not None
            else torch.float32
        )
        tensor_list: list[torch.Tensor] = []
        for col in self.input_cols:
            col_tensor = inputs[col]
            if len(col_tensor.shape) == 1:
                # The input shape should be [batch_size, vector_dim].
                col_tensor = torch.unsqueeze(col_tensor, 1)
            tensor_list.append(col_tensor)
        tensor = torch.cat(tensor_list, dim=-1).float()
        min_value = torch.as_tensor(
            self.min_value, dtype=tensor.dtype, device=tensor.device
        )
        max_value = torch.as_tensor(
            self.max_value, dtype=tensor.dtype, device=tensor.device
        )
        clipped = torch.clamp(tensor, min=min_value, max=max_value)
        scaled = self.scale_factor * clipped
        return {self.output_cols[0]: scaled.to(dtype=dtype)}
