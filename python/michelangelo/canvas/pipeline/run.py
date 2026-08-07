"""Minimal local execution entry point for ``pipeline_conf.yaml`` pipelines.

This is intentionally thin: it proves the ``pipeline_task``
(:mod:`michelangelo.canvas.pipeline.task`) + config loader
(:mod:`michelangelo.canvas.pipeline.config_loader`) wiring end-to-end for the
YAML-authoring POC. It does not replicate the distributed (Ray/Spark) local-run
environment setup that internal's equivalent tooling does — that's a separate,
follow-up concern once real local-execution parity is needed.
"""

from pathlib import Path
from typing import Any, Union

from michelangelo.canvas.pipeline.config_loader import load_pipeline_config


def run_pipeline(pipeline_conf_path: Union[str, Path]) -> Any:
    """Load and run a ``pipeline_conf.yaml`` pipeline in-process.

    Args:
        pipeline_conf_path: Path to a ``pipeline_conf.yaml`` file.

    Returns:
        The workflow function's return value.
    """
    pipeline_config = load_pipeline_config(pipeline_conf_path)
    return pipeline_config.call_workflow()
