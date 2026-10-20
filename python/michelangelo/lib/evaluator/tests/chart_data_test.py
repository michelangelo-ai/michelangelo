"""Tests for michelangelo.lib.evaluator.chart_data."""

from __future__ import annotations

from unittest import TestCase

import numpy as np
import pandas as pd
from pandas.errors import UndefinedVariableError

from michelangelo.lib.evaluator.chart_data import (
    apply_filter_expr,
    chart_segment_labels,
    compute_chart_data_for_group,
    filter_chart_df_by_segments,
    filter_ray_to_surviving_segments,
    filter_small_segments,
)
from michelangelo.workflow.schema.evaluator import (
    ColumnMapping,
    EvaluatorConfig,
    MetricConfig,
)


def _binary_config(filter_expr: str | None = None) -> EvaluatorConfig:
    """Config with one binary metric over (pred, label)."""
    return EvaluatorConfig(
        metrics=[
            MetricConfig(
                name="auroc",
                metric="torchmetrics.classification.BinaryAUROC",
                columns=ColumnMapping(prediction_col="pred", target_col="label"),
                filter_expr=filter_expr,
            )
        ]
    )


def _regression_config() -> EvaluatorConfig:
    """Config with one regression metric over (pred, label)."""
    return EvaluatorConfig(
        metrics=[
            MetricConfig(
                name="mse",
                metric="torchmetrics.regression.MeanSquaredError",
                columns=ColumnMapping(prediction_col="pred", target_col="label"),
            )
        ]
    )


_BINARY_DF = pd.DataFrame(
    {"pred": [0.9, 0.7, 0.4, 0.1], "label": [1, 0, 1, 0], "city": ["SF"] * 4}
)


class ApplyFilterExprTest(TestCase):
    """apply_filter_expr is the single df.query call site."""

    def test_no_expression_returns_the_frame_itself(self):
        """No expression returns the frame itself."""
        self.assertIs(apply_filter_expr(_BINARY_DF, None), _BINARY_DF)
        self.assertIs(apply_filter_expr(_BINARY_DF, ""), _BINARY_DF)

    def test_filters_rows(self):
        """Filters rows."""
        filtered = apply_filter_expr(_BINARY_DF, "label == 1")
        self.assertEqual(len(filtered), 2)

    def test_bad_expression_raises_by_default(self):
        """Bad expression raises by default."""
        with self.assertRaises(UndefinedVariableError):
            apply_filter_expr(_BINARY_DF, "no_such_column == 1")

    def test_bad_expression_returns_none_when_skipping(self):
        """Bad expression returns none when skipping."""
        result = apply_filter_expr(_BINARY_DF, "no_such_column == 1", on_error="skip")
        self.assertIsNone(result)

    def test_cache_is_reused(self):
        """Cache is reused."""
        cache: dict = {}
        first = apply_filter_expr(_BINARY_DF, "label == 1", cache=cache)
        second = apply_filter_expr(_BINARY_DF, "label == 1", cache=cache)
        self.assertIs(first, second)
        self.assertEqual(list(cache), ["label == 1"])

    def test_cache_remembers_a_failure(self):
        """Cache remembers a failure."""
        cache: dict = {}
        apply_filter_expr(_BINARY_DF, "nope == 1", cache=cache, on_error="skip")
        self.assertIsNone(cache["nope == 1"])


class ComputeBinaryCurvesTest(TestCase):
    """compute_chart_data_for_group builds ROC/PR points."""

    def test_one_point_per_distinct_prediction_plus_the_origin(self):
        """One point per distinct prediction plus the origin."""
        result = compute_chart_data_for_group(_BINARY_DF, _binary_config())
        self.assertEqual(set(result["row_type"]), {"binary"})
        self.assertEqual(len(result), 5)

    def test_curve_values_are_correct(self):
        """Curve values are correct."""
        result = compute_chart_data_for_group(_BINARY_DF, _binary_config())
        self.assertEqual(result["tp"].tolist(), [0.0, 1.0, 1.0, 2.0, 2.0])
        self.assertEqual(result["fp"].tolist(), [0.0, 0.0, 1.0, 1.0, 2.0])
        self.assertEqual(result["fn"].tolist(), [2.0, 1.0, 1.0, 0.0, 0.0])
        self.assertEqual(result["tn"].tolist(), [2.0, 2.0, 1.0, 1.0, 0.0])
        self.assertEqual(result["tpr"].tolist(), [0.0, 0.5, 0.5, 1.0, 1.0])
        self.assertEqual(result["fpr"].tolist(), [0.0, 0.0, 0.5, 0.5, 1.0])

    def test_undefined_first_precision_is_one(self):
        """Undefined first precision is one."""
        result = compute_chart_data_for_group(_BINARY_DF, _binary_config())
        self.assertEqual(result["precision"].iloc[0], 1.0)

    def test_thresholds_descend(self):
        """Thresholds descend."""
        result = compute_chart_data_for_group(_BINARY_DF, _binary_config())
        thresholds = result["threshold"].tolist()
        self.assertEqual(thresholds, sorted(thresholds, reverse=True))

    def test_regression_columns_are_nan(self):
        """Regression columns are nan."""
        result = compute_chart_data_for_group(_BINARY_DF, _binary_config())
        self.assertTrue(result["percentile"].isna().all())
        self.assertTrue(result["abs_error"].isna().all())

    def test_metric_names_are_joined(self):
        """Metric names are joined."""
        config = EvaluatorConfig(
            metrics=[
                MetricConfig(
                    name=name,
                    metric="torchmetrics.classification.BinaryAUROC",
                    columns=ColumnMapping(prediction_col="pred", target_col="label"),
                )
                for name in ("auroc", "ap")
            ]
        )
        result = compute_chart_data_for_group(_BINARY_DF, config)
        self.assertEqual(set(result["metric_names"]), {"auroc, ap"})

    def test_pairs_are_keyed_on_filter_expr(self):
        """Pairs are keyed on filter expr."""
        config = EvaluatorConfig(
            metrics=[
                MetricConfig(
                    name="all",
                    metric="torchmetrics.classification.BinaryAUROC",
                    columns=ColumnMapping(prediction_col="pred", target_col="label"),
                ),
                MetricConfig(
                    name="hi",
                    metric="torchmetrics.classification.BinaryAUROC",
                    columns=ColumnMapping(prediction_col="pred", target_col="label"),
                    filter_expr="pred > 0.5",
                ),
            ]
        )
        result = compute_chart_data_for_group(_BINARY_DF, config)
        self.assertEqual(set(result["filter_expr"].fillna("")), {"", "pred > 0.5"})

    def test_filter_is_applied_to_the_curve(self):
        """Filter is applied to the curve."""
        result = compute_chart_data_for_group(
            _BINARY_DF, _binary_config(filter_expr="pred > 0.5")
        )
        # Two rows survive, giving two distinct thresholds plus the origin.
        self.assertEqual(len(result), 3)

    def test_targets_outside_zero_one_are_skipped(self):
        """Targets outside zero one are skipped."""
        df = _BINARY_DF.assign(label=[1, -1, 1, 0])
        result = compute_chart_data_for_group(df, _binary_config())
        self.assertTrue(result.empty)

    def test_missing_column_is_skipped(self):
        """Missing column is skipped."""
        result = compute_chart_data_for_group(
            _BINARY_DF.drop(columns=["pred"]), _binary_config()
        )
        self.assertTrue(result.empty)

    def test_empty_filter_result_is_skipped(self):
        """Empty filter result is skipped."""
        result = compute_chart_data_for_group(
            _BINARY_DF, _binary_config(filter_expr="pred > 100")
        )
        self.assertTrue(result.empty)

    def test_unusable_filter_is_skipped_rather_than_raised(self):
        """Unusable filter is skipped rather than raised."""
        result = compute_chart_data_for_group(
            _BINARY_DF, _binary_config(filter_expr="nope > 1")
        )
        self.assertTrue(result.empty)

    def test_empty_result_still_has_the_expected_columns(self):
        """Empty result still has the expected columns."""
        result = compute_chart_data_for_group(_BINARY_DF, EvaluatorConfig())
        self.assertTrue(result.empty)
        for column in ("row_type", "threshold", "percentile", "filter_expr"):
            self.assertIn(column, result.columns)

    def test_large_input_is_subsampled(self):
        """Large input is subsampled."""
        n = 5000
        df = pd.DataFrame(
            {
                "pred": np.linspace(0, 1, n),
                "label": np.tile([0, 1], n // 2),
            }
        )
        result = compute_chart_data_for_group(df, _binary_config())
        self.assertLess(len(result), 1100)
        self.assertGreater(len(result), 100)

    def test_all_negative_targets_give_zero_tpr(self):
        """All negative targets give zero tpr."""
        df = pd.DataFrame({"pred": [0.9, 0.1], "label": [0, 0]})
        result = compute_chart_data_for_group(df, _binary_config())
        self.assertEqual(set(result["tpr"]), {0.0})


class ComputeRegressionErrorsTest(TestCase):
    """compute_chart_data_for_group builds error percentiles."""

    def test_one_row_per_percentile(self):
        """One row per percentile."""
        df = pd.DataFrame({"pred": [1.0, 2.0, 3.0], "label": [1.0, 1.0, 1.0]})
        result = compute_chart_data_for_group(df, _regression_config())
        self.assertEqual(len(result), 101)
        self.assertEqual(set(result["row_type"]), {"regression"})

    def test_error_percentiles_span_the_error_range(self):
        """Error percentiles span the error range."""
        df = pd.DataFrame({"pred": [1.0, 2.0, 3.0], "label": [1.0, 1.0, 1.0]})
        result = compute_chart_data_for_group(df, _regression_config())
        self.assertEqual(result["error"].iloc[0], 0.0)
        self.assertEqual(result["error"].iloc[-1], 2.0)
        self.assertEqual(result["abs_error"].iloc[-1], 2.0)

    def test_signed_and_absolute_errors_differ(self):
        """Signed and absolute errors differ."""
        df = pd.DataFrame({"pred": [0.0, 2.0], "label": [1.0, 1.0]})
        result = compute_chart_data_for_group(df, _regression_config())
        self.assertEqual(result["error"].iloc[0], -1.0)
        self.assertEqual(result["abs_error"].iloc[0], 1.0)

    def test_binary_columns_are_nan(self):
        """Binary columns are nan."""
        df = pd.DataFrame({"pred": [1.0], "label": [1.0]})
        result = compute_chart_data_for_group(df, _regression_config())
        self.assertTrue(result["threshold"].isna().all())
        self.assertTrue(result["tp"].isna().all())


class SegmentTaggingTest(TestCase):
    """Segment columns tag the computed rows."""

    def test_single_segment_column(self):
        """Single segment column."""
        result = compute_chart_data_for_group(
            _BINARY_DF, _binary_config(), segment_columns=["city"]
        )
        self.assertEqual(set(result["segment"]), {"SF"})

    def test_multiple_segment_columns_are_slash_joined(self):
        """Multiple segment columns are slash joined."""
        df = _BINARY_DF.assign(day="mon")
        result = compute_chart_data_for_group(
            df, _binary_config(), segment_columns=["city", "day"]
        )
        self.assertEqual(set(result["segment"]), {"SF / mon"})

    def test_empty_result_is_not_tagged(self):
        """Empty result is not tagged."""
        result = compute_chart_data_for_group(
            _BINARY_DF, EvaluatorConfig(), segment_columns=["city"]
        )
        self.assertNotIn("segment", result.columns)


class ChartSegmentLabelsTest(TestCase):
    """chart_segment_labels mirrors the tagging format."""

    def test_single_column(self):
        """Single column."""
        df = pd.DataFrame({"city": ["SF", "LA", "SF"]})
        self.assertEqual(chart_segment_labels(df, ["city"]), {"SF", "LA"})

    def test_multiple_columns(self):
        """Multiple columns."""
        df = pd.DataFrame({"city": ["SF"], "day": ["mon"]})
        self.assertEqual(chart_segment_labels(df, ["city", "day"]), {"SF / mon"})

    def test_values_are_stringified(self):
        """Values are stringified."""
        df = pd.DataFrame({"city_id": [1, 2]})
        self.assertEqual(chart_segment_labels(df, ["city_id"]), {"1", "2"})

    def test_no_columns_or_no_rows_gives_an_empty_set(self):
        """No columns or no rows gives an empty set."""
        df = pd.DataFrame({"city": ["SF"]})
        self.assertEqual(chart_segment_labels(df, []), set())
        self.assertEqual(chart_segment_labels(df.iloc[:0], ["city"]), set())


class FilterChartDfBySegmentsTest(TestCase):
    """filter_chart_df_by_segments keeps chart_df in step with the metrics."""

    def test_keeps_only_named_segments(self):
        """Keeps only named segments."""
        chart_df = pd.DataFrame({"segment": ["SF", "LA", "NY"], "tpr": [0.0] * 3})
        kept = filter_chart_df_by_segments(chart_df, {"SF", "NY"})
        self.assertEqual(kept["segment"].tolist(), ["SF", "NY"])

    def test_empty_frame_passes_through(self):
        """Empty frame passes through."""
        chart_df = pd.DataFrame(columns=["segment"])
        self.assertTrue(filter_chart_df_by_segments(chart_df, {"SF"}).empty)


class FilterSmallSegmentsTest(TestCase):
    """filter_small_segments applies the min_segment_row_count floor."""

    @staticmethod
    def _segments() -> pd.DataFrame:
        return pd.DataFrame(
            {"city": ["SF", "LA"], "auroc": [0.9, 0.8], "_segment_row_count": [500, 5]}
        )

    def test_drops_thin_segments(self):
        """Drops thin segments."""
        result = filter_small_segments(self._segments(), 100)
        self.assertEqual(result["city"].tolist(), ["SF"])

    def test_helper_column_is_always_removed(self):
        """Helper column is always removed."""
        self.assertNotIn(
            "_segment_row_count", filter_small_segments(self._segments(), 100).columns
        )
        self.assertNotIn(
            "_segment_row_count", filter_small_segments(self._segments(), 0).columns
        )

    def test_zero_threshold_keeps_everything(self):
        """Zero threshold keeps everything."""
        result = filter_small_segments(self._segments(), 0)
        self.assertEqual(len(result), 2)


class _FakeRayDataset:
    """Minimal stand-in exposing the map_batches surface used here."""

    def __init__(self, df: pd.DataFrame) -> None:
        self.df = df
        self.calls = 0

    def map_batches(self, fn, batch_format: str) -> _FakeRayDataset:
        self.calls += 1
        return _FakeRayDataset(fn(self.df))


class FilterRayToSurvivingSegmentsTest(TestCase):
    """filter_ray_to_surviving_segments drops already-filtered segments."""

    @staticmethod
    def _dataset() -> _FakeRayDataset:
        return _FakeRayDataset(
            pd.DataFrame({"city": ["SF", "LA", "SF"], "pred": [0.1, 0.2, 0.3]})
        )

    def test_keeps_only_surviving_rows(self):
        """Keeps only surviving rows."""
        segments = pd.DataFrame({"city": ["SF"]})
        result = filter_ray_to_surviving_segments(
            self._dataset(), segments, ["city"], 100
        )
        self.assertEqual(result.df["city"].tolist(), ["SF", "SF"])

    def test_keys_are_compared_as_strings(self):
        """Keys are compared as strings."""
        dataset = _FakeRayDataset(pd.DataFrame({"city_id": [1, 2]}))
        segments = pd.DataFrame({"city_id": ["1"]})
        result = filter_ray_to_surviving_segments(dataset, segments, ["city_id"], 100)
        self.assertEqual(result.df["city_id"].tolist(), [1])

    def test_disabled_filtering_is_a_noop(self):
        """Disabled filtering is a noop."""
        dataset = self._dataset()
        self.assertIs(
            filter_ray_to_surviving_segments(
                dataset, pd.DataFrame({"city": ["SF"]}), ["city"], 0
            ),
            dataset,
        )
        self.assertEqual(dataset.calls, 0)

    def test_empty_segment_frame_is_a_noop(self):
        """Empty segment frame is a noop."""
        dataset = self._dataset()
        self.assertIs(
            filter_ray_to_surviving_segments(
                dataset, pd.DataFrame(columns=["city"]), ["city"], 100
            ),
            dataset,
        )
