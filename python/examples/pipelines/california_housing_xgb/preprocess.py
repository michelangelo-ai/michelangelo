"""Spark preprocessing task for the California Housing XGBoost workflow.

Casts selected columns to float type using Spark and returns the preprocessed
training and validation datasets.
"""

from __future__ import annotations

import logging
from dataclasses import dataclass
from typing import TYPE_CHECKING

import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.spark import SparkTask
from michelangelo.workflow.variables import DatasetVariable

if TYPE_CHECKING:
    from pyspark.sql import DataFrame

log = logging.getLogger(__name__)

__all__ = ["PreprocessResult", "preprocess"]


@dataclass
class PreprocessResult:
    """Container for preprocessing results.

    Attributes:
        train_data: Training dataset.
        validation_data: Validation dataset.
    """

    train_data: DatasetVariable
    validation_data: DatasetVariable


@uniflow.task(
    config=SparkTask(
        driver_cpu=1,
        driver_memory="4G",
        executor_cpu=1,
        executor_memory="2G",
        executor_instances=1,
    ),
    cache_enabled=False,  # off for tutorial simplicity; enable in production
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

    log.info("Processed Train Spark schema:\n%s", train_data_pr.schema.simpleString())

    return PreprocessResult(
        train_data=train_dv_pr,
        validation_data=validation_dv_pr,
    )
