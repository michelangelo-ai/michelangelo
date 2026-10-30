"""I/O handlers for Spark DataFrames in Uniflow workflows.

This module provides I/O functionality for reading and writing Spark DataFrames in
Uniflow workflows. It handles S3A filesystem configuration for MinIO compatibility
and supports Parquet format for data persistence. S3A credentials and endpoint are
read from the standard `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, and
`AWS_ENDPOINT_URL` environment variables; path-style access is enabled automatically
for MinIO compatibility.

Example:
```python
from michelangelo.uniflow.plugins.spark.io import SparkIO
from pyspark.sql import SparkSession

spark = SparkSession.builder.getOrCreate()
df = spark.createDataFrame([(1, "a"), (2, "b")], ["id", "label"])

io_handler = SparkIO()
io_handler.write("s3://bucket/data.parquet", df)
loaded_df = io_handler.read("s3://bucket/data.parquet", None)
```
"""

import logging
import os
from typing import Any, Optional

from pyspark.sql import DataFrame, SparkSession

from michelangelo.uniflow.core.io_registry import IO

__all__ = ["SparkIO", "s3a_spark_config"]

log = logging.getLogger(__name__)

_s3a_configured = False  # module-level flag so _ensure_s3a_config() is idempotent


def s3a_spark_config() -> dict[str, str]:
    """Return the Spark properties that point s3:// and s3a:// at MinIO/S3 via S3A.

    These must be applied on the session builder, before the SparkContext starts:
    Hadoop caches FileSystem instances, so credentials set on a live context can
    miss an S3AFileSystem that was already created. Credentials and endpoint come
    from `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` and `AWS_ENDPOINT_URL`; unset
    ones are left out so the default S3A credential chain still applies.
    """
    conf = {
        "spark.hadoop.fs.s3a.impl": "org.apache.hadoop.fs.s3a.S3AFileSystem",
        # fs.s3.impl maps s3:// URIs to S3AFileSystem (needed in Hadoop 3.x)
        "spark.hadoop.fs.s3.impl": "org.apache.hadoop.fs.s3a.S3AFileSystem",
        "spark.hadoop.fs.AbstractFileSystem.s3a.impl": "org.apache.hadoop.fs.s3a.S3A",
        "spark.hadoop.fs.s3a.path.style.access": "true",
    }
    for key, env in (
        ("spark.hadoop.fs.s3a.access.key", "AWS_ACCESS_KEY_ID"),
        ("spark.hadoop.fs.s3a.secret.key", "AWS_SECRET_ACCESS_KEY"),
        ("spark.hadoop.fs.s3a.endpoint", "AWS_ENDPOINT_URL"),
    ):
        if value := os.getenv(env):
            conf[key] = value
    return conf


def _ensure_s3a_config():
    """Make sure a Spark session with the S3A settings exists before any I/O.

    Called lazily on first I/O operation so that importing this module in a
    non-Spark runtime (e.g. a Ray task container) does not start a SparkContext.
    If no session exists yet, creates one with `s3a_spark_config()`. A session
    started by SparkTask.pre_run already carries those settings; one started
    elsewhere without them is used as is, with a warning, since they cannot be
    applied safely to a running context.

    The function is idempotent: subsequent calls after the first are no-ops.
    """
    global _s3a_configured
    if _s3a_configured:
        return
    spark = SparkSession.getActiveSession()
    if spark is None:
        sb = SparkSession.builder.appName("SparkIO-S3A-Inject")
        for k, v in s3a_spark_config().items():
            sb = sb.config(k, v)
        sb.getOrCreate()
    elif spark.conf.get("spark.hadoop.fs.s3a.impl", None) is None:
        log.warning(
            "Active Spark session was created without the S3A settings; s3:// and "
            "s3a:// I/O may fail. Start it via SparkTask or apply "
            "michelangelo.uniflow.plugins.spark.io.s3a_spark_config() on the builder."
        )
    _s3a_configured = True


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
