"""Workflow variables for Michelangelo pipeline management."""

from __future__ import annotations

import contextlib

# flake8: noqa:F401
from michelangelo.workflow.variables.metadata import ModelMetadata
from michelangelo.workflow.variables.types import (
    AssembledModel,
    ModelArtifact,
    PusherResult,
)

with contextlib.suppress(ImportError):
    from michelangelo.workflow.variables._private.dataset import DatasetVariable
