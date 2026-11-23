---
sidebar_label: metrics
title: lib.evaluator.metrics
---

Metric instantiation, dtype resolution, and tensor extraction.

Two metrics share a ``MetricCollection`` only when they want the same columns,
the same filter, *and* the same tensor dtypes. Dtype is resolved per metric --
from an explicit ``_dtypes`` attribute, from a ``_metric_type`` tag, or by
inferring the task from the metric's import path -- so a regression metric
cannot land in a collection of classification metrics that happen to read the
same columns and silently cast their targets to float.

#### cast\_array\_to\_dtype

```python
def cast_array_to_dtype(array_data: np.ndarray,
                        dtype: torch.dtype) -> torch.Tensor
```

Convert a numpy array to a torch tensor with an explicit dtype.

**Arguments**:

- `array_data` - Source array. Coerced to numpy first -- a column read back
  from Ray arrives as a pandas ``ArrowExtensionArray``, which
  ``torch.from_numpy`` rejects. Copied, so the tensor never aliases a
  DataFrame column.
- `dtype` - Target torch dtype.
  

**Returns**:

  The converted tensor.

#### encode\_categorical\_column\_to\_tensor

```python
def encode_categorical_column_to_tensor(
        column_data: np.ndarray,
        target_dtype: torch.dtype = torch.long,
        factorize_strings: bool = True) -> torch.Tensor
```

Encode a categorical column to a torch tensor, handling string data.

**Arguments**:

- `column_data` - Input numpy array, typically a DataFrame column.
- `target_dtype` - Target torch dtype (e.g. ``torch.long``, ``torch.float``).
- `factorize_strings` - Convert string/object data to numeric via
  ``pandas.factorize``, which gives each distinct string a stable
  integer code.
  

**Returns**:

  The converted tensor.
  

**Example**:

```python
>>> encode_categorical_column_to_tensor(np.array(["a", "b", "a", "c"]))
tensor([0, 1, 0, 2])
>>> encode_categorical_column_to_tensor(
...     np.array([1.0, 2.5]), torch.float, factorize_strings=False
... )
tensor([1.0000, 2.5000])
```

#### extract\_data\_for\_collection

```python
def extract_data_for_collection(
        df: pd.DataFrame, column_mapping: ColumnMapping,
        pred_dtype: torch.dtype,
        target_dtype: torch.dtype | None) -> dict[str, torch.Tensor]
```

Extract the tensors one ``MetricCollection`` needs from a DataFrame.

Casting is driven by the explicit ``(pred_dtype, target_dtype)`` pair the
bundling layer resolved, so this function never has to pick a default.

**Arguments**:

- `df` - Rows to extract from, already filtered by the metric's
  ``filter_expr``.
- `column_mapping` - Which columns feed predictions, targets, and extras.
- `pred_dtype` - Dtype for the prediction tensor.
- `target_dtype` - Dtype for the target tensor. ``None`` marks an
  aggregation-style collection that does not read the target column.
  

**Returns**:

  A dict with ``predictions``, and optionally ``targets``, ``indexes``,
  ``sample_weights``, and ``extra_cols``.

#### infer\_task\_type\_from\_path

```python
def infer_task_type_from_path(metric_path: str) -> str
```

Infer a metric's task type from its import path.

**Arguments**:

- `metric_path` - Dotted import path of the metric class.
  

**Returns**:

  One of ``"regression"``, ``"classification"``, ``"retrieval"``,
  ``"aggregation"``. Unknown paths fall back to ``"classification"``;
  tag the class with ``_metric_type`` to override.
  

**Example**:

```python
>>> infer_task_type_from_path("torchmetrics.classification.Accuracy")
'classification'
>>> infer_task_type_from_path("torchmetrics.retrieval.RetrievalMAP")
'retrieval'
```

#### resolve\_dtypes\_for\_metric

```python
def resolve_dtypes_for_metric(
        metric_path: str,
        metric_instance: Any) -> tuple[torch.dtype, torch.dtype | None]
```

Resolve the ``(pred_dtype, target_dtype)`` pair for one metric.

Resolution order:

1. A ``_dtypes`` class attribute -- a direct override for combinations no
task-type shorthand covers.
2. A ``_metric_type`` class attribute naming a key of
:data:`TASK_TYPE_TO_DTYPES`. This is the recommended way for a custom
metric to declare intent.
3. Path-based task inference, mapped through :data:`TASK_TYPE_TO_DTYPES`.

**Arguments**:

- `metric_path` - Dotted import path the metric was created from.
- `metric_instance` - The instantiated metric.
  

**Returns**:

  The resolved dtype pair. ``target_dtype`` is ``None`` for aggregation
  metrics.

#### create\_metric\_from\_path

```python
def create_metric_from_path(metric_path: str, params: dict[str, Any]) -> Any
```

Create a metric instance from a dotted import path.

**Arguments**:

- `metric_path` - Import path such as
  ``"torchmetrics.classification.MulticlassAUROC"``.
- `params` - Keyword arguments for the metric constructor.
  

**Returns**:

  The instantiated metric.

#### metric\_state\_for\_logging

```python
def metric_state_for_logging(metric: Any) -> dict[str, Any]
```

Read a metric's accumulated internal state, for debug logging.

TorchMetrics registers state via ``add_state(persistent=False)``, so
``state_dict()`` is usually empty. This reads the tensor attributes
directly instead, surfacing counters like ``tp``, ``fp``, and ``total``.

**Arguments**:

- `metric` - The metric to inspect.
  

**Returns**:

  Attribute name -> scalar or list value, for every tensor attribute.

#### create\_metric\_collections

```python
def create_metric_collections(
    config: EvaluatorConfig
) -> tuple[dict[tuple, Any], dict[tuple, tuple[torch.dtype, torch.dtype
                                               | None]]]
```

Group the configured metrics into TorchMetrics ``MetricCollection``s.

The bundling key is ``(ColumnMapping, filter_expr, pred_dtype,
target_dtype)``. Two metrics co-bundle only when they want the same data
*and* the same tensor dtypes.

**Arguments**:

- `config` - Evaluator configuration holding the metric list.
  

**Returns**:

  A ``(collections, dtype_map)`` pair, both keyed by the bundling key.
  

**Raises**:

- `ImportError` - If torchmetrics is not installed.

