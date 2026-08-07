"""Unified task decorator + config loader for YAML-driven pipeline authoring."""

from michelangelo.canvas.pipeline.config_loader import (
    PipelineConfig,
    load_pipeline_config,
)
from michelangelo.canvas.pipeline.register import (
    register_pipeline,
    resolve_workflow_call,
)
from michelangelo.canvas.pipeline.run import run_pipeline
from michelangelo.canvas.pipeline.task import pipeline_task

__all__ = [
    "PipelineConfig",
    "load_pipeline_config",
    "pipeline_task",
    "register_pipeline",
    "resolve_workflow_call",
    "run_pipeline",
]
