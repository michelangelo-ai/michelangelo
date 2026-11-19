---
sidebar_label: evaluator
title: lib.evaluator.evaluator
---

TorchMetrics-based evaluator driven by `EvaluatorConfig`.

`TorchMetricEvaluator` groups the configured metrics into TorchMetrics
``MetricCollection``s keyed by the data they need, then evaluates a pandas
DataFrame against all of them in one pass.

## TorchMetricEvaluator Objects

```python
class TorchMetricEvaluator()
```

Evaluate a pandas DataFrame against a config's metrics.

Metrics are organized into TorchMetrics ``MetricCollection``s by their
column configuration, filter, and tensor dtypes, so every metric that wants
the same data computes from one extraction.

#### \_\_init\_\_

```python
def __init__(config: EvaluatorConfig | dict[str, Any] | str | Path)
```

Initialize the evaluator.

**Arguments**:

- `config` - An `EvaluatorConfig`, a dict in the same shape, or a
  path to a YAML config file.
  

**Raises**:

- `TypeError` - If ``config`` is none of those.

#### evaluate

```python
def evaluate(df: pd.DataFrame) -> dict[str, Any]
```

Compute every configured metric over a DataFrame.

Each collection is updated, computed, and reset in turn, so metric state
for all collections is never held at once.

**Arguments**:

- `df` - The rows to evaluate.
  

**Returns**:

  Metric name -> computed value.

#### compute

```python
def compute() -> dict[str, Any]
```

Compute every collection from its currently accumulated state.

Use this when collections were updated incrementally rather than through
`evaluate`.

**Returns**:

  Metric name -> computed value.

#### get\_metric\_names

```python
def get_metric_names() -> list[str]
```

List every metric name this evaluator will compute.

**Returns**:

  The metric names, in collection order.

#### validate\_dataset

```python
def validate_dataset(df: pd.DataFrame) -> None
```

Check that a DataFrame carries every column the metrics need.

**Arguments**:

- `df` - The DataFrame to validate.
  

**Raises**:

- `ValueError` - If any required column is missing, naming all of them.

#### get\_required\_columns

```python
def get_required_columns() -> set[str]
```

Collect every column the configured metrics read.

**Returns**:

  The column names that must be present in the input data, including
  the config's segment columns.

#### get\_column\_mappings

```python
def get_column_mappings() -> list[ColumnMapping]
```

List the column mappings the configured metrics use.

**Returns**:

  One mapping per collection that has one.

#### \_\_repr\_\_

```python
def __repr__() -> str
```

Render the evaluator's metric and collection counts.

#### create\_evaluator

```python
def create_evaluator(
    config: EvaluatorConfig | dict[str, Any] | str | Path
) -> TorchMetricEvaluator
```

Create a `TorchMetricEvaluator`.

**Arguments**:

- `config` - An `EvaluatorConfig`, a dict in the same shape, or a path
  to a YAML config file.
  

**Returns**:

  The initialized evaluator.

