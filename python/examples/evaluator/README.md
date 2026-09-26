# Evaluator workflow example

Runnable reference for the evaluator task: metrics declared in config, results split per segment, and a threshold that fails the pipeline when the model regresses.

## What it demonstrates

| Capability | Where in the code |
|---|---|
| **Metrics from config** | `MetricConfig(metric="torchmetrics.classification.BinaryAUROC", ...)` — no metric code is written |
| **Column mapping** | `ColumnMapping(prediction_col="score", target_col="label")` |
| **Segmentation** | `segment_columns=["region"]` — one row per region plus the aggregate `GLOBAL` row |
| **Thin-segment filtering** | `min_segment_row_count=50` — segments too small to conclude from are dropped, not gated |
| **Gating** | `SanityCheck(metric="auc", threshold=min_auc)` → `SanityCheckFailedError` |
| **Reading results back** | `result.metrics["validation"].summary_metrics` |

## Run it

```bash
poetry install --extras "example" --extras "evaluator"
PYTHONPATH="." poetry run python ./examples/evaluator/evaluator.py
```

The model is healthy by default, and every check passes:

```
INFO | michelangelo.workflow.tasks.evaluator.task | All 4 sanity check(s) passed
Aggregate metrics: {'auc': 0.949..., 'accuracy': 0.875}
```

Set `BREAK_IT=1` to shuffle one region's scores against its labels. (It is an
environment variable because `create_context()` reads the first CLI argument as
the run target.)

```bash
BREAK_IT=1 PYTHONPATH="." poetry run python ./examples/evaluator/evaluator.py
```

That region's AUROC collapses to chance. The aggregate still clears 0.7, so an
aggregate-only check would have passed. The per-region check is what fails the run:

```
[SANITY] [FAIL] validation[region=us]/auc: actual 0.476239, required value >= 0.7
SanityCheckFailedError: Evaluator sanity checks failed: 1 of 4 check(s) breached.
  - validation[region=us]/auc: actual 0.476239, required value >= 0.7
```

The reports are generated *before* the exception is raised, so a breach never
costs you the report that explains it.

**Many-core machines.** Locally, Ray claims every CPU. The per-region
`groupby` uses a hash shuffle that starts roughly one aggregator per CPU, each
reserving about 1 GiB of memory. On a box with more cores than GiB of RAM, the
aggregators cannot all start and the run hangs. Cap the local cluster:

```bash
_RAY_INIT_KWARGS="{'num_cpus': 4}" PYTHONPATH="." poetry run python ./examples/evaluator/evaluator.py
```

## What to change next

- Add a metric: append another `MetricConfig`. Metrics that read the same
  columns with the same dtypes share one pass over the data.
- Segment by more than one column: `segment_columns=["region", "datestr"]`.
- Evaluate several datasets: pass `{"validation": ..., "test": ...}`. Each gets
  its own report, plus a combined one under `"performance_evaluation_report"`.
- Reshape rows before scoring: write a `@declare_inputs([...])` function and
  name it in `EvaluatorConfig.data_preprocessor`. The declared columns are the
  only ones read from parquet.
