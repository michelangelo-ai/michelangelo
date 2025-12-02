"""Spark plugin for Michelangelo Uniflow.

This package provides Apache Spark-based execution support for Uniflow workflows.
It includes task configuration for Spark clusters and I/O handlers for Spark DataFrames.

Spark enables distributed data processing with SQL and DataFrame APIs. This plugin
allows Uniflow workflows to leverage Spark's capabilities for large-scale batch
processing and data transformations.
"""

from michelangelo.uniflow.plugins.spark.task import SparkTask

__all__ = [
    "SparkTask",
]
