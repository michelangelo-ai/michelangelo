"""Unit tests for ``...tabular_assembler._private.data.fuse``."""

from __future__ import annotations

import unittest

import numpy as np

from michelangelo.workflow.tasks.tabular_assembler._private.data.fuse import (
    fuse_sample_data,
)


class FuseSampleDataTest(unittest.TestCase):
    """Tests for ``fuse_sample_data``."""

    def test_merges_tx_and_predictor_rows(self) -> None:
        """Columns from the tx and predictor rows are merged into one row."""
        tx_data = [{"a": np.array([1.0], dtype=np.float32)}]
        pred_data = [{"b": np.array([2.0], dtype=np.float32)}]
        rows = fuse_sample_data(tx_data, pred_data, columns_to_keep=["a", "b"])
        self.assertEqual(len(rows), 1)
        self.assertEqual(rows[0]["a"].tolist(), [1.0])
        self.assertEqual(rows[0]["b"].tolist(), [2.0])

    def test_tx_row_wins_over_predictor_for_same_key(self) -> None:
        """On a key collision, the tx row's value takes precedence."""
        rows = fuse_sample_data(
            [{"x": np.array([3.0], dtype=np.float32)}],
            [{"x": np.array([9.0], dtype=np.float32)}],
            columns_to_keep=["x"],
        )
        self.assertEqual(rows[0]["x"].tolist(), [3.0])

    def test_predictor_supplies_column_missing_on_tx_row(self) -> None:
        """A column absent from the tx row falls back to the predictor row."""
        tx_data = [{"a": np.array([1.0], dtype=np.float32)}]
        pred_data = [
            {
                "emb": np.array([0.0, 0.0], dtype=np.float32),
                "b": np.array([2.0], dtype=np.float32),
            }
        ]
        rows = fuse_sample_data(tx_data, pred_data, columns_to_keep=["a", "b"])
        self.assertEqual(set(rows[0].keys()), {"a", "b"})
        self.assertEqual(rows[0]["b"].tolist(), [2.0])

    def test_returns_concatenation_when_one_side_empty(self) -> None:
        """When one side has no rows, the other side's rows pass through unchanged."""
        tx_data = [{"a": np.array([1.0], dtype=np.float32)}]
        pred_data = [{"b": np.array([2.0], dtype=np.float32)}]
        for tx, pred, expected in (
            (tx_data, [], tx_data),
            ([], pred_data, pred_data),
        ):
            with self.subTest(tx=tx, pred=pred):
                self.assertEqual(fuse_sample_data(tx, pred, None), expected)

    def test_returns_empty_when_both_sides_empty(self) -> None:
        """With no tx rows and no predictor rows, the result is an empty list."""
        self.assertEqual(fuse_sample_data(None, None, None), [])

    def test_defaults_columns_to_keep_to_union_of_first_row_keys(self) -> None:
        """With no explicit columns_to_keep, all keys from both rows are kept."""
        rows = fuse_sample_data(
            [{"a": np.array([1.0], dtype=np.float32)}],
            [{"b": np.array([2.0], dtype=np.float32)}],
            columns_to_keep=None,
        )
        self.assertEqual(len(rows), 1)
        self.assertEqual(set(rows[0].keys()), {"a", "b"})
        self.assertEqual(rows[0]["a"].tolist(), [1.0])
        self.assertEqual(rows[0]["b"].tolist(), [2.0])

    def test_raises_when_required_column_missing_from_both_rows(self) -> None:
        """A requested column absent from both tx and predictor rows raises."""
        with self.assertRaises(ValueError):
            fuse_sample_data(
                [{"a": np.array([1.0], dtype=np.float32)}],
                [{}],
                columns_to_keep=["a", "b"],
            )

    def test_empty_columns_to_keep_yields_empty_dict_per_zipped_row(self) -> None:
        """An empty columns_to_keep list yields one empty dict per zipped row pair."""
        rows = fuse_sample_data(
            [{"a": np.array([1.0])}],
            [{"b": np.array([2.0])}],
            columns_to_keep=[],
        )
        self.assertEqual(rows, [{}])

    def test_zip_stops_at_shorter_list(self) -> None:
        """Fusion is truncated to the length of the shorter of the two row lists."""
        rows = fuse_sample_data(
            [
                {"x": np.array([1.0], dtype=np.float32)},
                {"x": np.array([2.0], dtype=np.float32)},
            ],
            [{"x": np.array([9.0], dtype=np.float32)}],
            columns_to_keep=["x"],
        )
        self.assertEqual(len(rows), 1)
        self.assertEqual(rows[0]["x"].tolist(), [1.0])

    def test_output_shares_array_with_source_when_from_tx(self) -> None:
        """A fused value taken from the tx row is the same array, not a copy."""
        a = np.array([1.0], dtype=np.float32)
        rows = fuse_sample_data(
            [{"x": a}],
            [{"x": np.array([2.0])}],
            columns_to_keep=["x"],
        )
        rows[0]["x"][0] = 99.0
        self.assertEqual(a.tolist(), [99.0])
