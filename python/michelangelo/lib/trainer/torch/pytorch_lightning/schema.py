"""Schema dataclasses used by the Lightning trainer.

Ported (snapshot) from the internal
``uber.ai.michelangelo.sdk.common.schema`` module so the OSS trainer keeps the
same typed surface as the internal trainer.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum
from typing import Optional


class TrainingType(Enum):
    """Enum for training types in incremental training."""

    BASE_MODEL_TRAINING = 0
    INCREMENTAL_TRAINING = 1


class LearningMode(Enum):
    """Enum for learning modes in transfer learning."""

    DISABLED = 0
    TRANSFER_LEARNING = 1


@dataclass
class ModelSpec:
    """A reference to a model that may be loaded for incremental training or transfer learning."""

    project_name: str
    model_name: str
    revision_id: Optional[str] = None


@dataclass
class IncrementalTrainingMetadata:
    """Metadata for incremental training."""

    training_type: TrainingType
    baseline_model: ModelSpec
    deployment_name: Optional[str] = None
    skip_training: bool = False
    log_layer_weights: bool = False


@dataclass
class IncrementalTrainingSpec:
    """Consolidated specification for all incremental training configurations."""

    metadata: IncrementalTrainingMetadata
    load_optimizer_weights: bool = False
    override_incremental_training_epoch: Optional[int] = None


@dataclass
class TransferLearningMetadata:
    """Metadata for transfer learning."""

    learning_mode: LearningMode
    baseline_model: Optional[ModelSpec]


@dataclass
class TransferLearningSpec:
    """Consolidated specification for all transfer learning configurations."""

    metadata: TransferLearningMetadata

    model_loader_function: Optional[str] = None
    layer_names_to_inherit: list[str] = field(default_factory=list)
    layer_names_to_inherit_regex: list[str] = field(default_factory=list)
    layer_names_to_freeze: list[str] = field(default_factory=list)
    layer_names_to_freeze_regex: list[str] = field(default_factory=list)
