from pyspark.sql import DataFrame

import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.spark import SparkTask


@uniflow.task(
    config=SparkTask(
        driver_cpu=4,
        executor_cpu=16,
    )
)
def preprocess(
        train_data: DataFrame,
        validation_data: DataFrame,
        test_data: DataFrame,
) -> tuple[DataFrame, DataFrame, DataFrame]:
    train_data: DataFrame = train_data.value
    validation_data: DataFrame = validation_data.value

    def touch(df: DataFrame) -> DataFrame:
        col_to_rename = df.columns[0] if df.columns else "dummy"
        return df.withColumnRenamed(col_to_rename, col_to_rename + "_tmp") \
            .withColumnRenamed(col_to_rename + "_tmp", col_to_rename) \
            .cache()


    train_data = touch(train_data)
    validation_data = touch(validation_data)

    return train_data, validation_data, test_data
