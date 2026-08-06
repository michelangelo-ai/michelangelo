"""Workflow + tasks for the CanvasFlex tabular_train YAML example.

A minimal form of the internal ``tabular_train`` pipeline, authored for
``pipeline_conf.yaml`` (see the file next to this module): a Spark task loads a
dataset from a configurable Spark SQL query and splits it, then a Ray task fits
a (deliberately simple) threshold model on the training split.

The workflow is generic: everything use-case-specific — the data source query,
the split ratio, the feature/target columns, the trainer hyperparameters, and
per-task cluster sizing (``job_specs``) — comes from ``pipeline_conf.yaml``.
Pointing ``source.spark_sql.query`` at a real table (instead of this demo's
``range()``-generated rows) is all it takes to run it on real data.

Mirrors the internal conventions: task functions take a typed ``config`` first
parameter resolved from YAML by annotation; the decorator carries no resource
values (bare ``SparkTask()``/``RayTask()`` — cluster sizing belongs to the
YAML's ``job_specs``, falling back to the Starlark defaults when absent).
"""

import ray
from pyspark.sql import SparkSession

import michelangelo.uniflow.core as uniflow
from michelangelo.canvas.lib.shared.json_data.json_data import JSONData
from michelangelo.canvas.pipeline.task import pipeline_task
from michelangelo.canvas.schema.v2alpha1.config import TaskConfig
from michelangelo.uniflow.plugins.ray import RayTask
from michelangelo.uniflow.plugins.spark import SparkTask


class SparkSqlSpec(JSONData):
    """A data source expressed as a Spark SQL query."""

    query: str


class DataSourceSpec(JSONData):
    """Where ``tabular_feature_prep`` reads its input from.

    ``spark_sql`` is the only variant today. The internal DataSource also
    supports registered datasets (``dataset.namespace``/``dataset.name``);
    OSS has no dataset registry yet, so that variant is future work.
    """

    spark_sql: SparkSqlSpec


class SplitRatioSpec(JSONData):
    """Train/validation split ratio."""

    train_ratio: float


class SplitSpec(JSONData):
    """How to split the loaded dataset."""

    ratio: SplitRatioSpec


class TabularFeaturePrepConfig(JSONData):
    """Config for the ``tabular_feature_prep`` Spark task."""

    source: DataSourceSpec
    split: SplitSpec


class TabularTrainerConfig(JSONData):
    """Config for the ``tabular_trainer`` Ray task."""

    feature_column: str
    target_column: str
    num_thresholds: int


@pipeline_task(config=SparkTask())
def tabular_feature_prep(config: TabularFeaturePrepConfig) -> dict:
    """Load the configured Spark SQL source and split into train/validation.

    Uses the Spark session created by ``SparkTask.pre_run`` and returns plain
    Python rows so downstream tasks don't need Spark installed to consume them.
    """
    spark = SparkSession.getActiveSession()
    assert spark is not None, "SparkTask.pre_run should have created a session"

    df = spark.sql(config.source.spark_sql.query)
    train_ratio = config.split.ratio.train_ratio
    train_df, validation_df = df.randomSplit([train_ratio, 1.0 - train_ratio], seed=42)

    datasets = {
        "train": [row.asDict() for row in train_df.collect()],
        "validation": [row.asDict() for row in validation_df.collect()],
    }
    print(
        "tabular_feature_prep: train rows:",
        len(datasets["train"]),
        "validation rows:",
        len(datasets["validation"]),
    )
    return datasets


@ray.remote
def _threshold_accuracy(
    rows: list, threshold: float, feature: str, target: str
) -> float:
    """Accuracy of ``feature >= threshold`` as a classifier for ``target``."""
    if not rows:
        return 0.0
    correct = sum(1 for r in rows if (r[feature] >= threshold) == (r[target] == 1))
    return correct / len(rows)


@pipeline_task(config=RayTask())
def tabular_trainer(
    config: TabularTrainerConfig, train_dataset: list, validation_dataset: list
) -> dict:
    """Fit a threshold classifier on Ray: grid-search candidates in parallel.

    A stand-in for a real trainer with the same task contract as the internal
    ``tabular_trainer`` (config + train/validation datasets in, model out).
    """
    candidates = [i / (config.num_thresholds - 1) for i in range(config.num_thresholds)]
    futures = [
        _threshold_accuracy.remote(
            train_dataset, t, config.feature_column, config.target_column
        )
        for t in candidates
    ]
    train_accuracies = ray.get(futures)

    best_index = max(range(len(candidates)), key=lambda i: train_accuracies[i])
    best_threshold = candidates[best_index]
    validation_accuracy = ray.get(
        _threshold_accuracy.remote(
            validation_dataset,
            best_threshold,
            config.feature_column,
            config.target_column,
        )
    )

    model = {
        "threshold": best_threshold,
        "feature_column": config.feature_column,
        "target_column": config.target_column,
        "train_accuracy": train_accuracies[best_index],
        "validation_accuracy": validation_accuracy,
    }
    print("tabular_trainer: model", model)
    return model


@uniflow.workflow()
def tabular_train(task_configs: dict[str, TaskConfig]):
    """Minimal tabular_train: Spark feature prep, then Ray training."""
    datasets = tabular_feature_prep(config=task_configs["tabular_feature_prep"])
    model = tabular_trainer(
        config=task_configs["tabular_trainer"],
        train_dataset=datasets["train"],
        validation_dataset=datasets["validation"],
    )
    return {"model": model}
