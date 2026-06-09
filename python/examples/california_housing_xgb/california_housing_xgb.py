"""XGBoost regression workflow for California Housing price prediction.

Workflow entry point that orchestrates the California Housing data pipeline:
feature preparation, Spark preprocessing, and distributed XGBoost training
with Ray. The individual task implementations live in sibling modules
(``feature_prep``, ``preprocess``, ``train``).

Model push is added in the pusher integration layer — see ``push_model.py``.
"""

from __future__ import annotations

import michelangelo.uniflow.core as uniflow
from examples.california_housing_xgb.feature_prep import feature_prep
from examples.california_housing_xgb.preprocess import PreprocessResult, preprocess
from examples.california_housing_xgb.train import TrainResult, train
from michelangelo.uniflow.plugins.ray import RayTask
from michelangelo.uniflow.plugins.spark import SparkTask

__all__ = [
    "PreprocessResult",
    "TrainResult",
    "feature_prep",
    "preprocess",
    "train",
    "train_workflow",
]

# California Housing features + target column order.
# MedHouseVal (the sklearn target) is renamed to "target" in feature_prep.


@uniflow.workflow()
def train_workflow(
    dataset_cols: str = (
        "MedInc,HouseAge,AveRooms,AveBedrms,Population,AveOccup,Latitude,Longitude,target"
    ),
):
    """Data pipeline workflow: feature prep, preprocessing, and training.

    Orchestrates the California Housing data pipeline: feature preparation,
    preprocessing with Spark, and distributed training with Ray XGBoost.

    Args:
        dataset_cols: Comma-separated string of column names including
            features and target. Example:
            "feature1,feature2,feature3,target".

    Returns:
        TrainResult containing the checkpoint path and training metrics.
    """
    _dataset_cols = dataset_cols.split(",")
    feature_prep_overrides = feature_prep.with_overrides(
        alias="feature_prep_overrides",
        config=RayTask(
            head_cpu=2,
            worker_instances=1,
        ),
    )
    train_dv, validation_dv = feature_prep_overrides(
        columns=_dataset_cols,
    )
    pr = preprocess.with_overrides(
        alias="preprocess_overrides",
        config=SparkTask(executor_cpu=1, driver_cpu=1),
    )(
        cast_float_columns=_dataset_cols,
        train_dv=train_dv,
        validation_dv=validation_dv,
    )
    return train(
        pr,
        params={
            "objective": "reg:squarederror",
            "colsample_bytree": 0.3,
            "learning_rate": 0.1,
            "max_depth": 5,
            "alpha": 10,
            "n_estimators": 10,
        },
    )


if __name__ == "__main__":
    ctx = uniflow.create_context()

    ctx.environ["IMAGE_PULL_POLICY"] = "IfNotPresent"

    # Pass MINIO_* and REGISTRY_* via --environ flags on the command line
    # so values reach remote Ray workers (see README Remote Run section).

    ctx.run(train_workflow)
