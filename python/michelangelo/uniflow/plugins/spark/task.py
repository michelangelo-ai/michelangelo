from dataclasses import dataclass
import logging
import os
from pathlib import Path
from typing import Optional, Any, Dict

from pyspark.sql import DataFrame
from pyspark.sql import SparkSession

from fsspec.spec import AbstractFileSystem
from fsspec.registry import register_implementation, get_filesystem_class
import s3fs

from michelangelo.uniflow.core.io_registry import io_registry
from michelangelo.uniflow.core.task_config import TaskConfig, TaskBinding
from michelangelo.uniflow.plugins.spark.io import SparkIO

log = logging.getLogger(__name__)


class BifrostS3FileSystem(AbstractFileSystem):
    """A filesystem interface for S3 with Grab Bifrost authentication"""

    protocol = "bifrost-s3"

    def __init__(self, *args: Any, **storage_options: Any) -> None:
        super().__init__(*args, **storage_options)
        # Initialize s3fs with Bifrost plugin
        self.s3fs = s3fs.S3FileSystem()
        from grab.bifrost.client import PortaS3AccessGrantsPlugin
        PortaS3AccessGrantsPlugin(s3_client=self.s3fs.s3, use_async_io=True)

    def _open(
        self,
        path: str,
        mode: str = "rb",
        **kwargs: Any,
    ):
        """Open a file using the underlying s3fs with Bifrost authentication."""
        log.info("[+] _open %r with mode %r", path, mode)
        # Remove the protocol prefix if present
        s3_path = path.replace("bifrost-s3://", "s3://") if path.startswith("bifrost-s3://") else path
        return self.s3fs.open(s3_path, mode, **kwargs)


def register_bifrost_s3_into_fsspec():
    """Register the Bifrost S3 filesystem with fsspec"""
    register_implementation("bifrost-s3", BifrostS3FileSystem)
    # Verify registration
    registered_fs = get_filesystem_class("bifrost-s3")
    if not registered_fs or not issubclass(registered_fs, BifrostS3FileSystem):
        raise RuntimeError("BifrostS3FileSystem failed to register with fsspec")
    log.info("BifrostS3FileSystem successfully registered with fsspec")

# Register the filesystem on module import
register_bifrost_s3_into_fsspec()

io_registry()[DataFrame] = SparkIO

_binding = TaskBinding(
    star_file=Path(__file__).resolve().parent / "task.star",
    function="spark_task",
    export="__spark_task",
)

_config_binding = TaskBinding(
    star_file=Path(__file__).resolve().parent / "task.star",
    function="spark_config",
    export="__spark_config",
)


@dataclass
class SparkTask(TaskConfig):
    """
    This class encapsulates the configuration properties required to orchestrate a Spark task, including resource
    specifications. It is designed to interface with a Starlark function that executes a Spark job according to these
    configurations.

    The class intentionally avoids defining default values for its properties. Instead, defaults should be provided
    through the keyword arguments of the associated Starlark function. This approach facilitates more flexible
    configuration management, allowing runtime overrides of the default settings.
    """

    driver_cpu: Optional[int] = None
    driver_memory: Optional[str] = None
    driver_disk: Optional[str] = None
    driver_gpu: Optional[int] = None
    executor_cpu: Optional[int] = None
    executor_memory: Optional[str] = None
    executor_disk: Optional[str] = None
    executor_gpu: Optional[int] = None
    executor_instances: Optional[int] = None
    spark_conf: Optional[Dict[str, str]] = None

    def get_binding(self) -> TaskBinding:
        return _binding

    @classmethod
    def get_config_binding(self) -> TaskBinding:
        return _config_binding

    def pre_run(self):
        sb = SparkSession.builder.enableHiveSupport()

        if props := os.environ.get("_SPARK_PROPERTIES"):
            for kv in props.split(","):
                k, v = kv.split("=", 1)
                sb = sb.config(k, v)

        spark = sb.getOrCreate()
        assert spark

    def post_run(self):
        spark = SparkSession.getActiveSession()
        if spark:
            spark.stop()
