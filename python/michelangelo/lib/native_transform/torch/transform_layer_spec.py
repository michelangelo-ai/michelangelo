"""Pydantic specs for native transform layers.

Declarative, serializable descriptions of the individual layers in a
``TransformSpec`` DAG (the DAG engine and layer-name-to-spec-class registry
that consume these specs are added in a follow-up module). Every concrete
``*LayerSpec`` subclasses :class:`TorchTransformLayerSpec`, which carries the
fields common to every native transform layer (input/output columns, dtypes,
shape, training-feature flags, and incremental-training mode).

A handful of specs (e.g. :class:`StandardScalerLayerSpec`,
:class:`MinMaxScalerLayerSpec`, :class:`PercentileBucketizationLayerSpec`) are
placeholders: they describe a layer configuration that is resolved to a
concrete layer spec (named in ``_resolved_layer_type``) once fitted
statistics are computed, rather than mapping directly to a transform layer
class themselves.
"""

from __future__ import annotations

from enum import Enum
from typing import ClassVar

import torch
from pydantic import BaseModel, Field, field_validator, model_validator

from michelangelo.lib.native_transform.torch.constants import (
    DEFAULT_NUMERICAL_OUTPUT_DTYPE,
)
from michelangelo.lib.native_transform.torch.utils import generate_layer_name

__all__ = [
    "BucketizationLayerSpec",
    "CaseWhenLayerSpec",
    "CastLayerSpec",
    "CeilLayerSpec",
    "ClipLayerSpec",
    "CompareLayerSpec",
    "ConcatenateLayerSpec",
    "ConstantLayerSpec",
    "DivideLayerSpec",
    "FloorLayerSpec",
    "IDHashTokenizerLayerSpec",
    "IdentityTransformLayerSpec",
    "LogTransformLayerSpec",
    "MinMaxLayerSpec",
    "MinMaxScalerLayerSpec",
    "NormalizationLayerSpec",
    "NumericalStandardTransformLayerSpec",
    "PadOrCrop1DLayerSpec",
    "PercentileBucketizationLayerSpec",
    "ScaleLayerSpec",
    "StackLayerSpec",
    "StandardScalerLayerSpec",
    "SubtractLayerSpec",
    "TensorColFillNoneLayerSpec",
    "TileLayerSpec",
    "TorchTransformLayerSpec",
    "TransformerMode",
]


class TransformerMode(str, Enum):
    """Per-layer mode controlling behavior during incremental training.

    Attributes:
        INVALID: Not set by the user; defaults to ``REUSE`` when incremental
            training is active.
        REFIT: Compute fitted statistics from the current data (a fresh fit).
        REUSE: Reuse the base model's pre-fitted statistics as-is.
        INC_REFIT: Placeholder for a future incremental-refit mode; currently
            unsupported (schema-only).
    """

    INVALID = "INVALID"
    REFIT = "REFIT"
    REUSE = "REUSE"
    INC_REFIT = "INC_REFIT"


class TorchTransformLayerSpec(BaseModel, arbitrary_types_allowed=True):
    """Base configuration shared by every native transform layer spec.

    Args:
        name: The layer's name. Auto-generated from the spec's class name
            when omitted.
        input_cols: Column names of the layer's input tensors.
        output_cols: Column names of the layer's output tensors.
        input_dtype: Optional input dtype override.
        output_dtype: Optional output dtype override.
        input_shape: Static per-sample input shape (excluding the batch
            dimension) for each input column. For ragged arrays, set the
            fixed max length used to package the served model.
        is_training_features: Whether each output column is a training
            feature. Defaults to ``True`` for every output column when
            omitted.
        mode: The per-layer incremental-training mode.
    """

    # Declared as ClassVar (not a pydantic field) so it isn't treated as
    # model data; pydantic v2 checks ClassVar annotations before its
    # leading-underscore private-attribute inference, so this is not
    # swallowed into PrivateAttr handling.
    _resolved_layer_type: ClassVar[str | None] = None

    name: str = Field(..., description="Name")
    input_cols: list[str] = Field(..., description="Input columns")
    output_cols: list[str] = Field(..., description="Output columns")
    input_dtype: torch.dtype | str | None = Field(None, description="Input dtype")
    output_dtype: torch.dtype | str | None = Field(None, description="Output dtype")
    input_shape: list[int] | None = Field(
        None,
        description=(
            "Static per-sample input shape (no batch dim) for each input "
            "col; for ragged arrays set the fixed max length used to "
            "package the served model."
        ),
    )
    is_training_features: list[bool] | None = Field(
        None, description="Is training features"
    )
    mode: TransformerMode = Field(
        TransformerMode.INVALID, description="Per-layer incremental training mode"
    )

    @model_validator(mode="before")
    @classmethod
    def set_defaults(cls, values: dict) -> dict:
        """Default ``is_training_features`` and auto-generate ``name``.

        Args:
            values: The raw field values supplied to the model.

        Returns:
            ``values``, with ``is_training_features`` defaulted to
            ``[True] * len(output_cols)`` and ``name`` auto-generated from
            the spec's class name when either was omitted.
        """
        output_cols = values.get("output_cols")
        is_training_features = values.get("is_training_features")
        if is_training_features is None and output_cols is not None:
            values["is_training_features"] = [True] * len(output_cols)
        if values.get("name") is None:
            values["name"] = generate_layer_name(
                cls.__name__.replace("LayerSpec", "").lower()
            )
        return values


class NormalizationLayerSpec(TorchTransformLayerSpec):
    """Spec for the ``Normalization`` layer (a fitted ``StandardScaler``).

    Args:
        mean: The per-feature mean values used for standardization.
        std: The per-feature standard deviation values used for
            standardization.
        dim: The dimension along which input columns are concatenated.
    """

    mean: list[float] = Field([0.0], description="Mean")
    std: list[float] = Field([1.0], description="Std")
    dim: int = Field(
        -1, description="Dimension along which input columns are concatenated"
    )


class MinMaxLayerSpec(TorchTransformLayerSpec):
    """Spec for the ``MinMax`` layer (a fitted ``MinMaxScaler``).

    Args:
        min: The per-feature minimum values used for scaling.
        max: The per-feature maximum values used for scaling.
        dim: The dimension along which input columns are concatenated.
    """

    min: list[float] = Field([0.0], description="Min value")
    max: list[float] = Field([1.0], description="Max value")
    dim: int = Field(
        -1, description="Dimension along which input columns are concatenated"
    )


class StandardScalerLayerSpec(TorchTransformLayerSpec):
    """Placeholder resolved to :class:`NormalizationLayerSpec` after fitting.

    Args:
        with_mean: Whether to center the data by subtracting the mean.
        with_std: Whether to scale the data by the standard deviation.
    """

    _resolved_layer_type: ClassVar[str | None] = "NormalizationLayerSpec"

    with_mean: bool = Field(
        True, description="Whether to center the data by subtracting the mean"
    )
    with_std: bool = Field(
        False, description="Whether to scale the data by the standard deviation"
    )


class MinMaxScalerLayerSpec(TorchTransformLayerSpec):
    """Placeholder resolved to :class:`MinMaxLayerSpec` after fitting."""

    _resolved_layer_type: ClassVar[str | None] = "MinMaxLayerSpec"


class ConcatenateLayerSpec(TorchTransformLayerSpec):
    """Spec for the ``Concatenate`` layer.

    Args:
        dtype: Optional output dtype. When ``None``, the input dtype is
            preserved.
    """

    dtype: torch.dtype | str | None = Field(
        None, description="Optional output dtype. When None, preserves input dtype."
    )


class StackLayerSpec(TorchTransformLayerSpec):
    """Spec for the ``Stack`` layer.

    Args:
        dim: The dimension along which to stack tensors.
    """

    dim: int = Field(-1, description="Dimension along which to stack tensors")


class NumericalStandardTransformLayerSpec(TorchTransformLayerSpec):
    """Spec for a numerical standardization transform with capping and log scaling.

    Args:
        scale_factor: The scale factor applied to the transformed value.
        default_value: The value substituted for missing data, or a string
            in ``"0.0"``-``"0.99"`` to select a percentile instead.
        cap_min: The minimum cap value, or a percentile string as above.
        cap_max: The maximum cap value, or a percentile string as above.
        log_base: The logarithm base used when ``is_log_value`` is set.
        is_cap_value: Whether to cap the value to ``[cap_min, cap_max]``.
        is_log_value: Whether to apply a log transform to the value.
        dtype: The output dtype.

    Raises:
        ValueError: If a percentile string is outside ``[0.0, 1.0]``, or if
            ``cap_min`` is greater than ``cap_max``.
    """

    scale_factor: float = Field(1.0, description="Scale factor")
    default_value: float | str = Field(
        "0.5",
        description=(
            "Default value, string value will be '0.0 - 0.99' for percentile choice"
        ),
    )
    cap_min: float | str = Field(
        "0.01",
        description=(
            "Min value, string value will be '0.0 - 0.99' for percentile choice"
        ),
    )
    cap_max: float | str = Field(
        "0.99",
        description=(
            "Max value, string value will be '0.0 - 0.99' for percentile choice"
        ),
    )
    log_base: float = Field(10.0, description="Log base")
    is_cap_value: bool = Field(True, description="Is cap value")
    is_log_value: bool = Field(True, description="Is log value")
    dtype: torch.dtype = Field(
        DEFAULT_NUMERICAL_OUTPUT_DTYPE, description="Output dtype"
    )

    @model_validator(mode="after")
    def set_output_dtype(self) -> NumericalStandardTransformLayerSpec:
        """Mirror ``dtype`` onto ``output_dtype`` and validate the cap range.

        Returns:
            ``self``, with ``output_dtype`` set to ``dtype``.

        Raises:
            ValueError: If a percentile string is outside ``[0.0, 1.0]``, or
                if ``cap_min`` is greater than ``cap_max``.
        """
        self.output_dtype = self.dtype
        cap_max = self.cap_max
        cap_min = self.cap_min
        if isinstance(cap_max, str) and (float(cap_max) < 0.0 or float(cap_max) > 1.0):
            raise ValueError(f"cap_max {cap_max} is not in [0.0, 1.0]")
        if isinstance(cap_min, str) and (float(cap_min) < 0.0 or float(cap_min) > 1.0):
            raise ValueError(f"cap_min {cap_min} is not in [0.0, 1.0]")
        if isinstance(self.default_value, str) and (
            float(self.default_value) < 0.0 or float(self.default_value) > 1.0
        ):
            raise ValueError(f"default_value {self.default_value} is not in [0.0, 1.0]")
        if float(cap_min) > float(cap_max):
            raise ValueError(f"cap_min {cap_min} is greater than cap_max {cap_max}")
        return self


class PercentileBucketizationLayerSpec(TorchTransformLayerSpec):
    """Placeholder resolved to :class:`BucketizationLayerSpec` after fitting.

    Args:
        percentiles: Percentile values (``0``-``1`` or ``1``-``100``) to
            compute boundaries from.
        dtype: The output dtype.
    """

    _resolved_layer_type: ClassVar[str | None] = "BucketizationLayerSpec"

    percentiles: list[float] = Field(
        ..., description="Percentile values (0-1 or 1-100) to compute boundaries from"
    )
    dtype: torch.dtype | str = Field(torch.int64, description="Output dtype")


class BucketizationLayerSpec(TorchTransformLayerSpec):
    """Spec for the ``Bucketization`` layer, with pre-computed boundaries.

    Args:
        boundaries: Pre-computed boundary values for bucketization.
        dtype: The output dtype.
    """

    boundaries: list[float] = Field(
        ..., description="Pre-computed boundary values for bucketization"
    )
    dtype: torch.dtype | str = Field(torch.int64, description="Output dtype")


class TensorColFillNoneLayerSpec(TorchTransformLayerSpec):
    """Spec for the ``TensorColFillNone`` layer.

    None detection is derived from the actual tensor dtype at runtime. The
    ``input_dtype`` field from the base class is accepted for backward
    compatibility with existing configs but is not used by the layer.

    Args:
        default_value: The value to fill ``None`` positions with.
    """

    default_value: int | float = Field(..., description="Value to fill None with")


class CastLayerSpec(TorchTransformLayerSpec):
    """Spec for the ``Cast`` layer.

    Args:
        dtype: The target dtype to cast to.
    """

    dtype: torch.dtype | str = Field(
        torch.float32, description="Target data type for casting"
    )


class CaseWhenLayerSpec(TorchTransformLayerSpec):
    """Spec for the ``CaseWhen`` layer.

    Args:
        default_value: The value output when no condition matches.
    """

    default_value: int | float | bool | torch.Tensor = Field(
        ..., description="Default value if no conditions match"
    )


class CompareLayerSpec(TorchTransformLayerSpec):
    """Spec for the ``Compare`` layer.

    Args:
        compare_op: The comparison operator, e.g. ``"equal"``, ``"greater"``,
            ``"less"``, ``"greater_equal"``, ``"less_equal"``,
            ``"not_equal"``.
    """

    compare_op: str = Field(
        ...,
        description=(
            "Comparison operator, e.g., 'equal', 'greater', 'less', "
            "'greater_equal', 'less_equal', 'not_equal'"
        ),
    )


class ConstantLayerSpec(TorchTransformLayerSpec):
    """Spec for the ``Constant`` layer.

    Args:
        constant: The constant value to output.
        dtype: The output dtype.
    """

    constant: int | float | bool = Field(..., description="Constant value to output")
    dtype: torch.dtype | str = Field(torch.float32, description="Output dtype")


class DivideLayerSpec(TorchTransformLayerSpec):
    """Spec for the ``Divide`` layer.

    Args:
        add_constant_to_divisor: A constant added to the divisor to avoid
            division by zero.
    """

    add_constant_to_divisor: float = Field(
        0.0,
        description="Constant to add to the divisor to avoid division by zero",
    )


class PadOrCrop1DLayerSpec(TorchTransformLayerSpec):
    """Spec for the ``PadOrCrop1D`` layer.

    Args:
        max_length: The fixed target length for padding or cropping.
        dtype: Optional output dtype. When ``None``, the input dtype is
            preserved.
        pad_value: The value used for padding.
        align: Which end is kept when cropping: ``"left"`` (default) keeps
            the first ``max_length`` elements; ``"right"`` keeps the last
            ``max_length`` elements.
    """

    max_length: int = Field(..., description="Max length for padding or cropping")
    dtype: torch.dtype | str | None = Field(
        None, description="Output dtype. When None, preserves input dtype."
    )
    pad_value: int | float = Field(0, description="Value to use for padding")
    align: str = Field(
        "left",
        description=(
            'Controls which end is kept when cropping. "left" (default) '
            'keeps the first max_length elements; "right" keeps the last '
            "max_length elements."
        ),
    )


class TileLayerSpec(TorchTransformLayerSpec):
    """Spec for the ``Tile`` layer.

    Args:
        axis: The axis along which to tile the tensor.
        count: The number of times to tile the tensor, if not derived from a
            target tensor.
        target_tensor_provided: Whether a target tensor is provided to tile
            against, instead of a fixed ``count``.
    """

    axis: int = Field(0, description="Axis along which to tile the tensor")
    count: int | None = Field(
        None, description="Number of times to tile the tensor (optional)"
    )
    target_tensor_provided: bool = Field(
        False, description="Whether a target tensor is provided for tiling"
    )


class LogTransformLayerSpec(TorchTransformLayerSpec):
    """Spec for the ``LogTransform`` layer.

    Args:
        add_constant: A constant added before the log transformation.
    """

    add_constant: float = Field(
        1.0, description="Constant to add before log transformation"
    )


class SubtractLayerSpec(TorchTransformLayerSpec):
    """Spec for the ``Subtract`` layer."""


class ScaleLayerSpec(TorchTransformLayerSpec):
    """Spec for the ``Scale`` layer.

    Args:
        factor: The scalar multiplier.
    """

    factor: float = Field(..., description="The scalar multiplier")


class FloorLayerSpec(TorchTransformLayerSpec):
    """Spec for the ``Floor`` layer."""


class CeilLayerSpec(TorchTransformLayerSpec):
    """Spec for the ``Ceil`` layer."""


class ClipLayerSpec(TorchTransformLayerSpec):
    """Spec for the ``Clip`` layer.

    Args:
        min_value: The lower bound, or ``None`` for no lower bound.
        max_value: The upper bound, or ``None`` for no upper bound.
        ignore_value: An optional value preserved unchanged rather than
            clamped.
    """

    min_value: float | None = Field(None, description="Minimum value (lower bound)")
    max_value: float | None = Field(None, description="Maximum value (upper bound)")
    ignore_value: float | None = Field(
        None, description="Optional value to ignore during clipping"
    )


class IDHashTokenizerLayerSpec(TorchTransformLayerSpec):
    """Spec for the ``IDHashTokenizer`` layer.

    Args:
        vocabulary: The list of integer values making up the vocabulary.

    Raises:
        ValueError: If ``vocabulary`` is empty.
    """

    vocabulary: list[int] = Field(
        ..., description="List of integer values representing the vocabulary"
    )

    @field_validator("vocabulary")
    @classmethod
    def validate_vocabulary(cls, v: list[int]) -> list[int]:
        """Reject an empty vocabulary.

        Args:
            v: The vocabulary value being validated.

        Returns:
            ``v`` unchanged.

        Raises:
            ValueError: If ``v`` is empty.
        """
        if not v:
            raise ValueError("Vocabulary cannot be empty")
        return v


class IdentityTransformLayerSpec(TorchTransformLayerSpec):
    """Spec for the ``IdentityTransform`` layer.

    Passes input through unchanged; used to ensure fields are included in
    the native transform input schema for downstream model assembly.
    """
