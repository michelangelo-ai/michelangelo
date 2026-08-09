"""Tests for :mod:`michelangelo.lib.native_transform.torch.transform_utils`.

Covers each ``generate_*_transformation`` helper's imperative tensor_map
mutation, including the "missing feature is a no-op" and
"empty vocabulary is skipped" edge cases matched from internal behavior.
"""

from __future__ import annotations

import math

import pytest

# These helpers operate on real torch tensors. Skip cleanly if torch is
# unavailable in a lightweight environment.
torch = pytest.importorskip("torch")

from michelangelo.lib.native_transform.torch.transform_utils import (  # noqa: E402
    generate_cast_transformation,
    generate_concatenation_transformation,
    generate_duration_transformation,
    generate_idhash_tokenization_transformation,
    generate_numerical_scaled_transformation,
    update_output_tensor_map,
)

_ONE_DAY_MS = 24 * 60 * 60 * 1000


class TestGenerateNumericalScaledTransformation:
    """generate_numerical_scaled_transformation clips and scales in place."""

    def test_basic_scaling(self) -> None:
        """Values are clamped to [min_value, max_value]."""
        tensor_map = {"ratings_seq": torch.tensor([[-0.2], [3.5], [5.2], [4.7]])}
        specs = {"ratings_seq": {"min_value": 0.0, "max_value": 5.0}}
        generate_numerical_scaled_transformation(specs, tensor_map)
        expected = torch.tensor([[0.0], [3.5], [5.0], [4.7]], dtype=torch.float32)
        torch.testing.assert_close(tensor_map["scaled_ratings_seq"], expected)

    def test_scale_factor_and_output_type(self) -> None:
        """scale_factor and output_type are forwarded to ClipAndScale."""
        tensor_map = {"eta_values": torch.tensor([[10.0], [15.0], [25.0]])}
        specs = {
            "eta_values": {
                "min_value": 5.0,
                "max_value": 20.0,
                "scale_factor": 0.5,
                "output_type": "float32",
            }
        }
        generate_numerical_scaled_transformation(specs, tensor_map)
        expected = torch.tensor([[5.0], [7.5], [10.0]], dtype=torch.float32)
        torch.testing.assert_close(tensor_map["scaled_eta_values"], expected)

    def test_missing_feature_is_noop(self) -> None:
        """A feature absent from tensor_map is skipped without error."""
        tensor_map = {"existing_feature": torch.tensor([[1.0], [2.0]])}
        specs = {"missing_feature": {"min_value": 0.0, "max_value": 5.0}}
        original_keys = set(tensor_map.keys())
        generate_numerical_scaled_transformation(specs, tensor_map)
        assert set(tensor_map.keys()) == original_keys


class TestGenerateConcatenationTransformation:
    """generate_concatenation_transformation concatenates in place."""

    def test_default_axis(self) -> None:
        """Default axis is 1 (feature dimension)."""
        tensor_map = {
            "category_tag_0": torch.tensor([[1.0], [2.0], [3.0]]),
            "category_tag_1": torch.tensor([[4.0], [5.0], [6.0]]),
        }
        specs = {"category_tag": {"input_cols": ["category_tag_0", "category_tag_1"]}}
        generate_concatenation_transformation(specs, tensor_map)
        expected = torch.tensor(
            [[1.0, 4.0], [2.0, 5.0], [3.0, 6.0]], dtype=torch.float32
        )
        torch.testing.assert_close(tensor_map["concatenated_category_tag"], expected)

    def test_explicit_axis(self) -> None:
        """An explicit axis is respected."""
        tensor_map = {
            "feature_a": torch.tensor([[1.0, 2.0], [3.0, 4.0]]),
            "feature_b": torch.tensor([[5.0, 6.0], [7.0, 8.0]]),
        }
        specs = {"combined": {"input_cols": ["feature_a", "feature_b"], "axis": 0}}
        generate_concatenation_transformation(specs, tensor_map)
        expected = torch.tensor(
            [[1.0, 2.0], [3.0, 4.0], [5.0, 6.0], [7.0, 8.0]], dtype=torch.float32
        )
        torch.testing.assert_close(tensor_map["concatenated_combined"], expected)


class TestGenerateCastTransformation:
    """generate_cast_transformation casts in place."""

    def test_bool_to_float(self) -> None:
        """Boolean tensors cast to a numeric dtype."""
        tensor_map = {"is_active": torch.tensor([[True], [False], [True]])}
        specs = {"is_active": {"dtype": "float32"}}
        generate_cast_transformation(specs, tensor_map)
        expected = torch.tensor([[1.0], [0.0], [1.0]], dtype=torch.float32)
        torch.testing.assert_close(tensor_map["casted_is_active"], expected)

    def test_int_to_float(self) -> None:
        """A torch.dtype spec value is honored directly."""
        tensor_map = {"counts": torch.tensor([[1], [2], [3]], dtype=torch.int32)}
        specs = {"counts": {"dtype": torch.float64}}
        generate_cast_transformation(specs, tensor_map)
        expected = torch.tensor([[1.0], [2.0], [3.0]], dtype=torch.float64)
        torch.testing.assert_close(tensor_map["casted_counts"], expected)


class TestGenerateDurationTransformation:
    """generate_duration_transformation composes TimeDuration in place."""

    def test_basic(self) -> None:
        """Duration is floor-divided by unit."""
        tensor_map = {
            "target_time": torch.tensor(
                [[1727171357801], [1727213336630]], dtype=torch.float32
            ),
            "source_time": torch.tensor(
                [
                    [1727171357801, 1727085467217, 1726997165128],
                    [1727213336630, 1727196549297, 1727149789328],
                ],
                dtype=torch.float32,
            ),
        }
        specs = {
            "position_encoding": {
                "target": "target_time",
                "source": "source_time",
                "unit": _ONE_DAY_MS,
            }
        }
        generate_duration_transformation(specs, tensor_map)
        expected = torch.tensor([[0, 0, 2], [0, 0, 0]], dtype=torch.float32)
        torch.testing.assert_close(tensor_map["duration_position_encoding"], expected)

    def test_with_clipping_log_scale_and_reshape(self) -> None:
        """Log scaling is applied before clipping, matching the layer's contract."""
        tensor_map = {
            "current_epoch": torch.tensor(
                [[1727171357801], [1727213336630]], dtype=torch.float32
            ),
            "historical_epochs": torch.tensor(
                [
                    [1727171357801, 1726997165128, 1726097165128],
                    [1727213336630, 1727196549297, 1727149789328],
                ],
                dtype=torch.float32,
            ),
        }
        specs = {
            "time_diff": {
                "target": "current_epoch",
                "source": "historical_epochs",
                "unit": _ONE_DAY_MS,
                "min_value": 1,
                "max_value": 10,
                "log_scale": True,
                "target_shape": (-1, 1),
                "source_shape": (-1, 3),
            }
        }
        generate_duration_transformation(specs, tensor_map)
        expected = torch.tensor(
            [[1.0, math.log1p(2.0), math.log1p(12.0)], [1.0, 1.0, 1.0]],
            dtype=torch.float32,
        )
        torch.testing.assert_close(tensor_map["duration_time_diff"], expected)


class TestGenerateIdhashTokenizationTransformation:
    """generate_idhash_tokenization_transformation composes IDHashTokenizer in place."""

    def test_basic_vocabulary(self) -> None:
        """Known IDs map to their vocabulary index; unknown IDs map to the unk index."""
        tensor_map = {
            "store_id": torch.tensor([1001, 2002, 9999, 3003], dtype=torch.long)
        }
        specs = {"store_id": {"vocabulary": [1001, 2002, 3003]}}
        generate_idhash_tokenization_transformation(specs, tensor_map)
        expected = torch.tensor([0, 1, 3, 2], dtype=torch.long)
        torch.testing.assert_close(tensor_map["tokenized_store_id"], expected)

    def test_multiple_features(self) -> None:
        """Each feature in specs is tokenized independently."""
        tensor_map = {
            "store_id": torch.tensor([100, 200, 300], dtype=torch.long),
            "user_id": torch.tensor([10, 20, 30], dtype=torch.long),
        }
        specs = {
            "store_id": {"vocabulary": [100, 200, 300]},
            "user_id": {"vocabulary": [10, 20, 30, 40]},
        }
        generate_idhash_tokenization_transformation(specs, tensor_map)
        torch.testing.assert_close(
            tensor_map["tokenized_store_id"], torch.tensor([0, 1, 2], dtype=torch.long)
        )
        torch.testing.assert_close(
            tensor_map["tokenized_user_id"], torch.tensor([0, 1, 2], dtype=torch.long)
        )

    def test_unknown_values(self) -> None:
        """Values outside the vocabulary map to the unk index."""
        tensor_map = {"item_id": torch.tensor([5, 99, 10, 88, 15], dtype=torch.long)}
        specs = {"item_id": {"vocabulary": [5, 10, 15]}}
        generate_idhash_tokenization_transformation(specs, tensor_map)
        expected = torch.tensor([0, 3, 1, 3, 2], dtype=torch.long)
        torch.testing.assert_close(tensor_map["tokenized_item_id"], expected)

    def test_missing_feature_is_noop(self) -> None:
        """A feature absent from tensor_map is skipped without error."""
        tensor_map = {"existing_id": torch.tensor([1, 2, 3], dtype=torch.long)}
        specs = {"missing_id": {"vocabulary": [1, 2, 3]}}
        original_keys = set(tensor_map.keys())
        generate_idhash_tokenization_transformation(specs, tensor_map)
        assert set(tensor_map.keys()) == original_keys

    def test_empty_vocabulary_is_skipped(self) -> None:
        """An empty vocabulary skips tokenization for that feature."""
        tensor_map = {"store_id": torch.tensor([1, 2, 3], dtype=torch.long)}
        specs = {"store_id": {"vocabulary": []}}
        original_keys = set(tensor_map.keys())
        generate_idhash_tokenization_transformation(specs, tensor_map)
        assert set(tensor_map.keys()) == original_keys

    def test_custom_output_col(self) -> None:
        """output_col overrides the default tokenized_<feature> naming."""
        tensor_map = {"store_id": torch.tensor([100, 200, 300], dtype=torch.long)}
        specs = {
            "store_id": {
                "vocabulary": [100, 200, 300],
                "output_col": "my_custom_store_token",
            }
        }
        generate_idhash_tokenization_transformation(specs, tensor_map)
        assert "my_custom_store_token" in tensor_map
        assert "tokenized_store_id" not in tensor_map
        expected = torch.tensor([0, 1, 2], dtype=torch.long)
        torch.testing.assert_close(tensor_map["my_custom_store_token"], expected)


class TestUpdateOutputTensorMap:
    """update_output_tensor_map writes a tensor into the map under a given name."""

    def test_writes_tensor_under_name(self) -> None:
        """The tensor is stored under tensor_name in tensor_map."""
        tensor_map: dict = {}
        tensor = torch.tensor([1.0, 2.0])
        update_output_tensor_map(tensor, "my_feature", tensor_map)
        assert tensor_map["my_feature"] is tensor
