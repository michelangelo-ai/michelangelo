"""Feature preparation task for the California Housing XGBoost workflow.

Loads the California Housing dataset, performs a train/test split, and
converts the result to Ray Datasets for distributed processing.
"""

from __future__ import annotations

import logging

import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask
from michelangelo.workflow.variables import DatasetVariable

log = logging.getLogger(__name__)

__all__ = ["feature_prep"]


@uniflow.task(
    config=RayTask(
        head_cpu=1,
        head_gpu=0,
        head_memory="4Gi",
        worker_cpu=1,
        worker_gpu=0,
        worker_memory="4Gi",
        worker_instances=0,
    ),
    cache_enabled=True,
)
def feature_prep(
    columns: list[str],
    test_size: float = 0.25,
    seed: int = 1,
) -> tuple[DatasetVariable, DatasetVariable]:
    """Prepare features from the California Housing dataset.

    Loads the California Housing dataset via scikit-learn, performs a
    train/test split, and converts to Ray Datasets for distributed processing.

    Args:
        columns: List of column names to select (features + ``"target"``).
        test_size: Fraction of data to use for validation. Defaults to 0.25.
        seed: Random seed for reproducibility. Defaults to 1.

    Returns:
        Tuple of (train_dataset, validation_dataset) as DatasetVariables.
    """
    import ray.data
    from sklearn.datasets import fetch_california_housing

    housing = fetch_california_housing(as_frame=True)
    df = housing.frame.rename(columns={"MedHouseVal": "target"})

    data = ray.data.from_pandas(df).select_columns(columns)

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
