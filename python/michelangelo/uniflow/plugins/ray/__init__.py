"""Ray plugin for Michelangelo Uniflow.

This package provides Ray-based execution support for Uniflow workflows. It includes
task configuration for Ray clusters and I/O handlers for Ray datasets.

Ray enables distributed Python execution with flexible resource management and
automatic scaling. This plugin allows Uniflow workflows to leverage Ray's capabilities
for parallel data processing and distributed task execution.
"""

from michelangelo.uniflow.plugins.ray.io import UF_PLUGIN_RAY_USE_FSSPEC, RayDatasetIO
from michelangelo.uniflow.plugins.ray.task import RayTask

__all__ = [
    "UF_PLUGIN_RAY_USE_FSSPEC",
    "RayDatasetIO",
    "RayTask",
]
