"""PyTorch native transform layers.

TorchScript- and ONNX-exportable ``nn.Module`` transform layers that operate on a
``dict[str, torch.Tensor]`` in/out contract so the exact same transform runs at
train and serve time. Every layer subclasses :class:`TorchTransformBaseLayer` and
uses :func:`~michelangelo.lib.native_transform.torch.utils.format_inputs` /
:func:`~michelangelo.lib.native_transform.torch.utils.format_outputs` to map its
declared input/output columns to and from a single stacked tensor.

This module provides the foundation (stateless, elementwise) layers. Structural,
fitted-statistics, and tokenizer layers are added in follow-up modules.
"""

from __future__ import annotations

import abc

import torch

from michelangelo.lib.native_transform.torch.constants import DEFAULT_EPSILON
from michelangelo.lib.native_transform.torch.utils import (
    format_inputs,
    format_outputs,
    generate_layer_name,
    initialize_dtype,
)

__all__ = [
    "Cast",
    "Ceil",
    "Concatenate",
    "Constant",
    "Divide",
    "Floor",
    "IdentityTransform",
    "LogTransform",
    "Stack",
    "Subtract",
    "TorchTransformBaseLayer",
]


class TorchTransformBaseLayer(torch.nn.Module, abc.ABC):
    """Abstract base for native PyTorch transform layers.

    All layers consume and produce ``dict[str, torch.Tensor]`` so they compose
    into a single TorchScript-exportable graph. Subclasses select their inputs by
    ``input_cols`` and write their results under ``output_cols``.

    Args:
        input_cols: Column names of the input tensors.
        output_cols: Column names of the output tensors.
        **kwargs: Additional options. ``name`` (str) sets the layer name, which
            must be unique within a model. When omitted, a unique name is
            generated automatically from the layer's class name (e.g.
            ``"stack_A1B2C3D4E5"``).
    """

    def __init__(self, input_cols: list[str], output_cols: list[str], **kwargs) -> None:
        """Initialize the base layer.

        Args:
            input_cols: Column names of the input tensors.
            output_cols: Column names of the output tensors.
            **kwargs: Additional options; ``name`` (str) sets the layer name. When
                omitted, a unique name is generated from the class name.
        """
        super().__init__()
        self.input_cols = input_cols
        self.output_cols = output_cols
        name = kwargs.get("name")
        self.name = (
            name if name is not None else generate_layer_name(self.__class__.__name__)
        )

    @abc.abstractmethod
    def forward(self, inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]:
        """Apply the transform.

        Args:
            inputs: Mapping from column name to tensor for at least every column
                in ``input_cols``.

        Returns:
            A mapping from each column in ``output_cols`` to its result tensor.

        Raises:
            NotImplementedError: If a subclass does not override this method.
        """
        raise NotImplementedError("Please implement the method in your subclass.")


class Concatenate(TorchTransformBaseLayer):
    """Concatenate input tensors along the last dimension.

    When ``dtype`` is ``None`` (default) the output dtype follows torch's
    standard type-promotion rules (e.g. ``int32`` + ``float64`` -> ``float64``).
    When ``dtype`` is given, the output is explicitly cast to it.

    Args:
        input_cols: Column names of the input tensors.
        output_cols: Single-element list naming the concatenated output column.
        dtype: Optional output dtype. When ``None``, the input dtype is
            preserved via type promotion.
        **kwargs: Additional base-layer options (e.g. ``name``).
    """

    def __init__(
        self,
        input_cols: list[str],
        output_cols: list[str],
        dtype: torch.dtype | str | None = None,
        **kwargs,
    ) -> None:
        """Initialize the Concatenate layer.

        Args:
            input_cols: Column names of the input tensors.
            output_cols: Single-element list naming the concatenated output.
            dtype: Optional output dtype; when ``None``, preserves input dtype.
            **kwargs: Additional base-layer options (e.g. ``name``).
        """
        super().__init__(input_cols, output_cols, **kwargs)
        self.dtype = initialize_dtype(dtype, None)

    def forward(self, inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]:
        """Concatenate the input columns along the last dimension.

        Args:
            inputs: Mapping from column name to tensor.

        Returns:
            A single-entry mapping from the output column to the concatenated
            tensor, cast to ``dtype`` when one was provided.
        """
        tensors: list[torch.Tensor] = []
        for in_col in self.input_cols:
            input_tensor = inputs[in_col]
            tensors.append(input_tensor)
        concatenated = torch.cat(tensors, dim=-1)
        if self.dtype is not None:
            concatenated = concatenated.to(self.dtype)
        return {self.output_cols[0]: concatenated}


class Stack(TorchTransformBaseLayer):
    """Stack input tensors along a new dimension.

    Inputs are cast to ``float32`` before stacking. For ``N`` input tensors each
    of shape ``(B, L)``, the output has shape ``(B, L, N)`` when ``dim=-1`` or
    ``(B, N, L)`` when ``dim=1``.

    Args:
        input_cols: Column names of the input tensors.
        output_cols: Single-element list naming the stacked output column.
        dim: The dimension along which to stack (default ``-1``).
        **kwargs: Additional base-layer options (e.g. ``name``).
    """

    def __init__(
        self,
        input_cols: list[str],
        output_cols: list[str],
        dim: int = -1,
        **kwargs,
    ) -> None:
        """Initialize the Stack layer.

        Args:
            input_cols: Column names of the input tensors.
            output_cols: Single-element list naming the stacked output column.
            dim: The new dimension along which to stack (default ``-1``).
            **kwargs: Additional base-layer options (e.g. ``name``).
        """
        super().__init__(input_cols, output_cols, **kwargs)
        self.dim: int = dim

    def forward(self, inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]:
        """Stack the input columns along ``dim``.

        Args:
            inputs: Mapping from column name to tensor.

        Returns:
            A single-entry mapping from the output column to the stacked tensor.
        """
        tensors: list[torch.Tensor] = []
        for in_col in self.input_cols:
            input_tensor = inputs[in_col]
            tensors.append(input_tensor.to(torch.float32))
        return {self.output_cols[0]: torch.stack(tensors, dim=self.dim)}


class Cast(TorchTransformBaseLayer):
    """Cast input tensors to a target dtype.

    Args:
        input_cols: Column names of the input tensors.
        output_cols: Column names of the output tensors; must match the length
            of ``input_cols``.
        dtype: Target dtype to cast to. May be a ``torch.dtype`` or a string
            alias (e.g. ``"float32"`` or ``"torch.float32"``). Defaults to
            ``torch.int64`` when ``None``. An unrecognized string alias raises
            ``ValueError``.
        **kwargs: Additional base-layer options (e.g. ``name``).

    Raises:
        ValueError: If ``input_cols`` and ``output_cols`` differ in length, or if
            ``dtype`` is a string that names no recognized dtype.
    """

    def __init__(
        self,
        input_cols: list[str],
        output_cols: list[str],
        dtype: torch.dtype | str | None = None,
        **kwargs,
    ) -> None:
        """Initialize the Cast layer.

        Args:
            input_cols: Column names of the input tensors.
            output_cols: Column names of the output tensors; must match the
                length of ``input_cols``.
            dtype: Target dtype (``torch.dtype`` or string alias); defaults to
                ``torch.int64`` when ``None``.
            **kwargs: Additional base-layer options (e.g. ``name``).

        Raises:
            ValueError: If ``input_cols`` and ``output_cols`` differ in length,
                or if ``dtype`` is a string that names no recognized dtype.
        """
        if len(input_cols) != len(output_cols):
            raise ValueError(
                "Input columns and output columns must have the same length."
            )
        super().__init__(input_cols, output_cols, **kwargs)
        self.dtype = initialize_dtype(dtype, torch.int64)

    def forward(self, inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]:
        """Cast each input column to ``dtype``.

        Args:
            inputs: Mapping from column name to tensor.

        Returns:
            A mapping from each output column to its cast tensor.
        """
        stacked_input = format_inputs(self.input_cols, inputs)
        stacked_output = stacked_input.to(self.dtype)
        outputs = format_outputs(self.output_cols, stacked_output)
        return outputs


class Constant(TorchTransformBaseLayer):
    """Produce a constant tensor shaped like the input.

    Useful for migrating conditional expressions (``if (cond) {...} else {...}``)
    whose branches return constants: the constant is materialized as a tensor
    matching the reference input's shape.

    Args:
        input_cols: Column names of the input tensors, used only for shape
            reference; must match the length of ``output_cols``.
        output_cols: Column names of the output tensors.
        constant: The value to fill the output tensor with.
        dtype: Output dtype. When ``None``, it is inferred from ``constant``.
        **kwargs: Additional base-layer options (e.g. ``name``).

    Raises:
        ValueError: If ``input_cols`` and ``output_cols`` differ in length, or if
            ``input_cols`` is empty (no shape reference available).
    """

    def __init__(
        self,
        input_cols: list[str],
        output_cols: list[str],
        constant: int | float | bool,
        dtype: torch.dtype | str | None = None,
        **kwargs,
    ) -> None:
        """Initialize the Constant layer.

        Args:
            input_cols: Column names used for shape reference; must match the
                length of ``output_cols`` and be non-empty.
            output_cols: Column names of the output tensors.
            constant: The value to fill the output tensor with.
            dtype: Output dtype; inferred from ``constant`` when ``None``.
            **kwargs: Additional base-layer options (e.g. ``name``).

        Raises:
            ValueError: If ``input_cols`` and ``output_cols`` differ in length,
                or if ``input_cols`` is empty.
        """
        super().__init__(input_cols, output_cols, **kwargs)
        if len(input_cols) != len(output_cols):
            raise ValueError(
                "Input columns and output columns must have the same length."
            )
        if not self.input_cols:
            raise ValueError(
                "Constant requires at least one input column for shape reference."
            )
        self.constant = constant
        self.dtype = initialize_dtype(dtype, torch.tensor(self.constant).dtype)

    def forward(self, inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]:
        """Create a constant tensor matching the input's shape.

        Args:
            inputs: Mapping from column name to tensor.

        Returns:
            A mapping from each output column to a constant-filled tensor.
        """
        stacked_inputs = format_inputs(self.input_cols, inputs)
        shape = stacked_inputs.shape

        # ``new_full`` inherits the input's device dynamically. Under
        # ``torch.jit.trace`` a literal ``torch.tensor(...).to(device)`` would
        # bake the trace-time device in and break inference after ``.to("cuda")``.
        stacked_outputs = stacked_inputs.new_full(
            shape, self.constant, dtype=self.dtype
        )
        outputs = format_outputs(self.output_cols, stacked_outputs)
        return outputs


class Divide(TorchTransformBaseLayer):
    """Divide input columns pairwise, element-wise, with zero-safe handling.

    Input columns are read in ``(numerator, denominator)`` pairs (even indices
    are numerators, odd indices denominators), so ``len(input_cols)`` must be
    even and ``output_cols`` half its length. Both operands are upcast to
    ``float64`` before division. A zero denominator is replaced with ``eps`` to
    avoid division by zero; when both operands are zero the result is forced to
    ``0``.

    Args:
        input_cols: Column names as ``(numerator, denominator)`` pairs.
        output_cols: Column names of the quotient outputs.
        add_constant_to_divisor: Constant added to every denominator before
            division.
        eps: Small value substituted for a zero denominator to avoid division by
            zero (default
            :data:`~michelangelo.lib.native_transform.torch.constants.DEFAULT_EPSILON`).
        **kwargs: Additional base-layer options (e.g. ``name``).

    Raises:
        ValueError: If ``input_cols`` is not even, or ``output_cols`` is not half
            its length.
    """

    def __init__(
        self,
        input_cols: list[str],
        output_cols: list[str],
        add_constant_to_divisor: float = 0.0,
        eps: float = DEFAULT_EPSILON,
        **kwargs,
    ) -> None:
        """Initialize the Divide layer.

        Args:
            input_cols: Column names as ``(numerator, denominator)`` pairs.
            output_cols: Column names of the quotient outputs.
            add_constant_to_divisor: Constant added to every denominator.
            eps: Small value substituted for a zero denominator (default
                :data:`~michelangelo.lib.native_transform.torch.constants.DEFAULT_EPSILON`).
            **kwargs: Additional base-layer options (e.g. ``name``).

        Raises:
            ValueError: If ``input_cols`` is not even, or ``output_cols`` is not
                half its length.
        """
        super().__init__(input_cols, output_cols, **kwargs)
        if (len(input_cols) % 2 != 0) or (len(input_cols) / 2 != len(output_cols)):
            raise ValueError(
                "Input columns must be even and output columns must be half of "
                "input columns."
            )
        self.add_constant_to_divisor = add_constant_to_divisor
        self.eps = eps

    def forward(self, inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]:
        """Divide numerators by denominators, pairwise and zero-safe.

        Args:
            inputs: Mapping from column name to tensor.

        Returns:
            A mapping from each output column to its quotient tensor.
        """
        evens, odds = self.input_cols[0::2], self.input_cols[1::2]
        stacked_evens = format_inputs(evens, inputs).to(torch.float64)
        stacked_odds = format_inputs(odds, inputs).to(torch.float64)

        stacked_odds += self.add_constant_to_divisor

        safe_tensor2 = torch.where(
            stacked_odds == 0,
            torch.full_like(stacked_odds, self.eps),
            stacked_odds,
        )

        result_tensor = torch.div(stacked_evens, safe_tensor2)
        result_tensor = torch.where(
            (stacked_evens == 0) & (stacked_odds == 0),
            torch.zeros_like(result_tensor),
            result_tensor,
        )

        outputs = format_outputs(self.output_cols, result_tensor)
        return outputs


class LogTransform(TorchTransformBaseLayer):
    """Apply a logarithmic transform with an offset and output clamping.

    Computes ``log(x + add_constant)`` and clamps the result to ``[1.0, 1e20]``.

    Args:
        input_cols: Column names of the input tensors.
        output_cols: Column names of the output tensors; must match the length of
            ``input_cols``.
        add_constant: Value added before the logarithm to avoid ``log(0)``
            (default ``1.0``).
        **kwargs: Additional base-layer options (e.g. ``name``).

    Raises:
        ValueError: If ``input_cols`` and ``output_cols`` differ in length.
    """

    def __init__(
        self,
        input_cols: list[str],
        output_cols: list[str],
        add_constant: float = 1.0,
        **kwargs,
    ) -> None:
        """Initialize the LogTransform layer.

        Args:
            input_cols: Column names of the input tensors.
            output_cols: Column names of the output tensors; must match the
                length of ``input_cols``.
            add_constant: Value added before the logarithm (default ``1.0``).
            **kwargs: Additional base-layer options (e.g. ``name``).

        Raises:
            ValueError: If ``input_cols`` and ``output_cols`` differ in length.
        """
        super().__init__(input_cols, output_cols, **kwargs)
        if len(input_cols) != len(output_cols):
            raise ValueError(
                "Input columns and output columns must have the same length."
            )
        self.add_constant = add_constant

    def forward(self, inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]:
        """Apply the log transform to each input column.

        Args:
            inputs: Mapping from column name to tensor.

        Returns:
            A mapping from each output column to its transformed, clamped tensor.
        """
        stacked_inputs = format_inputs(self.input_cols, inputs)
        stacked_outputs = torch.log(stacked_inputs + self.add_constant)
        stacked_outputs = torch.clamp(stacked_outputs, min=1.0, max=1e20)
        return format_outputs(self.output_cols, stacked_outputs)


class Subtract(TorchTransformBaseLayer):
    """Subtract input columns pairwise, element-wise.

    Input columns are read in ``(minuend, subtrahend)`` pairs (even indices are
    minuends, odd indices subtrahends), so ``len(input_cols)`` must be even and
    ``output_cols`` half its length. Both operands are upcast to ``float64``
    before subtraction.

    Args:
        input_cols: Column names as ``(minuend, subtrahend)`` pairs.
        output_cols: Column names of the difference outputs.
        **kwargs: Additional base-layer options (e.g. ``name``).

    Raises:
        ValueError: If ``input_cols`` is not even, or ``output_cols`` is not half
            its length.
    """

    def __init__(self, input_cols: list[str], output_cols: list[str], **kwargs) -> None:
        """Initialize the Subtract layer.

        Args:
            input_cols: Column names as ``(minuend, subtrahend)`` pairs.
            output_cols: Column names of the difference outputs.
            **kwargs: Additional base-layer options (e.g. ``name``).

        Raises:
            ValueError: If ``input_cols`` is not even, or ``output_cols`` is not
                half its length.
        """
        super().__init__(input_cols, output_cols, **kwargs)
        if (len(input_cols) % 2 != 0) or (len(input_cols) / 2 != len(output_cols)):
            raise ValueError(
                "Input columns must be even and output columns must be half of "
                "input columns."
            )

    def forward(self, inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]:
        """Subtract subtrahends from minuends, pairwise.

        Args:
            inputs: Mapping from column name to tensor.

        Returns:
            A mapping from each output column to its difference tensor.
        """
        evens, odds = self.input_cols[0::2], self.input_cols[1::2]
        stacked_evens = format_inputs(evens, inputs).to(torch.float64)
        stacked_odds = format_inputs(odds, inputs).to(torch.float64)

        result_tensor = torch.sub(stacked_evens, stacked_odds)

        outputs = format_outputs(self.output_cols, result_tensor)
        return outputs


class Floor(TorchTransformBaseLayer):
    """Apply an element-wise floor to input columns.

    Args:
        input_cols: Column names of the input tensors.
        output_cols: Column names of the output tensors; must match the length of
            ``input_cols``.
        **kwargs: Additional base-layer options (e.g. ``name``).

    Raises:
        ValueError: If ``input_cols`` and ``output_cols`` differ in length.
    """

    def __init__(self, input_cols: list[str], output_cols: list[str], **kwargs) -> None:
        """Initialize the Floor layer.

        Args:
            input_cols: Column names of the input tensors.
            output_cols: Column names of the output tensors; must match the
                length of ``input_cols``.
            **kwargs: Additional base-layer options (e.g. ``name``).

        Raises:
            ValueError: If ``input_cols`` and ``output_cols`` differ in length.
        """
        super().__init__(input_cols, output_cols, **kwargs)
        if len(input_cols) != len(output_cols):
            raise ValueError(
                "Input columns and output columns must have the same length."
            )

    def forward(self, inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]:
        """Apply floor to each input column.

        Args:
            inputs: Mapping from column name to tensor.

        Returns:
            A mapping from each output column to its floored tensor.
        """
        stacked_inputs = format_inputs(self.input_cols, inputs)
        output_tensor = torch.floor(stacked_inputs)
        return format_outputs(self.output_cols, output_tensor)


class Ceil(TorchTransformBaseLayer):
    """Apply an element-wise ceiling to input columns.

    Args:
        input_cols: Column names of the input tensors.
        output_cols: Column names of the output tensors; must match the length of
            ``input_cols``.
        **kwargs: Additional base-layer options (e.g. ``name``).

    Raises:
        ValueError: If ``input_cols`` and ``output_cols`` differ in length.
    """

    def __init__(self, input_cols: list[str], output_cols: list[str], **kwargs) -> None:
        """Initialize the Ceil layer.

        Args:
            input_cols: Column names of the input tensors.
            output_cols: Column names of the output tensors; must match the
                length of ``input_cols``.
            **kwargs: Additional base-layer options (e.g. ``name``).

        Raises:
            ValueError: If ``input_cols`` and ``output_cols`` differ in length.
        """
        super().__init__(input_cols, output_cols, **kwargs)
        if len(input_cols) != len(output_cols):
            raise ValueError(
                "Input columns and output columns must have the same length."
            )

    def forward(self, inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]:
        """Apply ceiling to each input column.

        Args:
            inputs: Mapping from column name to tensor.

        Returns:
            A mapping from each output column to its ceiled tensor.
        """
        stacked_inputs = format_inputs(self.input_cols, inputs)
        output_tensor = torch.ceil(stacked_inputs)
        return format_outputs(self.output_cols, output_tensor)


class IdentityTransform(TorchTransformBaseLayer):
    """Pass input tensors through unchanged.

    Explicitly includes fields in a native transform's input schema without
    modifying them — useful for bypass fields that downstream model assembly
    needs available.

    Args:
        input_cols: Column names of the input tensors.
        output_cols: Column names of the output tensors; must match the length of
            ``input_cols``.
        **kwargs: Additional base-layer options (e.g. ``name``).

    Raises:
        ValueError: If ``input_cols`` and ``output_cols`` differ in length.
    """

    def __init__(self, input_cols: list[str], output_cols: list[str], **kwargs) -> None:
        """Initialize the IdentityTransform layer.

        Args:
            input_cols: Column names of the input tensors.
            output_cols: Column names of the output tensors; must match the
                length of ``input_cols``.
            **kwargs: Additional base-layer options (e.g. ``name``).

        Raises:
            ValueError: If ``input_cols`` and ``output_cols`` differ in length.
        """
        super().__init__(input_cols, output_cols, **kwargs)
        if len(input_cols) != len(output_cols):
            raise ValueError(
                "Input columns and output columns must have the same length."
            )

    def forward(self, inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]:
        """Pass each input column through unchanged.

        Args:
            inputs: Mapping from column name to tensor.

        Returns:
            A mapping from each output column to the corresponding input tensor.
        """
        stacked_inputs = format_inputs(self.input_cols, inputs)
        return format_outputs(self.output_cols, stacked_inputs)
