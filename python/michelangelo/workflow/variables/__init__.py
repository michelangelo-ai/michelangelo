"""Workflow variables for Michelangelo pipeline management."""

from __future__ import annotations

from michelangelo.workflow.variables._private.dataset import DatasetVariable
from michelangelo.workflow.variables._private.model import ModelVariable
from michelangelo.workflow.variables.metadata import (
    FeaturePackageMetadata,
    ModelMetadata,
)
from michelangelo.workflow.variables.types import (
    AssembledModel,
    FeaturePackageArtifact,
    ModelArtifact,
    PusherResult,
)

__all__ = [
    "AssembledModel",
    "DatasetVariable",
    "FeaturePackageArtifact",
    "FeaturePackageMetadata",
    "ModelArtifact",
    "ModelMetadata",
    "ModelVariable",
    "PusherResult",
]
