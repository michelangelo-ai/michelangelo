"""Scala plugin for Michelangelo Uniflow.

This package provides support for running pre-compiled Scala/JVM Spark jobs
(a JAR plus a main class) as Uniflow tasks. Unlike the ``spark`` plugin, the
task body is not a Python function executed inside the driver — it is an
external JAR that Spark invokes directly via ``spark-submit``.
"""

from michelangelo.uniflow.plugins.scala.task import ScalaTask

__all__ = [
    "ScalaTask",
]
