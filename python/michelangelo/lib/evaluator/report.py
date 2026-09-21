"""Chart builders for the evaluator's ``EvaluationReport``.

Each function here turns a slice of the evaluator's output DataFrames into one
``Chart`` proto. ``report_generator`` assembles them into a report; nothing in
this module reads or writes the report itself except
:func:`create_default_evaluation_report`.

Two DataFrame shapes are consumed:

``summary_df``
    One row per (segment, dataset) with one column per metric. Rendered as the
    metrics table.

``chart_df``
    Long format, one row per curve point, with a ``row_type`` column selecting
    the family: ``"binary"`` rows carry ``threshold``/``tp``/``fp``/``fn``/``tn``/
    ``tpr``/``fpr``/``precision`` and drive the ROC, PR, and confusion-matrix
    charts; ``"regression"`` rows carry ``percentile``/``error``/``abs_error``
    and drive the error charts. Both carry ``segment``, ``pred_col``,
    ``target_col``, and ``metric_names``.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

from michelangelo.gen.api.v2.chart_pb2 import (
    Chart,
    ChartCellValue,
    ChartDataPoint,
    ChartDataset,
    ChartType,
    ConfusionMatrix,
    DataPointSource,
    Filter,
    LineChart,
    Matrix,
    MatrixRow,
    MatrixSeries,
    Table,
    TableRow,
)
from michelangelo.gen.api.v2.evaluation_report_pb2 import EvaluationReport
from michelangelo.lib.evaluator.utils import sample_curve_indices

if TYPE_CHECKING:
    import pandas as pd

__all__ = [
    "SOURCE_PIPELINE_TYPE_LABEL_NAME",
    "create_absolute_errors_chart",
    "create_confusion_matrix_chart",
    "create_default_evaluation_report",
    "create_errors_chart",
    "create_metrics_table",
    "create_pr_chart",
    "create_roc_chart",
    "create_segment_filter_chart",
    "create_threshold_filter_chart",
]

SOURCE_PIPELINE_TYPE_LABEL_NAME = "michelangelo/SourcePipelineType"

_GLOBAL_SEGMENT = "GLOBAL"


def create_metrics_table(
    column_names: list[str],
    values: list[list[tuple[str | float, str]]],
    title: str = "Metrics",
    section_id: str = "Metrics",
) -> Chart:
    """Create the metrics table chart.

    Args:
        column_names: Names of the table columns, in order.
        values: One list per row, each holding ``(value, field)`` pairs in column
            order. ``field`` names the ``ChartCellValue`` field the value is
            written to -- ``"str_value"`` or ``"number"``. A ``None`` value
            leaves the cell empty.
        title: Chart title.
        section_id: Section ID used to group the chart in the UI.

    Returns:
        A ``Chart`` of type ``CHART_TYPE_TABLE``.
    """
    metrics_chart = Chart()
    metrics_chart.chart_type = ChartType.CHART_TYPE_TABLE
    if section_id is not None:
        metrics_chart.section_id = section_id
    if title is not None:
        metrics_chart.title = title

    metrics_table = Table()
    metrics_table.column_names.extend(column_names)

    for row in values:
        metric_row = TableRow()
        for value, field in row:
            chart_cell_value = ChartCellValue()
            if value is not None:
                setattr(chart_cell_value, field, value)
            metric_row.values.append(chart_cell_value)
        metrics_table.rows.append(metric_row)
    metrics_chart.table.CopyFrom(metrics_table)
    return metrics_chart


def create_default_evaluation_report(
    title: str, source_pipeline_type: str = ""
) -> EvaluationReport:
    """Create an empty evaluation report for charts to be appended to.

    Args:
        title: Report title, shown as the page heading.
        source_pipeline_type: Optional pipeline-type label (e.g. ``"train"``),
            recorded under ``SOURCE_PIPELINE_TYPE_LABEL_NAME``.

    Returns:
        An ``EvaluationReport`` with an embedded data-point source and no charts.
    """
    evaluation_report = EvaluationReport()
    evaluation_report.spec.data_point_source.type = (
        DataPointSource.DATA_POINT_SOURCE_TYPE_EMBEDDED
    )
    evaluation_report.spec.title = title
    if source_pipeline_type:
        evaluation_report.metadata.labels[SOURCE_PIPELINE_TYPE_LABEL_NAME] = (
            source_pipeline_type
        )

    return evaluation_report


def _binary_segments(chart_df: pd.DataFrame) -> list[str]:
    """Return unique segment labels from binary rows, GLOBAL first."""
    segs = chart_df.loc[chart_df["row_type"] == "binary", "segment"].unique().tolist()
    return sorted(segs, key=lambda s: (s != _GLOBAL_SEGMENT, s))


def _regression_segments(chart_df: pd.DataFrame) -> list[str]:
    """Return unique segment labels from regression rows, GLOBAL first."""
    segs = chart_df["segment"].unique().tolist()
    return sorted(segs, key=lambda s: (s != _GLOBAL_SEGMENT, s))


def _curve_chart(
    chart_df: pd.DataFrame,
    *,
    row_type: str,
    segments: list[str],
    title: str,
    section_id: str,
    x_axis: str,
    y_axis: str,
    x_col: str,
    y_col: str,
    sort_by: str,
    ascending: bool,
    inline_width: str,
) -> Chart:
    """Build a line chart with one line per (segment, metric pair).

    Shared by the ROC, PR, and both error charts -- they differ only in which
    columns they read and how the points are ordered.
    """
    multi = len(segments) > 1

    chart = Chart()
    chart.chart_type = ChartType.CHART_TYPE_LINE_CHART
    chart.title = title
    chart.section_id = section_id
    if multi:
        chart.filter_group.append("segment")

    line_chart = LineChart()
    line_chart.x_axis = x_axis
    line_chart.y_axis = y_axis

    typed_df = chart_df[chart_df["row_type"] == row_type]
    for seg in segments:
        seg_df = typed_df[typed_df["segment"] == seg]
        grouped = seg_df.groupby(["pred_col", "target_col", "metric_names"])
        for (_pred_col, _target_col, metric_names), pair_df in grouped:
            label = f"{seg} / {metric_names}" if multi else metric_names
            line = ChartDataset()
            line.line_name = label
            if multi:
                line.filter_value.append(ChartCellValue(str_value=seg))
            ordered = pair_df.sort_values(sort_by, ascending=ascending)
            for _, row in ordered.iterrows():
                line.points.append(
                    ChartDataPoint(x=float(row[x_col]), y=float(row[y_col]))
                )
            line_chart.lines.append(line)

    chart.line_chart.CopyFrom(line_chart)
    chart.chart_layout_spec.is_inline = True
    chart.chart_layout_spec.width = inline_width
    return chart


def create_roc_chart(chart_df: pd.DataFrame, inline_width: str = "50%") -> Chart:
    """Create a ROC curve chart with one line per (segment, binary pair).

    Args:
        chart_df: Long-format chart data; only ``row_type == "binary"`` is read.
        inline_width: CSS width for side-by-side layout.

    Returns:
        A ``Chart`` of type ``CHART_TYPE_LINE_CHART``.
    """
    return _curve_chart(
        chart_df,
        row_type="binary",
        segments=_binary_segments(chart_df),
        title="ROC Curve",
        section_id="ROCChart",
        x_axis="False Positive Rate",
        y_axis="True Positive Rate",
        x_col="fpr",
        y_col="tpr",
        sort_by="threshold",
        ascending=False,
        inline_width=inline_width,
    )


def create_pr_chart(chart_df: pd.DataFrame, inline_width: str = "50%") -> Chart:
    """Create a Precision-Recall chart with one line per (segment, binary pair).

    Args:
        chart_df: Long-format chart data; only ``row_type == "binary"`` is read.
        inline_width: CSS width for side-by-side layout.

    Returns:
        A ``Chart`` of type ``CHART_TYPE_LINE_CHART``.
    """
    # Drop the tp == fp == 0 endpoint: precision is undefined there, and it plots
    # at recall 0 as a vertical line. ROC keeps it -- (0, 0) is that curve's origin.
    binary_df = chart_df[chart_df["row_type"] == "binary"]
    plotted = binary_df[(binary_df["tp"] + binary_df["fp"]) > 0]
    return _curve_chart(
        plotted,
        row_type="binary",
        segments=_binary_segments(chart_df),
        title="Precision Recall",
        section_id="PRChart",
        x_axis="Recall",
        y_axis="Precision",
        x_col="tpr",
        y_col="precision",
        sort_by="threshold",
        ascending=False,
        inline_width=inline_width,
    )


def create_absolute_errors_chart(
    chart_df: pd.DataFrame, inline_width: str = "50%"
) -> Chart:
    """Create an Absolute Errors chart: ``|prediction - target|`` by percentile.

    Args:
        chart_df: Long-format chart data; only ``row_type == "regression"`` is read.
        inline_width: CSS width for side-by-side layout.

    Returns:
        A ``Chart`` of type ``CHART_TYPE_LINE_CHART``.
    """
    reg_df = chart_df[chart_df["row_type"] == "regression"]
    return _curve_chart(
        chart_df,
        row_type="regression",
        segments=_regression_segments(reg_df),
        title="Absolute Errors",
        section_id="AbsErrors",
        x_axis="percentile",
        y_axis="error",
        x_col="percentile",
        y_col="abs_error",
        sort_by="percentile",
        ascending=True,
        inline_width=inline_width,
    )


def create_errors_chart(chart_df: pd.DataFrame, inline_width: str = "50%") -> Chart:
    """Create an Errors chart: signed ``prediction - target`` by percentile.

    Args:
        chart_df: Long-format chart data; only ``row_type == "regression"`` is read.
        inline_width: CSS width for side-by-side layout.

    Returns:
        A ``Chart`` of type ``CHART_TYPE_LINE_CHART``.
    """
    reg_df = chart_df[chart_df["row_type"] == "regression"]
    return _curve_chart(
        chart_df,
        row_type="regression",
        segments=_regression_segments(reg_df),
        title="Errors Chart",
        section_id="ErrorsChart",
        x_axis="percentile",
        y_axis="error",
        x_col="percentile",
        y_col="error",
        sort_by="percentile",
        ascending=True,
        inline_width=inline_width,
    )


def create_confusion_matrix_chart(
    chart_df: pd.DataFrame,
    pred_col: str,
    target_col: str,
) -> Chart:
    """Create a confusion-matrix chart with one ``MatrixSeries`` per segment.

    Every segment is sampled onto one canonical threshold grid, taken from
    GLOBAL when present, so the threshold slider moves all segments together.
    Each segment contributes the row nearest to each canonical threshold.

    Args:
        chart_df: Long-format chart data; only ``row_type == "binary"`` is read.
        pred_col: Prediction column the matrix is built for.
        target_col: Target column the matrix is built for.

    Returns:
        A ``Chart`` of type ``CHART_TYPE_CONFUSION_MATRIX``. The matrix is empty
        when the pair has no threshold rows.
    """
    binary_df = chart_df[
        (chart_df["row_type"] == "binary")
        & (chart_df["pred_col"] == pred_col)
        & (chart_df["target_col"] == target_col)
    ]
    segments = _binary_segments(binary_df)
    multi = len(segments) > 1
    filter_id = f"threshold_{pred_col}_{target_col}"

    chart = Chart()
    chart.chart_type = ChartType.CHART_TYPE_CONFUSION_MATRIX
    chart.title = f"Confusion Matrix - {pred_col} vs {target_col}"
    chart.section_id = f"ConfusionMatrix_{pred_col}_{target_col}"
    chart.filter_group.append(filter_id)
    if multi:
        chart.filter_group.append("segment")

    canonical_thresholds = _canonical_thresholds(binary_df, segments)
    if not canonical_thresholds:
        chart.confusion_matrix.CopyFrom(ConfusionMatrix())
        return chart

    confusion_matrix = ConfusionMatrix()
    for seg in segments:
        seg_df = binary_df[binary_df["segment"] == seg].sort_values(
            "threshold", ascending=False
        )
        if seg_df.empty:
            continue
        # The first row is the all-negative endpoint; it has no matrix to show.
        thresholds_rest = seg_df["threshold"].tolist()[1:]
        tp_rest = seg_df["tp"].tolist()[1:]
        fp_rest = seg_df["fp"].tolist()[1:]
        fn_rest = seg_df["fn"].tolist()[1:]
        tn_rest = seg_df["tn"].tolist()[1:]

        matrix_series = MatrixSeries()
        for canonical_t in canonical_thresholds:
            if not thresholds_rest:
                continue
            nearest_i = min(
                range(len(thresholds_rest)),
                key=lambda i, t=canonical_t: abs(thresholds_rest[i] - t),
            )
            matrix = Matrix()
            matrix.name = f"t={canonical_t:.3f}"
            matrix.filter_values.append(ChartCellValue(number=float(canonical_t)))
            if multi:
                matrix.filter_values.append(ChartCellValue(str_value=seg))
            row_pos = MatrixRow()
            row_pos.cells.append(
                ChartCellValue(str_value="TP", number=float(tp_rest[nearest_i]))
            )
            row_pos.cells.append(
                ChartCellValue(str_value="FN", number=float(fn_rest[nearest_i]))
            )
            row_neg = MatrixRow()
            row_neg.cells.append(
                ChartCellValue(str_value="FP", number=float(fp_rest[nearest_i]))
            )
            row_neg.cells.append(
                ChartCellValue(str_value="TN", number=float(tn_rest[nearest_i]))
            )
            matrix.matrix_rows.extend([row_pos, row_neg])
            matrix_series.matrices.append(matrix)
        confusion_matrix.matrix_series.append(matrix_series)

    chart.confusion_matrix.CopyFrom(confusion_matrix)
    return chart


def _canonical_thresholds(binary_df: pd.DataFrame, segments: list[str]) -> list[float]:
    """Return the sampled threshold grid, taken from GLOBAL when it is present."""
    ordered = [_GLOBAL_SEGMENT] + [s for s in segments if s != _GLOBAL_SEGMENT]
    for seg in ordered:
        seg_df = binary_df[binary_df["segment"] == seg].sort_values(
            "threshold", ascending=False
        )
        if not seg_df.empty:
            t_rest = seg_df["threshold"].tolist()[1:]
            return [t_rest[i] for i in sample_curve_indices(len(t_rest))][::-1]
    return []


def create_segment_filter_chart(chart_df: pd.DataFrame) -> Chart:
    """Create a MULTI_SEGMENT filter for toggling segments on the curve charts.

    Args:
        chart_df: Long-format chart data; only ``row_type == "binary"`` is read.

    Returns:
        A ``Chart`` of type ``CHART_TYPE_FILTER``, with GLOBAL preselected when
        it is one of the segments.
    """
    segments = _binary_segments(chart_df)

    segment_filter = Filter()
    segment_filter.filter_id = "segment"
    segment_filter.filter_type = Filter.FilterType.FILTER_TYPE_MULTI_SEGMENT
    for name in segments:
        segment_filter.segment_values.append(name)
    if _GLOBAL_SEGMENT in segments:
        segment_filter.selected_segment_values.append(_GLOBAL_SEGMENT)

    chart = Chart()
    chart.chart_type = ChartType.CHART_TYPE_FILTER
    chart.section_id = "SegmentFilter"
    chart.title = "Segment Filter"
    chart.filter.CopyFrom(segment_filter)
    return chart


def create_threshold_filter_chart(
    chart_df: pd.DataFrame,
    pred_col: str,
    target_col: str,
) -> Chart:
    """Create a threshold slider filter adjacent to a confusion matrix.

    Args:
        chart_df: Long-format chart data; only ``row_type == "binary"`` is read.
        pred_col: Prediction column the slider drives.
        target_col: Target column the slider drives.

    Returns:
        A ``Chart`` of type ``CHART_TYPE_FILTER`` whose values are the same
        sampled grid the confusion matrix was built on, ascending.
    """
    binary_df = chart_df[
        (chart_df["row_type"] == "binary")
        & (chart_df["pred_col"] == pred_col)
        & (chart_df["target_col"] == target_col)
    ]
    filter_id = f"threshold_{pred_col}_{target_col}"

    segments = _binary_segments(binary_df)
    threshold_vals: list[float] = []
    for seg in [_GLOBAL_SEGMENT] + [s for s in segments if s != _GLOBAL_SEGMENT]:
        seg_df = binary_df[binary_df["segment"] == seg].sort_values(
            "threshold", ascending=False
        )
        if not seg_df.empty:
            threshold_vals = seg_df["threshold"].tolist()[1:]
            break

    threshold_filter = Filter()
    threshold_filter.filter_id = filter_id
    threshold_filter.filter_type = Filter.FilterType.FILTER_TYPE_THRESHOLD
    sample_idx = sample_curve_indices(len(threshold_vals))
    for i in sample_idx[::-1]:
        threshold_filter.filter_values.append(threshold_vals[i])

    chart = Chart()
    chart.chart_type = ChartType.CHART_TYPE_FILTER
    chart.section_id = f"ThresholdFilter_{pred_col}_{target_col}"
    chart.title = f"Threshold - {pred_col} vs {target_col}"
    chart.filter.CopyFrom(threshold_filter)
    return chart
