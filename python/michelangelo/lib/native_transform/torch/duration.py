"""Time-duration transform layer.

Computes an elapsed-time feature between two epoch-millisecond tensors, used by
the imperative helpers in
:mod:`~michelangelo.lib.native_transform.torch.transform_utils`.
"""

from __future__ import annotations

import torch

from michelangelo.lib.native_transform.torch.base_layers import TorchTransformBaseLayer
from michelangelo.lib.native_transform.torch.constants import DEFAULT_TIME_DURATION_UNIT

__all__ = ["TimeDuration"]


class TimeDuration(TorchTransformBaseLayer):
    """Compute a floored, unit-scaled duration between two timestamp tensors.

    Given a target and a source timestamp tensor (both in epoch milliseconds),
    computes ``floor((target - source) / unit)``, optionally applies
    ``log1p(abs(x))``, and optionally clips the result to ``[min_value,
    max_value]`` (applied after log scaling).

    Args:
        input_cols: Two-element list ``[target_col, source_col]``.
        output_cols: Single-element list naming the output column.
        unit: Divisor applied to the millisecond difference before flooring,
            e.g. milliseconds in a day.
        target_shape: If given, the target tensor is reshaped to this shape
            before computing the difference.
        source_shape: If given, the source tensor is reshaped to this shape
            before computing the difference.
        min_value: Lower clip bound, applied after log scaling. Must be given
            together with ``max_value``.
        max_value: Upper clip bound, applied after log scaling. Must be given
            together with ``min_value``.
        log_scale: If ``True``, apply ``log1p(abs(x))`` to the floored
            difference before clipping.
        **kwargs: Additional base-layer options (e.g. ``name``).

    Raises:
        AssertionError: If exactly one of ``min_value``/``max_value`` is
            given.
    """

    def __init__(
        self,
        input_cols: list[str],
        output_cols: list[str],
        unit: float = DEFAULT_TIME_DURATION_UNIT,
        target_shape: tuple[int, ...] | None = None,
        source_shape: tuple[int, ...] | None = None,
        min_value: float | None = None,
        max_value: float | None = None,
        log_scale: bool = False,
        **kwargs,
    ) -> None:
        """Initialize the TimeDuration layer.

        Args:
            input_cols: Two-element list ``[target_col, source_col]``.
            output_cols: Single-element list naming the output column.
            unit: Divisor applied to the millisecond difference before
                flooring, e.g. milliseconds in a day.
            target_shape: If given, the target tensor is reshaped to this
                shape before computing the difference.
            source_shape: If given, the source tensor is reshaped to this
                shape before computing the difference.
            min_value: Lower clip bound, applied after log scaling. Must be
                given together with ``max_value``.
            max_value: Upper clip bound, applied after log scaling. Must be
                given together with ``min_value``.
            log_scale: If ``True``, apply ``log1p(abs(x))`` to the floored
                difference before clipping.
            **kwargs: Additional base-layer options (e.g. ``name``).

        Raises:
            AssertionError: If exactly one of ``min_value``/``max_value`` is
                given.
        """
        super().__init__(input_cols, output_cols, **kwargs)
        self.unit = unit
        self.target_shape = target_shape
        self.source_shape = source_shape
        self.min_value = min_value
        self.max_value = max_value
        self.log_scale = log_scale
        assert (min_value is None and max_value is None) or (
            min_value is not None and max_value is not None
        ), "Both min_value and max_value should be provided or None."

    def forward(self, inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]:
        """Compute the floored, unit-scaled duration between the two inputs.

        Args:
            inputs: Mapping from column name to tensor; must contain both
                columns in ``input_cols``.

        Returns:
            A single-key mapping from ``output_cols[0]`` to the resulting
            duration tensor.
        """
        target_tensor = inputs[self.input_cols[0]]
        source_tensor = inputs[self.input_cols[1]]
        target_tensor = (
            torch.reshape(target_tensor, self.target_shape)
            if self.target_shape is not None
            else target_tensor
        )
        source_tensor = (
            torch.reshape(source_tensor, self.source_shape)
            if self.source_shape is not None
            else source_tensor
        )
        # Ensure computation in float for division and floor.
        diff = (
            target_tensor.to(dtype=torch.float32)
            - source_tensor.to(dtype=torch.float32)
        ) / float(self.unit)
        floored = torch.floor(diff)
        scaled = torch.log1p(torch.abs(floored)) if self.log_scale else floored
        # Apply clipping after log scaling.
        if self.min_value is not None and self.max_value is not None:
            scaled = torch.clamp(scaled, min=self.min_value, max=self.max_value)
        return {self.output_cols[0]: scaled}
