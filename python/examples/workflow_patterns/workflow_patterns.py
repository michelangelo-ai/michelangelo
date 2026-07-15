r"""Uniflow workflow patterns: sequential, branching, loops, concurrent, datasets.

Demonstrates every core Uniflow orchestration pattern in a single runnable
file. The example builds a toy multi-shard data pipeline that generates
synthetic data, normalises it, and sweeps over decision thresholds in parallel.

Run from ``python/`` (the OSS Poetry root):

    # Install required extras (ray and pandas are needed)
    poetry install --extras "example"
    python -m examples.workflow_patterns.workflow_patterns

For remote execution on a Michelangelo cluster:

    python -m examples.workflow_patterns.workflow_patterns remote-run \
        --project ma-examples \
        --image ghcr.io/michelangelo-ai/examples:main

Starlark / workflow-function restrictions (applies to remote execution via
Cadence/Temporal — not enforced in local runs, but follow them to keep the
workflow portable):
  - No standard-library imports (put imports inside ``@uniflow.task`` bodies)
  - No f-strings — use ``"{}".format(x)`` instead
  - No ``is`` / ``is not`` comparisons — use ``==`` / ``!=``
  - No ``try`` / ``except``
  - No chained comparisons (``if 1 < x < 5`` — split into ``and``)
  - No global variables referenced from the workflow body
"""

from __future__ import annotations

import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.core.lib.concurrent import (
    batch_run as concurrent_batch_run,
)
from michelangelo.uniflow.core.lib.concurrent import (
    new_callable,
)
from michelangelo.uniflow.core.lib.concurrent import (
    run as concurrent_run,
)
from michelangelo.uniflow.plugins.ray import RayTask
from michelangelo.workflow.variables import DatasetVariable

# ---------------------------------------------------------------------------
# Tasks
# All task bodies run in isolated containers (Ray clusters in remote mode).
# Any Python code is allowed inside task bodies.
# ---------------------------------------------------------------------------


@uniflow.task(config=RayTask(head_cpu=1, head_memory="2Gi"))
def generate_shard(shard_id: int, n_rows: int = 500) -> DatasetVariable:
    """Generate a synthetic data shard and persist it as a pandas DataFrame.

    Returns a DatasetVariable so downstream workflow steps can pass the shard
    across task boundaries without materialising it in the workflow process.
    """
    import numpy as np
    import pandas as pd

    rng = np.random.default_rng(shard_id)
    df = pd.DataFrame(
        {
            "feature_a": rng.normal(0, 1, n_rows),
            "feature_b": rng.normal(1, 2, n_rows),
            "score": rng.uniform(0, 1, n_rows),
            "label": rng.integers(0, 2, n_rows),
        }
    )
    dv = DatasetVariable.create(df)
    dv.save_pandas_dataframe()
    return dv


@uniflow.task(config=RayTask(head_cpu=1, head_memory="2Gi"))
def normalize_shard(shard: DatasetVariable) -> DatasetVariable:
    """Z-score normalize ``feature_a`` and ``feature_b`` in-place."""
    shard.load_pandas_dataframe()
    df = shard.value.copy()
    for col in ["feature_a", "feature_b"]:
        std = df[col].std()
        if std > 0:
            df[col] = (df[col] - df[col].mean()) / std
    result = DatasetVariable.create(df)
    result.save_pandas_dataframe()
    return result


@uniflow.task(config=RayTask(head_cpu=1, head_memory="2Gi"))
def compute_stats(shard: DatasetVariable) -> dict:
    """Return summary statistics for the shard (row count and label balance)."""
    shard.load_pandas_dataframe()
    df = shard.value
    return {
        "n_rows": len(df),
        "label_mean": round(float(df["label"].mean()), 4),
        "feature_a_mean": round(float(df["feature_a"].mean()), 4),
    }


@uniflow.task(config=RayTask(head_cpu=1, head_memory="2Gi"))
def evaluate_threshold(shard: DatasetVariable, threshold: float) -> dict:
    """Score a decision threshold on ``score`` vs ``label``.

    Returns precision (fraction of predictions matching labels) and the
    threshold so results can be ranked after the batch completes.
    """
    shard.load_pandas_dataframe()
    df = shard.value
    predictions = (df["score"] >= threshold).astype(int)
    precision = float((predictions == df["label"]).mean())
    return {"threshold": threshold, "precision": round(precision, 4)}


@uniflow.task(config=RayTask(head_cpu=1, head_memory="2Gi"))
def pick_best_threshold(calibration_results: list) -> dict:
    """Select the threshold with the highest precision."""
    best = max(calibration_results, key=lambda r: r["precision"])
    return {
        "best_threshold": best["threshold"],
        "best_precision": best["precision"],
        "n_candidates": len(calibration_results),
    }


# ---------------------------------------------------------------------------
# Workflow
#
# In local runs, the workflow body is plain Python. In remote runs via
# Cadence/Temporal it is transpiled to Starlark for deterministic replay —
# follow the restrictions in the module docstring to keep it portable.
# ---------------------------------------------------------------------------


@uniflow.workflow()
def pipeline(
    n_shards: int = 4,
    large_dataset: bool = False,
):
    """End-to-end workflow showing all Uniflow orchestration patterns.

    Pattern 1 — Sequential task calls
    Pattern 2 — If/else branching
    Pattern 3 — For loop
    Pattern 4 — Concurrent run (fan-out / fan-in with Futures)
    Pattern 5 — Parallel batch execution (new_callable + concurrent_batch_run)
    Pattern 6 — DatasetVariable for cross-task data passing
    """
    # ------------------------------------------------------------------
    # Pattern 1: Sequential task calls
    # ------------------------------------------------------------------
    # Calling a task directly blocks the workflow until it completes.
    # Output is automatically checkpointed so the run can resume if it
    # fails mid-flight.
    baseline = generate_shard(0)  # DatasetVariable returned by task
    baseline_stats = compute_stats(baseline)  # receives the DatasetVariable

    # ------------------------------------------------------------------
    # Pattern 2: If/else branching
    # ------------------------------------------------------------------
    # Standard Python if/else works in workflows.  The condition is
    # evaluated at runtime.
    # Note: no f-strings in remote mode — use .format() for string interpolation.
    n_rows = 500
    if large_dataset:
        n_rows = 5000

    # ------------------------------------------------------------------
    # Pattern 3: For loop — generate and normalize every shard
    # ------------------------------------------------------------------
    # Loops run sequentially in the workflow.  Each iteration waits for
    # the previous task to complete before starting the next one.
    # Use Pattern 4 (concurrent_run) when shards are independent.
    normalized_shards = []
    for i in range(n_shards):
        raw = generate_shard(i, n_rows)
        norm = normalize_shard(raw)  # DatasetVariable passes through
        normalized_shards.append(norm)

    # ------------------------------------------------------------------
    # Pattern 4: Concurrent run — fan-out / fan-in
    # ------------------------------------------------------------------
    # concurrent_run(task, *args) kicks off a task asynchronously and
    # returns a Future immediately. The workflow continues to the next
    # line while the task executes in the background.
    #
    # Call future.result() to block and collect the return value.
    # Place all concurrent_run() calls before any .result() calls to
    # maximise overlap:
    #
    #   future_a = concurrent_run(task, ...)   # start both
    #   future_b = concurrent_run(task, ...)
    #   result_a = future_a.result()           # then collect
    #   result_b = future_b.result()
    #
    # In local mode futures are pre-resolved (sequential execution).
    # In remote mode tasks run in parallel on separate Ray clusters.
    future_a = concurrent_run(generate_shard, 100, n_rows)
    future_b = concurrent_run(generate_shard, 101, n_rows)
    raw_a = future_a.result()
    raw_b = future_b.result()
    # Normalize the concurrently-generated shards and add them to the pool
    norm_a = normalize_shard(raw_a)
    norm_b = normalize_shard(raw_b)
    all_shards = [*normalized_shards, norm_a, norm_b]

    # ------------------------------------------------------------------
    # Pattern 5: Parallel batch execution
    # ------------------------------------------------------------------
    # new_callable(task, *args) creates a deferred call object.
    # concurrent_batch_run(callables, max_concurrency=N) submits all
    # callables and runs up to N at a time. batch_future.get() blocks
    # until all complete and returns results in submission order.
    # Sweep thresholds across all shards (for-loop shards + fan-out shards).
    thresholds = [0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9]
    callables = [
        new_callable(evaluate_threshold, shard, t)
        for shard in all_shards
        for t in thresholds
    ]
    batch_future = concurrent_batch_run(callables, max_concurrency=3)
    calibration_results = batch_future.get()  # list in submission order

    # ------------------------------------------------------------------
    # Pattern 6: DatasetVariable — cross-task data sharing (recap)
    # ------------------------------------------------------------------
    # DatasetVariable wraps a dataset (pandas, Spark, or Ray) so it can
    # be persisted to object storage and passed between tasks as a
    # lightweight reference rather than serialised in the workflow state.
    #
    # Producer side (inside a task body):
    #   dv = DatasetVariable.create(df)
    #   dv.save_pandas_dataframe()     # or save_ray_dataset(), save_spark_dataframe()
    #   return dv
    #
    # Consumer side (inside a task body):
    #   shard.load_pandas_dataframe()  # or load_ray_dataset(), load_spark_dataframe()
    #   df = shard.value
    #
    # See generate_shard / normalize_shard / evaluate_threshold above for
    # the complete producer-consumer implementation.

    result = pick_best_threshold(calibration_results)
    return {"best_threshold": result, "baseline_stats": baseline_stats}


# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    ctx = uniflow.create_context()
    ctx.run(pipeline)
