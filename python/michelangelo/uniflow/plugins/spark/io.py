"""I/O handlers for Spark DataFrames in Uniflow workflows.

This module provides I/O functionality for reading and writing Spark DataFrames in
Uniflow workflows. It handles S3A filesystem configuration for MinIO compatibility
and supports Parquet format for data persistence.
"""

import os
from typing import Any, Optional

from pyspark.sql import DataFrame, SparkSession

from michelangelo.uniflow.core.io_registry import IO

_s3a_configured = False  # module-level flag so _ensure_s3a_config() is idempotent


def _ensure_s3a_config():
    """Inject S3A filesystem settings into the active Spark session.

    Called lazily on first I/O operation so that importing this module in a
    non-Spark runtime (e.g. a Ray task container) does not start a SparkContext.
    If no session exists yet, creates one. If one already exists (started by
    SparkTask.pre_run), reconfigures it in place via the Hadoop configuration API
    so the S3A credentials are available before any read/write call.

    The function is idempotent: subsequent calls after the first are no-ops.
    """
    global _s3a_configured
    if _s3a_configured:
        return
    spark = SparkSession.getActiveSession()
    if spark is None:
        spark = (
            SparkSession.builder.appName("SparkIO-S3A-Inject")
            .config(
                "spark.hadoop.fs.s3a.impl", "org.apache.hadoop.fs.s3a.S3AFileSystem"
            )
            # fs.s3.impl maps s3:// URIs to S3AFileSystem (needed in Hadoop 3.x)
            .config("spark.hadoop.fs.s3.impl", "org.apache.hadoop.fs.s3a.S3AFileSystem")
            .config(
                "spark.hadoop.fs.AbstractFileSystem.s3a.impl",
                "org.apache.hadoop.fs.s3a.S3A",
            )
            .config(
                "spark.hadoop.fs.s3a.access.key", os.getenv("AWS_ACCESS_KEY_ID", "")
            )
            .config(
                "spark.hadoop.fs.s3a.secret.key", os.getenv("AWS_SECRET_ACCESS_KEY", "")
            )
            .config("spark.hadoop.fs.s3a.endpoint", os.getenv("AWS_ENDPOINT_URL", ""))
            .config("spark.hadoop.fs.s3a.path.style.access", "true")
            .getOrCreate()
        )
    else:
        # Session already started by SparkTask.pre_run — inject S3A config at runtime.
        # Uses the Hadoop Configuration API (Py4J bridge) to set keys on the live
        # SparkContext. Note: if S3AFileSystem instances are already cached by the JVM
        # FileSystem cache, those instances retain their original (empty) credentials.
        # See GitHub issue #1286 for the proper fix via SparkTask.pre_run injection.
        hadoop_conf = spark.sparkContext._jsc.hadoopConfiguration()  # type: ignore[attr-defined]
        hadoop_conf.set("fs.s3a.impl", "org.apache.hadoop.fs.s3a.S3AFileSystem")
        # fs.s3.impl redirects legacy s3:// URIs to S3AFileSystem (needed in Hadoop 3.x)
        hadoop_conf.set("fs.s3.impl", "org.apache.hadoop.fs.s3a.S3AFileSystem")
        hadoop_conf.set(
            "fs.AbstractFileSystem.s3a.impl", "org.apache.hadoop.fs.s3a.S3A"
        )
        hadoop_conf.set("fs.s3a.access.key", os.getenv("AWS_ACCESS_KEY_ID", ""))
        hadoop_conf.set("fs.s3a.secret.key", os.getenv("AWS_SECRET_ACCESS_KEY", ""))
        hadoop_conf.set("fs.s3a.endpoint", os.getenv("AWS_ENDPOINT_URL", ""))
        hadoop_conf.set("fs.s3a.path.style.access", "true")
    _s3a_configured = True


def read_data(url: str) -> DataFrame:
    """Read a Spark DataFrame from a Parquet file.

    Args:
        url: The URL or path to read from. Supports local paths and S3 URLs.

    Returns:
        The loaded Spark DataFrame.
    """
    _ensure_s3a_config()
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
        _ensure_s3a_config()
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
        _ensure_s3a_config()
        url = os.path.expanduser(url)
        spark = SparkSession.getActiveSession() or SparkSession.builder.getOrCreate()
        return spark.read.parquet(url)
