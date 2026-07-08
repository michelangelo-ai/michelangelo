"""Schema dataclasses used by the Lightning trainer."""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum
from typing import Any, Protocol, runtime_checkable


@runtime_checkable
class TrainingObserver(Protocol):
    """Protocol for observing training events.

    Implement this protocol to receive notifications when training completes
    or checkpoints are saved.

    **Picklability:** Implementations must be picklable when using per-epoch
    observation (``on_checkpoint_saved``), because Ray serializes the training
    config — including the observer — to worker processes. Avoid storing
    non-picklable objects (open file handles, DB connections, lambdas) as
    instance attributes.

    **Worker-side behavior:** ``on_checkpoint_saved`` is called on **every**
    Ray worker (all ranks), not just rank 0. Implementations should be
    idempotent or guard on rank internally if side effects (DB writes,
    HTTP calls) should only happen once.

    Example::

        from michelangelo.lib.trainer.torch.pytorch_lightning import (
            LightningTrainer,
            LightningTrainerParam,
            TrainingObserver,
        )

        class MyObserver:
            def on_result(self, metrics: dict[str, Any], checkpoint_path: str | None) -> None:
                print(f"Training done: {metrics}")

            def on_checkpoint_saved(
                self, epoch: int, step: int, metrics: dict[str, Any], checkpoint_path: str,
            ) -> None:
                print(f"Checkpoint at epoch {epoch}")

        trainer = LightningTrainer(
            trainer_param=LightningTrainerParam(
                create_model_fn=my_model_factory,
                train_data=train_ds,
                val_data=val_ds,
                training_observer=MyObserver(),
            ),
        )
    """

    def on_result(self, metrics: dict[str, Any], checkpoint_path: str | None) -> None:
        """Called on the driver after training completes successfully.

        Args:
            metrics: Final training metrics dict.
            checkpoint_path: Path to the final checkpoint, or ``None`` if no
                checkpoint was saved.
        """
        ...

    def on_checkpoint_saved(
        self,
        epoch: int,
        step: int,
        metrics: dict[str, Any],
        checkpoint_path: str,
    ) -> None:
        """Called on each worker after a checkpoint is saved and reported.

        Note: this is called on **all** workers, not just rank 0.
        The ``checkpoint_path`` is a local temporary path that may be
        cleaned up shortly after this method returns.

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
    """Consolidated specification for all incremental training configurations.

    Attributes:
        metadata: Baseline model and training-type metadata for this run.
        load_optimizer_weights: Whether to restore optimizer state from the
            baseline checkpoint in addition to model weights.
        override_incremental_training_epoch: Explicit starting epoch for the
            incremental run. ``None`` continues from the baseline's own epoch
            count.
        fused_model_submodule: Optional submodule-prefix used to select a
            slice of a fused checkpoint's combined state dict before loading
            it (e.g. ``"predictor_module"`` for the DL predictor half of a
            fused native-transform package). Schema-only in OSS today —
            carried through for forward compatibility with internal
            Michelangelo's warm-start config shape; no OSS code currently
            strips or consumes this prefix. Defaults to ``None`` here,
            unlike internal Michelangelo's ``"predictor_module"`` default —
            reconcile this divergence deliberately once OSS implements the
            stripping behavior (see the PR that ports it).
    """

    metadata: IncrementalTrainingMetadata
    load_optimizer_weights: bool = False
    override_incremental_training_epoch: int | None = None
    fused_model_submodule: str | None = None


@dataclass
class TransferLearningMetadata:
    """Metadata for transfer learning."""

    learning_mode: LearningMode
    baseline_model: ModelSpec | None


@dataclass
class TransferLearningSpec:
    """Consolidated specification for all transfer learning configurations.

    Attributes:
        metadata: Baseline model and learning-mode metadata for this run.
        model_loader_function: Optional dotted path to a custom function for
            loading the baseline model, overriding the default loader.
        layer_names_to_inherit: Exact layer names to copy weights for from
            the baseline model.
        layer_names_to_inherit_regex: Regex patterns matching layer names to
            copy weights for from the baseline model.
        layer_names_to_freeze: Exact layer names to freeze (exclude from
            gradient updates) after loading baseline weights.
        layer_names_to_freeze_regex: Regex patterns matching layer names to
            freeze after loading baseline weights.
        fused_model_submodule: Optional submodule-prefix used to select a
            slice of a fused checkpoint's combined state dict before loading
            it (e.g. ``"predictor_module"`` for the DL predictor half of a
            fused native-transform package). Schema-only in OSS today —
            carried through for forward compatibility with internal
            Michelangelo's warm-start config shape; no OSS code currently
            strips or consumes this prefix. Defaults to ``None`` here,
            unlike internal Michelangelo's ``"predictor_module"`` default —
            reconcile this divergence deliberately once OSS implements the
            stripping behavior (see the PR that ports it).
    """

    metadata: TransferLearningMetadata

    model_loader_function: str | None = None
    layer_names_to_inherit: list[str] = field(default_factory=list)
    layer_names_to_inherit_regex: list[str] = field(default_factory=list)
    layer_names_to_freeze: list[str] = field(default_factory=list)
    layer_names_to_freeze_regex: list[str] = field(default_factory=list)
    fused_model_submodule: str | None = None
