---
sidebar_label: report
title: lib.evaluator.report
---

Chart builders for the evaluator's ``EvaluationReport``.

Each function here turns a slice of the evaluator's output DataFrames into one
``Chart`` proto. ``report_generator`` assembles them into a report; nothing in
this module reads or writes the report itself except
`create_default_evaluation_report`.

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

#### create\_metrics\_table

```python
def create_metrics_table(column_names: list[str],
                         values: list[list[tuple[str | float, str]]],
                         title: str = "Metrics",
                         section_id: str = "Metrics") -> Chart
```

Create the metrics table chart.

**Arguments**:

- `column_names` - Names of the table columns, in order.
- `values` - One list per row, each holding ``(value, field)`` pairs in column
  order. ``field`` names the ``ChartCellValue`` field the value is
  written to -- ``"str_value"`` or ``"number"``. A ``None`` value
  leaves the cell empty.
- `title` - Chart title.
- `section_id` - Section ID used to group the chart in the UI.
  

**Returns**:

  A ``Chart`` of type ``CHART_TYPE_TABLE``.

#### create\_default\_evaluation\_report

```python
def create_default_evaluation_report(title: str,
                                     source_pipeline_type: str = ""
                                     ) -> EvaluationReport
```

Create an empty evaluation report for charts to be appended to.

**Arguments**:

- `title` - Report title, shown as the page heading.
- `source_pipeline_type` - Optional pipeline-type label (e.g. ``"train"``),
  recorded under ``SOURCE_PIPELINE_TYPE_LABEL_NAME``.
  

**Returns**:

  An ``EvaluationReport`` with an embedded data-point source and no charts.

#### create\_roc\_chart

```python
def create_roc_chart(chart_df: pd.DataFrame,
                     inline_width: str = "50%") -> Chart
```

Create a ROC curve chart with one line per (segment, binary pair).

**Arguments**:

- `chart_df` - Long-format chart data; only ``row_type == "binary"`` is read.
- `inline_width` - CSS width for side-by-side layout.
  

**Returns**:

  A ``Chart`` of type ``CHART_TYPE_LINE_CHART``.

#### create\_pr\_chart

```python
def create_pr_chart(chart_df: pd.DataFrame,
                    inline_width: str = "50%") -> Chart
```

Create a Precision-Recall chart with one line per (segment, binary pair).

**Arguments**:

- `chart_df` - Long-format chart data; only ``row_type == "binary"`` is read.
- `inline_width` - CSS width for side-by-side layout.
  

**Returns**:

  A ``Chart`` of type ``CHART_TYPE_LINE_CHART``.

#### create\_absolute\_errors\_chart

```python
def create_absolute_errors_chart(chart_df: pd.DataFrame,
                                 inline_width: str = "50%") -> Chart
```

Create an Absolute Errors chart: ``|prediction - target|`` by percentile.

**Arguments**:

- `chart_df` - Long-format chart data; only ``row_type == "regression"`` is read.
- `inline_width` - CSS width for side-by-side layout.
  

**Returns**:

  A ``Chart`` of type ``CHART_TYPE_LINE_CHART``.

#### create\_errors\_chart

```python
def create_errors_chart(chart_df: pd.DataFrame,
                        inline_width: str = "50%") -> Chart
```

Create an Errors chart: signed ``prediction - target`` by percentile.

**Arguments**:

- `chart_df` - Long-format chart data; only ``row_type == "regression"`` is read.
- `inline_width` - CSS width for side-by-side layout.
  

**Returns**:

  A ``Chart`` of type ``CHART_TYPE_LINE_CHART``.

#### create\_confusion\_matrix\_chart

```python
def create_confusion_matrix_chart(chart_df: pd.DataFrame, pred_col: str,
                                  target_col: str) -> Chart
```

Create a confusion-matrix chart with one ``MatrixSeries`` per segment.

Every segment is sampled onto one canonical threshold grid, taken from
GLOBAL when present, so the threshold slider moves all segments together.
Each segment contributes the row nearest to each canonical threshold.

**Arguments**:

- `chart_df` - Long-format chart data; only ``row_type == "binary"`` is read.
- `pred_col` - Prediction column the matrix is built for.
- `target_col` - Target column the matrix is built for.
  

**Returns**:

  A ``Chart`` of type ``CHART_TYPE_CONFUSION_MATRIX``. The matrix is empty
  when the pair has no threshold rows.

#### create\_segment\_filter\_chart

```python
def create_segment_filter_chart(chart_df: pd.DataFrame) -> Chart
```

Create a MULTI_SEGMENT filter for toggling segments on the curve charts.

**Arguments**:

- `chart_df` - Long-format chart data; only ``row_type == "binary"`` is read.
  

**Returns**:

  A ``Chart`` of type ``CHART_TYPE_FILTER``, with GLOBAL preselected when
  it is one of the segments.

#### create\_threshold\_filter\_chart

```python
def create_threshold_filter_chart(chart_df: pd.DataFrame, pred_col: str,
                                  target_col: str) -> Chart
```

Create a threshold slider filter adjacent to a confusion matrix.

**Arguments**:

- `chart_df` - Long-format chart data; only ``row_type == "binary"`` is read.
- `pred_col` - Prediction column the slider drives.
- `target_col` - Target column the slider drives.
  

**Returns**:

  A ``Chart`` of type ``CHART_TYPE_FILTER`` whose values are the same
  sampled grid the confusion matrix was built on, ascending.

