"""Scala/Spark task step for the Scala test example.

Wraps the pre-compiled ScalaTest JAR (see src/ScalaTest.scala, build.sh)
in a ScalaSparkTask. Unlike a SparkTask, this function's body does no work
itself — ScalaSparkTask.pre_run() already ran the JAR (local-run: downloaded
main_file via fsspec and invoked it with spark-submit; remote-run: the
SparkJob CRD's driver pod runs it directly) before this body executes.
"""

from __future__ import annotations

import logging

import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.scala import ScalaSparkTask

log = logging.getLogger(__name__)

__all__ = ["scala_test"]


@uniflow.task(
    # real values set via with_overrides()
    config=ScalaSparkTask(main_file="", main_class=""),
    # off for tutorial simplicity; no result contract to cache anyway
    cache_enabled=False,
)
def scala_test() -> None:
    """No-op body — ScalaTest.scala's main() does the actual work."""
    log.info("scala_test: JAR execution already completed in ScalaSparkTask.pre_run()")
