import os
from typing import Optional, Any

from pyspark.sql import DataFrame, SparkSession

from michelangelo.uniflow.core.io_registry import IO

def _ensure_s3a_config():
    spark = (
        SparkSession.builder
        .appName("SparkIO-S3A-Inject")
        .config("spark.hadoop.fs.s3a.impl", "org.apache.hadoop.fs.s3a.S3AFileSystem")
        .config("spark.hadoop.fs.s3.impl", "org.apache.hadoop.fs.s3a.S3AFileSystem")
        .config("spark.hadoop.fs.AbstractFileSystem.s3a.impl", "org.apache.hadoop.fs.s3a.S3A")
        .config("spark.hadoop.fs.s3a.access.key", os.getenv("AWS_ACCESS_KEY_ID", ""))
        .config("spark.hadoop.fs.s3a.secret.key", os.getenv("AWS_SECRET_ACCESS_KEY", ""))
        .config("spark.hadoop.fs.s3a.endpoint", os.getenv("AWS_ENDPOINT_URL", ""))
        .config("spark.hadoop.fs.s3a.path.style.access", "true")
        .getOrCreate()
    )
_ensure_s3a_config()

def read_data(url: str) -> DataFrame:
    spark = SparkSession.getActiveSession() or SparkSession.builder.getOrCreate()
    return spark.read.parquet(url)

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
