r"""Uniflow example: score a model, segment the results, gate on a threshold.

Builds a two-step workflow -- generate predictions, then evaluate them -- to
show the three things the evaluator task is for:

  1. **Metrics from config.** ``MetricConfig`` names a TorchMetrics class and
     the columns to feed it. No metric code is written here.
  2. **Segmentation.** ``segment_columns`` splits the results per region, on
     top of the aggregate row, so a model that looks fine overall but is broken
     in one market is visible.
  3. **Gating.** ``sanity_checks`` fails the pipeline when a metric breaches a
     threshold, *after* the reports are written, so the breach can be debugged.

Run from ``python/`` (the OSS Poetry root):

    poetry install --extras "example" --extras "evaluator"
    PYTHONPATH="." poetry run python ./examples/evaluator/evaluator.py

Set ``BREAK_IT=1`` to see the gate fire on a deliberately broken model:

    BREAK_IT=1 PYTHONPATH="." poetry run python ./examples/evaluator/evaluator.py

Starlark / workflow-function restrictions (remote execution via
Cadence/Temporal; not enforced locally, but follow them to keep the workflow
portable):
  - No standard-library imports (put imports inside ``@uniflow.task`` bodies)
  - No f-strings -- use ``"{}".format(x)`` instead
  - No ``is`` / ``is not`` comparisons -- use ``==`` / ``!=``
  - No ``try`` / ``except``
  - No chained comparisons, no globals referenced from the workflow body
  - No return annotation on the workflow itself -- the transpiler resolves it
    as a name against the module's globals, so even ``-> dict`` fails
"""

from __future__ import annotations

import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask
from michelangelo.workflow.variables import DatasetVariable

# ---------------------------------------------------------------------------
# Tasks — run in isolated containers (Ray clusters in remote mode). Ordinary
# Python is allowed inside task bodies, imports included.
# ---------------------------------------------------------------------------


@uniflow.task(config=RayTask(head_cpu=1, head_memory="2Gi"))
def score_dataset(n_rows: int = 2000, broken_region: str = "") -> DatasetVariable:
    """Stand in for an inference task: emit scores, labels, and a segment column.

    Args:
        n_rows: Number of rows to generate.
        broken_region: When non-empty, that region's scores are shuffled
            against its labels, so its AUROC collapses to chance while the
            other regions stay healthy.

    Returns:
        A ``DatasetVariable`` holding ``score``, ``label``, and ``region``.
    """
    import numpy as np
    import pandas as pd

    rng = np.random.default_rng(0)
    labels = rng.integers(0, 2, n_rows)
    # A usable but imperfect model: the score correlates with the label, with
    # enough noise that the AUROC lands short of 1.0.
    scores = np.clip(labels * 0.45 + rng.normal(0.3, 0.2, n_rows), 0.0, 1.0)
    regions = rng.choice(["us", "eu", "ap"], n_rows)

    if broken_region:
        mask = regions == broken_region
        scores[mask] = rng.permutation(scores[mask])

    dataset = DatasetVariable.create(
        pd.DataFrame({"score": scores, "label": labels, "region": regions})
    )
    dataset.save_pandas_dataframe()
    return dataset


@uniflow.task(config=RayTask(head_cpu=2, head_memory="4Gi"))
def evaluate_predictions(predictions: DatasetVariable, min_auc: float) -> dict:
    """Evaluate the scored dataset and gate the run on its AUROC.

    Args:
        predictions: The scored dataset produced by :func:`score_dataset`.
        min_auc: Threshold the AUROC must reach in every scope -- the
            aggregate and each region. A breach raises
            ``SanityCheckFailedError`` once the reports have been written.

    Returns:
        A mapping of metric name to its aggregate value.
    """
    from michelangelo.workflow.schema.evaluator import (
        ColumnMapping,
        EvaluatorConfig,
        MetricConfig,
        SanityCheck,
    )
    from michelangelo.workflow.tasks.evaluator import evaluate

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
        # One row per region, on top of the aggregate GLOBAL row.
        segment_columns=["region"],
        # Segments thinner than this are dropped rather than gated: a handful
        # of rows cannot support a conclusion, and a single-class segment
        # yields a degenerate AUROC.
        min_segment_row_count=50,
        sanity_checks=[
            SanityCheck(metric="auc", threshold=min_auc, datasets=["validation"])
        ],
    )

    result = evaluate(config, {"validation": predictions})

    # summary_metrics holds one row per scope; the first is the aggregate.
    result.metrics["validation"].summary_metrics.load_ray_dataset()
    summary = result.metrics["validation"].summary_metrics.value.to_pandas()
    aggregate = summary[summary["region"] == "GLOBAL"].iloc[0]
    metrics = {
        "auc": float(aggregate["auc"]),
        "accuracy": float(aggregate["accuracy"]),
    }
    # Printed here rather than in main(): a local ctx.run() does not hand the
    # workflow's return value back to the caller.
    print(f"Aggregate metrics: {metrics}")
    return metrics


# ---------------------------------------------------------------------------
# Workflow — the orchestration body. Keep it to task calls and plain control
# flow so it stays portable to remote (Starlark) execution.
# ---------------------------------------------------------------------------


@uniflow.workflow()
def evaluation_workflow(broken_region: str = "", min_auc: float = 0.7):
    """Score a dataset, then evaluate and gate it.

    Args:
        broken_region: Region to corrupt, or ``""`` for a healthy model.
        min_auc: AUROC every scope (aggregate and each region) must reach.

    Returns:
        The aggregate metrics reported by the evaluator.
    """
    predictions = score_dataset(2000, broken_region)
    return evaluate_predictions(predictions, min_auc)


def main() -> None:
    """Run the workflow locally, healthy by default."""
    import os

    # An environment variable rather than a CLI argument: create_context()
    # reads sys.argv[1] as the run target (local-run / remote-run).
    break_it = os.environ.get("BREAK_IT") == "1"
    # Corrupting one of three regions sinks that region's AUROC to chance
    # while the aggregate stays above 0.7: only the per-region check fires.
    broken_region = "us" if break_it else ""

    ctx = uniflow.create_context()
    ctx.run(evaluation_workflow, broken_region=broken_region, min_auc=0.7)


if __name__ == "__main__":
    main()
