"""Fitted-statistics PyTorch native transform layers.

Layers whose behavior is parameterized by fitted statistics (a min/max range,
a mean/std, or bucket boundaries) computed ahead of time and supplied at
construction. Like the layers in
:mod:`~michelangelo.lib.native_transform.torch.base_layers`, every layer here
subclasses
:class:`~michelangelo.lib.native_transform.torch.base_layers.TorchTransformBaseLayer`
and uses the same ``dict[str, torch.Tensor]`` in/out contract, but stores its
fitted parameters as buffers (via ``register_buffer``) so they move with
``.to(device)`` and persist through TorchScript save/load.
"""

from __future__ import annotations

import torch

from michelangelo.lib.constants.sentinel import INT32_SENTINEL
from michelangelo.lib.native_transform.torch.base_layers import (
    TorchTransformBaseLayer,
)
from michelangelo.lib.native_transform.torch.constants import DEFAULT_EPSILON
from michelangelo.lib.native_transform.torch.utils import (
    format_inputs,
    format_outputs,
    initialize_dtype,
)

__all__ = [
    "Bucketization",
    "MinMax",
    "Normalization",
]


class MinMax(TorchTransformBaseLayer):
    """Min-max scale input columns to a ``[0, 1]`` range.

    Concatenates the input columns along ``dim`` and rescales with
    ``(x - min) / max(max - min, eps)``.

    Args:
        input_cols: Column names of the input tensors.
        output_cols: Column names of the output tensor; must contain exactly
            one column, since the inputs are concatenated into a single
            output.
        min: The per-feature minimum values used for scaling.
        max: The per-feature maximum values used for scaling.
        dim: The dimension along which input columns are concatenated.
            Defaults to ``-1``, since dimension ``0`` is typically the batch
            dimension.
        eps: A small value added to the denominator to avoid division by zero
            when ``min == max``. Defaults to ``1e-7``.
        **kwargs: Additional base-layer options (e.g. ``name``).

    Raises:
        ValueError: If ``output_cols`` does not contain exactly one column.
    """

    def __init__(
        self,
        input_cols: list[str],
        output_cols: list[str],
        min: float | list[float] | torch.Tensor,
        max: float | list[float] | torch.Tensor,
        dim: int = -1,
        eps: float = DEFAULT_EPSILON,
        **kwargs,
    ) -> None:
        """Initialize the MinMax layer.

        Args:
            input_cols: Column names of the input tensors.
            output_cols: Column names of the output tensor; must contain
                exactly one column.
            min: The per-feature minimum values used for scaling.
            max: The per-feature maximum values used for scaling.
            dim: The dimension along which input columns are concatenated.
            eps: A small value added to the denominator to avoid division by
                zero when ``min == max``.
            **kwargs: Additional base-layer options (e.g. ``name``).

        Raises:
            ValueError: If ``output_cols`` does not contain exactly one
                column.
        """
        super().__init__(input_cols, output_cols, **kwargs)
        if len(output_cols) != 1:
            raise ValueError(
                "output_cols must contain exactly one column, since the "
                "input columns are concatenated into a single output."
            )
        # Registered as buffers so they move with .to(device) and persist
        # through TorchScript save/load.
        self.register_buffer("min_buffer", torch.as_tensor(min, dtype=torch.float32))
        self.register_buffer("max_buffer", torch.as_tensor(max, dtype=torch.float32))
        self.dim = dim
        self.eps = eps

    def forward(self, inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]:
        """Min-max scale the concatenated input columns.

        Args:
            inputs: Mapping from column name to tensor.

        Returns:
            A single-entry mapping from the output column to the scaled
            tensor.
        """
        tensor_list: list[torch.Tensor] = []
        for col in self.input_cols:
            col_tensor = inputs[col]
            if self.dim != 0 and len(col_tensor.shape) == 1:
                # The input shape should be [batch_size, vector_dim].
                col_tensor = torch.unsqueeze(col_tensor, 1)
            tensor_list.append(col_tensor)

        concatenated_tensor = torch.cat(tensor_list, dim=self.dim).float()
        min_val = self.min_buffer.to(dtype=concatenated_tensor.dtype)
        max_val = self.max_buffer.to(dtype=concatenated_tensor.dtype)
        # Clamp the denominator to avoid division by zero when min == max.
        safe_denominator = torch.clamp(max_val - min_val, min=self.eps)
        concatenated_tensor.sub_(min_val).div_(safe_denominator)
        return {self.output_cols[0]: concatenated_tensor}


class Normalization(TorchTransformBaseLayer):
    """Feature-wise standardization: ``(x - mean) / std``.

    Concatenates the input columns along ``dim`` and standardizes the result
    to be centered around 0 with a standard deviation of 1.

    Args:
        input_cols: Column names of the input tensors.
        output_cols: Column names of the output tensor; must contain exactly
            one column, since the inputs are concatenated into a single
            output.
        mean: The per-feature mean values used for standardization.
        std: The per-feature standard deviation values used for
            standardization.
        dim: The dimension along which input columns are concatenated.
            Defaults to ``-1``, since dimension ``0`` is typically the batch
            dimension.
        eps: A small value clamped onto ``std`` to avoid division by zero when
            ``std == 0``. Defaults to ``1e-7``.
        **kwargs: Additional base-layer options (e.g. ``name``).

    Raises:
        ValueError: If ``output_cols`` does not contain exactly one column.
    """

    def __init__(
        self,
        input_cols: list[str],
        output_cols: list[str],
        mean: float | list[float] | torch.Tensor,
        std: float | list[float] | torch.Tensor,
        dim: int = -1,
        eps: float = DEFAULT_EPSILON,
        **kwargs,
    ) -> None:
        """Initialize the Normalization layer.

        Args:
            input_cols: Column names of the input tensors.
            output_cols: Column names of the output tensor; must contain
                exactly one column.
            mean: The per-feature mean values used for standardization.
            std: The per-feature standard deviation values used for
                standardization.
            dim: The dimension along which input columns are concatenated.
            eps: A small value clamped onto ``std`` to avoid division by zero
                when ``std == 0``.
            **kwargs: Additional base-layer options (e.g. ``name``).

        Raises:
            ValueError: If ``output_cols`` does not contain exactly one
                column.
        """
        super().__init__(input_cols, output_cols, **kwargs)
        if len(output_cols) != 1:
            raise ValueError(
                "output_cols must contain exactly one column, since the "
                "input columns are concatenated into a single output."
            )
        self.register_buffer("mean_buffer", torch.as_tensor(mean, dtype=torch.float32))
        self.register_buffer("std_buffer", torch.as_tensor(std, dtype=torch.float32))
        self.dim = dim
        self.eps = eps

    def forward(self, inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]:
        """Standardize the concatenated input columns.

        Args:
            inputs: Mapping from column name to tensor.

        Returns:
            A single-entry mapping from the output column to the standardized
            tensor.
        """
        tensor_list: list[torch.Tensor] = []
        for col in self.input_cols:
            col_tensor = inputs[col]
            if self.dim != 0 and len(col_tensor.shape) == 1:
                # The input shape should be [batch_size, vector_dim].
                col_tensor = torch.unsqueeze(col_tensor, 1)
            tensor_list.append(col_tensor)

        concatenated_tensor = torch.cat(tensor_list, dim=self.dim).float()
        mean_val = self.mean_buffer.to(dtype=concatenated_tensor.dtype)
        std_val = self.std_buffer.to(dtype=concatenated_tensor.dtype)
        # Clamp std to avoid division by zero.
        safe_std = torch.clamp(std_val, min=self.eps)
        concatenated_tensor.sub_(mean_val).div_(safe_std)
        return {self.output_cols[0]: concatenated_tensor}


class Bucketization(TorchTransformBaseLayer):
    """Bucketize each input column using shared boundaries.

    Sentinel positions from upstream ragged-batch collation (``NaN`` for float
    inputs, ``INT32_SENTINEL`` for integer inputs) are automatically detected
    and preserved through bucketization (rewritten as ``INT32_SENTINEL`` in
    the output) so downstream layers can distinguish padding from real,
    bucketized data.

    Args:
        input_cols: Column names of the input tensors.
        output_cols: Column names of the output tensors; must match the
            length of ``input_cols``.
        boundaries: The boundary values used for bucketization. Bucket edges
            are closed on the left (``torch.bucketize(..., right=True)``).
        dtype: Output tensor dtype. Defaults to ``torch.int32``.
        **kwargs: Additional base-layer options (e.g. ``name``).

    Raises:
        ValueError: If ``input_cols`` and ``output_cols`` differ in length.
    """

    def __init__(
        self,
        input_cols: list[str],
        output_cols: list[str],
        boundaries: list[float],
        dtype: torch.dtype | str | None = None,
        **kwargs,
    ) -> None:
        """Initialize the Bucketization layer.

        Args:
            input_cols: Column names of the input tensors.
            output_cols: Column names of the output tensors; must match the
                length of ``input_cols``.
            boundaries: The boundary values used for bucketization.
            dtype: Output tensor dtype. Defaults to ``torch.int32``.
            **kwargs: Additional base-layer options (e.g. ``name``).

        Raises:
            ValueError: If ``input_cols`` and ``output_cols`` differ in
                length.
        """
        super().__init__(input_cols, output_cols, **kwargs)
        if len(input_cols) != len(output_cols):
            raise ValueError(
                "Input columns and output columns must have the same length."
            )
        self.register_buffer("boundaries", torch.tensor(boundaries))
        self.dtype = initialize_dtype(dtype, torch.int32)
        # Stored as an instance attribute (rather than read from a module) for
        # TorchScript compatibility.
        self._int_sentinel = INT32_SENTINEL

    def forward(self, inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]:
        """Bucketize the stacked input columns, preserving sentinel positions.

        Args:
            inputs: Mapping from column name to tensor.

        Returns:
            A mapping from each output column to its bucketized tensor.
        """
        stacked_input = format_inputs(self.input_cols, inputs)

        # torch.bucketize with right=True means the left boundary is closed:
        # https://pytorch.org/docs/stable/generated/torch.bucketize.html
        stacked_output = torch.bucketize(stacked_input, self.boundaries, right=True).to(
            self.dtype
        )

        # Restore sentinel positions so downstream layers can distinguish
        # padding from real data; without this, padding would be mapped to
        # bucket 0 and become indistinguishable from genuine data. The
        # restored sentinel matches the *output* dtype: NaN when the output
        # is floating-point (even for integer input), INT32_SENTINEL
        # otherwise.
        if stacked_input.is_floating_point():
            mask = torch.isnan(stacked_input)
            if stacked_output.is_floating_point():
                sentinel_out = stacked_output.new_full((), float("nan"))
            else:
                sentinel_out = stacked_output.new_full((), self._int_sentinel)
        else:
            mask = stacked_input == self._int_sentinel
            sentinel_out = stacked_output.new_full((), self._int_sentinel)
        stacked_output = torch.where(mask, sentinel_out, stacked_output)

        return format_outputs(self.output_cols, stacked_output)
