"""Scala/Spark task step for the Scala Hello World example.

Wraps the pre-compiled HelloScala JAR (see src/HelloScala.scala, build.sh)
in a ScalaTask. Unlike a SparkTask, this function's body does no work
itself — ScalaTask.pre_run() already ran the JAR (local-run: downloaded
main_file via fsspec and invoked it with spark-submit; remote-run: the
SparkJob CRD's driver pod runs it directly) before this body executes.
"""

from __future__ import annotations

import logging

import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.scala import ScalaTask

log = logging.getLogger(__name__)

__all__ = ["hello_scala"]


@uniflow.task(
    config=ScalaTask(main_file="", main_class=""),  # real values set via with_overrides()
    cache_enabled=False,  # off for tutorial simplicity; no result contract to cache anyway
)
def hello_scala() -> None:
    """No-op body — HelloScala.scala's main() does the actual work."""
    log.info("hello_scala: JAR execution already completed in ScalaTask.pre_run()")
