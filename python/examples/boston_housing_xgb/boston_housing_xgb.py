"""XGBoost regression workflow for Boston Housing price prediction.

Example workflow demonstrating XGBoost training with Ray for distributed model
training on the Boston Housing dataset.
"""

import logging
from dataclasses import dataclass
from typing import Optional

import numpy as np
import pandas as pd
import ray
import ray.data
import xgboost  # noqa: F401 - needed for metabuild dependency discovery
import xgboost_ray  # noqa: F401 - needed for metabuild dependency discovery
from pyspark.sql import DataFrame
from ray.train import CheckpointConfig, RunConfig, ScalingConfig
from ray.train.xgboost import XGBoostTrainer

import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask
from michelangelo.uniflow.plugins.spark import SparkTask
from michelangelo.workflow.variables import DatasetVariable

log = logging.getLogger(__name__)


@dataclass
class PreprocessResult:
    """Container for preprocessing results.

    Attributes:
        train_data: Training dataset.
        validation_data: Validation dataset.
    """

    train_data: DatasetVariable
    validation_data: DatasetVariable


@dataclass
class TrainResult:
    """Container for training results.

    Attributes:
        path: Path to saved model.
        metrics: Optional dictionary of evaluation metrics.
    """

    path: str
    metrics: Optional[dict] = None


@uniflow.task(
    config=RayTask(
        head_cpu=1,
        head_gpu=0,
        head_memory="4Gi",
        worker_cpu=1,
        worker_gpu=0,
        worker_memory="4Gi",
        worker_instances=0,
        # breakpoint=True,
    ),
    cache_enabled=True,
)
def feature_prep(
    columns: list[str],
    test_size: float = 0.25,
    seed: int = 1,
) -> tuple[DatasetVariable, DatasetVariable]:
    """Prepare features from Boston Housing dataset.

    Downloads the Boston Housing dataset, performs train/test split, and converts
    to Ray Datasets for distributed processing.

    Args:
        columns: List of feature column names.
        test_size: Fraction of data to use for validation. Defaults to 0.25.
        seed: Random seed for reproducibility. Defaults to 1.

    Returns:
        Tuple of (train_dataset, validation_dataset) as DatasetVariables.
    """
    data_url = "http://lib.stat.cmu.edu/datasets/boston"
    raw_df = pd.read_csv(data_url, sep=r"\s+", skiprows=22, header=None)
    X = np.hstack([raw_df.values[::2, :], raw_df.values[1::2, :2]])  # noqa: N806
    y = raw_df.values[1::2, 2]

    feature_names = columns[:-1]  # assuming the last column is 'target'

    dataset = [
        dict(zip(feature_names, features), target=target)
        for features, target in zip(X, y)
    ]
    data = ray.data.from_items(dataset).select_columns(columns)

    train_data, validation_data = data.train_test_split(
        test_size=test_size, shuffle=True, seed=seed
    )

    train_dv = DatasetVariable.create(train_data)
    train_dv.save_ray_dataset()

    validation_dv = DatasetVariable.create(validation_data)
    validation_dv.save_ray_dataset()

    log.info("Train dataset schema: %s", train_data.schema())
    log.info("Train dataset sample: %s", train_data.take(1))

    return train_dv, validation_dv


@uniflow.task(
    config=SparkTask(
        driver_cpu=1,
        executor_cpu=1,
    ),
    cache_enabled=True,
)
def preprocess(
    cast_float_columns: list[str],
    train_dv: DatasetVariable,
    validation_dv: DatasetVariable,
) -> PreprocessResult:
    """Preprocess datasets using Spark to cast columns to float type.

    Args:
        cast_float_columns: List of column names to cast to float type.
        train_dv: Training DatasetVariable containing Spark DataFrame.
        validation_dv: Validation DatasetVariable containing Spark DataFrame.

    Returns:
        PreprocessResult containing preprocessed training and validation datasets.
    """
    train_dv.load_spark_dataframe()
    train_data: DataFrame = train_dv.value

    validation_dv.load_spark_dataframe()
    validation_data: DataFrame = validation_dv.value

    def cast_float(df: DataFrame) -> DataFrame:
        cols = {col: df[col].cast("float") for col in cast_float_columns}
        return df.withColumns(cols)

    train_data_pr = cast_float(train_data)
    validation_data_pr = cast_float(validation_data)

    train_dv_pr = DatasetVariable.create(train_data_pr)
    train_dv_pr.save_spark_dataframe()

    validation_dv_pr = DatasetVariable.create(validation_data_pr)
    validation_dv_pr.save_spark_dataframe()

    log.info(
        "Processed Train Spark schema:\n%s", train_data_pr._jdf.schema().treeString()
    )

    return PreprocessResult(
        train_data=train_dv_pr,
        validation_data=validation_dv_pr,
    )


@uniflow.task(
    config=RayTask(
        head_cpu=1,
        head_gpu=0,
        head_memory="12Gi",
        worker_cpu=2,
        worker_gpu=0,
        worker_memory="12Gi",
        worker_instances=1,
        runtime_env={
            "env_vars": {
                "TEST_ENV_VAR": "test_value",
            },
        },
    ),
)
def train(
    pr: PreprocessResult,
    params: dict,
) -> TrainResult:
    """Train XGBoost model using Ray for distributed training.

    Trains an XGBoost regression model on preprocessed Boston Housing data using
    Ray's distributed XGBoostTrainer with automatic hyperparameter tuning.

    Args:
        pr: PreprocessResult containing preprocessed training and validation datasets.
        params: Dictionary of XGBoost hyperparameters (e.g., max_depth, learning_rate).

    Returns:
        TrainResult containing the path to saved model and training metrics.
    """
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

    # 🛠️ Debug print: schema and first rows
    log.info("Train dataset schema: %s", train_data.schema())
    log.info("Train dataset sample: %s", train_data.take(1))

    def create_scaling_config(
        *,
        cpu_per_worker: int,
        trainer_cpu: Optional[int] = None,
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
        cpu_per_worker=4,
    )
    log.info("scaling_config: %r", scaling_config)

    run_config = RunConfig(
        checkpoint_config=CheckpointConfig(
            checkpoint_at_end=True,
        ),
    )
    log.info("run_config: %r", run_config)

    data_schema = train_data.schema()
    assert data_schema

    trainer = XGBoostTrainer(
        label_column=data_schema.names[-1],  # assuming the last column is the label
        params=params,
        num_boost_round=10,
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


@uniflow.workflow()
def train_workflow(
    dataset_cols: str,
):
    """Complete XGBoost training workflow for Boston Housing dataset.

    Orchestrates the end-to-end ML workflow: feature preparation,
    preprocessing with Spark, and distributed training with Ray XGBoost.

    Args:
        dataset_cols: Comma-separated string of column names including
            features and target. Example:
            "feature1,feature2,feature3,target".
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
    train_result = train(
        pr,
        params={
            "objective": "reg:linear",
            "colsample_bytree": 0.3,
            "learning_rate": 0.1,
            "max_depth": 5,
            "alpha": 10,
            "n_estimators": 10,
        },
    )
    print("train_result.path:", train_result.path)
    return train_result


if __name__ == "__main__":
    ctx = uniflow.create_context()

    ctx.environ["IMAGE_PULL_POLICY"] = "Never"
    ctx.run(
        train_workflow,
        dataset_cols="CRIM,ZN,INDUS,CHAS,NOX,RM,AGE,DIS,RAD,TAX,PTRATIO,B,LSTAT,target",
    )
