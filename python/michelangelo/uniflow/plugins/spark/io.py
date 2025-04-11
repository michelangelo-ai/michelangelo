import os
from typing import Optional, Any

from pyspark.sql import DataFrame, SparkSession

from uber.ai.uniflow.core.io_registry import IO


class SparkIO(IO[DataFrame]):
    def write(self, url: str, value: DataFrame) -> Optional[Any]:
        self.write_data(url, value)
        return None

    def read(self, url: str, _metadata) -> DataFrame:
        return self.read_data(url)

    @staticmethod
    def write_data(url: str, data: DataFrame):
        url = os.path.expanduser(url)
        data.write.parquet(url)

    @staticmethod
    def read_data(url: str) -> DataFrame:
        url = os.path.expanduser(url)
        spark = SparkSession.getActiveSession()
        return spark.read.parquet(url)
