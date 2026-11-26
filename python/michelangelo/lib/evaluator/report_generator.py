"""Assemble evaluator output DataFrames into an ``EvaluationReport``.

:func:`generate_evaluation_report` renders one dataset; :func:`build_combined_report`
merges several, prefixing each dataset's name onto its segments so a single report
can compare them.
"""

from __future__ import annotations

import logging
from dataclasses import dataclass, field
from typing import TYPE_CHECKING, Any

import pandas as pd

from michelangelo.gen.api.v2.chart_pb2 import Chart
from michelangelo.lib.evaluator.evaluator import (
    SCORED_DIM_PREFIX,
    SCORED_ROWS_PREFIX,
)
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
from michelangelo.lib.evaluator.sanity import GLOBAL_SCOPE, segment_scope_label

if TYPE_CHECKING:
    from collections.abc import Mapping

    from michelangelo.gen.api.v2.evaluation_report_pb2 import EvaluationReport

__all__ = [
    "DEFAULT_REPORT_TITLE",
    "DatasetResult",
    "build_combined_report",
    "generate_evaluation_report",
    "split_scoring_details",
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
        extra_charts: Charts built elsewhere (e.g. embedding projections),
            appended after the generated ones.
    """

    name: str
    summary_df: pd.DataFrame
    chart_df: pd.DataFrame = field(default_factory=pd.DataFrame)
    extra_charts: list[Chart] = field(default_factory=list)


def split_scoring_details(
    summary_df: pd.DataFrame,
    metrics: Mapping[str, Any],
    segment_columns: list[str],
    global_first: bool,
) -> tuple[pd.DataFrame, Chart | None]:
    """Move the rows each embedding metric scored, and their dimension, into a table.

    Args:
        summary_df: The evaluator's summary, one row per scope.
        metrics: Maps each metric name to its instance, which supplies the sampling
            settings.
        segment_columns: Segment column names, used to label each row's scope.
        global_first: Whether the first row of a segmented ``summary_df`` is the
            aggregate. That row keeps its real ``datestr``, so its segment values
            cannot tell.

    Returns:
        ``summary_df`` without the evaluator's scored-rows and dimension columns,
        and a table with one row per scope and embedding metric -- or ``None``
        (and ``summary_df`` untouched) when no embedding metric reported any.
    """
    detail_columns = [
        col
        for col in summary_df.columns
        if col.startswith((SCORED_ROWS_PREFIX, SCORED_DIM_PREFIX))
    ]
    if not detail_columns:
        return summary_df, None
    names = [
        col.removeprefix(SCORED_ROWS_PREFIX)
        for col in detail_columns
        if col.startswith(SCORED_ROWS_PREFIX)
    ]
    segment_columns = [col for col in segment_columns if col in summary_df.columns]

    def cell(value: Any) -> tuple[str | float, str]:
        if value is None or pd.isna(value):
            return ("-", "str_value")
        return (float(value), "number")

    rows: list[list[tuple[str | float, str]]] = []
    for i, (_, row) in enumerate(summary_df.iterrows()):
        if not segment_columns or (global_first and i == 0):
            scope = GLOBAL_SCOPE
        else:
            scope = segment_scope_label(row, segment_columns)
        for name in names:
            metric = metrics.get(name)
            max_samples = getattr(metric, "max_samples", None)
            sampled = max_samples is not None
            rows.append(
                [
                    (scope, "str_value"),
                    (name, "str_value"),
                    cell(row[SCORED_DIM_PREFIX + name]),
                    cell(row[SCORED_ROWS_PREFIX + name]),
                    cell(max_samples) if sampled else ("all rows", "str_value"),
                    cell(getattr(metric, "num_pairs", None) if sampled else None),
                    cell(getattr(metric, "seed", None) if sampled else None),
                ]
            )
    table = create_metrics_table(
        column_names=[
            "scope",
            "metric",
            "dim",
            "rows scored",
            "sample limit",
            "num_pairs",
            "seed",
        ],
        values=rows,
        title="Embedding metric settings",
        section_id="EmbeddingMetricSettings",
    )
    return summary_df.drop(columns=detail_columns), table


def generate_evaluation_report(
    summary_df: pd.DataFrame,
    segment_columns: list[str] | None = None,
    title: str = DEFAULT_REPORT_TITLE,
    source_pipeline_type: str = "",
    chart_df: pd.DataFrame | None = None,
    extra_charts: list[Chart] | None = None,
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
        extra_charts: Ready-made charts appended after the generated ones.

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

    report.spec.charts.extend(extra_charts or [])

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
            extra_charts=r.extra_charts,
        )

    dfs = []
    chart_dfs = []
    extra_charts: list[Chart] = []
    for r in results:
        for chart in r.extra_charts:
            # Same projection per dataset: name each copy after its dataset.
            named = Chart()
            named.CopyFrom(chart)
            named.title = f"{r.name} / {chart.title}"
            named.section_id = f"{r.name} / {chart.section_id}"
            extra_charts.append(named)
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
        extra_charts=extra_charts,
    )
