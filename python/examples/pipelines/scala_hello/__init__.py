"""Scala Hello World example.

Intentionally contains no imports. Importing this package in a task
container would eagerly load ScalaTask, mirroring the same avoidance in
california_housing_xgb/__init__.py for SparkTask. Keep this file
import-free so task containers only load what they need.
"""
