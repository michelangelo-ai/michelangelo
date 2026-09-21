"""Tests for michelangelo.lib.evaluator.report_generator."""

from __future__ import annotations

from unittest import TestCase

import pandas as pd

from michelangelo.gen.api.v2.chart_pb2 import ChartType
from michelangelo.lib.evaluator.report_generator import (
    DEFAULT_REPORT_TITLE,
    DatasetResult,
    build_combined_report,
    generate_evaluation_report,
)


def _binary_rows(segment: str, pred_col: str = "pred") -> pd.DataFrame:
    """Build three binary chart rows for one segment and prediction column."""
    return pd.DataFrame(
        {
            "row_type": ["binary"] * 3,
            "segment": [segment] * 3,
            "pred_col": [pred_col] * 3,
            "target_col": ["label"] * 3,
            "metric_names": ["auc"] * 3,
            "threshold": [0.9, 0.5, 0.1],
            "tp": [0.0, 1.0, 2.0],
            "fp": [0.0, 1.0, 2.0],
            "fn": [3.0, 2.0, 1.0],
            "tn": [3.0, 2.0, 1.0],
            "tpr": [0.0, 0.5, 1.0],
            "fpr": [0.0, 0.5, 1.0],
            "precision": [1.0, 0.5, 0.0],
        }
    )


def _regression_rows(segment: str) -> pd.DataFrame:
    """Build two regression chart rows for one segment."""
    return pd.DataFrame(
        {
            "row_type": ["regression"] * 2,
            "segment": [segment] * 2,
            "pred_col": ["pred"] * 2,
            "target_col": ["label"] * 2,
            "metric_names": ["mae"] * 2,
            "percentile": [0.5, 0.9],
            "error": [-0.5, -0.9],
            "abs_error": [0.5, 0.9],
        }
    )


def _summary(**cols) -> pd.DataFrame:
    """Build a one-row summary DataFrame from the given columns."""
    return pd.DataFrame({k: [v] for k, v in cols.items()})


def _sections(report) -> list[str]:
    """Return the section ids of every chart in the report, in order."""
    return [chart.section_id for chart in report.spec.charts]


class SummaryTableTest(TestCase):
    """Tests for the metrics table generate_evaluation_report always emits."""

    def test_empty_summary_yields_no_charts(self):
        """It returns a chartless report when there is nothing to tabulate."""
        report = generate_evaluation_report(pd.DataFrame())
        self.assertEqual(len(report.spec.charts), 0)

    def test_default_title_is_used(self):
        """It titles the report with DEFAULT_REPORT_TITLE unless told otherwise."""
        report = generate_evaluation_report(pd.DataFrame())
        self.assertEqual(report.spec.title, DEFAULT_REPORT_TITLE)

    def test_metrics_table_is_the_only_chart_without_chart_df(self):
        """It emits just the metrics table when no curve data is supplied."""
        report = generate_evaluation_report(_summary(auc=0.75))
        self.assertEqual(_sections(report), ["Metrics"])
        self.assertEqual(report.spec.charts[0].chart_type, ChartType.CHART_TYPE_TABLE)

    def test_segment_columns_come_first(self):
        """It orders segment columns ahead of metric columns."""
        report = generate_evaluation_report(
            _summary(auc=0.75, city="SF"), segment_columns=["city"]
        )
        self.assertEqual(
            list(report.spec.charts[0].table.column_names), ["city", "auc"]
        )

    def test_numeric_metric_is_written_as_a_number(self):
        """It stores a numeric metric in the number field."""
        report = generate_evaluation_report(_summary(auc=0.75))
        self.assertAlmostEqual(
            report.spec.charts[0].table.rows[0].values[0].number, 0.75
        )

    def test_nan_metric_renders_as_na(self):
        """It renders a missing metric as the string N/A."""
        report = generate_evaluation_report(_summary(auc=float("nan")))
        self.assertEqual(report.spec.charts[0].table.rows[0].values[0].str_value, "N/A")

    def test_non_numeric_metric_is_stringified(self):
        """It stores a non-numeric metric in the string field."""
        report = generate_evaluation_report(_summary(note="skipped"))
        self.assertEqual(
            report.spec.charts[0].table.rows[0].values[0].str_value, "skipped"
        )

    def test_summary_of_only_segment_columns_yields_no_charts(self):
        """It bails out when every column is a segment column."""
        report = generate_evaluation_report(
            _summary(city="SF"), segment_columns=["city"]
        )
        self.assertEqual(len(report.spec.charts), 0)


class ChartSelectionTest(TestCase):
    """Tests for which charts generate_evaluation_report appends."""

    def test_binary_only_adds_roc_and_pr(self):
        """It adds the ROC and PR charts for binary curve data."""
        report = generate_evaluation_report(
            _summary(auc=0.75), chart_df=_binary_rows("GLOBAL")
        )
        self.assertEqual(
            _sections(report),
            [
                "Metrics",
                "ROCChart",
                "PRChart",
                "ConfusionMatrix_pred_label",
                "ThresholdFilter_pred_label",
            ],
        )

    def test_regression_only_adds_the_error_charts(self):
        """It adds both error charts for regression curve data."""
        report = generate_evaluation_report(
            _summary(mae=0.2), chart_df=_regression_rows("GLOBAL")
        )
        self.assertEqual(_sections(report), ["Metrics", "AbsErrors", "ErrorsChart"])

    def test_binary_and_regression_share_the_row(self):
        """It narrows the inline width to a quarter when four charts share a row."""
        chart_df = pd.concat(
            [_binary_rows("GLOBAL"), _regression_rows("GLOBAL")], ignore_index=True
        )
        report = generate_evaluation_report(_summary(auc=0.75), chart_df=chart_df)
        widths = {
            chart.chart_layout_spec.width
            for chart in report.spec.charts
            if chart.chart_layout_spec.is_inline
        }
        self.assertEqual(widths, {"25%"})

    def test_two_inline_charts_take_half_the_row(self):
        """It gives each chart half the row when only two are inline."""
        report = generate_evaluation_report(
            _summary(auc=0.75), chart_df=_binary_rows("GLOBAL")
        )
        widths = {
            chart.chart_layout_spec.width
            for chart in report.spec.charts
            if chart.chart_layout_spec.is_inline
        }
        self.assertEqual(widths, {"50%"})

    def test_multi_segment_adds_the_segment_filter(self):
        """It adds the segment filter once more than one segment is present."""
        chart_df = pd.concat(
            [_binary_rows("GLOBAL"), _binary_rows("city_1")], ignore_index=True
        )
        report = generate_evaluation_report(_summary(auc=0.75), chart_df=chart_df)
        self.assertEqual(_sections(report)[1], "SegmentFilter")

    def test_single_segment_has_no_segment_filter(self):
        """It omits the segment filter when there is nothing to toggle."""
        report = generate_evaluation_report(
            _summary(auc=0.75), chart_df=_binary_rows("GLOBAL")
        )
        self.assertNotIn("SegmentFilter", _sections(report))

    def test_one_matrix_and_slider_per_prediction_pair(self):
        """It adds a confusion matrix and slider for each prediction/target pair."""
        chart_df = pd.concat(
            [_binary_rows("GLOBAL", "p2"), _binary_rows("GLOBAL", "p1")],
            ignore_index=True,
        )
        report = generate_evaluation_report(_summary(auc=0.75), chart_df=chart_df)
        self.assertEqual(
            _sections(report)[-4:],
            [
                "ConfusionMatrix_p1_label",
                "ThresholdFilter_p1_label",
                "ConfusionMatrix_p2_label",
                "ThresholdFilter_p2_label",
            ],
        )

    def test_empty_chart_df_is_treated_as_absent(self):
        """It emits only the metrics table for an empty chart frame."""
        report = generate_evaluation_report(_summary(auc=0.75), chart_df=pd.DataFrame())
        self.assertEqual(_sections(report), ["Metrics"])

    def test_chart_df_missing_required_columns_is_skipped(self):
        """It drops unusable curve data instead of raising."""
        bad = pd.DataFrame({"row_type": ["binary"], "threshold": [0.5]})
        with self.assertLogs(
            "michelangelo.lib.evaluator.report_generator",
            level="WARNING",
        ) as logs:
            report = generate_evaluation_report(_summary(auc=0.75), chart_df=bad)
        self.assertEqual(_sections(report), ["Metrics"])
        self.assertIn("missing required columns", logs.output[0])


class BuildCombinedReportTest(TestCase):
    """Tests for build_combined_report."""

    def test_single_result_is_rendered_as_is(self):
        """It adds no DatasetName column for a lone dataset."""
        report = build_combined_report(
            [DatasetResult(name="test", summary_df=_summary(auc=0.75))], None
        )
        self.assertEqual(list(report.spec.charts[0].table.column_names), ["auc"])

    def test_single_result_keeps_segment_columns(self):
        """It passes the caller's segment columns straight through."""
        report = build_combined_report(
            [DatasetResult(name="test", summary_df=_summary(auc=0.75, city="SF"))],
            ["city"],
        )
        self.assertEqual(
            list(report.spec.charts[0].table.column_names), ["city", "auc"]
        )

    def test_multiple_results_gain_a_dataset_column(self):
        """It prepends DatasetName when several datasets are merged."""
        report = build_combined_report(
            [
                DatasetResult(name="train", summary_df=_summary(auc=0.80)),
                DatasetResult(name="test", summary_df=_summary(auc=0.75)),
            ],
            None,
        )
        table = report.spec.charts[0].table
        self.assertEqual(list(table.column_names), ["DatasetName", "auc"])
        self.assertEqual(
            [row.values[0].str_value for row in table.rows], ["train", "test"]
        )

    def test_dataset_name_precedes_the_segment_columns(self):
        """It orders DatasetName ahead of the caller's segment columns."""
        report = build_combined_report(
            [
                DatasetResult(name="train", summary_df=_summary(auc=0.8, city="SF")),
                DatasetResult(name="test", summary_df=_summary(auc=0.7, city="SF")),
            ],
            ["city"],
        )
        self.assertEqual(
            list(report.spec.charts[0].table.column_names),
            ["DatasetName", "city", "auc"],
        )

    def test_chart_segments_are_prefixed_with_the_dataset_name(self):
        """It namespaces each dataset's curves so they stay distinguishable."""
        report = build_combined_report(
            [
                DatasetResult(
                    name="train",
                    summary_df=_summary(auc=0.8),
                    chart_df=_binary_rows("GLOBAL"),
                ),
                DatasetResult(
                    name="test",
                    summary_df=_summary(auc=0.7),
                    chart_df=_binary_rows("GLOBAL"),
                ),
            ],
            None,
        )
        segment_filter = report.spec.charts[1].filter
        self.assertEqual(
            sorted(segment_filter.segment_values),
            ["test / GLOBAL", "train / GLOBAL"],
        )

    def test_datasets_without_curves_contribute_only_rows(self):
        """It merges a dataset that produced no curve data without failing."""
        report = build_combined_report(
            [
                DatasetResult(
                    name="train",
                    summary_df=_summary(auc=0.8),
                    chart_df=_binary_rows("GLOBAL"),
                ),
                DatasetResult(name="test", summary_df=_summary(auc=0.7)),
            ],
            None,
        )
        self.assertEqual(len(report.spec.charts[0].table.rows), 2)
        self.assertIn("ROCChart", _sections(report))

    def test_no_curves_at_all_yields_only_the_table(self):
        """It emits just the metrics table when no dataset produced curves."""
        report = build_combined_report(
            [
                DatasetResult(name="train", summary_df=_summary(auc=0.8)),
                DatasetResult(name="test", summary_df=_summary(auc=0.7)),
            ],
            None,
        )
        self.assertEqual(_sections(report), ["Metrics"])
