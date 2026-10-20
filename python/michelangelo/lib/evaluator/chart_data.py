"""Compute the long-format ``chart_df`` that backs the evaluation report charts.

:func:`compute_chart_data_for_group` runs over one group of rows -- a segment, a
GLOBAL pass, or a whole dataset -- and emits one row per curve point. It returns
a DataFrame rather than protos so it can run under a distributed ``map_groups``
and be concatenated before the report is built.

Curves are computed on exactly the rows the corresponding scalar metrics see:
every pair is keyed on ``filter_expr`` as well as its columns, because two
metrics can share prediction and target columns while filtering differently.
"""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING, Any

import numpy as np
import pandas as pd

from michelangelo.lib.evaluator.metrics import infer_task_type_from_path
from michelangelo.lib.evaluator.utils import sample_curve_indices

if TYPE_CHECKING:
    from michelangelo.workflow.schema.evaluator import EvaluatorConfig

__all__ = [
    "MAX_CURVE_POINTS",
    "apply_filter_expr",
    "chart_segment_labels",
    "compute_chart_data_for_group",
    "filter_chart_df_by_segments",
    "filter_ray_to_surviving_segments",
    "filter_small_segments",
]

_logger = logging.getLogger(__name__)

# Cap chart_df at this many rows per (pred, target) pair, keeping chart_df and
# the downstream LineChart protos O(1) regardless of input dataset size.
MAX_CURVE_POINTS = 1000

_CHART_DF_COLUMNS = [
    "row_type",
    "pred_col",
    "target_col",
    "filter_expr",
    "metric_names",
    "threshold",
    "fpr",
    "tpr",
    "precision",
    "tp",
    "tn",
    "fp",
    "fn",
    "percentile",
    "abs_error",
    "error",
]


def apply_filter_expr(
    df: pd.DataFrame,
    filter_expr: str | None,
    cache: dict[str, pd.DataFrame | None] | None = None,
    *,
    on_error: str = "raise",
) -> pd.DataFrame | None:
    """Select the rows a metric's ``filter_expr`` keeps.

    The single ``df.query`` call site for both the scalar-metric and chart
    paths, so the two cannot drift and resolve one expression to different row
    sets.

    Args:
        df: Frame to filter.
        filter_expr: A pandas query expression. Empty or ``None`` returns ``df``
            itself.
        cache: Optional dict so metrics sharing an expression query once. Keyed
            by expression only, so pass a fresh dict per frame.
        on_error: ``"raise"`` propagates a bad expression; ``"skip"`` logs and
            returns ``None``.

    Returns:
        The filtered frame; ``df`` when no expression was given; or ``None``
        when the expression failed and ``on_error="skip"``.
    """
    if not filter_expr:
        return df
    if cache is not None and filter_expr in cache:
        return cache[filter_expr]
    try:
        filtered: pd.DataFrame | None = df.query(filter_expr)
    except Exception as exc:
        if on_error != "skip":
            raise
        _logger.error(
            "filter_expr %r could not be applied (%s: %s); skipping affected charts",
            filter_expr,
            type(exc).__name__,
            exc,
        )
        filtered = None
    if cache is not None:
        cache[filter_expr] = filtered
    return filtered


def _binary_curve_rows(
    df: pd.DataFrame,
    df_pair: pd.DataFrame,
    pred_col: str,
    target_col: str,
    filter_expr: str | None,
    metric_names: list[str],
) -> pd.DataFrame | None:
    """Compute the ROC/PR curve points for one binary (pred, target) pair.

    Args:
        df: The unfiltered group, used only for logging row counts.
        df_pair: The rows this pair's ``filter_expr`` keeps.
        pred_col: Prediction column name.
        target_col: Target column name.
        filter_expr: The expression that produced ``df_pair``.
        metric_names: Names of the metrics sharing this curve.

    Returns:
        A chart_df fragment, or ``None`` when the targets are not in ``{0, 1}``.
    """
    # Check the {0, 1} contract before the int32 cast: the cast truncates toward
    # zero, so -0.4 and 0.7 would both arrive as 0 and pass.
    unique_targets = pd.unique(df_pair[target_col])
    if not np.isin(unique_targets, (0, 1)).all():
        _logger.error(
            "Binary curve targets for %r contain values outside {0, 1}: %s "
            "(pred_col=%r, filter_expr=%r). Padding/sentinel rows must be "
            "excluded before curve computation -- give every binary metric on "
            "this column a filter_expr that drops them. Skipping curves for "
            "this pair; scalar metrics are unaffected.",
            target_col,
            np.sort(unique_targets)[:10].tolist(),
            pred_col,
            filter_expr,
        )
        return None

    preds_np = df_pair[pred_col].to_numpy(dtype=np.float64)
    targets_np = df_pair[target_col].to_numpy(dtype=np.int32)

    num_pos = int(targets_np.sum())
    num_neg = len(targets_np) - num_pos
    n = len(preds_np)

    sort_idx_desc = np.argsort(preds_np, kind="stable")[::-1]
    sorted_preds_desc = preds_np[sort_idx_desc]
    sorted_targets_desc = targets_np[sort_idx_desc]
    cum_tp = np.concatenate([[0], np.cumsum(sorted_targets_desc)])

    # One curve point per distinct prediction value, plus both endpoints.
    change_at = np.zeros(n + 1, dtype=bool)
    change_at[0] = True
    change_at[n] = True
    change_at[1:n] = sorted_preds_desc[1:] < sorted_preds_desc[:-1]
    k_idx = np.where(change_at)[0]

    tp_arr = cum_tp[k_idx]
    fp_arr = k_idx - tp_arr
    fn_arr = num_pos - tp_arr
    tn_arr = num_neg - fp_arr
    # np.where evaluates both branches, so a single-class group divides by zero
    # before the guard selects the 0.0. tp == fp == 0 at k_idx[0] likewise
    # leaves precision undefined; take sklearn's 1.0 there. create_pr_chart
    # drops that point outright, since no constant plots well.
    with np.errstate(invalid="ignore", divide="ignore"):
        tpr_arr = np.where(num_pos > 0, tp_arr / num_pos, 0.0)
        fpr_arr = np.where(num_neg > 0, fp_arr / num_neg, 0.0)
        precision_arr = np.where((tp_arr + fp_arr) > 0, tp_arr / (tp_arr + fp_arr), 1.0)

    threshold_vals = sorted_preds_desc[np.maximum(k_idx - 1, 0)].copy()
    threshold_vals[0] = sorted_preds_desc[0]

    # Mixed even/geometric subsample: keeps the steep head from collapsing.
    if len(k_idx) > MAX_CURVE_POINTS:
        keep = sample_curve_indices(len(k_idx), MAX_CURVE_POINTS)
        threshold_vals = threshold_vals[keep]
        tp_arr, fp_arr = tp_arr[keep], fp_arr[keep]
        tn_arr, fn_arr = tn_arr[keep], fn_arr[keep]
        tpr_arr, fpr_arr = tpr_arr[keep], fpr_arr[keep]
        precision_arr = precision_arr[keep]

    _logger.info(
        "Computed binary curves for (%s, %s): rows=%d/%d filter=%r "
        "P=%d N=%d unique_thresholds=%d",
        pred_col,
        target_col,
        n,
        len(df),
        filter_expr,
        num_pos,
        num_neg,
        len(k_idx),
    )
    return pd.DataFrame(
        {
            "row_type": "binary",
            "pred_col": pred_col,
            "target_col": target_col,
            # Pairs are keyed on filter_expr, so (pred_col, target_col) alone
            # no longer identifies a single curve.
            "filter_expr": filter_expr,
            "metric_names": ", ".join(metric_names),
            "threshold": threshold_vals.astype(np.float64),
            "fpr": fpr_arr.astype(np.float64),
            "tpr": tpr_arr.astype(np.float64),
            "precision": precision_arr.astype(np.float64),
            "tp": tp_arr.astype(np.float64),
            "tn": tn_arr.astype(np.float64),
            "fp": fp_arr.astype(np.float64),
            "fn": fn_arr.astype(np.float64),
            "percentile": np.nan,
            "abs_error": np.nan,
            "error": np.nan,
        }
    )


def _regression_error_rows(
    df: pd.DataFrame,
    df_pair: pd.DataFrame,
    pred_col: str,
    target_col: str,
    filter_expr: str | None,
    metric_names: list[str],
    percentile_points: np.ndarray,
) -> pd.DataFrame:
    """Compute the error percentiles for one regression (pred, target) pair.

    Args:
        df: The unfiltered group, used only for logging row counts.
        df_pair: The rows this pair's ``filter_expr`` keeps.
        pred_col: Prediction column name.
        target_col: Target column name.
        filter_expr: The expression that produced ``df_pair``.
        metric_names: Names of the metrics sharing these errors.
        percentile_points: Percentiles to evaluate, in ``[0, 100]``.

    Returns:
        A chart_df fragment.
    """
    preds = df_pair[pred_col].to_numpy(dtype=np.float64)
    targets = df_pair[target_col].to_numpy(dtype=np.float64)
    err = preds - targets
    if not np.isfinite(err).all():
        _logger.warning(
            "Regression errors for (%s, %s) contain %d non-finite values; "
            "percentiles may be NaN",
            pred_col,
            target_col,
            int((~np.isfinite(err)).sum()),
        )
    abs_errors = np.percentile(np.abs(err), percentile_points)
    errors = np.percentile(err, percentile_points)

    _logger.info(
        "Computed regression errors for (%s, %s): rows=%d/%d filter=%r, "
        "%d percentile points",
        pred_col,
        target_col,
        len(preds),
        len(df),
        filter_expr,
        len(percentile_points),
    )
    return pd.DataFrame(
        {
            "row_type": "regression",
            "pred_col": pred_col,
            "target_col": target_col,
            "filter_expr": filter_expr,
            "metric_names": ", ".join(metric_names),
            "threshold": np.nan,
            "fpr": np.nan,
            "tpr": np.nan,
            "precision": np.nan,
            "tp": np.nan,
            "tn": np.nan,
            "fp": np.nan,
            "fn": np.nan,
            "percentile": percentile_points.astype(np.float64),
            "abs_error": abs_errors.astype(np.float64),
            "error": errors.astype(np.float64),
        }
    )


def compute_chart_data_for_group(
    df: pd.DataFrame,
    config: EvaluatorConfig,
    segment_columns: list[str] | None = None,
) -> pd.DataFrame:
    """Compute ROC/PR curves and regression errors for one group of rows.

    Safe to call on any group -- a segment, GLOBAL, or the full dataset -- and
    compatible with Ray's ``map_groups``, which requires a DataFrame return.

    Args:
        df: The group's rows.
        config: Evaluator configuration; its metric list decides which
            (prediction, target, filter) pairs get curves.
        segment_columns: When given, the resulting rows are tagged with a
            ``segment`` label built from these columns' first value.

    Returns:
        A long-format DataFrame with one row per curve point, with the columns
        ``row_type``, ``pred_col``, ``target_col``, ``filter_expr``,
        ``metric_names``, the binary columns (``threshold``, ``fpr``, ``tpr``,
        ``precision``, ``tp``, ``tn``, ``fp``, ``fn``), and the regression
        columns (``percentile``, ``abs_error``, ``error``). Columns not
        applicable to a row's ``row_type`` are NaN.
    """
    # Build per-pair DataFrames columnarly and concat once at the end. A
    # row-dict approach allocates one ~14-key dict per unique threshold; on a
    # 200M-row global pass with float predictions that is ~200M dicts (~100 GB
    # of intermediate Python objects) before pandas reclaims them.
    result_dfs: list[pd.DataFrame] = []

    # Curves must be computed on the same rows as the scalar metrics.
    # Multi-task pipelines pad per-slot label arrays with a -1 sentinel and drop
    # it per metric via filter_expr; unfiltered, `targets.sum()` nets positives
    # against sentinels and can go negative, corrupting P/N, TPR, and the
    # confusion-matrix cells.
    filtered_cache: dict[str, pd.DataFrame | None] = {}
    empty_filter_warned: set[str | None] = set()

    def warn_empty_once(filter_expr: str | None, kind: str) -> None:
        if filter_expr in empty_filter_warned:
            return
        empty_filter_warned.add(filter_expr)
        _logger.warning(
            "filter_expr %r left 0 of %d rows; skipping %s for every pair using it",
            filter_expr,
            len(df),
            kind,
        )

    def rows_for_pair(
        pred_col: str, target_col: str, filter_expr: str | None, kind: str
    ) -> pd.DataFrame | None:
        if pred_col not in df.columns or target_col not in df.columns:
            _logger.warning(
                "Column %s or %s missing; skipping %s for pair (%s, %s)",
                pred_col,
                target_col,
                kind,
                pred_col,
                target_col,
            )
            return None
        df_pair = apply_filter_expr(
            df, filter_expr, cache=filtered_cache, on_error="skip"
        )
        if df_pair is None:
            return None
        if df_pair.empty:
            warn_empty_once(filter_expr, kind)
            return None
        return df_pair

    # --- Binary curves ---
    # Keyed on filter_expr as well as the columns: two metrics can share columns
    # while filtering differently, and must not collapse into one curve.
    binary_pairs: dict[tuple[str, str, str | None], list[str]] = {}
    for metric in config.metrics:
        if metric.columns is None:
            continue
        if metric.params.get("task") == "binary" or "Binary" in metric.metric:
            key = (
                metric.columns.prediction_col,
                metric.columns.target_col,
                metric.filter_expr,
            )
            binary_pairs.setdefault(key, []).append(metric.name)

    for (pred_col, target_col, filter_expr), metric_names in binary_pairs.items():
        df_pair = rows_for_pair(pred_col, target_col, filter_expr, "binary curves")
        if df_pair is None:
            continue
        rows = _binary_curve_rows(
            df, df_pair, pred_col, target_col, filter_expr, metric_names
        )
        if rows is not None:
            result_dfs.append(rows)

    # --- Regression errors ---
    # Same filter_expr requirement as the binary curves: `pred - (-1)` offsets
    # every percentile. There is no label domain to assert here, so the filter
    # is the only protection -- a sentinel is an ordinary real number.
    regression_pairs: dict[tuple[str, str, str | None], list[str]] = {}
    for metric in config.metrics:
        if metric.columns is None:
            continue
        if infer_task_type_from_path(metric.metric) == "regression":
            key = (
                metric.columns.prediction_col,
                metric.columns.target_col,
                metric.filter_expr,
            )
            regression_pairs.setdefault(key, []).append(metric.name)

    percentile_points = np.linspace(0, 100, 101)
    for (pred_col, target_col, filter_expr), metric_names in regression_pairs.items():
        df_pair = rows_for_pair(pred_col, target_col, filter_expr, "regression errors")
        if df_pair is None:
            continue
        result_dfs.append(
            _regression_error_rows(
                df,
                df_pair,
                pred_col,
                target_col,
                filter_expr,
                metric_names,
                percentile_points,
            )
        )

    result = (
        pd.concat(result_dfs, ignore_index=True)
        if result_dfs
        else pd.DataFrame(columns=_CHART_DF_COLUMNS)
    )

    if segment_columns and not result.empty:
        key_vals = [str(df[col].iloc[0]) for col in segment_columns]
        result["segment"] = " / ".join(key_vals) if len(key_vals) > 1 else key_vals[0]

    return result


def chart_segment_labels(df: pd.DataFrame, segment_columns: list[str]) -> set[str]:
    """Build the chart_df-format segment labels for the rows of ``df``.

    Mirrors the format :func:`compute_chart_data_for_group` writes into
    chart_df's ``segment`` column: ``str(value)`` for a single segment column,
    ``" / "``-joined str-cast values for several. Use this to derive the
    surviving-segment set from a metrics DataFrame so it can be handed to
    :func:`filter_chart_df_by_segments`.

    Args:
        df: Frame holding the segment columns.
        segment_columns: The columns that define a segment.

    Returns:
        The set of segment labels present in ``df``.
    """
    if not segment_columns or df.empty:
        return set()
    if len(segment_columns) == 1:
        return set(df[segment_columns[0]].astype(str))
    return set(df[segment_columns].astype(str).agg(" / ".join, axis=1))


def filter_chart_df_by_segments(
    chart_df: pd.DataFrame, segments: set[str]
) -> pd.DataFrame:
    """Keep only the chart_df rows whose ``segment`` is in ``segments``.

    Pair with :func:`chart_segment_labels` to keep chart_df consistent with a
    metrics DataFrame that has been row-filtered, e.g. by
    ``min_segment_row_count``.

    Args:
        chart_df: Chart data to filter.
        segments: Segment labels to keep.

    Returns:
        The filtered chart data.
    """
    if chart_df.empty:
        return chart_df
    return chart_df[chart_df["segment"].isin(segments)]


def filter_small_segments(segment_df: pd.DataFrame, min_row_count: int) -> pd.DataFrame:
    """Drop segments backed by fewer than ``min_row_count`` rows.

    Args:
        segment_df: Segment results carrying a ``_segment_row_count`` column.
        min_row_count: Minimum rows a segment needs to survive. Values <= 0
            disable filtering.

    Returns:
        The filtered results, with the ``_segment_row_count`` helper column
        removed.
    """
    if min_row_count > 0:
        _logger.info(
            "Applying min_segment_row_count filter: %d rows required per segment",
            min_row_count,
        )
        initial_count = len(segment_df)
        segment_df = segment_df[segment_df["_segment_row_count"] >= min_row_count]
        _logger.info(
            "Filtered out %d small segments, kept %d segments",
            initial_count - len(segment_df),
            len(segment_df),
        )
    else:
        _logger.info(
            "No min_segment_row_count filtering (min_segment_row_count=%d)",
            min_row_count,
        )

    if "_segment_row_count" in segment_df.columns:
        segment_df = segment_df.drop(columns=["_segment_row_count"])

    return segment_df


def filter_ray_to_surviving_segments(
    ray_dataset: Any,
    segment_df: pd.DataFrame,
    segment_columns: list[str],
    min_row_count: int,
) -> Any:
    """Drop the Ray Dataset rows belonging to already-filtered-out segments.

    Called after :func:`filter_small_segments` so the chart groupby does not
    waste work on segments ``min_segment_row_count`` already dropped. Keys are
    compared as strings, to match the ``"UNKNOWN"`` null-fill applied earlier.

    Args:
        ray_dataset: The Ray Dataset to filter. Duck-typed, so Ray stays an
            optional dependency.
        segment_df: The surviving segments.
        segment_columns: The columns that define a segment.
        min_row_count: The threshold that produced ``segment_df``.

    Returns:
        A filtered Ray Dataset, or ``ray_dataset`` unchanged when filtering is
        disabled or there is nothing to keep -- so callers can invoke this
        unconditionally without paying for a no-op ``map_batches`` pass.
    """
    if min_row_count <= 0 or segment_df.empty:
        return ray_dataset

    surviving_keys = {
        tuple(str(v) for v in row)
        for row in segment_df[segment_columns].values.tolist()
    }
    _logger.info(
        "Pre-filtering ray_dataset to %d surviving segments", len(surviving_keys)
    )

    def keep_surviving(
        batch: pd.DataFrame, keys: set = surviving_keys, cols: list = segment_columns
    ) -> pd.DataFrame:
        return batch[batch[cols].astype(str).apply(tuple, axis=1).isin(keys)]

    return ray_dataset.map_batches(keep_surviving, batch_format="pandas")
