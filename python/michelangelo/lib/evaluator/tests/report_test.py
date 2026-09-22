"""Tests for michelangelo.lib.evaluator.report."""

from __future__ import annotations

from unittest import TestCase

import pandas as pd

from michelangelo.gen.api.v2.chart_pb2 import ChartType, DataPointSource, Filter
from michelangelo.lib.evaluator.report import (
    SOURCE_PIPELINE_TYPE_LABEL_NAME,
    create_absolute_errors_chart,
    create_confusion_matrix_chart,
    create_default_evaluation_report,
    create_errors_chart,
    create_metrics_table,
    create_pr_chart,
    create_roc_chart,
    create_segment_filter_chart,
    create_threshold_filter_chart,
)
from michelangelo.lib.evaluator.utils import sample_curve_indices


def _binary_rows(segment: str, thresholds: list[float], **overrides) -> pd.DataFrame:
    """Build binary chart rows for one segment, one prediction/target pair."""
    n = len(thresholds)
    data = {
        "row_type": ["binary"] * n,
        "segment": [segment] * n,
        "pred_col": ["pred"] * n,
        "target_col": ["label"] * n,
        "metric_names": ["auc"] * n,
        "threshold": thresholds,
        "tp": [float(i) for i in range(n)],
        "fp": [float(i) for i in range(n)],
        "fn": [float(n - i) for i in range(n)],
        "tn": [float(n - i) for i in range(n)],
        "tpr": [i / max(n - 1, 1) for i in range(n)],
        "fpr": [i / max(n - 1, 1) for i in range(n)],
        "precision": [1.0 - i / max(n - 1, 1) for i in range(n)],
    }
    data.update({k: [v] * n for k, v in overrides.items()})
    return pd.DataFrame(data)


def _regression_rows(segment: str, percentiles: list[float]) -> pd.DataFrame:
    """Build regression chart rows for one segment."""
    n = len(percentiles)
    return pd.DataFrame(
        {
            "row_type": ["regression"] * n,
            "segment": [segment] * n,
            "pred_col": ["pred"] * n,
            "target_col": ["label"] * n,
            "metric_names": ["mae"] * n,
            "percentile": percentiles,
            "error": [-1.0 * p for p in percentiles],
            "abs_error": list(percentiles),
        }
    )


class SampleCurveIndicesTest(TestCase):
    """Tests for sample_curve_indices."""

    def test_short_curve_is_returned_whole(self):
        """It returns every index when the curve is under budget."""
        self.assertEqual(sample_curve_indices(5, max_count=100), [0, 1, 2, 3, 4])

    def test_long_curve_is_capped(self):
        """It stays within two of the budget on a long curve."""
        self.assertLessEqual(len(sample_curve_indices(10_000, max_count=20)), 22)

    def test_endpoints_are_always_included(self):
        """It always keeps the first and last index."""
        idx = sample_curve_indices(10_000, max_count=20)
        self.assertEqual(idx[0], 0)
        self.assertEqual(idx[-1], 9999)

    def test_indices_are_sorted_and_unique(self):
        """It returns a sorted, deduplicated list."""
        idx = sample_curve_indices(500, max_count=40)
        self.assertEqual(idx, sorted(set(idx)))


class MetricsTableTest(TestCase):
    """Tests for create_metrics_table."""

    def test_columns_and_rows(self):
        """It copies the column names and one TableRow per input row."""
        chart = create_metrics_table(
            column_names=["segment", "auc"],
            values=[[("GLOBAL", "str_value"), (0.75, "number")]],
        )
        self.assertEqual(chart.chart_type, ChartType.CHART_TYPE_TABLE)
        self.assertEqual(list(chart.table.column_names), ["segment", "auc"])
        self.assertEqual(len(chart.table.rows), 1)

    def test_cell_field_selects_the_proto_field(self):
        """It writes each value into the ChartCellValue field the caller named."""
        chart = create_metrics_table(
            column_names=["segment", "auc"],
            values=[[("GLOBAL", "str_value"), (0.75, "number")]],
        )
        cells = chart.table.rows[0].values
        self.assertEqual(cells[0].str_value, "GLOBAL")
        self.assertAlmostEqual(cells[1].number, 0.75)

    def test_none_value_leaves_the_cell_empty(self):
        """It emits an empty cell rather than failing on a None value."""
        chart = create_metrics_table(column_names=["auc"], values=[[(None, "number")]])
        self.assertEqual(chart.table.rows[0].values[0].number, 0.0)

    def test_title_and_section_id_default(self):
        """It defaults both the title and the section id to 'Metrics'."""
        chart = create_metrics_table(column_names=[], values=[])
        self.assertEqual(chart.title, "Metrics")
        self.assertEqual(chart.section_id, "Metrics")


class DefaultEvaluationReportTest(TestCase):
    """Tests for create_default_evaluation_report."""

    def test_title_and_embedded_source(self):
        """It sets the title and marks the data points as embedded."""
        report = create_default_evaluation_report("My Report")
        self.assertEqual(report.spec.title, "My Report")
        self.assertEqual(
            report.spec.data_point_source.type,
            DataPointSource.DATA_POINT_SOURCE_TYPE_EMBEDDED,
        )

    def test_pipeline_type_label_is_set(self):
        """It records the pipeline type as a metadata label."""
        report = create_default_evaluation_report("t", "train")
        self.assertEqual(
            report.metadata.labels[SOURCE_PIPELINE_TYPE_LABEL_NAME], "train"
        )

    def test_no_label_when_pipeline_type_is_empty(self):
        """It adds no label when no pipeline type is given."""
        self.assertEqual(len(create_default_evaluation_report("t").metadata.labels), 0)

    def test_report_starts_with_no_charts(self):
        """It returns a report the caller appends charts to."""
        self.assertEqual(len(create_default_evaluation_report("t").spec.charts), 0)


class RocChartTest(TestCase):
    """Tests for create_roc_chart."""

    def test_single_segment_has_one_line_and_no_filter_group(self):
        """It emits one line and no segment filter group for a single segment."""
        chart = create_roc_chart(_binary_rows("GLOBAL", [0.9, 0.5, 0.1]))
        self.assertEqual(chart.chart_type, ChartType.CHART_TYPE_LINE_CHART)
        self.assertEqual(len(chart.line_chart.lines), 1)
        self.assertEqual(list(chart.filter_group), [])

    def test_line_name_is_the_metric_when_single_segment(self):
        """It names the line after the metric alone when there is one segment."""
        chart = create_roc_chart(_binary_rows("GLOBAL", [0.9, 0.1]))
        self.assertEqual(chart.line_chart.lines[0].line_name, "auc")

    def test_multi_segment_adds_filter_group_and_labels(self):
        """It adds the segment filter group and prefixes each line with its segment."""
        df = pd.concat(
            [_binary_rows("GLOBAL", [0.9, 0.1]), _binary_rows("city_1", [0.9, 0.1])],
            ignore_index=True,
        )
        chart = create_roc_chart(df)
        self.assertEqual(list(chart.filter_group), ["segment"])
        self.assertEqual(
            [line.line_name for line in chart.line_chart.lines],
            ["GLOBAL / auc", "city_1 / auc"],
        )

    def test_points_are_ordered_by_descending_threshold(self):
        """It plots points from the highest threshold down, not in row order."""
        df = _binary_rows("GLOBAL", [0.1, 0.9, 0.5])
        # fpr is assigned by row position, so descending threshold reorders it.
        expected = df.sort_values("threshold", ascending=False)["fpr"].tolist()
        chart = create_roc_chart(df)
        xs = [p.x for p in chart.line_chart.lines[0].points]
        self.assertEqual(xs, expected)
        self.assertNotEqual(xs, df["fpr"].tolist())

    def test_regression_rows_are_ignored(self):
        """It draws nothing from regression rows."""
        df = pd.concat(
            [_binary_rows("GLOBAL", [0.9, 0.1]), _regression_rows("GLOBAL", [0.5])],
            ignore_index=True,
        )
        self.assertEqual(len(create_roc_chart(df).line_chart.lines[0].points), 2)

    def test_inline_width_is_applied(self):
        """It records the requested inline width on the layout spec."""
        chart = create_roc_chart(_binary_rows("GLOBAL", [0.9]), inline_width="25%")
        self.assertTrue(chart.chart_layout_spec.is_inline)
        self.assertEqual(chart.chart_layout_spec.width, "25%")


class PrChartTest(TestCase):
    """Tests for create_pr_chart."""

    def test_undefined_precision_endpoint_is_dropped(self):
        """It drops the tp == fp == 0 row where precision is undefined."""
        df = _binary_rows("GLOBAL", [0.9, 0.5, 0.1])
        # _binary_rows makes the first row tp == fp == 0.
        self.assertEqual(len(create_pr_chart(df).line_chart.lines[0].points), 2)

    def test_axes_are_recall_and_precision(self):
        """It labels the axes Recall and Precision."""
        chart = create_pr_chart(_binary_rows("GLOBAL", [0.9, 0.5]))
        self.assertEqual(chart.line_chart.x_axis, "Recall")
        self.assertEqual(chart.line_chart.y_axis, "Precision")

    def test_segments_counted_before_the_endpoint_is_dropped(self):
        """It keeps the multi-segment labelling even after filtering rows."""
        df = pd.concat(
            [
                _binary_rows("GLOBAL", [0.9, 0.5]),
                _binary_rows("city_1", [0.9, 0.5]),
            ],
            ignore_index=True,
        )
        chart = create_pr_chart(df)
        self.assertEqual(list(chart.filter_group), ["segment"])


class ErrorChartTest(TestCase):
    """Tests for create_absolute_errors_chart and create_errors_chart."""

    def test_absolute_errors_plots_abs_error(self):
        """It plots abs_error against percentile."""
        chart = create_absolute_errors_chart(_regression_rows("GLOBAL", [0.5, 0.9]))
        ys = [p.y for p in chart.line_chart.lines[0].points]
        self.assertEqual(ys, [0.5, 0.9])

    def test_errors_plots_signed_error(self):
        """It plots the signed error against percentile."""
        chart = create_errors_chart(_regression_rows("GLOBAL", [0.5, 0.9]))
        ys = [p.y for p in chart.line_chart.lines[0].points]
        self.assertEqual(ys, [-0.5, -0.9])

    def test_points_are_ordered_by_ascending_percentile(self):
        """It plots percentiles left to right regardless of input order."""
        chart = create_errors_chart(_regression_rows("GLOBAL", [0.9, 0.1, 0.5]))
        xs = [p.x for p in chart.line_chart.lines[0].points]
        self.assertEqual(xs, [0.1, 0.5, 0.9])

    def test_binary_rows_are_ignored(self):
        """It draws nothing from binary rows."""
        df = pd.concat(
            [_regression_rows("GLOBAL", [0.5]), _binary_rows("GLOBAL", [0.9, 0.1])],
            ignore_index=True,
        )
        self.assertEqual(len(create_errors_chart(df).line_chart.lines[0].points), 1)

    def test_global_segment_is_ordered_first(self):
        """It lists GLOBAL before the other segments."""
        df = pd.concat(
            [
                _regression_rows("city_1", [0.5]),
                _regression_rows("GLOBAL", [0.5]),
            ],
            ignore_index=True,
        )
        chart = create_errors_chart(df)
        self.assertEqual(
            [line.line_name for line in chart.line_chart.lines],
            ["GLOBAL / mae", "city_1 / mae"],
        )


class ConfusionMatrixChartTest(TestCase):
    """Tests for create_confusion_matrix_chart."""

    def test_one_matrix_series_per_segment(self):
        """It emits one MatrixSeries per segment."""
        df = pd.concat(
            [
                _binary_rows("GLOBAL", [0.9, 0.5, 0.1]),
                _binary_rows("city_1", [0.9, 0.5, 0.1]),
            ],
            ignore_index=True,
        )
        chart = create_confusion_matrix_chart(df, "pred", "label")
        self.assertEqual(len(chart.confusion_matrix.matrix_series), 2)

    def test_each_matrix_has_two_rows_of_two_cells(self):
        """It lays each matrix out as TP/FN over FP/TN."""
        chart = create_confusion_matrix_chart(
            _binary_rows("GLOBAL", [0.9, 0.5, 0.1]), "pred", "label"
        )
        matrix = chart.confusion_matrix.matrix_series[0].matrices[0]
        self.assertEqual(len(matrix.matrix_rows), 2)
        self.assertEqual(
            [c.str_value for c in matrix.matrix_rows[0].cells], ["TP", "FN"]
        )
        self.assertEqual(
            [c.str_value for c in matrix.matrix_rows[1].cells], ["FP", "TN"]
        )

    def test_first_threshold_row_is_excluded(self):
        """It skips the all-negative endpoint, which has no matrix to show."""
        chart = create_confusion_matrix_chart(
            _binary_rows("GLOBAL", [0.9, 0.5, 0.1]), "pred", "label"
        )
        self.assertEqual(len(chart.confusion_matrix.matrix_series[0].matrices), 2)

    def test_segments_share_one_threshold_grid(self):
        """It samples every segment onto the same canonical thresholds."""
        df = pd.concat(
            [
                _binary_rows("GLOBAL", [0.9, 0.6, 0.3]),
                _binary_rows("city_1", [0.91, 0.61, 0.31]),
            ],
            ignore_index=True,
        )
        chart = create_confusion_matrix_chart(df, "pred", "label")
        names = [
            [m.name for m in series.matrices]
            for series in chart.confusion_matrix.matrix_series
        ]
        self.assertEqual(names[0], names[1])

    def test_filter_group_carries_threshold_and_segment(self):
        """It joins the matrix to its threshold slider and the segment filter."""
        df = pd.concat(
            [_binary_rows("GLOBAL", [0.9, 0.1]), _binary_rows("city_1", [0.9, 0.1])],
            ignore_index=True,
        )
        chart = create_confusion_matrix_chart(df, "pred", "label")
        self.assertEqual(list(chart.filter_group), ["threshold_pred_label", "segment"])

    def test_unknown_pair_yields_an_empty_matrix(self):
        """It returns an empty matrix rather than failing on an unmatched pair."""
        chart = create_confusion_matrix_chart(
            _binary_rows("GLOBAL", [0.9, 0.1]), "other", "label"
        )
        self.assertEqual(len(chart.confusion_matrix.matrix_series), 0)


class SegmentFilterChartTest(TestCase):
    """Tests for create_segment_filter_chart."""

    def test_segments_are_listed_with_global_first(self):
        """It orders GLOBAL ahead of the other segments."""
        df = pd.concat(
            [_binary_rows("city_1", [0.9]), _binary_rows("GLOBAL", [0.9])],
            ignore_index=True,
        )
        chart = create_segment_filter_chart(df)
        self.assertEqual(list(chart.filter.segment_values), ["GLOBAL", "city_1"])

    def test_global_is_preselected(self):
        """It preselects GLOBAL when it is present."""
        chart = create_segment_filter_chart(_binary_rows("GLOBAL", [0.9]))
        self.assertEqual(list(chart.filter.selected_segment_values), ["GLOBAL"])

    def test_nothing_preselected_without_global(self):
        """It preselects nothing when GLOBAL is absent."""
        chart = create_segment_filter_chart(_binary_rows("city_1", [0.9]))
        self.assertEqual(list(chart.filter.selected_segment_values), [])

    def test_filter_type_is_multi_segment(self):
        """It builds a MULTI_SEGMENT filter."""
        chart = create_segment_filter_chart(_binary_rows("GLOBAL", [0.9]))
        self.assertEqual(
            chart.filter.filter_type, Filter.FilterType.FILTER_TYPE_MULTI_SEGMENT
        )


class ThresholdFilterChartTest(TestCase):
    """Tests for create_threshold_filter_chart."""

    def test_values_ascend(self):
        """It lists the slider values low to high."""
        chart = create_threshold_filter_chart(
            _binary_rows("GLOBAL", [0.9, 0.5, 0.1]), "pred", "label"
        )
        values = list(chart.filter.filter_values)
        self.assertEqual(values, sorted(values))

    def test_first_threshold_row_is_excluded(self):
        """It drops the all-negative endpoint, matching the confusion matrix."""
        chart = create_threshold_filter_chart(
            _binary_rows("GLOBAL", [0.9, 0.5, 0.1]), "pred", "label"
        )
        self.assertEqual(len(chart.filter.filter_values), 2)

    def test_filter_id_matches_the_matrix_filter_group(self):
        """It uses the same filter id the confusion matrix subscribes to."""
        df = _binary_rows("GLOBAL", [0.9, 0.5])
        slider = create_threshold_filter_chart(df, "pred", "label")
        matrix = create_confusion_matrix_chart(df, "pred", "label")
        self.assertIn(slider.filter.filter_id, list(matrix.filter_group))

    def test_grid_is_taken_from_global(self):
        """It builds the grid from GLOBAL even when another segment sorts first."""
        df = pd.concat(
            [
                _binary_rows("city_1", [0.91, 0.61]),
                _binary_rows("GLOBAL", [0.9, 0.6]),
            ],
            ignore_index=True,
        )
        chart = create_threshold_filter_chart(df, "pred", "label")
        self.assertEqual([round(v, 2) for v in chart.filter.filter_values], [0.6])

    def test_unknown_pair_yields_no_values(self):
        """It returns an empty slider rather than failing on an unmatched pair."""
        chart = create_threshold_filter_chart(
            _binary_rows("GLOBAL", [0.9, 0.1]), "other", "label"
        )
        self.assertEqual(len(chart.filter.filter_values), 0)
