---
sidebar_label: chart_data
title: lib.evaluator.chart_data
---

Compute the long-format ``chart_df`` that backs the evaluation report charts.

`compute_chart_data_for_group` runs over one group of rows -- a segment, a
GLOBAL pass, or a whole dataset -- and emits one row per curve point. It returns
a DataFrame rather than protos so it can run under a distributed ``map_groups``
and be concatenated before the report is built.

Curves are computed on exactly the rows the corresponding scalar metrics see:
every pair is keyed on ``filter_expr`` as well as its columns, because two
metrics can share prediction and target columns while filtering differently.

#### apply\_filter\_expr

```python
def apply_filter_expr(df: pd.DataFrame,
                      filter_expr: str | None,
                      cache: dict[str, pd.DataFrame | None] | None = None,
                      *,
                      on_error: str = "raise") -> pd.DataFrame | None
```

Select the rows a metric's ``filter_expr`` keeps.

The single ``df.query`` call site for both the scalar-metric and chart
paths, so the two cannot drift and resolve one expression to different row
sets.

**Arguments**:

- `df` - Frame to filter.
- `filter_expr` - A pandas query expression. Empty or ``None`` returns ``df``
  itself.
- `cache` - Optional dict so metrics sharing an expression query once. Keyed
  by expression only, so pass a fresh dict per frame.
- `on_error` - ``"raise"`` propagates a bad expression; ``"skip"`` logs and
  returns ``None``.
  

**Returns**:

  The filtered frame; ``df`` when no expression was given; or ``None``
  when the expression failed and ``on_error="skip"``.

#### compute\_chart\_data\_for\_group

```python
def compute_chart_data_for_group(
        df: pd.DataFrame,
        config: EvaluatorConfig,
        segment_columns: list[str] | None = None) -> pd.DataFrame
```

Compute ROC/PR curves and regression errors for one group of rows.

Safe to call on any group -- a segment, GLOBAL, or the full dataset -- and
compatible with Ray's ``map_groups``, which requires a DataFrame return.

**Arguments**:

- `df` - The group's rows.
- `config` - Evaluator configuration; its metric list decides which
  (prediction, target, filter) pairs get curves.
- `segment_columns` - When given, the resulting rows are tagged with a
  ``segment`` label built from these columns' first value.
  

**Returns**:

  A long-format DataFrame with one row per curve point, with the columns
  ``row_type``, ``pred_col``, ``target_col``, ``filter_expr``,
  ``metric_names``, the binary columns (``threshold``, ``fpr``, ``tpr``,
  ``precision``, ``tp``, ``tn``, ``fp``, ``fn``), and the regression
  columns (``percentile``, ``abs_error``, ``error``). Columns not
  applicable to a row's ``row_type`` are NaN.

#### chart\_segment\_labels

```python
def chart_segment_labels(df: pd.DataFrame,
                         segment_columns: list[str]) -> set[str]
```

Build the chart_df-format segment labels for the rows of ``df``.

Mirrors the format `compute_chart_data_for_group` writes into
chart_df's ``segment`` column: ``str(value)`` for a single segment column,
``" / "``-joined str-cast values for several. Use this to derive the
surviving-segment set from a metrics DataFrame so it can be handed to
`filter_chart_df_by_segments`.

**Arguments**:

- `df` - Frame holding the segment columns.
- `segment_columns` - The columns that define a segment.
  

**Returns**:

  The set of segment labels present in ``df``.

#### filter\_chart\_df\_by\_segments

```python
def filter_chart_df_by_segments(chart_df: pd.DataFrame,
                                segments: set[str]) -> pd.DataFrame
```

Keep only the chart_df rows whose ``segment`` is in ``segments``.

Pair with `chart_segment_labels` to keep chart_df consistent with a
metrics DataFrame that has been row-filtered, e.g. by
``min_segment_row_count``.

**Arguments**:

- `chart_df` - Chart data to filter.
- `segments` - Segment labels to keep.
  

**Returns**:

  The filtered chart data.

#### filter\_small\_segments

```python
def filter_small_segments(segment_df: pd.DataFrame,
                          min_row_count: int) -> pd.DataFrame
```

Drop segments backed by fewer than ``min_row_count`` rows.

**Arguments**:

- `segment_df` - Segment results carrying a ``_segment_row_count`` column.
- `min_row_count` - Minimum rows a segment needs to survive. Values of 0 or less
  disable filtering.
  

**Returns**:

  The filtered results, with the ``_segment_row_count`` helper column
  removed.

#### filter\_ray\_to\_surviving\_segments

```python
def filter_ray_to_surviving_segments(ray_dataset: Any,
                                     segment_df: pd.DataFrame,
                                     segment_columns: list[str],
                                     min_row_count: int) -> Any
```

Drop the Ray Dataset rows belonging to already-filtered-out segments.

Called after `filter_small_segments` so the chart groupby does not
waste work on segments ``min_segment_row_count`` already dropped. Keys are
compared as strings, to match the ``"UNKNOWN"`` null-fill applied earlier.

**Arguments**:

- `ray_dataset` - The Ray Dataset to filter. Duck-typed, so Ray stays an
  optional dependency.
- `segment_df` - The surviving segments.
- `segment_columns` - The columns that define a segment.
- `min_row_count` - The threshold that produced ``segment_df``.
  

**Returns**:

  A filtered Ray Dataset, or ``ray_dataset`` unchanged when filtering is
  disabled or there is nothing to keep -- so callers can invoke this
  unconditionally without paying for a no-op ``map_batches`` pass.

