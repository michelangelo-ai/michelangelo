"""I/O handlers for Spark DataFrames in Uniflow workflows.

This module provides I/O functionality for reading and writing Spark DataFrames in
Uniflow workflows. It handles S3A filesystem configuration for MinIO compatibility
and supports Parquet format for data persistence.
"""

import os
from typing import Any, Optional

from pyspark.sql import DataFrame, SparkSession

from michelangelo.uniflow.core.io_registry import IO


def _ensure_s3a_config():
    """Configure Spark session with S3A filesystem settings.

    Initializes or retrieves a Spark session configured for S3A access, including
    MinIO compatibility. Reads AWS credentials and endpoint from environment variables.

    This function is called at module import time to ensure S3A configuration is
    available before any I/O operations.
    """
    SparkSession.builder.appName("SparkIO-S3A-Inject").config(
        "spark.hadoop.fs.s3a.impl", "org.apache.hadoop.fs.s3a.S3AFileSystem"
    ).config(
        "spark.hadoop.fs.s3.impl", "org.apache.hadoop.fs.s3a.S3AFileSystem"
    ).config(
        "spark.hadoop.fs.AbstractFileSystem.s3a.impl",
        "org.apache.hadoop.fs.s3a.S3A",
    ).config(
        "spark.hadoop.fs.s3a.access.key", os.getenv("AWS_ACCESS_KEY_ID", "")
    ).config(
        "spark.hadoop.fs.s3a.secret.key", os.getenv("AWS_SECRET_ACCESS_KEY", "")
    ).config("spark.hadoop.fs.s3a.endpoint", os.getenv("AWS_ENDPOINT_URL", "")).config(
        "spark.hadoop.fs.s3a.path.style.access", "true"
    ).getOrCreate()


_ensure_s3a_config()


def read_data(url: str) -> DataFrame:
    """Read a Spark DataFrame from a Parquet file.

    Args:
        url: The URL or path to read from. Supports local paths and S3 URLs.

    Returns:
        The loaded Spark DataFrame.
    """
    spark = SparkSession.getActiveSession() or SparkSession.builder.getOrCreate()
    return spark.read.parquet(url)


class SparkIO(IO[DataFrame]):
    """I/O handler for Spark DataFrame objects.

    This class provides read and write operations for Spark DataFrames, storing them
    in Parquet format. It supports local filesystem paths and S3 URLs via S3A protocol.

    The implementation expands tilde (~) paths and uses the active Spark session for
    all I/O operations.
    """

    def write(self, url: str, value: DataFrame) -> Optional[Any]:
        """Write a Spark DataFrame to the specified URL in Parquet format.

        Args:
            url: Target URL where the DataFrame should be written. Supports local paths
                (including ~-prefixed paths) and S3 URLs.
            value: The Spark DataFrame to write.

        Returns:
            None. This implementation does not return metadata.
        """
        self.write_data(url, value)
        return None

    def read(self, url: str, _metadata) -> DataFrame:
        """Read a Spark DataFrame from the specified URL.

        Args:
            url: Source URL from which to read the DataFrame. Supports local paths
                (including ~-prefixed paths) and S3 URLs.
            _metadata: Optional metadata from write operation. Currently unused.

        Returns:
            The loaded Spark DataFrame.
        """
        return self.read_data(url)

    @staticmethod
    def write_data(url: str, data: DataFrame):
        """Write DataFrame to Parquet format at the given URL.

        Args:
            url: Target URL for writing. Tilde paths are expanded.
            data: The Spark DataFrame to write.
        """
        url = os.path.expanduser(url)
        data.write.parquet(url)

    @staticmethod
    def read_data(url: str) -> DataFrame:
        """Read DataFrame from Parquet format at the given URL.

        Args:
            url: Source URL for reading. Tilde paths are expanded.

        Returns:
            The loaded Spark DataFrame.
        """
        url = os.path.expanduser(url)
        spark = SparkSession.getActiveSession()
        return spark.read.parquet(url)
