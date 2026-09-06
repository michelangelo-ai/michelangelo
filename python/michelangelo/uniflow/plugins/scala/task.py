"""Scala task configuration and execution for Uniflow workflows.

This module provides task configuration for executing pre-compiled Scala/JVM
Spark jobs (a JAR plus a main class) as Uniflow tasks. Unlike ``SparkTask``,
the task body is not a Python function that runs inside the driver — it is an
external JAR that Spark invokes directly via ``spark-submit``. There is
therefore no Python function for the JVM to call back into, and no
``--args``/``--kwargs``/``--result-url`` contract: success or failure is
determined purely by the JAR's exit / the SparkJob's terminal condition.

On a cluster (remote-run), the driver pod runs the JAR directly via
``spark-submit`` as part of the ``SparkJob`` CRD — the Python side has
nothing to set up. In local-run mode there is no cluster to submit to, so
``pre_run()`` downloads ``main_file`` via fsspec and runs it locally with
``spark-submit --master local[*]``.
"""

import logging
import os
import subprocess
import tempfile
from dataclasses import dataclass
from pathlib import Path
from typing import Optional

from michelangelo.uniflow.core.task_config import TaskBinding, TaskConfig

log = logging.getLogger(__name__)

_binding = TaskBinding(
    star_file=Path(__file__).resolve().parent / "task.star",
    function="scala_task",
    export="__scala_task",
)

_config_binding = TaskBinding(
    star_file=Path(__file__).resolve().parent / "task.star",
    function="scala_config",
    export="__scala_config",
)


@dataclass
class ScalaSparkTask(TaskConfig):
    """Configuration for running a pre-compiled Scala/JVM Spark job.

    Attributes:
        main_file: Location of the JAR to run. Any URL fsspec understands
            (local path, ``s3://``, ``gs://``, ``hdfs://``, etc.) On a
            cluster run this must be reachable by the Spark driver/executor
            pods directly (e.g. an object-store URL); for local runs it is
            downloaded to a temp directory first.
        main_class: Fully-qualified Spark main class to invoke in the JAR.
        driver_cpu: Number of CPUs allocated to the driver node.
        driver_memory: Memory allocation for the driver node (e.g., "4G", "512M").
        driver_disk: Disk space allocation for the driver node (e.g., "10G").
        driver_gpu: Number of GPUs allocated to the driver node.
        executor_cpu: Number of CPUs allocated per executor.
        executor_memory: Memory allocation per executor (e.g., "4G", "512M").
        executor_disk: Disk space allocation per executor (e.g., "10G").
        executor_gpu: Number of GPUs allocated per executor.
        executor_instances: Number of executor instances to launch.
    """

    main_file: str
    main_class: str
    driver_cpu: Optional[int] = None
    driver_memory: Optional[str] = None
    driver_disk: Optional[str] = None
    driver_gpu: Optional[int] = None
    executor_cpu: Optional[int] = None
    executor_memory: Optional[str] = None
    executor_disk: Optional[str] = None
    executor_gpu: Optional[int] = None
    executor_instances: Optional[int] = None

    def get_binding(self) -> TaskBinding:
        """Return the TaskBinding linking this config to its Starlark function.

        Returns:
            TaskBinding that specifies the Starlark file and function for
            Scala task execution.
        """
        return _binding

    @classmethod
    def get_config_binding(cls) -> TaskBinding:
        """Return the TaskBinding for Scala configuration.

        Returns:
            TaskBinding that specifies the Starlark file and function for
            Scala configuration.
        """
        return _config_binding

    def pre_run(self):
        """Run the JAR locally when in local-run mode.

        On a cluster run, the SparkJob's driver pod runs the JAR directly
        via ``spark-submit`` (set up entirely in Starlark/Go) — nothing to
        do here. In local-run mode there is no cluster, so this downloads
        ``main_file`` via fsspec and runs it with ``spark-submit --master
        local[*]``, synchronously, raising on failure.
        """
        if os.environ.get("UF_LOCAL_RUN") != "1":
            return

        import fsspec

        local_dir = tempfile.mkdtemp(prefix="michelangelo_scala_")
        local_jar = os.path.join(local_dir, os.path.basename(self.main_file))
        log.info("scala task: downloading %s to %s", self.main_file, local_jar)
        with (
            fsspec.open(self.main_file, mode="rb") as src,
            open(local_jar, "wb") as dst,
        ):
            dst.write(src.read())

        cmd = [
            "spark-submit",
            "--master",
            "local[*]",
            "--class",
            self.main_class,
            local_jar,
        ]
        log.info("scala task: running locally: %s", " ".join(cmd))
        result = subprocess.run(cmd, check=False)
        if result.returncode != 0:
            raise RuntimeError(
                f"scala task: spark-submit failed with exit code {result.returncode} "
                f"(main_class={self.main_class!r}, main_file={self.main_file!r})"
            )

    def post_run(self):
        """No-op — the local run in pre_run() already completed synchronously."""
