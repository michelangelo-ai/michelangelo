"""XGBoost training task for the California Housing workflow.

Trains an XGBoost regression model on preprocessed California Housing data
using Ray's distributed XGBoostTrainer.
"""

from __future__ import annotations

import logging
import os
from dataclasses import dataclass
from typing import TYPE_CHECKING

import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask

if TYPE_CHECKING:
    import ray.data

    from examples.pipelines.california_housing_xgb.preprocess import PreprocessResult

log = logging.getLogger(__name__)

__all__ = ["TrainResult", "train"]


@dataclass
class TrainResult:
    """Container for training results.

    Attributes:
        path: Path to saved model.
        metrics: Optional dictionary of evaluation metrics.
    """

    path: str
    metrics: dict | None = None


@uniflow.task(
    config=RayTask(
        head_cpu=1,
        head_gpu=0,
        head_memory="4Gi",
        worker_cpu=1,
        worker_gpu=0,
        worker_memory="4Gi",
        worker_instances=2,
    ),
)
def train(
    pr: PreprocessResult,
    params: dict[str, float | int],
) -> TrainResult:
    """Train XGBoost model using Ray for distributed training.

    Trains an XGBoost regression model on preprocessed California Housing data
    using Ray's distributed XGBoostTrainer.

    Args:
        pr: PreprocessResult containing preprocessed training and validation datasets.
        params: Dictionary of XGBoost hyperparameters (e.g., max_depth, learning_rate).

    Returns:
        TrainResult containing the path to saved model and training metrics.
    """
    import ray
    import ray.data
    import xgboost
    import xgboost_ray  # noqa: F401 - needed for metabuild dependency discovery
    from ray.train import RunConfig, ScalingConfig
    from ray.train.xgboost import RayTrainReportCallback, XGBoostTrainer

    pr.train_data.load_ray_dataset()
    train_data: ray.data.Dataset = pr.train_data.value

    pr.validation_data.load_ray_dataset()
    validation_data: ray.data.Dataset = pr.validation_data.value

    # Drop problematic columns
    for col in ["uuid", "datestr"]:
        if col in train_data.schema().names:
            log.warning("Dropping column %s from training data", col)
            train_data = train_data.drop_columns([col])
        if col in validation_data.schema().names:
            log.warning("Dropping column %s from validation data", col)
            validation_data = validation_data.drop_columns([col])

    # Debug: log schema and first rows
    log.info("Train dataset schema: %s", train_data.schema())
    log.info("Train dataset sample: %s", train_data.take(1))

    def create_scaling_config(
        *,
        cpu_per_worker: int,
        trainer_cpu: int | None = None,
    ) -> ScalingConfig:
        """Create optimized ScalingConfig for Ray trainer resource allocation.

        Optimized to utilize the maximum available resources of the cluster.
        Dynamically calculates the optimal number of workers based on the
        current Ray cluster's resources. The function assumes that if the
        cluster has GPUs, each worker should use one GPU. If no GPUs are
        available, workers are configured to run without GPU resources.

        Parameters:
            cpu_per_worker (int): The number of CPU cores to allocate for
                each worker.
            trainer_cpu (int, optional): The number of CPU cores to allocate
                for the trainer.

        Returns:
            ScalingConfig: A configuration object that includes the
            calculated number of workers, the resource allocations for the
            trainer and each worker, and whether to use GPUs (if available).

        Raises:
            ValueError: If the cluster does not have sufficient CPU resources
                to meet the minimum requirements for the trainer and at least
                one worker.
        """
        if trainer_cpu is None:
            trainer_resources = None
            trainer_cpu = 1  # Reserve 1 CPU for trainer not letting workers
            # to occupy all available CPUs.
        else:
            trainer_resources = {"CPU": trainer_cpu}

        # Retrieve the total resources available in the current Ray cluster
        cluster_resources = ray.cluster_resources()
        cluster_cpu = cluster_resources["CPU"]
        cluster_gpu = cluster_resources.get(
            "GPU", 0.0
        )  # Default to 0 if no GPUs are found
        reserved_cpu = int(cluster_cpu * 0.5)
        available_cpu = cluster_cpu - reserved_cpu

        # Validate that the cluster has enough CPUs to meet the minimum requirement
        min_required_cpus = trainer_cpu + cpu_per_worker
        if available_cpu < min_required_cpus:
            raise ValueError(
                f"Insufficient cluster CPU resources: Total {cluster_cpu} "
                f"CPUs, Ray data reserved {reserved_cpu} CPUs, available "
                f"{available_cpu} CPUs, but {min_required_cpus} CPUs are "
                f"required (including {trainer_cpu} CPUs for the trainer "
                f"and {cpu_per_worker} per worker). Please ensure the Ray "
                f"cluster has sufficient CPU resources or scale down the "
                f"resource requirements.",
            )

        # Determine GPU allocation per worker, if GPUs are available
        gpu_per_worker = 1 if cluster_gpu > 0 else 0

        # Calculate the maximum number of workers based on CPU availability
        num_workers = (available_cpu - trainer_cpu) // cpu_per_worker

        # Adjust the number of workers based on GPU availability, if necessary
        num_workers = (
            min(num_workers, cluster_gpu // gpu_per_worker)
            if gpu_per_worker > 0
            else num_workers
        )

        return ScalingConfig(
            trainer_resources=trainer_resources,
            num_workers=int(num_workers),
            use_gpu=gpu_per_worker > 0,
            resources_per_worker={"CPU": cpu_per_worker, "GPU": gpu_per_worker},
        )

    scaling_config = create_scaling_config(
        trainer_cpu=None,
        cpu_per_worker=1,
    )
    log.info("scaling_config: %r", scaling_config)

    storage_url = os.environ.get("UF_STORAGE_URL", "")
    run_config = RunConfig(storage_path=f"{storage_url}/ray_results")
    log.info("run_config: %r", run_config)

    data_schema = train_data.schema()
    assert data_schema

    label_column = data_schema.names[-1]  # assuming the last column is the label

    def train_loop_per_worker():
        train_shard = ray.train.get_dataset_shard("train")
        validation_shard = ray.train.get_dataset_shard("validation")

        train_df = train_shard.materialize().to_pandas()
        validation_df = validation_shard.materialize().to_pandas()

        feature_cols = [c for c in train_df.columns if c != label_column]
        dtrain = xgboost.DMatrix(train_df[feature_cols], label=train_df[label_column])
        dvalidation = xgboost.DMatrix(
            validation_df[feature_cols], label=validation_df[label_column]
        )

        xgboost.train(
            params,
            dtrain=dtrain,
            evals=[(dvalidation, "validation")],
            num_boost_round=10,
            callbacks=[RayTrainReportCallback()],
        )

    trainer = XGBoostTrainer(
        train_loop_per_worker,
        scaling_config=scaling_config,
        run_config=run_config,
        datasets={
            "train": train_data,
            "validation": validation_data,
        },
    )
    result = trainer.fit()
    if result.error:
        raise result.error
    return TrainResult(
        path=result.path,
        metrics=result.metrics,
    )
