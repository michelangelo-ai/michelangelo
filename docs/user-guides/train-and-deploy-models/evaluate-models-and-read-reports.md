---
sidebar_position: 2.3
sidebar_label: "Evaluate Models and Read Reports"
last_verified: 2026-11-18
---

# Evaluate Models and Read Reports

Score a model's predictions inside a Michelangelo AI workflow, split the results by segment, fail the run when a metric regresses, and read the evaluation report the run produces.

This guide is for ML engineers who already have a task that writes predictions and labels to a dataset. By the end you will know how to declare metrics in config, run the evaluator task, read its summary table and report, and publish the report with `push()`.

All paths below are relative to `python/michelangelo/`.

## What you'll learn

- What the evaluator does and what it returns
- How to declare metrics, segments and sanity checks in an `EvaluatorConfig`
- How to run the evaluator from a `@uniflow.task`
- What the evaluation report contains and how to read it
- Where the report goes, and what happens when a sanity check fails

## What the evaluator does

The evaluator is a library plus a workflow task. The library, `michelangelo.lib.evaluator`, computes [TorchMetrics](https://lightning.ai/docs/torchmetrics/stable/) metrics over a pandas DataFrame. The task, `michelangelo.workflow.tasks.evaluator.evaluate`, runs that library over one or more datasets on Ray and does four things:

1. Computes every metric declared in the config, once for the whole dataset and, if you ask for it, once per segment.
2. Saves a summary table per dataset (one row per scope, one column per metric).
3. Renders an `EvaluationReport` per dataset, plus a combined report across all of them.
4. Checks the metrics against your thresholds and fails the task if any of them is breached. The failure is raised only after every report is written.

No metric code is written by the pipeline author. A metric is a TorchMetrics class named by its import path.

## Install

The evaluator needs the `evaluator` extra (`torch`, `torchmetrics`, `numpy`, `pandas`, `ray`, `pyarrow` and `scikit-learn`). From `python/`:

```bash
poetry install --extras "evaluator"
```

## Configure the evaluator

Configuration is an `EvaluatorConfig` from `michelangelo.workflow.schema.evaluator`. It is a plain dataclass with validation in `__post_init__`.

```python
from michelangelo.workflow.schema.evaluator import (
    ColumnMapping,
    EvaluatorConfig,
    MetricConfig,
    SanityCheck,
)

columns = ColumnMapping(prediction_col="score", target_col="label")

config = EvaluatorConfig(
    metrics=[
        MetricConfig(
            name="auc",
            metric="torchmetrics.classification.BinaryAUROC",
            columns=columns,
        ),
        MetricConfig(
            name="accuracy",
            metric="torchmetrics.classification.BinaryAccuracy",
            columns=columns,
        ),
    ],
    segment_columns=["region"],
    min_segment_row_count=50,
    sanity_checks=[
        SanityCheck(metric="auc", threshold=0.7, datasets=["validation"]),
    ],
)
```

### Metrics

Each `MetricConfig` declares one metric:

| Field | Meaning |
| --- | --- |
| `name` | Key for this metric in results, in the summary table, and in `SanityCheck.metric`. |
| `metric` | Full import path of the metric class, such as `torchmetrics.classification.BinaryAUROC`. |
| `params` | Constructor arguments for the metric. Default: `{}`. |
| `columns` | A `ColumnMapping` naming the DataFrame columns the metric reads. |
| `filter_expr` | Optional pandas `query` string. Rows that do not match are dropped before this metric is computed. It applies per metric, so metrics can filter differently. |

A `ColumnMapping` has `prediction_col` and `target_col` (both required), plus optional `index_col` (query or group ids for ranking metrics), `sample_weight_col`, and `extra_cols`. `extra_cols` maps logical names to column names and is forwarded as keyword arguments only to custom metrics whose `update()` accepts `**kwargs`. Standard TorchMetrics are never called with it.

Metrics that read the same columns, with the same filter and tensor dtypes, are computed in one pass over the data.

Whether a metric is treated as regression, classification, retrieval or aggregation is inferred from its import path (for example, a path containing `.regression.`). A class that cannot be classified that way falls back to classification. A custom metric can declare its type with a `_metric_type` class attribute set to `"regression"`, `"classification"`, `"retrieval"` or `"aggregation"`.

### Embedding metrics

`michelangelo.lib.evaluator.embedding_metrics` ships label-free checks on an embedding column. Point `prediction_col` at a column holding one vector per row:

| Metric class | Parameters | What it reports |
| --- | --- | --- |
| `EmbeddingCosineSimilarity` | `stat` (`"mean"` or `"std"`, default `"mean"`) | Pairwise cosine similarity over distinct row pairs. A mean near 1 means the space has collapsed. |
| `EmbeddingNorm` | `stat` (`"mean"` or `"std"`, default `"mean"`) | L2 norm of the rows. Flags exploding or vanishing vectors. |
| `EmbeddingEffectiveRank` | none | Effective rank of the centered sample, from 1 up to the embedding dimension. A value far below the dimension indicates dimensional collapse. |
| `EmbeddingSilhouette` | `n_clusters` (default `8`) | Silhouette score, from -1 to 1, of a K-means clustering of the sample. |

All four also accept `max_samples` (default `5000`) and `seed` (default `0`). They score a seeded random sample of at most `max_samples` rows, so results are reproducible and the cost does not grow with dataset size.

```python
MetricConfig(
    name="user_emb_cosine_mean",
    metric="michelangelo.lib.evaluator.embedding_metrics.EmbeddingCosineSimilarity",
    params={"stat": "mean"},
    columns=ColumnMapping(prediction_col="user_embedding", target_col="user_id"),
)
```

:::note target_col is still required
These metrics never read the target, but `ColumnMapping` requires `target_col` and the evaluator still loads that column. Point it at a small, cheap column such as an id, not at another embedding.
:::

### Segments

Set `segment_columns` to split every metric by the values of one or more columns.

| Field | Default | Meaning |
| --- | --- | --- |
| `segment_columns` | `[]` | Columns to group by, such as `["region"]` or `["region", "datestr"]`. Empty means no segmentation. |
| `enable_global_segment_evaluation` | `True` | Also compute the aggregate over all rows, as a row whose segment columns are all `"GLOBAL"`. Set to `False` to get only segment rows. |
| `min_segment_row_count` | `0` | Segments with fewer rows are dropped from the table, the charts and the checks. `0` disables the filter. |

Null values in a segment column become their own segment, labelled `UNKNOWN`.

### Sanity checks

A `SanityCheck` asserts a bound on one metric:

| Field | Default | Meaning |
| --- | --- | --- |
| `metric` | required | A `MetricConfig.name` declared in `metrics`. |
| `threshold` | required | The value to compare against. |
| `operator` | `">="` | One of `">"`, `">="`, `"<"`, `"<="`, read as `value <operator> threshold`. |
| `use_abs` | `False` | Compare `abs(value)`, for two-sided metrics such as bias. |
| `datasets` | `["test"]` | Which evaluated datasets the check applies to. |

A few rules to know before you write one:

- **The default dataset is `test`.** Train and validation metrics are reported but not gated unless you name them in `datasets`.
- **Checks run against every scope.** That is the `GLOBAL` aggregate plus every surviving segment. One bad segment fails the run even when `GLOBAL` passes.
- **Segmented checks need `min_segment_row_count` above zero.** `EvaluatorConfig` raises `ConfigurationError` if you combine `sanity_checks` with `segment_columns` while it is `0`. A one-row segment would otherwise gate on a degenerate value, such as a single-class AUROC.
- **A missing, non-scalar, or non-finite value fails the check.** `NaN`, `inf`, a metric that was not computed for a scope, and a vector-valued metric all count as breaches, not skips.
- **`use_abs=True` with `"<"` or `"<="` needs a positive threshold** (non-negative for `"<="`), because `abs()` is never negative. Otherwise `SanityCheck` raises `ConfigurationError`.
- **Quote the operator in YAML.** An unquoted `operator: >=` is a YAML scanner error and an unquoted `operator: >` parses as an empty string. Write `operator: ">="`.

Typos are caught before any data is read. A check naming an unknown metric, an empty `datasets` list, or a dataset the task was not given raises `ConfigurationError` immediately.

### Preprocessing rows

If the metrics need rows reshaped first, name a function in `data_preprocessor` by its fully qualified import path. It takes a pandas DataFrame and returns one, and is applied per Ray batch before metrics are computed. Decorate it with `declare_inputs` so the evaluator knows which raw columns to read from parquet:

```python
from michelangelo.workflow.tasks.evaluator import declare_inputs


@declare_inputs(["pred_arr", "label_arr", "valid_mask"])
def explode(df):
    ...  # return one row per item, with the columns the metrics name
    return df
```

`declare_inputs` raises `ValueError` on an empty list. The evaluator raises `ConfigurationError` if the path does not import, is not callable, or lacks `input_columns`.

:::warning Dict and YAML loaders read only `metrics`
`load_config_from_dict` and `load_config_from_yaml` in `michelangelo.lib.evaluator.config`, which `create_evaluator(...)` uses when you hand it a dict or a YAML path, read only the `metrics` list. They silently ignore `segment_columns`, `min_segment_row_count`, `sanity_checks` and the other `EvaluatorConfig` fields. To segment or gate, build an `EvaluatorConfig` directly, as above.
:::

## Run it in a workflow

Call `evaluate(config, datasets)` from inside a `@uniflow.task`. `datasets` maps a name (`"train"`, `"validation"`, `"test"`) to a `DatasetVariable` holding the predictions and labels. The names matter: they key the results and are what `SanityCheck.datasets` refers to. `None` entries are skipped, and an empty `"test"` dataset is treated as absent. Any other empty dataset raises `ValueError`.

```python
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask
from michelangelo.workflow.variables import DatasetVariable


@uniflow.task(config=RayTask(head_cpu=2, head_memory="4Gi"))
def evaluate_predictions(predictions: DatasetVariable, min_auc: float) -> dict:
    from michelangelo.workflow.schema.evaluator import (
        ColumnMapping,
        EvaluatorConfig,
        MetricConfig,
        SanityCheck,
    )
    from michelangelo.workflow.tasks.evaluator import evaluate

    config = EvaluatorConfig(
        metrics=[
            MetricConfig(
                name="auc",
                metric="torchmetrics.classification.BinaryAUROC",
                columns=ColumnMapping(prediction_col="score", target_col="label"),
            ),
        ],
        segment_columns=["region"],
        min_segment_row_count=50,
        sanity_checks=[
            SanityCheck(metric="auc", threshold=min_auc, datasets=["validation"])
        ],
    )

    result = evaluate(config, {"validation": predictions})

    result.metrics["validation"].summary_metrics.load_ray_dataset()
    summary = result.metrics["validation"].summary_metrics.value.to_pandas()
    aggregate = summary[summary["region"] == "GLOBAL"].iloc[0]
    return {"auc": float(aggregate["auc"])}
```

This is a trimmed copy of [`python/examples/evaluator/evaluator.py`](https://github.com/michelangelo-ai/michelangelo/blob/main/python/examples/evaluator/evaluator.py), which also generates the synthetic predictions and the `@uniflow.workflow()` that wires the two tasks together. Imports go inside the task body because workflow functions are restricted when they run remotely; the example file lists those restrictions.

To run the full example locally from `python/`:

```bash
poetry install --extras "example" --extras "evaluator"
PYTHONPATH="." poetry run python ./examples/evaluator/evaluator.py
```

### What `evaluate()` returns

`evaluate()` returns an `EvaluationResult`:

| Attribute | Contents |
| --- | --- |
| `metrics` | `dict` of dataset name to `EvaluationMetrics`. Its `summary_metrics` is a `DatasetVariable` holding the summary table. `instance_metrics` is always `None` on this path, because metrics are aggregated, not scored per row. |
| `reports` | `dict` of report name to a `MessageVariable` holding an `EvaluationReport`: one per evaluated dataset, keyed by dataset name, plus the combined report under `"performance_evaluation_report"`. |

Only datasets that were evaluated appear. `reports["performance_evaluation_report"]` is the one you normally publish.

## Read the results

### The summary table

`summary_metrics` has one row per scope and one column per metric. With `segment_columns=["region"]` the first row is the aggregate, with `region` set to `"GLOBAL"`, followed by one row per surviving region:

```python
summary = result.metrics["validation"].summary_metrics.value.to_pandas()
summary[summary["region"] == "GLOBAL"]        # the aggregate
summary.sort_values("auc").head()             # the weakest segments
```

Without `segment_columns` the table has a single row. `summary_metrics` is a `DatasetVariable`, so a downstream task can read it back, for instance to write it to a warehouse table, without the evaluator holding it in memory.

### The evaluation report

Each report is an `EvaluationReport` proto. Load one from its variable with `.value`, then walk `report.spec.charts`.

| Report | Title | Notes |
| --- | --- | --- |
| Per dataset | `Evaluation Report - <dataset name>` | One per evaluated dataset. |
| Combined | `Evaluation Report` | Under `"performance_evaluation_report"`. With more than one dataset the table gains a `DatasetName` column, and chart segments are prefixed with the dataset name. Its `metadata.name` is generated with the prefix `evaluation-report`. |

A report contains these charts, in this order:

| Chart | Section id | When it appears |
| --- | --- | --- |
| Metrics table | `Metrics` | Always. Segment columns first, then one column per metric. `NaN` values show as `N/A`. |
| Segment filter | `SegmentFilter` | Binary-classification curves exist and there is more than one segment. |
| ROC Curve, Precision Recall | `ROCChart`, `PRChart` | Binary-classification curves exist. |
| Absolute Errors, Errors Chart | `AbsErrors`, `ErrorsChart` | Regression error data exists. |
| Confusion matrix and threshold filter | `ConfusionMatrix_<pred>_<target>`, `ThresholdFilter_<pred>_<target>` | Binary-classification curves exist, one pair per prediction and target column pair. |

Curve data is computed from the same rows as the scalar metrics, using each metric's `filter_expr`. A report built from an empty summary has no charts.

## Where the report goes

`evaluate()` saves each report as a workflow variable and nothing more. It does not write to an external destination. Publishing is the job of `push()` with the `eval_report_plugin` item, which sends the report to one or more **sinks**:

| Sink | Behaviour |
| --- | --- |
| `LocalFileEvalReportSink` | The default when `sinks` is not set. Writes `<report name>.json` to `LocalFileEvalReportSinkConfig(output_dir=...)`, or to a fresh `michelangelo_reports_*` temp directory when no directory is given. |
| `APIClientEvalReportSink` | Creates the report through the API server's `EvaluationReportService`. It needs `MA_API_SERVER` set and does not set a namespace for you; set `report.metadata.namespace` first. |
| Your own | Subclass `EvalReportSink`. See [Evaluation report sinks](./push-models-and-artifacts.md#5-evaluation-report-sinks-evalreportsink). |

Pass `sinks=[]` for a dry run that writes nothing. `report_name` overrides the name on the proto, and `extra_fields` merges key-value pairs, such as a CI run id, into the JSON that file-writing sinks produce.

```python
import tempfile

from michelangelo.lib.artifact_manager.storage_backend import LocalStorageBackend
from michelangelo.workflow.schema.pusher import (
    EvalReportPluginConfig,
    PusherConfig,
    PusherPluginConfig,
)
from michelangelo.workflow.tasks.pusher import push

report = result.reports["performance_evaluation_report"].value

push(
    config=PusherConfig(
        items=[
            PusherPluginConfig(
                name="eval",
                eval_report_plugin=EvalReportPluginConfig(report_name="q1-eval"),
            )
        ]
    ),
    artifacts={"eval": report},
    storage_backend=LocalStorageBackend(tempfile.mkdtemp()),
)
```

`push()` requires a `storage_backend` even though the report plugin does not use it. See [Push Models and Artifacts](./push-models-and-artifacts.md) for the full `push()` contract, error handling and the other plugins.

The local file is the report proto as JSON with original proto field names, so you can read it with the standard library:

```python
import json

with open("/tmp/michelangelo_reports_abc123/q1-eval.json") as f:
    report = json.load(f)

print(report["spec"]["title"], len(report["spec"]["charts"]))
```

## When a sanity check fails

If any check breaches its threshold, `evaluate()` raises `SanityCheckFailedError` (from `michelangelo.workflow.tasks.evaluator`), and the evaluator task fails. Three details matter in practice:

- **The reports are written first.** Every summary table and every report is saved before the exception is raised, so the evidence for the breach exists. The exception means `evaluate()` never returns, so a `push()` in the same task never runs, and nothing is published to a sink.
- **The message names every breach.** Each line gives the dataset, the scope, the metric, the actual value and the requirement, for example:

  ```text
  SanityCheckFailedError: Evaluator sanity checks failed: 1 of 4 check(s) breached.
    - validation[region=us]/auc: actual 0.476239, required value >= 0.7
  ```

  The message lists at most 20 breaches and then reports how many more were hidden and which metrics they cover. Failures are also logged at INFO as `[SANITY] [FAIL] ...`, capped at 20 lines per run, and passes are logged at DEBUG.
- **It is a verdict on the model, not the config.** `SanityCheckFailedError` is distinct from `ConfigurationError`, which covers a malformed check and is raised before any data is read. Because a threshold breach is deterministic, retrying it only spends cluster time.

Other failures to expect from `evaluate()`:

| Exception | Cause |
| --- | --- |
| `ConfigurationError` | No `metrics`; `steps` set (step-based evaluation is not supported); a sanity check naming an unknown metric or a dataset that was not evaluated; a bad `data_preprocessor`; a check whose dataset had no scope, for example `enable_global_segment_evaluation=False` with every segment filtered out. Always `michelangelo.workflow.schema.exceptions.ConfigurationError`. |
| `ValueError` | A dataset is missing a column the metrics or preprocessor need, or a non-`test` dataset is empty. |

To see the gate fire, run the example with `BREAK_IT=1`. It shuffles one region's scores against its labels, so that region's AUROC drops to chance while the aggregate still passes. Only the per-region check fails the run.

```bash
BREAK_IT=1 PYTHONPATH="." poetry run python ./examples/evaluator/evaluator.py
```

## Use the library on its own

`michelangelo.lib.evaluator` does not need a pipeline. `create_evaluator` accepts an `EvaluatorConfig`, a dict, or a YAML path, and scores a pandas DataFrame:

```python
from michelangelo.lib.evaluator.evaluator import create_evaluator

evaluator = create_evaluator(config)
evaluator.validate_dataset(df)       # raises ValueError naming any missing columns
metrics = evaluator.evaluate(df)     # {"auc": 0.95, "accuracy": 0.87}
```

This computes the aggregate metrics only. Segmentation, charts, reports and sanity checks are applied by the workflow task. For generated reference material, see the [Python SDK reference](../../api-reference/python-sdk/index.md).

## Next steps

- [Push Models and Artifacts](./push-models-and-artifacts.md) to publish the report, and to write your own report sink.
- [Evaluator Workflow example](../examples/index.md#evaluation) for a runnable end-to-end workflow.
- [Model Training Guide](./train-and-register-a-model.md) for producing the predictions you evaluate.
