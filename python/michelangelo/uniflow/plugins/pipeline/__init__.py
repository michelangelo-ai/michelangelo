"""Pipeline plugin for running child pipelines from Uniflow workflows.

This module provides Python functions for creating and monitoring pipeline runs,
matching the functionality of the Go/Starlark pipeline plugin.
"""

from michelangelo.uniflow.plugins.pipeline.run import run_pipeline

__all__ = ["run_pipeline"]
