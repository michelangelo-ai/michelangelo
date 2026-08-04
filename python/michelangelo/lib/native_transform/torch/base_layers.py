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

from michelangelo.lib.constants.sentinel import INT32_SENTINEL
from michelangelo.lib.native_transform.torch.constants import DEFAULT_EPSILON
from michelangelo.lib.native_transform.torch.utils import (
    format_inputs,
    format_outputs,
    generate_layer_name,
    initialize_dtype,
)

__all__ = [
    "CaseWhen",
    "Cast",
    "Ceil",
    "Clip",
    "Compare",
    "Concatenate",
    "Constant",
    "Divide",
    "Floor",
    "IdentityTransform",
    "LogTransform",
    "PadOrCrop1D",
    "Scale",
    "Stack",
    "Subtract",
    "TensorColFillNone",
    "Tile",
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


class TensorColFillNone(TorchTransformBaseLayer):
    """Replace missing (``None``) positions in each input column with a default.

    Missing values are detected from the runtime tensor dtype rather than a
    passed-in flag: ``NaN`` marks missing values in floating-point tensors, and
    the dtype's minimum value marks them in ``int32``/``int64`` tensors (the
    convention used when ingesting nullable integer columns).

    Args:
        input_cols: Column names of the input tensors.
        output_cols: Column names of the output tensors; must match the length of
            ``input_cols``.
        default_value: Value substituted for every detected missing position.
        **kwargs: Additional base-layer options (e.g. ``name``).

    Raises:
        ValueError: If ``input_cols`` and ``output_cols`` differ in length.
    """

    def __init__(
        self,
        input_cols: list[str],
        output_cols: list[str],
        default_value: int | float,
        **kwargs,
    ) -> None:
        """Initialize the TensorColFillNone layer.

        Args:
            input_cols: Column names of the input tensors.
            output_cols: Column names of the output tensors; must match the
                length of ``input_cols``.
            default_value: Value substituted for every detected missing position.
            **kwargs: Additional base-layer options (e.g. ``name``).

        Raises:
            ValueError: If ``input_cols`` and ``output_cols`` differ in length.
        """
        super().__init__(input_cols, output_cols, **kwargs)
        if len(input_cols) != len(output_cols):
            raise ValueError(
                "Input columns and output columns must have the same length."
            )
        self.default_value = default_value
        # Precompute integer minimums so ``torch.iinfo`` is not called inside the
        # TorchScript-traced forward pass.
        self.int32_min = torch.iinfo(torch.int32).min
        self.int64_min = torch.iinfo(torch.int64).min

    def forward(self, inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]:
        """Fill missing positions in each input column.

        Args:
            inputs: Mapping from column name to tensor.

        Returns:
            A mapping from each output column to its filled tensor.
        """
        stacked_input = format_inputs(self.input_cols, inputs)
        if stacked_input.dtype == torch.int32:
            # The int32 minimum encodes a missing value.
            condition = stacked_input == self.int32_min
        elif stacked_input.dtype == torch.int64:
            # The int64 minimum encodes a missing value.
            condition = stacked_input == self.int64_min
        else:
            # NaN encodes a missing value in floating-point tensors.
            condition = torch.isnan(stacked_input)

        stacked_output = torch.where(
            condition,
            torch.ones_like(stacked_input) * self.default_value,
            stacked_input,
        )
        outputs = format_outputs(self.output_cols, stacked_output)
        return outputs


class CaseWhen(TorchTransformBaseLayer):
    """Select values by condition, like a SQL ``CASE WHEN`` expression.

    Input columns are read as ``(condition, value)`` pairs, so
    ``len(input_cols)`` must be even. For each element, the value of the first
    pair whose condition is ``True`` is returned; if no condition matches,
    ``default_value`` is used. Later pairs take lower priority than earlier ones.

    Args:
        input_cols: Column names ordered as ``condition1, value1, condition2,
            value2, ...``.
        output_cols: Single-element list naming the result column.
        default_value: Value used where no condition matches. A scalar
            (``int``/``float``/``bool``) is broadcast to the value shape; a list
            or tensor is used as-is.
        **kwargs: Additional base-layer options (e.g. ``name``).

    Raises:
        ValueError: If ``input_cols`` does not contain an even number of columns.
    """

    def __init__(
        self,
        input_cols: list[str],
        output_cols: list[str],
        default_value: int | float | bool | list | torch.Tensor,
        **kwargs,
    ) -> None:
        """Initialize the CaseWhen layer.

        Args:
            input_cols: Column names ordered as ``condition1, value1,
                condition2, value2, ...``.
            output_cols: Single-element list naming the result column.
            default_value: Value used where no condition matches. A scalar is
                broadcast to the value shape; a list or tensor is used as-is.
            **kwargs: Additional base-layer options (e.g. ``name``).

        Raises:
            ValueError: If ``input_cols`` does not contain an even number of
                columns.
        """
        if len(input_cols) % 2 != 0:
            raise ValueError(
                "CaseWhen layer must have an even number of input columns "
                f"(condition-value pairs). Got {len(input_cols)} columns."
            )
        super().__init__(input_cols, output_cols, **kwargs)
        self.default_value = default_value

    def forward(self, inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]:
        """Resolve each element to the first matching value, else the default.

        Args:
            inputs: Mapping from column name to tensor.

        Returns:
            A single-entry mapping from the output column to the resolved tensor.
        """
        if isinstance(self.default_value, (int, float, bool)):
            result = torch.ones_like(inputs[self.input_cols[1]]) * self.default_value
        else:
            # For a non-scalar default, allocate on the input's device via
            # ``new_empty`` and ``copy_`` the constant in. A literal
            # ``.to(input.device)`` would bake the trace-time device in.
            ref = inputs[self.input_cols[1]]
            src = torch.as_tensor(self.default_value)
            result = ref.new_empty(src.shape, dtype=src.dtype).copy_(src)

        # Iterate pairs in reverse so earlier conditions overwrite later ones and
        # thus win ties.
        for i in range(len(self.input_cols) - 2, -1, -2):
            condition = self.input_cols[i]
            value = self.input_cols[i + 1]
            result = torch.where(inputs[condition], inputs[value], result)

        return {self.output_cols[0]: result}


class Compare(TorchTransformBaseLayer):
    """Compare input columns pairwise with a named comparison operator.

    Input columns are read in ``(left, right)`` pairs (even indices are left
    operands, odd indices right operands), so ``len(input_cols)`` must be even
    and ``output_cols`` half its length. Each pair is compared element-wise and
    the boolean result is written to the corresponding output column.

    Args:
        input_cols: Column names as ``(left, right)`` pairs.
        output_cols: Column names of the boolean outputs.
        compare_op: One of ``"equal"``, ``"greater"``, ``"less"``,
            ``"greater_equal"``, ``"less_equal"``, or ``"not_equal"``.
        **kwargs: Additional base-layer options (e.g. ``name``).

    Raises:
        ValueError: If ``input_cols`` is not even or ``output_cols`` is not half
            its length, or if ``compare_op`` is not a supported operator.
    """

    def __init__(
        self,
        input_cols: list[str],
        output_cols: list[str],
        compare_op: str,
        **kwargs,
    ) -> None:
        """Initialize the Compare layer.

        Args:
            input_cols: Column names as ``(left, right)`` pairs.
            output_cols: Column names of the boolean outputs.
            compare_op: One of ``"equal"``, ``"greater"``, ``"less"``,
                ``"greater_equal"``, ``"less_equal"``, or ``"not_equal"``.
            **kwargs: Additional base-layer options (e.g. ``name``).

        Raises:
            ValueError: If ``input_cols`` is not even or ``output_cols`` is not
                half its length, or if ``compare_op`` is unsupported.
        """
        super().__init__(input_cols, output_cols, **kwargs)
        if (len(input_cols) % 2 != 0) or (len(input_cols) / 2 != len(output_cols)):
            raise ValueError(
                "Input columns must be even and output columns must be half of "
                "input columns."
            )
        if compare_op not in (
            "equal",
            "greater",
            "less",
            "greater_equal",
            "less_equal",
            "not_equal",
        ):
            raise ValueError(f"compare_op: {compare_op} is not supported")
        self.compare_op = compare_op

    def forward(self, inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]:
        """Compare each ``(left, right)`` pair element-wise.

        Args:
            inputs: Mapping from column name to tensor.

        Returns:
            A mapping from each output column to its boolean result tensor.
        """
        evens, odds = self.input_cols[0::2], self.input_cols[1::2]
        stacked_evens = format_inputs(evens, inputs)
        stacked_odds = format_inputs(odds, inputs)
        if self.compare_op == "equal":
            stacked_output = torch.eq(stacked_evens, stacked_odds)
        elif self.compare_op == "greater":
            stacked_output = torch.gt(stacked_evens, stacked_odds)
        elif self.compare_op == "less":
            stacked_output = torch.lt(stacked_evens, stacked_odds)
        elif self.compare_op == "greater_equal":
            stacked_output = torch.ge(stacked_evens, stacked_odds)
        elif self.compare_op == "less_equal":
            stacked_output = torch.le(stacked_evens, stacked_odds)
        elif self.compare_op == "not_equal":
            stacked_output = torch.ne(stacked_evens, stacked_odds)
        else:  # pragma: no cover
            # Unreachable: ``compare_op`` is validated in ``__init__``. Kept so
            # TorchScript sees ``stacked_output`` defined on every path.
            stacked_output = torch.eq(stacked_evens, stacked_odds)
        outputs = format_outputs(self.output_cols, stacked_output)
        return outputs


class Tile(TorchTransformBaseLayer):
    """Repeat each input tensor along an axis a fixed or inferred number of times.

    The repeat count is either given explicitly via ``count`` or inferred from a
    target tensor's size along ``axis``. When inferred, the target tensor is the
    last input column and the source tensors are the remaining columns.

    As a convenience, when ``axis == 1`` and every source tensor is 1D, sources
    are unsqueezed to 2D first so tiling produces a ``(batch, count)`` result
    rather than a flat ``(batch * count,)`` tensor.

    Args:
        input_cols: Column names of the source tensors. When a target tensor is
            used to infer the count, it is the last column and the sources are
            the columns before it.
        output_cols: Column names of the tiled outputs.
        axis: Axis along which to tile (default ``0``). Negative values index
            from the end.
        count: Explicit number of repetitions. Takes precedence over
            ``target_tensor_provided`` when set.
        target_tensor_provided: When ``True`` and ``count`` is ``None``, infer
            the count from the last input column's size along ``axis``.
        **kwargs: Additional base-layer options (e.g. ``name``).

    Raises:
        ValueError: If neither ``count`` is set nor ``target_tensor_provided``
            is ``True``.
    """

    def __init__(
        self,
        input_cols: list[str],
        output_cols: list[str],
        axis: int = 0,
        count: int | None = None,
        target_tensor_provided: bool = False,
        **kwargs,
    ) -> None:
        """Initialize the Tile layer.

        Args:
            input_cols: Column names of the source tensors (and, when inferring
                the count, a trailing target column).
            output_cols: Column names of the tiled outputs.
            axis: Axis along which to tile (default ``0``).
            count: Explicit number of repetitions; takes precedence over
                ``target_tensor_provided`` when set.
            target_tensor_provided: When ``True`` and ``count`` is ``None``,
                infer the count from the last input column's size along ``axis``.
            **kwargs: Additional base-layer options (e.g. ``name``).

        Raises:
            ValueError: If neither ``count`` is set nor
                ``target_tensor_provided`` is ``True``.
        """
        super().__init__(input_cols, output_cols, **kwargs)
        if count is None and not target_tensor_provided:
            raise ValueError(
                "Either count must be specified or target_tensor_provided must be True."
            )
        self.axis = axis
        self.count = count
        self.target_tensor_provided = target_tensor_provided

    def forward(self, inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]:
        """Tile each source column along ``axis``.

        Args:
            inputs: Mapping from column name to tensor.

        Returns:
            A mapping from each output column to its tiled tensor.

        Raises:
            ValueError: If neither ``count`` is set nor
                ``target_tensor_provided`` is ``True``.
        """
        if self.count is not None:
            source_cols = self.input_cols
            target_tensor: torch.Tensor | None = None
        elif self.target_tensor_provided:
            source_cols = self.input_cols[:-1]
            target_tensor = inputs[self.input_cols[-1]]
        else:  # pragma: no cover
            # Unreachable: validated in ``__init__``. Kept so TorchScript sees
            # every branch bind ``source_cols`` and ``target_tensor``.
            raise ValueError(
                "Either count must be specified or target_tensor_provided must be True."
            )

        # For axis=1 with all-1D sources, unsqueeze to 2D so the result is
        # ``(batch, count)`` rather than a flattened ``(batch * count,)``.
        sources = [inputs[col] for col in source_cols]
        if self.axis == 1 and all(s.dim() == 1 for s in sources):
            inputs = {col: inputs[col].unsqueeze(-1) for col in source_cols}
            if target_tensor is not None:
                inputs[self.input_cols[-1]] = target_tensor

        stacked_source = format_inputs(source_cols, inputs)

        # ``format_inputs`` prepends a stacking dimension, so a non-negative
        # ``axis`` shifts by one; a negative ``axis`` stays relative to the end.
        stack_axis = self.axis + 1 if self.axis >= 0 else self.axis

        multiples = [1] * len(stacked_source.shape)
        if self.count is not None:
            multiples[stack_axis] = self.count
        elif target_tensor is not None:
            multiples[stack_axis] = target_tensor.shape[self.axis]
        else:  # pragma: no cover
            # Unreachable given the guard above; keeps TorchScript's control flow
            # total.
            multiples[stack_axis] = 1

        tiled_stacked = torch.tile(stacked_source, multiples)
        return format_outputs(self.output_cols, tiled_stacked)


class PadOrCrop1D(TorchTransformBaseLayer):
    """Pad or crop each 1D input column to a fixed length.

    Each input column is normalized to exactly ``max_length`` along its last
    dimension: shorter sequences are padded with ``pad_value`` and longer ones
    are cropped. ``align`` controls which end is kept and padded.

    Sentinel positions from upstream ragged-batch collation (``NaN`` for float
    dtypes, ``INT32_SENTINEL`` for integer dtypes) are automatically replaced
    with ``pad_value`` before the pad/crop logic runs.

    Args:
        input_cols: Column names of the input tensors.
        output_cols: Column names of the output tensors; must match the length of
            ``input_cols``.
        max_length: The fixed target length; must be positive.
        dtype: Optional output dtype. When ``None``, the input dtype is
            preserved.
        pad_value: The value used for padding (default ``0``).
        align: ``"left"`` (default) pads on the right and keeps the first
            ``max_length`` elements; ``"right"`` pads on the left and keeps the
            last ``max_length`` elements of the real content, i.e. sentinel
            positions trailing the data are stripped before the crop rather
            than being cropped to. Retained elements keep their original order.
        **kwargs: Additional base-layer options (e.g. ``name``).

    Raises:
        ValueError: If ``input_cols`` and ``output_cols`` differ in length, if
            ``align`` is not ``"left"`` or ``"right"``, or if ``max_length`` is
            not positive.
    """

    def __init__(
        self,
        input_cols: list[str],
        output_cols: list[str],
        max_length: int,
        dtype: torch.dtype | str | None = None,
        pad_value: int | float = 0,
        align: str = "left",
        **kwargs,
    ) -> None:
        """Initialize the PadOrCrop1D layer.

        Args:
            input_cols: Column names of the input tensors.
            output_cols: Column names of the output tensors; must match the
                length of ``input_cols``.
            max_length: The fixed target length; must be positive.
            dtype: Optional output dtype; preserves input dtype when ``None``.
            pad_value: The value used for padding (default ``0``).
            align: ``"left"`` pads/crops on the right; ``"right"`` pads/crops on
                the left.
            **kwargs: Additional base-layer options (e.g. ``name``).

        Raises:
            ValueError: If ``input_cols`` and ``output_cols`` differ in length,
                if ``align`` is invalid, or if ``max_length`` is not positive.
        """
        super().__init__(input_cols, output_cols, **kwargs)
        if len(input_cols) != len(output_cols):
            raise ValueError(
                "Input columns and output columns must have the same length."
            )
        if align not in ("left", "right"):
            raise ValueError(f"align must be 'left' or 'right', got {align!r}")
        if max_length <= 0:
            raise ValueError(f"max_length must be a positive integer, got {max_length}")
        self.max_length = max_length
        self.dtype = initialize_dtype(dtype, None)
        # Store as float for TorchScript compatibility.
        self.pad_value = float(pad_value)
        self._int_sentinel = INT32_SENTINEL
        self.align = align

    def forward(self, inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]:
        """Normalize each input column to ``max_length``.

        Args:
            inputs: Mapping from column name to tensor.

        Returns:
            A mapping from each output column to its padded/cropped tensor.
        """
        stacked_input = format_inputs(self.input_cols, inputs)

        output_dtype = self.dtype if self.dtype is not None else stacked_input.dtype

        # Empty last dimension: return a full ``pad_value`` tensor of the target
        # length while preserving the batch dimensions.
        if stacked_input.size(-1) == 0:
            shape = list(stacked_input.shape)
            shape[-1] = self.max_length
            return format_outputs(
                self.output_cols,
                stacked_input.new_full(shape, self.pad_value, dtype=output_dtype),
            )

        # Positions holding upstream ragged padding rather than real data. The
        # mask is kept (not just applied) because ``align='right'`` must know
        # where the real content ends: once a sentinel is rewritten to
        # ``pad_value`` it is indistinguishable from padding, and cropping to the
        # "last max_length elements" would keep padding over real data.
        if stacked_input.is_floating_point():
            sentinel_mask = torch.isnan(stacked_input)
        else:
            sentinel = stacked_input.new_full((), self._int_sentinel)
            sentinel_mask = stacked_input == sentinel

        replacement = stacked_input.new_full((), self.pad_value)
        stacked_input = torch.where(sentinel_mask, replacement, stacked_input)

        if self.align == "right":
            # Gather the last ``max_length`` elements of the *real* content
            # (everything up to and including the last non-sentinel position),
            # left-padding when the content is shorter than ``max_length``.
            positions = torch.arange(
                stacked_input.size(-1), device=stacked_input.device
            )
            content_length = torch.where(
                sentinel_mask, torch.zeros_like(positions), positions + 1
            ).amax(dim=-1, keepdim=True)
            source_index = (
                torch.arange(self.max_length, device=stacked_input.device)
                + content_length
                - self.max_length
            )
            gathered = torch.gather(stacked_input, -1, source_index.clamp(min=0))
            output_tensor = torch.where(source_index >= 0, gathered, replacement).to(
                output_dtype
            )
        else:
            # ``align='left'`` pads on the right and keeps the first
            # ``max_length`` elements, which are always real content.
            padding = max(self.max_length - stacked_input.size(-1), 0)
            padded_tensor = torch.nn.functional.pad(
                stacked_input, (0, padding), value=self.pad_value
            )
            output_tensor = padded_tensor[..., : self.max_length].to(output_dtype)

        return format_outputs(self.output_cols, output_tensor)


class Scale(TorchTransformBaseLayer):
    """Multiply each input column by a scalar factor.

    Args:
        input_cols: Column names of the input tensors.
        output_cols: Column names of the output tensors; must match the length of
            ``input_cols``.
        factor: The scalar multiplier.
        **kwargs: Additional base-layer options (e.g. ``name``).

    Raises:
        ValueError: If ``input_cols`` and ``output_cols`` differ in length.
    """

    def __init__(
        self,
        input_cols: list[str],
        output_cols: list[str],
        factor: float,
        **kwargs,
    ) -> None:
        """Initialize the Scale layer.

        Args:
            input_cols: Column names of the input tensors.
            output_cols: Column names of the output tensors; must match the
                length of ``input_cols``.
            factor: The scalar multiplier.
            **kwargs: Additional base-layer options (e.g. ``name``).

        Raises:
            ValueError: If ``input_cols`` and ``output_cols`` differ in length.
        """
        super().__init__(input_cols, output_cols, **kwargs)
        if len(input_cols) != len(output_cols):
            raise ValueError(
                "Input columns and output columns must have the same length."
            )
        self.factor = factor

    def forward(self, inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]:
        """Scale each input column by ``factor``.

        Args:
            inputs: Mapping from column name to tensor.

        Returns:
            A mapping from each output column to its scaled tensor.
        """
        stacked_inputs = format_inputs(self.input_cols, inputs)
        output_tensor = stacked_inputs * self.factor
        return format_outputs(self.output_cols, output_tensor)


class Clip(TorchTransformBaseLayer):
    """Clamp each input column to a range, optionally exempting one value.

    Values are clamped to ``[min_value, max_value]``; a bound left as ``None`` is
    not enforced on that side. When ``ignore_value`` is set, positions equal to
    it are passed through unchanged even if they fall outside the range (useful
    for preserving a padding value such as ``-1.0``).

    Args:
        input_cols: Column names of the input tensors.
        output_cols: Column names of the output tensors; must match the length of
            ``input_cols``.
        min_value: Lower bound, or ``None`` for no lower bound.
        max_value: Upper bound, or ``None`` for no upper bound.
        ignore_value: Optional value that is preserved unchanged rather than
            clamped.
        **kwargs: Additional base-layer options (e.g. ``name``).

    Raises:
        ValueError: If ``input_cols`` and ``output_cols`` differ in length, or if
            both ``min_value`` and ``max_value`` are ``None``.
    """

    def __init__(
        self,
        input_cols: list[str],
        output_cols: list[str],
        min_value: float | None = None,
        max_value: float | None = None,
        ignore_value: float | None = None,
        **kwargs,
    ) -> None:
        """Initialize the Clip layer.

        Args:
            input_cols: Column names of the input tensors.
            output_cols: Column names of the output tensors; must match the
                length of ``input_cols``.
            min_value: Lower bound, or ``None`` for no lower bound.
            max_value: Upper bound, or ``None`` for no upper bound.
            ignore_value: Optional value preserved unchanged rather than clamped.
            **kwargs: Additional base-layer options (e.g. ``name``).

        Raises:
            ValueError: If ``input_cols`` and ``output_cols`` differ in length,
                or if both ``min_value`` and ``max_value`` are ``None``.
        """
        super().__init__(input_cols, output_cols, **kwargs)
        if len(input_cols) != len(output_cols):
            raise ValueError(
                "Input columns and output columns must have the same length."
            )
        if min_value is None and max_value is None:
            raise ValueError("At least one of min_value or max_value must not be None.")
        self.min_value = min_value
        self.max_value = max_value
        self.ignore_value = ignore_value

    def forward(self, inputs: dict[str, torch.Tensor]) -> dict[str, torch.Tensor]:
        """Clamp each input column, preserving ``ignore_value`` positions.

        Args:
            inputs: Mapping from column name to tensor.

        Returns:
            A mapping from each output column to its clamped tensor.
        """
        stacked_inputs = format_inputs(self.input_cols, inputs)

        clipped_tensor = torch.clamp(
            stacked_inputs, min=self.min_value, max=self.max_value
        )

        # Restore positions equal to ``ignore_value`` that clamping may have
        # moved (e.g. a padding value outside the range).
        if self.ignore_value is not None:
            mask = torch.isclose(
                stacked_inputs, stacked_inputs.new_full((), self.ignore_value)
            )
            clipped_tensor = torch.where(mask, stacked_inputs, clipped_tensor)

        return format_outputs(self.output_cols, clipped_tensor)
