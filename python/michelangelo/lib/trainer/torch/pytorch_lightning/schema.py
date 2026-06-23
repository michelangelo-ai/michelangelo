"""Schema dataclasses used by the Lightning trainer."""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum
from typing import Any, Protocol, runtime_checkable


@runtime_checkable
class TrainingObserver(Protocol):
    """Protocol for observing training events.

    Implement this protocol to receive notifications when training completes
    or checkpoints are saved. Implementations used with per-epoch observation
    (``on_checkpoint_saved``) must be picklable, since they are shipped to Ray
    Train workers.

    Example::

        class MyObserver:
            def on_result(self, metrics: dict[str, Any], checkpoint_path: str) -> None:
                print(f"Training done: {metrics}")

            def on_checkpoint_saved(
                self, epoch: int, step: int, metrics: dict[str, float], checkpoint_path: str,
            ) -> None:
                print(f"Checkpoint at epoch {epoch}")

        trainer = LightningTrainer(
            trainer_param=LightningTrainerParam(..., training_observer=MyObserver()),
            ...
        )
    """

    def on_result(self, metrics: dict[str, Any], checkpoint_path: str) -> None:
        """Called on the driver after training completes successfully.

        Args:
            metrics: Final training metrics dict.
            checkpoint_path: Path to the final checkpoint.
        """
        ...

    def on_checkpoint_saved(
        self,
        epoch: int,
        step: int,
        metrics: dict[str, float],
        checkpoint_path: str,
    ) -> None:
        """Called on each worker after a checkpoint is saved and reported.

        Args:
            epoch: Current training epoch.
            step: Current global step.
            metrics: Metrics dict reported with the checkpoint.
            checkpoint_path: Local path where the checkpoint was saved.
        """
        ...


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
    revision_id: str | None = None


@dataclass
class IncrementalTrainingMetadata:
    """Metadata for incremental training."""

    training_type: TrainingType
    baseline_model: ModelSpec
    deployment_name: str | None = None
    skip_training: bool = False
    log_layer_weights: bool = False


@dataclass
class IncrementalTrainingSpec:
    """Consolidated specification for all incremental training configurations."""

    metadata: IncrementalTrainingMetadata
    load_optimizer_weights: bool = False
    override_incremental_training_epoch: int | None = None


@dataclass
class TransferLearningMetadata:
    """Metadata for transfer learning."""

    learning_mode: LearningMode
    baseline_model: ModelSpec | None


@dataclass
class TransferLearningSpec:
    """Consolidated specification for all transfer learning configurations."""

    metadata: TransferLearningMetadata

    model_loader_function: str | None = None
    layer_names_to_inherit: list[str] = field(default_factory=list)
    layer_names_to_inherit_regex: list[str] = field(default_factory=list)
    layer_names_to_freeze: list[str] = field(default_factory=list)
    layer_names_to_freeze_regex: list[str] = field(default_factory=list)
