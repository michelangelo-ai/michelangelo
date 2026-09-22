"""Assemble evaluator output DataFrames into an ``EvaluationReport``.

:func:`generate_evaluation_report` renders one dataset; :func:`build_combined_report`
merges several, prefixing each dataset's name onto its segments so a single report
can compare them.
"""

from __future__ import annotations

import logging
from dataclasses import dataclass, field
from typing import TYPE_CHECKING

import pandas as pd

from michelangelo.lib.evaluator.report import (
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

if TYPE_CHECKING:
    from michelangelo.gen.api.v2.evaluation_report_pb2 import EvaluationReport

__all__ = [
    "DEFAULT_REPORT_TITLE",
    "DatasetResult",
    "build_combined_report",
    "generate_evaluation_report",
]

_logger = logging.getLogger(__name__)

DEFAULT_REPORT_TITLE = "Performance Evaluation Report"


@dataclass
class DatasetResult:
    """One evaluated dataset's output.

    Attributes:
        name: Dataset name, used as the table's ``DatasetName`` column and as the
            segment prefix in a combined report.
        summary_df: One row per segment, one column per metric.
        chart_df: Long-format curve data. Empty when the dataset produced no
            curves.
    """

    name: str
    summary_df: pd.DataFrame
    chart_df: pd.DataFrame = field(default_factory=pd.DataFrame)


def generate_evaluation_report(
    summary_df: pd.DataFrame,
    segment_columns: list[str] | None = None,
    title: str = DEFAULT_REPORT_TITLE,
    source_pipeline_type: str = "",
    chart_df: pd.DataFrame | None = None,
) -> EvaluationReport:
    """Generate an ``EvaluationReport`` from a summary metrics DataFrame.

    Args:
        summary_df: DataFrame containing metric values and optionally segment
            columns.
        segment_columns: Column names used for segmentation, if any.
        title: Title for the evaluation report.
        source_pipeline_type: Pipeline type label (e.g. ``"train"``, ``"retrain"``).
        chart_df: Long-format DataFrame of curve points with a ``segment`` column
            added by the caller. Rows with ``row_type="binary"`` drive the ROC,
            PR, and confusion-matrix charts; rows with ``row_type="regression"``
            drive Absolute Errors and Errors Chart. Pass ``None`` when no chart
            data is available.

    Returns:
        A populated ``EvaluationReport``. A report with no metric columns, or
        built from an empty ``summary_df``, comes back with no charts.
    """
    report = create_default_evaluation_report(title, source_pipeline_type)

    if summary_df.empty:
        _logger.warning("Empty summary DataFrame; returning report with no charts")
        return report

    if chart_df is not None and not chart_df.empty:
        missing = {"row_type", "segment"} - set(chart_df.columns)
        if missing:
            _logger.warning(
                "chart_df missing required columns %s; skipping charts. Got: %s",
                sorted(missing),
                sorted(chart_df.columns),
            )
            chart_df = None

    segment_columns = segment_columns or []
    metric_columns = [col for col in summary_df.columns if col not in segment_columns]

    if not metric_columns:
        _logger.warning("No metric columns found in summary DataFrame")
        return report

    column_names = segment_columns + metric_columns
    rows: list[list[tuple[str | float, str]]] = []
    for _, row in summary_df.iterrows():
        row_values: list[tuple[str | float, str]] = [
            (str(row[col]), "str_value") for col in segment_columns
        ]
        for col in metric_columns:
            val = row[col]
            if pd.isna(val):
                row_values.append(("N/A", "str_value"))
            elif isinstance(val, (int, float)):
                row_values.append((float(val), "number"))
            else:
                row_values.append((str(val), "str_value"))
        rows.append(row_values)

    report.spec.charts.append(
        create_metrics_table(
            column_names=column_names,
            values=rows,
            title="Metrics",
            section_id="Metrics",
        )
    )

    has_charts = chart_df is not None and not chart_df.empty
    has_binary = has_charts and (chart_df["row_type"] == "binary").any()
    has_regression = has_charts and (chart_df["row_type"] == "regression").any()

    inline_width = "50%"
    if has_binary:
        n_segments = chart_df[chart_df["row_type"] == "binary"]["segment"].nunique()
        if n_segments > 1:
            report.spec.charts.append(create_segment_filter_chart(chart_df))

        n_inline = 2 + (2 if has_regression else 0)
        inline_width = f"{100 // n_inline}%"

        report.spec.charts.append(create_roc_chart(chart_df, inline_width))
        report.spec.charts.append(create_pr_chart(chart_df, inline_width))

    if has_regression:
        report.spec.charts.append(create_absolute_errors_chart(chart_df, inline_width))
        report.spec.charts.append(create_errors_chart(chart_df, inline_width))

    if has_binary:
        binary_df = chart_df[chart_df["row_type"] == "binary"]
        pairs = (
            binary_df[["pred_col", "target_col"]]
            .drop_duplicates()
            .sort_values(["pred_col", "target_col"])
        )
        for _, pair in pairs.iterrows():
            report.spec.charts.append(
                create_confusion_matrix_chart(
                    chart_df, pair["pred_col"], pair["target_col"]
                )
            )
            report.spec.charts.append(
                create_threshold_filter_chart(
                    chart_df, pair["pred_col"], pair["target_col"]
                )
            )

    _logger.info(
        "Generated evaluation report with %d metric(s) and %d row(s)",
        len(metric_columns),
        len(rows),
    )
    return report


def build_combined_report(
    results: list[DatasetResult],
    segment_columns: list[str] | None,
) -> EvaluationReport:
    """Merge per-dataset results into a single combined ``EvaluationReport``.

    A single result is rendered as-is. Several are concatenated with a
    ``DatasetName`` column, and each dataset's chart segments are prefixed with
    its name so curves stay distinguishable in the shared segment filter.

    Args:
        results: One entry per evaluated dataset.
        segment_columns: Segment column names shared by every result.

    Returns:
        A populated ``EvaluationReport``.
    """
    seg_cols = list(segment_columns) if segment_columns else None

    if len(results) == 1:
        r = results[0]
        return generate_evaluation_report(
            summary_df=r.summary_df,
            segment_columns=seg_cols,
            title="Evaluation Report",
            chart_df=r.chart_df if not r.chart_df.empty else None,
        )

    dfs = []
    chart_dfs = []
    for r in results:
        df_copy = r.summary_df.copy()
        df_copy.insert(0, "DatasetName", r.name)
        dfs.append(df_copy)
        if not r.chart_df.empty:
            cdf = r.chart_df.copy()
            cdf["segment"] = r.name + " / " + cdf["segment"].astype(str)
            chart_dfs.append(cdf)

    combined_df = pd.concat(dfs, ignore_index=True)
    combined_chart_df = (
        pd.concat(chart_dfs, ignore_index=True) if chart_dfs else pd.DataFrame()
    )
    combined_seg_cols = ["DatasetName"] + (seg_cols or [])

    return generate_evaluation_report(
        summary_df=combined_df,
        segment_columns=combined_seg_cols,
        title="Evaluation Report",
        chart_df=combined_chart_df if not combined_chart_df.empty else None,
    )
