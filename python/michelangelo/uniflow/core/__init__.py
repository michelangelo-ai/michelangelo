"""Uniflow core API: task and workflow decorators, context and I/O."""

from michelangelo.uniflow.core.context import create_context
from michelangelo.uniflow.core.decorator import (
    star_plugin,
    task,
    task_context,
    workflow,
)
from michelangelo.uniflow.core.image_spec import ImageSpec
from michelangelo.uniflow.core.io_registry import IO
from michelangelo.uniflow.core.pipeline_metadata import PipelineMetadata

__all__ = [
    "IO",
    "ImageSpec",
    "PipelineMetadata",
    "create_context",
    "star_plugin",
    "task",
    "task_context",
    "workflow",
]
