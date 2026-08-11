"""Tests for :mod:`michelangelo.lib.native_transform.torch.transform_spec`.

Covers ``TransformSpec`` construction/validation, the topological sort, the
placeholder-hydration ``update_*`` methods, statistics-requirement
collection, layer materialization, and dict/JSON ser-de round-trips.
"""

from __future__ import annotations

from unittest.mock import mock_open, patch

import pytest

# TransformSpec composes real torch layers/pydantic specs. Skip cleanly if
# either is unavailable in a lightweight environment.
torch = pytest.importorskip("torch")
pytest.importorskip("pydantic")
yaml = pytest.importorskip("yaml")

from michelangelo.lib.native_transform.torch.transform_layer_spec import (  # noqa: E402
    BucketizationLayerSpec,
    MinMaxLayerSpec,
    NormalizationLayerSpec,
    TransformerMode,
)
from michelangelo.lib.native_transform.torch.transform_spec import (  # noqa: E402
    TORCH_TRANSFORM_LAYER_CLASS_NAME_TO_SPEC_CLASS_NAME_DICT,
    TORCH_TRANSFORM_LAYERS_DICT,
    TORCH_TRANSFORM_LAYERS_SPECS_DICT,
    TransformSpec,
)

RAW_SPECS = {
    "transform_specs": [
        {
            "transform_name": "Concatenate",
            "input_cols": ["col1"],
            "output_cols": ["col1_concatenated"],
        },
        {
            "transform_name": "NumericalStandardTransform",
            "input_cols": ["col2"],
            "output_cols": ["col2_transformed"],
            "cap_min": "0.01",
            "cap_max": "0.99",
            "default_value": "0.5",
        },
        {
            "transform_name": "StandardScaler",
            "input_cols": ["col3", "col4"],
            "output_cols": ["col3_scaled_col4_scaled"],
        },
        {
            "transform_name": "MinMaxScaler",
            "input_cols": ["col5"],
            "output_cols": ["col5_scaled"],
        },
    ]
}


class TestTransformSpecConstruction:
    """Loading, validation, and basic accessors."""

    def test_init_from_raw_transform_specs(self) -> None:
        """Constructing from a raw dict parses every spec and computes levels."""
        spec = TransformSpec(raw_transform_specs=RAW_SPECS)
        assert isinstance(spec.transform_specs, dict)
        assert isinstance(spec.transform_levels, dict)
        assert len(spec.transform_specs) == 4
        assert spec.transform_spec_yaml_path is None

    def test_init_from_yaml_path(self) -> None:
        """Constructing from a YAML path loads and parses it the same as a raw dict."""
        with (
            patch("builtins.open", mock_open()),
            patch("yaml.safe_load", return_value=RAW_SPECS),
        ):
            spec = TransformSpec("test.yaml")
        assert len(spec.transform_specs) == 4
        assert spec.transform_spec_yaml_path == "test.yaml"

    def test_yaml_path_takes_precedence_over_raw_specs(self) -> None:
        """When both are given, the YAML path wins over raw_transform_specs."""
        with (
            patch("builtins.open", mock_open()),
            patch("yaml.safe_load", return_value=RAW_SPECS) as mock_yaml,
        ):
            spec = TransformSpec(
                transform_spec_yaml_path="test.yaml",
                raw_transform_specs={"different": "data"},
            )
        assert spec.transform_spec_yaml_path == "test.yaml"
        mock_yaml.assert_called_once()

    def test_neither_yaml_nor_raw_specs_raises(self) -> None:
        """Neither argument given raises ValueError."""
        with pytest.raises(
            ValueError, match="Either transform_spec_yaml_path or raw_transform_specs"
        ):
            TransformSpec()

    def test_empty_raw_transform_specs(self) -> None:
        """An empty transform_specs list yields an empty spec/level mapping."""
        spec = TransformSpec(raw_transform_specs={"transform_specs": []})
        assert len(spec.transform_specs) == 0
        assert len(spec.transform_levels) == 0

    def test_invalid_transform_name_raises(self) -> None:
        """An unregistered transform_name raises ValueError."""
        with pytest.raises(ValueError, match=r"Transform layer spec .* not found"):
            TransformSpec(
                raw_transform_specs={
                    "transform_specs": [
                        {
                            "transform_name": "NotARealTransform",
                            "input_cols": ["a"],
                            "output_cols": ["b"],
                        }
                    ]
                }
            )

    def test_duplicate_specs_are_deduplicated(self) -> None:
        """Two field-for-field-identical specs collapse into one."""
        duplicate_data = {
            "transform_specs": [
                {
                    "transform_name": "Cast",
                    "input_cols": ["a"],
                    "output_cols": ["b"],
                    "dtype": "float32",
                },
                {
                    "transform_name": "Cast",
                    "input_cols": ["a"],
                    "output_cols": ["b"],
                    "dtype": "float32",
                },
            ]
        }
        spec = TransformSpec(raw_transform_specs=duplicate_data)
        assert len(spec.transform_specs) == 1

    def test_duplicate_output_columns_raises(self) -> None:
        """Two layers producing the same output column raises ValueError."""
        duplicate_data = {
            "transform_specs": [
                {
                    "transform_name": "Cast",
                    "input_cols": ["a"],
                    "output_cols": ["out"],
                    "dtype": "float32",
                },
                {
                    "transform_name": "Cast",
                    "input_cols": ["b"],
                    "output_cols": ["out"],
                    "dtype": "float64",
                },
            ]
        }
        with pytest.raises(ValueError, match="is produced by multiple layers"):
            TransformSpec(raw_transform_specs=duplicate_data)

    def test_circular_dependency_raises(self) -> None:
        """A cycle in input/output column references raises ValueError."""
        circular_specs = {
            "transform_specs": [
                {
                    "transform_name": "Cast",
                    "input_cols": ["col2"],
                    "output_cols": ["col1"],
                    "dtype": "float32",
                },
                {
                    "transform_name": "Cast",
                    "input_cols": ["col1"],
                    "output_cols": ["col2"],
                    "dtype": "float32",
                },
            ]
        }
        with pytest.raises(ValueError, match="Circular dependency detected"):
            TransformSpec(raw_transform_specs=circular_specs)

    def test_duplicate_layer_name_raises(self) -> None:
        """Two distinct specs sharing an explicit name raises ValueError."""
        duplicate_name_data = {
            "transform_specs": [
                {
                    "transform_name": "Cast",
                    "input_cols": ["a"],
                    "output_cols": ["out1"],
                    "dtype": "float32",
                    "name": "dup",
                },
                {
                    "transform_name": "Cast",
                    "input_cols": ["b"],
                    "output_cols": ["out2"],
                    "dtype": "float64",
                    "name": "dup",
                },
            ]
        }
        with pytest.raises(ValueError, match="Layer name dup is duplicated"):
            TransformSpec(raw_transform_specs=duplicate_name_data)

    def test_duplicate_output_column_via_load_from_dict_raises(self) -> None:
        """A duplicate output column raises even via the load_from_dict path.

        ``load_from_dict`` bypasses ``_validate_transform_specs``'s dedup/name
        checks (it assigns ``transform_specs`` directly), so a duplicate
        output column can only be caught by ``_topological_sort``'s own
        producer-uniqueness guard. Exercise that path directly.
        """
        spec = TransformSpec(raw_transform_specs={"transform_specs": []})
        with pytest.raises(ValueError, match="is produced by multiple layers"):
            spec.load_from_dict(
                {
                    "transform_spec_yaml_path": None,
                    "columns_to_keep": None,
                    "transform_specs": [
                        {
                            "layer_spec_class_name": "CastLayerSpec",
                            "name": "cast_a",
                            "input_cols": ["a"],
                            "output_cols": ["out"],
                        },
                        {
                            "layer_spec_class_name": "CastLayerSpec",
                            "name": "cast_b",
                            "input_cols": ["b"],
                            "output_cols": ["out"],
                        },
                    ],
                }
            )


class TestColumnsToKeep:
    """Validation of the optional ``columns_to_keep`` field."""

    def test_valid_columns_to_keep(self) -> None:
        """A well-formed columns_to_keep list is stored as-is."""
        spec = TransformSpec(
            raw_transform_specs={
                "columns_to_keep": ["col1", "col2"],
                "transform_specs": [],
            }
        )
        assert spec.columns_to_keep == ["col1", "col2"]

    def test_not_a_list_raises(self) -> None:
        """A non-list columns_to_keep raises ValueError."""
        with pytest.raises(ValueError, match="columns_to_keep must be a list"):
            TransformSpec(
                raw_transform_specs={
                    "columns_to_keep": "not_a_list",
                    "transform_specs": [],
                }
            )

    def test_empty_list_raises(self) -> None:
        """An empty columns_to_keep list raises ValueError."""
        with pytest.raises(ValueError, match="columns_to_keep is empty"):
            TransformSpec(
                raw_transform_specs={"columns_to_keep": [], "transform_specs": []}
            )

    def test_non_string_entries_raise(self) -> None:
        """A columns_to_keep list with a non-string entry raises ValueError."""
        with pytest.raises(
            ValueError, match="columns_to_keep must be a list of strings"
        ):
            TransformSpec(
                raw_transform_specs={"columns_to_keep": ["a", 1], "transform_specs": []}
            )


class TestTopologicalSort:
    """Level assignment and layer materialization across a chained DAG."""

    def test_chained_layers_get_increasing_levels(self) -> None:
        """A Cast->Scale->Floor->Clip chain gets one layer per level."""
        chained_specs = {
            "transform_specs": [
                {
                    "transform_name": "Cast",
                    "input_cols": ["col1"],
                    "output_cols": ["col1_cast"],
                    "dtype": "float32",
                },
                {
                    "transform_name": "Scale",
                    "input_cols": ["col1_cast"],
                    "output_cols": ["col1_scaled"],
                    "factor": 2.0,
                },
                {
                    "transform_name": "Floor",
                    "input_cols": ["col1_scaled"],
                    "output_cols": ["col1_floored"],
                },
                {
                    "transform_name": "Clip",
                    "input_cols": ["col1_floored"],
                    "output_cols": ["col1_clipped"],
                    "min_value": 0.0,
                    "max_value": 10.0,
                },
            ]
        }
        spec = TransformSpec(raw_transform_specs=chained_specs)
        assert spec.get_max_transform_level() == 3
        for level in range(4):
            layers = spec.to_transform_layers(target_transform_level=level)
            assert len(layers) == 1

    def test_independent_layers_share_a_level(self) -> None:
        """Specs with no cross-dependencies all land at level 0."""
        spec = TransformSpec(raw_transform_specs=RAW_SPECS)
        # None of the four RAW_SPECS entries depend on each other's outputs.
        assert spec.get_max_transform_level() == 0
        assert len(spec.get_transform_layer_spec(target_transform_level=0)) == 4


class TestUpdateInputDtype:
    """``update_input_dtype`` populates per-column dtypes at a target level."""

    def test_updates_matching_columns(self) -> None:
        """Matching columns get their input_dtype populated from the dtype dict."""
        spec = TransformSpec(raw_transform_specs=RAW_SPECS)
        spec.update_input_dtype(
            {
                "col1": torch.int64,
                "col2": torch.float64,
                "col3": torch.float64,
                "col4": torch.float64,
                "col5": torch.float64,
            }
        )
        by_col = {s.input_cols[0]: s for s in spec.transform_specs.values()}
        assert by_col["col1"].input_dtype == torch.int64
        assert by_col["col2"].input_dtype == torch.float64

    def test_missing_column_raises(self) -> None:
        """A spec's input column missing from the dtype dict raises ValueError."""
        spec = TransformSpec(raw_transform_specs=RAW_SPECS)
        with pytest.raises(ValueError, match="not found in data dtype dict"):
            spec.update_input_dtype({"col_missing": torch.int64})

    def test_other_level_is_skipped(self) -> None:
        """A level with no matching specs is a no-op, even with an empty dtype dict."""
        spec = TransformSpec(raw_transform_specs=RAW_SPECS)
        # target_transform_level=5 matches nothing (every RAW_SPECS entry is
        # level 0), so no column lookups happen and nothing raises even
        # though data_dtype_dict is missing every column.
        spec.update_input_dtype({}, target_transform_level=5)
        for layer_spec in spec.transform_specs.values():
            assert layer_spec.input_dtype is None


class TestUpdateNumericalStandardTransformParameters:
    """``update_numerical_standard_transform_parameters`` percentile resolution."""

    def test_resolves_percentile_strings(self) -> None:
        """Percentile-string cap/default values resolve to floats from the dict."""
        spec = TransformSpec(raw_transform_specs=RAW_SPECS)
        spec.update_numerical_standard_transform_parameters(
            {"col2_1": 0.1, "col2_99": 0.9, "col2_50": 0.5}
        )
        numerical_spec = next(
            s for s in spec.transform_specs.values() if s.input_cols == ["col2"]
        )
        assert numerical_spec.cap_min == 0.1
        assert numerical_spec.cap_max == 0.9
        assert numerical_spec.default_value == 0.5

    def test_already_float_values_are_left_unchanged(self) -> None:
        """Cap/default values that are already floats are not looked up again."""
        raw = {
            "transform_specs": [
                {
                    "transform_name": "NumericalStandardTransform",
                    "input_cols": ["col2"],
                    "output_cols": ["col2_transformed"],
                    "cap_min": 0.1,
                    "cap_max": 0.9,
                    "default_value": 0.5,
                }
            ]
        }
        spec = TransformSpec(raw_transform_specs=raw)
        spec.update_numerical_standard_transform_parameters({"col2_1": 999.0})
        (numerical_spec,) = spec.transform_specs.values()
        assert numerical_spec.cap_min == 0.1
        assert numerical_spec.cap_max == 0.9
        assert numerical_spec.default_value == 0.5

    def test_non_capping_spec_has_no_percentile_requirements(self) -> None:
        """is_cap_value=False means no percentile statistics are required."""
        raw = {
            "transform_specs": [
                {
                    "transform_name": "NumericalStandardTransform",
                    "input_cols": ["col2"],
                    "output_cols": ["col2_transformed"],
                    "is_cap_value": False,
                }
            ]
        }
        stats_specs = TransformSpec(
            raw_transform_specs=raw
        ).get_numerical_statistics_computation_specs()
        assert "col2" not in stats_specs

    def test_other_level_is_skipped(self) -> None:
        """A level with no matching specs is left unresolved."""
        spec = TransformSpec(raw_transform_specs=RAW_SPECS)
        spec.update_numerical_standard_transform_parameters(
            {"col2_1": 0.1}, target_transform_level=5
        )
        numerical_spec = next(
            s for s in spec.transform_specs.values() if s.input_cols == ["col2"]
        )
        assert numerical_spec.cap_min == "0.01"

    def test_resolves_with_explicit_custom_name_not_matching_prefix(self) -> None:
        """Percentile resolution must not depend on ``name`` looking like the prefix.

        Internal's name-string-prefix dispatch misses a spec given an
        explicit ``name`` that doesn't start with the class prefix, silently
        leaving ``cap_min``/``cap_max`` as unresolved percentile strings
        instead of raising. This spec's ``name`` is deliberately chosen not
        to match, to lock in that ``isinstance()`` dispatch has no equivalent
        blind spot.
        """
        raw = {
            "transform_specs": [
                {
                    "transform_name": "NumericalStandardTransform",
                    "name": "my_custom_numerical_transform",
                    "input_cols": ["col5"],
                    "output_cols": ["col5_transformed"],
                },
            ]
        }
        spec = TransformSpec(raw_transform_specs=raw)
        (placeholder,) = spec.transform_specs.values()
        assert placeholder.name == "my_custom_numerical_transform"

        spec.update_numerical_standard_transform_parameters(
            {"col5_1": 0.1, "col5_99": 99.9, "col5_50": 50.0}
        )

        (resolved,) = spec.transform_specs.values()
        assert resolved.cap_min == 0.1
        assert resolved.cap_max == 99.9
        assert resolved.default_value == 50.0


class TestUpdateStandardScalerSpecs:
    """``update_standard_scaler_specs`` placeholder-hydration behavior."""

    def test_hydrates_into_normalization_spec(self) -> None:
        """A StandardScaler placeholder hydrates into a fitted Normalization spec."""
        spec = TransformSpec(raw_transform_specs=RAW_SPECS)
        spec.update_standard_scaler_specs(
            {"col3_mean": 10.0, "col3_std": 2.0, "col4_mean": 20.0, "col4_std": 3.0}
        )
        hydrated = [
            s
            for s in spec.transform_specs.values()
            if isinstance(s, NormalizationLayerSpec)
        ]
        assert len(hydrated) == 1
        assert hydrated[0].mean == [10.0, 20.0]
        assert hydrated[0].std == [2.0, 3.0]

    def test_hydration_dispatches_via_generated_name_placeholder(self) -> None:
        """Placeholder hydration must survive round-tripping through a raw spec dict.

        ``StandardScalerLayerSpec`` auto-generates its ``name`` via the same
        ``generate_layer_name()`` used by every layer spec, seeded from its
        own class name (``"StandardScaler"``, already lowercased before
        ``generate_layer_name`` is called -- see
        ``TorchTransformLayerSpec.set_defaults``). ``TransformSpec``
        dispatches hydration by ``isinstance()`` against the placeholder's
        real pydantic type rather than by parsing that generated name, but
        this test locks in the full round trip: a raw dict spec produces a
        placeholder with a generated name in the expected format, and
        hydration still finds and resolves it.
        """
        raw = {
            "transform_specs": [
                {
                    "transform_name": "StandardScaler",
                    "input_cols": ["x"],
                    "output_cols": ["x_scaled"],
                },
            ]
        }
        spec = TransformSpec(raw_transform_specs=raw)
        (placeholder,) = spec.transform_specs.values()
        assert placeholder.name.startswith("standardscaler_")

        spec.update_standard_scaler_specs({"x_mean": 5.0, "x_std": 2.0})

        (hydrated,) = spec.transform_specs.values()
        assert isinstance(hydrated, NormalizationLayerSpec)
        assert hydrated.mean == [5.0]
        assert hydrated.std == [2.0]
        layers = spec.to_transform_layers(target_transform_level=0)
        assert len(layers) == 1

    def test_hydration_with_explicit_custom_name_not_matching_prefix(self) -> None:
        """Hydration must not depend on ``name`` looking like the class prefix.

        Internal's name-string-prefix dispatch (``layer_spec.name.startswith(
        "standardscaler")``) silently fails to hydrate a placeholder given an
        explicit, non-generated ``name`` that doesn't happen to start with the
        class prefix -- the spec is left un-hydrated with no error. This spec
        gives the placeholder an explicit custom name specifically chosen to
        not match that prefix, to lock in that ``isinstance()`` dispatch has
        no equivalent blind spot.
        """
        raw = {
            "transform_specs": [
                {
                    "transform_name": "StandardScaler",
                    "name": "my_custom_feature_scaler",
                    "input_cols": ["x"],
                    "output_cols": ["x_scaled"],
                },
            ]
        }
        spec = TransformSpec(raw_transform_specs=raw)
        (placeholder,) = spec.transform_specs.values()
        assert placeholder.name == "my_custom_feature_scaler"

        spec.update_standard_scaler_specs({"x_mean": 5.0, "x_std": 2.0})

        (hydrated,) = spec.transform_specs.values()
        assert isinstance(hydrated, NormalizationLayerSpec)
        assert hydrated.mean == [5.0]
        assert hydrated.std == [2.0]


class TestUpdateMinMaxScalerSpecs:
    """``update_min_max_scaler_specs`` placeholder-hydration behavior."""

    def test_hydrates_into_minmax_spec(self) -> None:
        """A MinMaxScaler placeholder hydrates into a fitted MinMaxLayerSpec."""
        spec = TransformSpec(raw_transform_specs=RAW_SPECS)
        spec.update_min_max_scaler_specs({"col5_min": 0.0, "col5_max": 100.0})
        hydrated = [
            s for s in spec.transform_specs.values() if isinstance(s, MinMaxLayerSpec)
        ]
        assert len(hydrated) == 1
        assert hydrated[0].min == [0.0]
        assert hydrated[0].max == [100.0]


class TestUpdateBucketizationSpecs:
    """``update_bucketization_specs`` placeholder-hydration behavior."""

    def test_hydrates_percentile_placeholder(self) -> None:
        """A PercentileBucketization placeholder hydrates into boundaries from stats."""
        raw = {
            "transform_specs": [
                {
                    "transform_name": "PercentileBucketization",
                    "input_cols": ["col1"],
                    "output_cols": ["col1_bucketed"],
                    "percentiles": [0.25, 0.5, 0.75],
                }
            ]
        }
        spec = TransformSpec(raw_transform_specs=raw)
        spec.update_bucketization_specs(
            {"col1_25": 10.0, "col1_50": 20.0, "col1_75": 30.0}
        )
        (hydrated,) = spec.transform_specs.values()
        assert isinstance(hydrated, BucketizationLayerSpec)
        assert hydrated.boundaries == [10.0, 20.0, 30.0]

    def test_precomputed_boundaries_are_unchanged(self) -> None:
        """A Bucketization spec with boundaries already set is left untouched."""
        raw = {
            "transform_specs": [
                {
                    "transform_name": "Bucketization",
                    "input_cols": ["col1"],
                    "output_cols": ["col1_bucketed"],
                    "boundaries": [10.0, 20.0, 30.0],
                }
            ]
        }
        spec = TransformSpec(raw_transform_specs=raw)
        spec.update_bucketization_specs({})
        (unchanged,) = spec.transform_specs.values()
        assert unchanged.boundaries == [10.0, 20.0, 30.0]


class TestGetNumericalStatisticsComputationSpecs:
    """``get_numerical_statistics_computation_specs`` requirement collection."""

    def test_collects_requirements_across_layer_kinds(self) -> None:
        """Statistics requirements are collected per column across layer kinds."""
        spec = TransformSpec(raw_transform_specs=RAW_SPECS)
        stats_specs = spec.get_numerical_statistics_computation_specs()
        assert "col2" in stats_specs
        assert "percentiles" in stats_specs["col2"]
        assert stats_specs["col3"]["mean"] is True
        assert stats_specs["col3"]["std"] is False
        assert stats_specs["col5"]["min"] is True
        assert stats_specs["col5"]["max"] is True

    def test_percentile_fractions_are_kept_as_is(self) -> None:
        """Percentiles already in [0, 1] are kept as fractions, not rescaled."""
        raw = {
            "transform_specs": [
                {
                    "transform_name": "PercentileBucketization",
                    "input_cols": ["col1"],
                    "output_cols": ["col1_bucketed"],
                    "percentiles": [0.25, 0.5, 0.75],
                }
            ]
        }
        stats_specs = TransformSpec(
            raw_transform_specs=raw
        ).get_numerical_statistics_computation_specs()
        assert stats_specs["col1"]["percentiles"] == [0.25, 0.5, 0.75]

    def test_percentages_are_converted_to_fractions(self) -> None:
        """Percentiles in [1, 100] are converted to the [0, 1] fraction form."""
        raw = {
            "transform_specs": [
                {
                    "transform_name": "PercentileBucketization",
                    "input_cols": ["col1"],
                    "output_cols": ["col1_bucketed"],
                    "percentiles": [25, 50, 75],
                }
            ]
        }
        stats_specs = TransformSpec(
            raw_transform_specs=raw
        ).get_numerical_statistics_computation_specs()
        assert stats_specs["col1"]["percentiles"] == [0.25, 0.5, 0.75]

    def test_precomputed_bucketization_has_no_requirements(self) -> None:
        """A Bucketization spec with boundaries needs no statistics."""
        raw = {
            "transform_specs": [
                {
                    "transform_name": "Bucketization",
                    "input_cols": ["col1"],
                    "output_cols": ["col1_bucketed"],
                    "boundaries": [10.0, 20.0, 30.0],
                }
            ]
        }
        stats_specs = TransformSpec(
            raw_transform_specs=raw
        ).get_numerical_statistics_computation_specs()
        assert "col1" not in stats_specs

    def test_with_std_true_adds_std_requirement(self) -> None:
        """with_std=True adds a std requirement alongside the default mean one."""
        raw = {
            "transform_specs": [
                {
                    "transform_name": "StandardScaler",
                    "input_cols": ["x"],
                    "output_cols": ["x_scaled"],
                    "with_std": True,
                },
            ]
        }
        stats_specs = TransformSpec(
            raw_transform_specs=raw
        ).get_numerical_statistics_computation_specs()
        assert stats_specs["x"]["std"] is True
        assert stats_specs["x"]["mean"] is True

    def test_non_positive_and_out_of_range_percentiles_are_ignored(self) -> None:
        """Percentiles <= 0 or >= 100 are dropped from the requirements."""
        raw = {
            "transform_specs": [
                {
                    "transform_name": "PercentileBucketization",
                    "input_cols": ["col1"],
                    "output_cols": ["col1_bucketed"],
                    "percentiles": [0, 100, 50],
                }
            ]
        }
        stats_specs = TransformSpec(
            raw_transform_specs=raw
        ).get_numerical_statistics_computation_specs()
        assert stats_specs["col1"]["percentiles"] == [0.5]

    def test_other_level_is_skipped(self) -> None:
        """A level with no matching specs has no statistics requirements."""
        spec = TransformSpec(raw_transform_specs=RAW_SPECS)
        assert (
            spec.get_numerical_statistics_computation_specs(target_transform_level=5)
            == {}
        )


class TestAccessors:
    """Level/type-filtered accessors on a constructed ``TransformSpec``."""

    def test_get_transform_layer_spec(self) -> None:
        """Every spec is returned, both unfiltered and filtered by level."""
        spec = TransformSpec(raw_transform_specs=RAW_SPECS)
        assert len(spec.get_transform_layer_spec()) == 4
        assert len(spec.get_transform_layer_spec(target_transform_level=0)) == 4

    def test_get_transform_layer_spec_no_match_raises(self) -> None:
        """A level with no matching specs raises ValueError."""
        spec = TransformSpec(raw_transform_specs=RAW_SPECS)
        with pytest.raises(ValueError, match="No transform layer spec found"):
            spec.get_transform_layer_spec(target_transform_level=99)

    def test_get_transform_input_and_output_cols(self) -> None:
        """Input/output column accessors return the expected column sets."""
        spec = TransformSpec(raw_transform_specs=RAW_SPECS)
        input_cols = spec.get_transform_input_cols()
        assert {"col2", "col3", "col4", "col5"} <= set(input_cols)
        output_cols = spec.get_transform_output_cols()
        assert "col2_transformed" in output_cols

    def test_get_input_dtype_map(self) -> None:
        """The dtype map reflects dtypes populated by update_input_dtype."""
        spec = TransformSpec(raw_transform_specs=RAW_SPECS)
        spec.update_input_dtype(
            {
                "col1": torch.int64,
                "col2": torch.float64,
                "col3": torch.float64,
                "col4": torch.float64,
                "col5": torch.float64,
            }
        )
        dtype_map = spec.get_input_dtype_map()
        assert dtype_map["col1"] == torch.int64

    def test_get_input_dtype_map_skips_other_levels_and_none_dtype(self) -> None:
        """Levels with no input_dtype set yield an empty map."""
        # RAW_SPECS specs are all level 0 and start with input_dtype=None
        # before update_input_dtype is called.
        spec = TransformSpec(raw_transform_specs=RAW_SPECS)
        assert spec.get_input_dtype_map(target_transform_level=0) == {}
        assert spec.get_input_dtype_map(target_transform_level=5) == {}

    def test_get_max_transform_level_empty(self) -> None:
        """An empty spec's max transform level is 0."""
        spec = TransformSpec(raw_transform_specs={"transform_specs": []})
        assert spec.get_max_transform_level() == 0

    def test_to_transform_layers_unregistered_placeholder_raises(self) -> None:
        """An unresolved placeholder spec cannot be materialized into a layer."""
        raw = {
            "transform_specs": [
                {
                    "transform_name": "StandardScaler",
                    "input_cols": ["x"],
                    "output_cols": ["x_scaled"],
                },
            ]
        }
        spec = TransformSpec(raw_transform_specs=raw)
        with pytest.raises(ValueError, match=r"Transform layer .* not found"):
            spec.to_transform_layers()


class TestSerialization:
    """to_dict/to_json/load_from_dict/load_from_json round-trip behavior."""

    def test_to_dict_shape(self) -> None:
        """to_dict returns the expected top-level keys and per-layer entries."""
        spec = TransformSpec(raw_transform_specs=RAW_SPECS)
        spec_dict = spec.to_dict()
        assert set(spec_dict) == {
            "transform_spec_yaml_path",
            "transform_specs",
            "columns_to_keep",
        }
        assert len(spec_dict["transform_specs"]) == 4

    def test_to_dict_serializes_enum_values_to_primitives(self) -> None:
        """Enum fields serialize to their .value, and the result stays yaml-safe."""
        spec = TransformSpec(
            raw_transform_specs={
                "transform_specs": [
                    {
                        "transform_name": "Scale",
                        "input_cols": ["x"],
                        "output_cols": ["x_scaled"],
                        "factor": 2.0,
                    },
                ]
            }
        )
        (layer,) = spec.to_dict()["transform_specs"]
        assert isinstance(layer["mode"], str)
        assert layer["mode"] == TransformerMode.INVALID.value
        # Must be yaml.safe_dump-compatible -- this was the original internal failure.
        yaml.safe_dump(spec.to_dict())

    def test_to_json_round_trips_dtype_and_enum(self) -> None:
        """to_json produces a valid JSON string."""
        spec = TransformSpec(raw_transform_specs=RAW_SPECS)
        json_str = spec.to_json()
        assert isinstance(json_str, str)

    def test_load_from_json_restores_state(self) -> None:
        """load_from_json restores the same specs and levels as the original."""
        spec = TransformSpec(raw_transform_specs=RAW_SPECS)
        json_str = spec.to_json()

        restored = TransformSpec(raw_transform_specs={"transform_specs": []})
        restored.load_from_json(json_str)

        assert len(restored.transform_specs) == 4
        assert restored.transform_levels == spec.transform_levels

    def test_load_from_dict_restores_state(self) -> None:
        """load_from_dict restores the same specs as the original."""
        spec = TransformSpec(raw_transform_specs=RAW_SPECS)
        spec_dict = spec.to_dict()

        restored = TransformSpec(raw_transform_specs={"transform_specs": []})
        restored.load_from_dict(spec_dict)

        assert len(restored.transform_specs) == 4

    def test_load_from_dict_invalid_layer_spec_class_name_raises(self) -> None:
        """An unregistered layer_spec_class_name raises ValueError."""
        restored = TransformSpec(raw_transform_specs={"transform_specs": []})
        with pytest.raises(ValueError, match=r"Layer spec class name .* not found"):
            restored.load_from_dict(
                {
                    "transform_spec_yaml_path": None,
                    "columns_to_keep": None,
                    "transform_specs": [
                        {
                            "layer_spec_class_name": "NotARealSpec",
                            "input_cols": ["a"],
                            "output_cols": ["b"],
                        }
                    ],
                }
            )

    def test_round_trip_preserves_hydrated_stats(self) -> None:
        """A hydrated (fitted) spec round-trips through to_dict/load_from_dict."""
        raw = {
            "transform_specs": [
                {
                    "transform_name": "StandardScaler",
                    "input_cols": ["x"],
                    "output_cols": ["x_scaled"],
                },
            ]
        }
        spec = TransformSpec(raw_transform_specs=raw)
        spec.update_standard_scaler_specs({"x_mean": 5.0, "x_std": 2.0})

        restored = TransformSpec(raw_transform_specs={"transform_specs": []})
        restored.load_from_dict(spec.to_dict())

        (restored_spec,) = restored.transform_specs.values()
        assert isinstance(restored_spec, NormalizationLayerSpec)
        assert restored_spec.mean == [5.0]
        assert restored_spec.std == [2.0]


class TestRegistries:
    """Module-level layer/spec registries."""

    def test_registries_are_populated_and_consistent(self) -> None:
        """The layer/spec/class-name registries are non-empty and consistent."""
        assert len(TORCH_TRANSFORM_LAYERS_DICT) > 0
        assert len(TORCH_TRANSFORM_LAYERS_SPECS_DICT) > 0
        assert len(TORCH_TRANSFORM_LAYER_CLASS_NAME_TO_SPEC_CLASS_NAME_DICT) > 0

        for layer_name, layer_cls in TORCH_TRANSFORM_LAYERS_DICT.items():
            assert layer_name == layer_cls.__name__
            assert layer_name in TORCH_TRANSFORM_LAYERS_SPECS_DICT

        for spec_cls in TORCH_TRANSFORM_LAYERS_SPECS_DICT.values():
            assert (
                spec_cls.__name__
                in TORCH_TRANSFORM_LAYER_CLASS_NAME_TO_SPEC_CLASS_NAME_DICT
            )
