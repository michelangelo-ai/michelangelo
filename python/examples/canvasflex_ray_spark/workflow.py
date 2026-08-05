"""Workflow + tasks for the CanvasFlex Ray/Spark YAML example.

A ``tabular_eval``-shaped pipeline authored for ``pipeline_conf.yaml`` (see the
file next to this module): a Spark task generates a small synthetic tabular
dataset, a Ray task scores it against a threshold, and an *optional* Ray task
summarizes the metrics — it only runs when the YAML defines a ``summarize``
entry under ``task_configs``.

All data is synthetic and generated at task runtime; there are no external
downloads, so the pipeline runs as-is in the examples Docker image.

The decorator ``RayTask``/``SparkTask`` values below are intentionally small
defaults. The ``job_specs`` blocks in ``pipeline_conf.yaml`` use *different*
values, so a run through the sandbox proves that YAML ``job_specs`` win over
the decorator defaults.
"""

import ray
from pyspark.sql import SparkSession
from pyspark.sql import functions as sf

import michelangelo.uniflow.core as uniflow
from michelangelo.canvas.lib.shared.json_data.json_data import JSONData
from michelangelo.canvas.pipeline.task import pipeline_task
from michelangelo.uniflow.plugins.ray import RayTask
from michelangelo.uniflow.plugins.spark import SparkTask


class PrepareDataConfig(JSONData):
    """Config for the ``prepare_data`` Spark task."""

    num_rows: int
    seed: int


class EvaluateConfig(JSONData):
    """Config for the ``evaluate`` Ray task."""

    threshold: float
    num_shards: int


class SummarizeConfig(JSONData):
    """Config for the optional ``summarize`` Ray task."""

    experiment_name: str


@pipeline_task(
    config=SparkTask(
        driver_cpu=1,
        driver_memory="1G",
        driver_disk="10G",
        driver_gpu=0,
        executor_cpu=1,
        executor_memory="1G",
        executor_disk="10G",
        executor_gpu=0,
        executor_instances=1,
    )
)
def prepare_data(config: PrepareDataConfig) -> list:
    """Generate a small synthetic tabular dataset with Spark.

    Uses the Spark session created by ``SparkTask.pre_run`` and returns plain
    Python rows so downstream tasks don't need Spark installed to consume them.
    """
    spark = SparkSession.getActiveSession()
    assert spark is not None, "SparkTask.pre_run should have created a session"

    df = (
        spark.range(config.num_rows)
        .withColumn("label", (sf.col("id") % 2).cast("int"))
        .withColumn("score", sf.rand(seed=config.seed))
    )
    rows = [row.asDict() for row in df.collect()]
    print("prepare_data: generated", len(rows), "rows")
    return rows


@ray.remote
def _score_shard(rows: list, threshold: float) -> dict:
    """Score one shard of rows: prediction is score >= threshold."""
    correct = sum(1 for r in rows if (r["score"] >= threshold) == (r["label"] == 1))
    return {"rows": len(rows), "correct": correct}


@pipeline_task(
    config=RayTask(
        head_cpu=1,
        head_memory="1Gi",
        head_disk="4Gi",
        head_gpu=0,
        worker_cpu=1,
        worker_memory="1Gi",
        worker_disk="4Gi",
        worker_gpu=0,
        worker_instances=1,
    )
)
def evaluate(config: EvaluateConfig, rows: list) -> dict:
    """Score the dataset on Ray, fanning shards out as Ray remote tasks."""
    shards = [rows[i :: config.num_shards] for i in range(config.num_shards)]
    futures = [
        _score_shard.remote(shard, config.threshold) for shard in shards if shard
    ]
    results = ray.get(futures)

    total = sum(r["rows"] for r in results)
    correct = sum(r["correct"] for r in results)
    metrics = {
        "rows": total,
        "correct": correct,
        "accuracy": correct / total if total else 0.0,
        "threshold": config.threshold,
    }
    print("evaluate: metrics", metrics)
    return metrics


@pipeline_task(
    config=RayTask(
        head_cpu=1,
        head_memory="1Gi",
        head_disk="4Gi",
        head_gpu=0,
        worker_cpu=1,
        worker_memory="1Gi",
        worker_disk="4Gi",
        worker_gpu=0,
        worker_instances=1,
    )
)
def summarize(config: SummarizeConfig, metrics: dict) -> dict:
    """Optional Ray task: tag metrics with the experiment name."""
    summary = dict(metrics)
    summary["experiment_name"] = config.experiment_name
    print("summarize: summary", summary)
    return summary


@uniflow.workflow()
def ray_spark_pipeline(task_configs: dict):
    """Spark data prep, Ray evaluation, and an optional Ray summary task."""
    rows = prepare_data(config=task_configs["prepare_data"])
    metrics = evaluate(config=task_configs["evaluate"], rows=rows)
    if "summarize" in task_configs:
        return summarize(config=task_configs["summarize"], metrics=metrics)
    return metrics
