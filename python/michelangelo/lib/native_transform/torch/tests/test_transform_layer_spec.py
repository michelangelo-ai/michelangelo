"""Tests for :mod:`michelangelo.lib.native_transform.torch.transform_layer_spec`.

Covers field defaults, the shared ``model_validator`` behavior (auto-name
generation, ``is_training_features`` defaulting), and the validation logic
on the specs that carry their own custom rules.
"""

from __future__ import annotations

import pytest

# These specs use torch dtypes as field types. Skip cleanly if torch is
# unavailable in a lightweight environment.
torch = pytest.importorskip("torch")

pydantic = pytest.importorskip("pydantic")

from michelangelo.lib.native_transform.torch.transform_layer_spec import (  # noqa: E402
    BucketizationLayerSpec,
    CastLayerSpec,
    IDHashTokenizerLayerSpec,
    MinMaxLayerSpec,
    MinMaxScalerLayerSpec,
    NormalizationLayerSpec,
    NumericalStandardTransformLayerSpec,
    PercentileBucketizationLayerSpec,
    StandardScalerLayerSpec,
    TileLayerSpec,
    TorchTransformLayerSpec,
    TransformerMode,
)


class TestTorchTransformLayerSpec:
    """Shared base-spec behavior: defaulting and validation."""

    def test_name_is_auto_generated_when_omitted(self) -> None:
        """A spec without an explicit name gets one generated from its class."""
        spec = CastLayerSpec(input_cols=["a"], output_cols=["b"])
        assert spec.name.startswith("cast_")

    def test_explicit_name_is_preserved(self) -> None:
        """An explicit name is used verbatim, not auto-generated."""
        spec = CastLayerSpec(input_cols=["a"], output_cols=["b"], name="my_cast")
        assert spec.name == "my_cast"

    def test_auto_generated_names_are_unique(self) -> None:
        """Two default-named specs of the same class get distinct names."""
        first = CastLayerSpec(input_cols=["a"], output_cols=["b"])
        second = CastLayerSpec(input_cols=["a"], output_cols=["b"])
        assert first.name != second.name

    def test_is_training_features_defaults_to_true_per_output_col(self) -> None:
        """Omitted is_training_features defaults to True for each output col."""
        spec = CastLayerSpec(input_cols=["a"], output_cols=["b", "c"])
        assert spec.is_training_features == [True, True]

    def test_is_training_features_explicit_value_is_preserved(self) -> None:
        """An explicit is_training_features list overrides the default."""
        spec = CastLayerSpec(
            input_cols=["a"],
            output_cols=["b", "c"],
            is_training_features=[True, False],
        )
        assert spec.is_training_features == [True, False]

    def test_default_mode_is_invalid(self) -> None:
        """The default per-layer mode is INVALID."""
        spec = CastLayerSpec(input_cols=["a"], output_cols=["b"])
        assert spec.mode == TransformerMode.INVALID

    def test_explicit_mode_is_preserved(self) -> None:
        """An explicit mode overrides the default."""
        spec = CastLayerSpec(
            input_cols=["a"], output_cols=["b"], mode=TransformerMode.REFIT
        )
        assert spec.mode == TransformerMode.REFIT

    def test_resolved_layer_type_defaults_to_none(self) -> None:
        """A spec with no placeholder resolution has _resolved_layer_type None."""
        assert CastLayerSpec._resolved_layer_type is None


class TestPlaceholderSpecs:
    """Placeholder specs resolve to a concrete layer spec via a class var."""

    def test_standard_scaler_resolves_to_normalization(self) -> None:
        """StandardScalerLayerSpec resolves to NormalizationLayerSpec."""
        assert StandardScalerLayerSpec._resolved_layer_type == "NormalizationLayerSpec"

    def test_min_max_scaler_resolves_to_min_max(self) -> None:
        """MinMaxScalerLayerSpec resolves to MinMaxLayerSpec."""
        assert MinMaxScalerLayerSpec._resolved_layer_type == "MinMaxLayerSpec"

    def test_percentile_bucketization_resolves_to_bucketization(self) -> None:
        """PercentileBucketizationLayerSpec resolves to BucketizationLayerSpec."""
        assert (
            PercentileBucketizationLayerSpec._resolved_layer_type
            == "BucketizationLayerSpec"
        )

    def test_standard_scaler_field_defaults(self) -> None:
        """StandardScalerLayerSpec carries its own with_mean/with_std defaults."""
        spec = StandardScalerLayerSpec(input_cols=["a"], output_cols=["b"])
        assert spec.with_mean is True
        assert spec.with_std is False

    def test_standard_scaler_multi_output_cols_raises(self) -> None:
        """StandardScalerLayerSpec requires exactly one output column.

        It resolves to NormalizationLayerSpec, which enforces the same
        constraint; catching it here (at spec-parse time, before fitting)
        avoids a config that only fails once resolved.
        """
        with pytest.raises(pydantic.ValidationError, match="exactly one column"):
            StandardScalerLayerSpec(input_cols=["a", "b"], output_cols=["c", "d"])

    def test_min_max_scaler_multi_output_cols_raises(self) -> None:
        """MinMaxScalerLayerSpec requires exactly one output column.

        It resolves to MinMaxLayerSpec, which enforces the same constraint;
        catching it here (at spec-parse time, before fitting) avoids a
        config that only fails once resolved.
        """
        with pytest.raises(pydantic.ValidationError, match="exactly one column"):
            MinMaxScalerLayerSpec(input_cols=["a", "b"], output_cols=["c", "d"])


class TestNormalizationAndMinMaxLayerSpec:
    """Fitted-statistics spec field defaults."""

    def test_normalization_defaults(self) -> None:
        """Normalization defaults to mean=[0.0], std=[1.0], dim=-1."""
        spec = NormalizationLayerSpec(input_cols=["a"], output_cols=["b"])
        assert spec.mean == [0.0]
        assert spec.std == [1.0]
        assert spec.dim == -1

    def test_min_max_defaults(self) -> None:
        """MinMax defaults to min=[0.0], max=[1.0], dim=-1."""
        spec = MinMaxLayerSpec(input_cols=["a"], output_cols=["b"])
        assert spec.min == [0.0]
        assert spec.max == [1.0]
        assert spec.dim == -1

    def test_normalization_multi_output_cols_raises(self) -> None:
        """Normalization requires exactly one output column."""
        with pytest.raises(pydantic.ValidationError, match="exactly one column"):
            NormalizationLayerSpec(input_cols=["a", "b"], output_cols=["c", "d"])

    def test_min_max_multi_output_cols_raises(self) -> None:
        """MinMax requires exactly one output column."""
        with pytest.raises(pydantic.ValidationError, match="exactly one column"):
            MinMaxLayerSpec(input_cols=["a", "b"], output_cols=["c", "d"])


class TestBucketizationLayerSpec:
    """Bucketization spec requires explicit boundaries."""

    def test_requires_boundaries(self) -> None:
        """Boundaries has no default and must be supplied."""
        with pytest.raises(pydantic.ValidationError):
            BucketizationLayerSpec(input_cols=["a"], output_cols=["b"])

    def test_default_dtype_is_int64(self) -> None:
        """The default output dtype is int64."""
        spec = BucketizationLayerSpec(
            input_cols=["a"], output_cols=["b"], boundaries=[0.0, 1.0]
        )
        assert spec.dtype == torch.int64


class TestTileLayerSpec:
    """Tile spec field defaults."""

    def test_defaults(self) -> None:
        """Tile defaults to axis=0, count=None, target_tensor_provided=False."""
        spec = TileLayerSpec(input_cols=["a"], output_cols=["b"])
        assert spec.axis == 0
        assert spec.count is None
        assert spec.target_tensor_provided is False


class TestIDHashTokenizerLayerSpec:
    """IDHashTokenizer spec validates a non-empty vocabulary."""

    def test_empty_vocabulary_raises(self) -> None:
        """An empty vocabulary raises a validation error."""
        with pytest.raises(pydantic.ValidationError, match="cannot be empty"):
            IDHashTokenizerLayerSpec(input_cols=["a"], output_cols=["b"], vocabulary=[])

    def test_valid_vocabulary_is_preserved(self) -> None:
        """A non-empty vocabulary is stored as given."""
        spec = IDHashTokenizerLayerSpec(
            input_cols=["a"], output_cols=["b"], vocabulary=[1, 2, 3]
        )
        assert spec.vocabulary == [1, 2, 3]


class TestNumericalStandardTransformLayerSpec:
    """Cap/log/scale spec cross-field validation."""

    def test_output_dtype_mirrors_dtype(self) -> None:
        """output_dtype is set to dtype by the after-validator."""
        spec = NumericalStandardTransformLayerSpec(
            input_cols=["a"], output_cols=["b"], dtype=torch.float64
        )
        assert spec.output_dtype == torch.float64

    def test_cap_min_greater_than_cap_max_raises(self) -> None:
        """cap_min > cap_max raises a validation error."""
        with pytest.raises(pydantic.ValidationError, match="greater than cap_max"):
            NumericalStandardTransformLayerSpec(
                input_cols=["a"], output_cols=["b"], cap_min="0.9", cap_max="0.1"
            )

    def test_percentile_cap_max_out_of_range_raises(self) -> None:
        """A percentile-string cap_max outside [0.0, 1.0] raises."""
        with pytest.raises(pydantic.ValidationError, match=r"cap_max .* not in"):
            NumericalStandardTransformLayerSpec(
                input_cols=["a"], output_cols=["b"], cap_max="1.5"
            )

    def test_percentile_cap_min_out_of_range_raises(self) -> None:
        """A percentile-string cap_min outside [0.0, 1.0] raises."""
        with pytest.raises(pydantic.ValidationError, match=r"cap_min .* not in"):
            NumericalStandardTransformLayerSpec(
                input_cols=["a"], output_cols=["b"], cap_min="-0.1"
            )

    def test_percentile_default_value_out_of_range_raises(self) -> None:
        """A percentile-string default_value outside [0.0, 1.0] raises."""
        with pytest.raises(pydantic.ValidationError, match=r"default_value .* not in"):
            NumericalStandardTransformLayerSpec(
                input_cols=["a"], output_cols=["b"], default_value="1.5"
            )

    def test_numeric_cap_values_are_not_range_checked(self) -> None:
        """Non-string (plain numeric) cap values skip the percentile-range check."""
        spec = NumericalStandardTransformLayerSpec(
            input_cols=["a"], output_cols=["b"], cap_min=-5.0, cap_max=5.0
        )
        assert spec.cap_min == -5.0
        assert spec.cap_max == 5.0


def test_base_spec_is_reused_by_every_layer_spec() -> None:
    """Every concrete layer spec inherits the shared base-spec contract."""
    for cls in (
        CastLayerSpec,
        MinMaxLayerSpec,
        NormalizationLayerSpec,
        TileLayerSpec,
        BucketizationLayerSpec,
        IDHashTokenizerLayerSpec,
    ):
        assert issubclass(cls, TorchTransformLayerSpec)
