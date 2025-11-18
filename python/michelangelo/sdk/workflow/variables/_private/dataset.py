from dataclasses import dataclass

from michelangelo.uniflow.plugins.ray.io import RayDatasetIO
from michelangelo.uniflow.plugins.spark.io import SparkIO

from .base import Variable

import pandas as pd
import pyspark
import ray

import importlib


def has_pyspark() -> bool:
    try:
        pyspark_sql = importlib.import_module("pyspark.sql")
    except ModuleNotFoundError:
        return False

    spark = pyspark_sql.SparkSession.getActiveSession()
    return spark is not None


def has_ray() -> bool:
    try:
        ray = importlib.import_module("ray")
    except ModuleNotFoundError:
        return None
    return ray.is_initialized()


@dataclass
class DatasetVariable(Variable):
    """
    Represents a piece of data. Underlying it could be a Spark DataFrame, a Ray Dataset or a Pandas DataFrame.
    """

    @classmethod
    def create(cls, value) -> "DatasetVariable":
        """
        A factory method to create a dataset variable with the given value.
        """
        res = super().create(value)
        return res

    def _load(self):
        """
        Load value from variable path.
        Automatically find the value type based on sys modules.
        If it does not work out, please call the type specific APIs to load the value.
        """
        if has_pyspark():
            self.load_spark_dataframe()
        elif has_ray():
            self.load_ray_dataset()
        else:
            self.load_pandas_dataframe()

    def load_spark_dataframe(self):
        """
        Load the value as Spark DataFrame.
        """
        self._load_value_using_io(SparkIO)

    def load_ray_dataset(self):
        """
        Load the value as Ray Dataset.
        """
        self._load_value_using_io(RayDatasetIO)

    def save(self):
        """
        Save value to variable path.
        Automatically find the value type based on the class of the value.
        If it does not work out, please call the type specific APIs to save the value.
        """

        if isinstance(self.value, pyspark.sql.DataFrame):
            self.save_spark_dataframe()

        elif isinstance(self.value, ray.data.Dataset):
            self.save_ray_dataset()

        elif isinstance(self.value, pd.DataFrame):
            self.save_pandas_dataframe()

        else:
            raise TypeError("Unsupported value type")

    def save_spark_dataframe(self):
        """
        Save the value as Spark DataFrame.
        """
        self._save_value_using_io(SparkIO)

    def save_ray_dataset(self):
        """
        Save the value as Ray Dataset.
        """
        self._save_value_using_io(RayDatasetIO)
