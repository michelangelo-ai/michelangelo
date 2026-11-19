---
sidebar_label: report_generator
title: lib.evaluator.report_generator
---

Assemble evaluator output DataFrames into an ``EvaluationReport``.

`generate_evaluation_report` renders one dataset; `build_combined_report`
merges several, prefixing each dataset's name onto its segments so a single report
can compare them.

## DatasetResult Objects

```python
@dataclass
class DatasetResult()
```

One evaluated dataset's output.

**Attributes**:

- `name` - Dataset name, used as the table's ``DatasetName`` column and as the
  segment prefix in a combined report.
- `summary_df` - One row per segment, one column per metric.
- `chart_df` - Long-format curve data. Empty when the dataset produced no
  curves.

#### generate\_evaluation\_report

```python
def generate_evaluation_report(
        summary_df: pd.DataFrame,
        segment_columns: list[str] | None = None,
        title: str = DEFAULT_REPORT_TITLE,
        source_pipeline_type: str = "",
        chart_df: pd.DataFrame | None = None) -> EvaluationReport
```

Generate an ``EvaluationReport`` from a summary metrics DataFrame.

**Arguments**:

- `summary_df` - DataFrame containing metric values and optionally segment
  columns.
- `segment_columns` - Column names used for segmentation, if any.
- `title` - Title for the evaluation report.
- `source_pipeline_type` - Pipeline type label (e.g. ``"train"``, ``"retrain"``).
- `chart_df` - Long-format DataFrame of curve points with a ``segment`` column
  added by the caller. Rows with ``row_type="binary"`` drive the ROC,
  PR, and confusion-matrix charts; rows with ``row_type="regression"``
  drive Absolute Errors and Errors Chart. Pass ``None`` when no chart
  data is available.
  

**Returns**:

  A populated ``EvaluationReport``. A report with no metric columns, or
  built from an empty ``summary_df``, comes back with no charts.

#### build\_combined\_report

```python
def build_combined_report(
        results: list[DatasetResult],
        segment_columns: list[str] | None) -> EvaluationReport
```

Merge per-dataset results into a single combined ``EvaluationReport``.

A single result is rendered as-is. Several are concatenated with a
``DatasetName`` column, and each dataset's chart segments are prefixed with
its name so curves stay distinguishable in the shared segment filter.

**Arguments**:

- `results` - One entry per evaluated dataset.
- `segment_columns` - Segment column names shared by every result.
  

**Returns**:

  A populated ``EvaluationReport``.

