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

    Example (observing training events)::

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


@runtime_checkable
class ExperimentStore(Protocol):
    """Pluggable locator for resumable Ray Train experiments.

    An ``ExperimentStore`` lets :class:`LightningTrainer` auto-resume a prior
    run's experiment directory. It bridges two moments that live in different
    processes:

    * :meth:`track` runs **on the worker (rank 0 only)** at the start of a run,
      recording where this run's Ray Train experiment directory lives so a
      *future* run can find it.
    * :meth:`locate_resumable` runs **on the driver** before ``fit()``, looking
      up a candidate experiment directory to restore from.

    The stable cross-run identity is ``(storage_path, run_name)`` — both taken
    from the driver's ``RunConfig`` and passed verbatim to both methods, so the
    two sides always agree on the key regardless of how Ray names the on-disk
    experiment directory.

    **Picklability:** implementations must be picklable — Ray serializes the
    training config (including this store) to worker processes for
    :meth:`track`. Avoid open file handles, DB connections, or lambdas as
    instance attributes (same constraint as :class:`TrainingObserver`).

    **Never raise:** neither method may raise on the "nothing to resume" or
    "couldn't persist" paths. :meth:`locate_resumable` returns ``None`` when
    there is nothing to resume; :meth:`track` is best-effort (a failed write
    must not fail an otherwise-successful training run). The trainer additionally
    guards both call sites, so a misbehaving custom store cannot crash training.

    Example (default fsspec backend)::

        from michelangelo.lib.trainer.torch.pytorch_lightning import (
            FsspecExperimentStore,
            LightningTrainerParam,
        )

        param = LightningTrainerParam(
            create_model_fn=my_model_factory,
            create_model_fn_kwargs={},
            train_data=train_ds,
            val_data=val_ds,
            experiment_store=FsspecExperimentStore(),
        )

    Example (bring your own backend)::

        class RedisExperimentStore:
            \"\"\"Record the resume pointer in Redis instead of a marker file.\"\"\"

            def __init__(self, client) -> None:
                self._client = client

            def _key(self, storage_path: str, run_name: str) -> str:
                return f"ma-resume:{storage_path}:{run_name}"

            def track(
                self, *, storage_path: str, run_name: str, experiment_path: str
            ) -> None:
                try:
                    self._client.set(
                        self._key(storage_path, run_name), experiment_path
                    )
                except Exception:  # best-effort: never fail training
                    pass

            def locate_resumable(
                self, *, storage_path: str, run_name: str
            ) -> str | None:
                try:
                    value = self._client.get(self._key(storage_path, run_name))
                except Exception:  # nothing to resume on any lookup failure
                    return None
                return value.decode() if value else None
    """

    def track(self, *, storage_path: str, run_name: str, experiment_path: str) -> None:
        """Record this run's experiment directory for future resumption.

        Called once, on worker rank 0, near the start of training.

        Args:
            storage_path: The driver's ``RunConfig.storage_path`` (the storage
                root; also the fsspec root under which markers are kept).
            run_name: The driver's ``RunConfig.name`` (stable run identity).
            experiment_path: Absolute path of *this* run's Ray Train experiment
                directory, derived by the caller from the driver's
                ``RunConfig.storage_path`` and
                ``ray.train.get_context().get_storage().experiment_dir_name``.
                Scheme-qualified for remote filesystems (e.g.
                ``s3://bucket/runs/my_run``) so a later resume can address it.

        Returns:
            ``None``.
        """
        ...

    def locate_resumable(self, *, storage_path: str, run_name: str) -> str | None:
        """Return a candidate experiment directory to resume, or ``None``.

        Called on the driver before ``fit()``. The returned path is only a
        *candidate*: the trainer resolves the latest Ray Train checkpoint within
        it and seeds resumption only if one exists, so this method must not
        itself check for a valid checkpoint. Returns ``None`` when no marker
        exists or it cannot be read.

        Args:
            storage_path: The driver's ``RunConfig.storage_path``.
            run_name: The driver's ``RunConfig.name``.

        Returns:
            A candidate experiment directory path, or ``None`` when there is
            nothing to resume.
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
        fused_model_submodule: Reserved for future use; has no effect today.
            Intended as a submodule-prefix selecting a slice of a fused
            checkpoint's combined state dict before loading it (e.g.
            ``"predictor_module"`` for the DL predictor half of a fused
            native-transform package). Nothing currently reads it — leave it
            unset.
    """

    metadata: IncrementalTrainingMetadata
    load_optimizer_weights: bool = False
    override_incremental_training_epoch: int | None = None
    # Maintainer note: defaults to None here, where internal Michelangelo AI
    # defaults to "predictor_module". Reconcile this divergence deliberately if
    # and when the prefix-stripping behavior is implemented here.
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
        fused_model_submodule: Reserved for future use; has no effect today.
            Intended as a submodule-prefix selecting a slice of a fused
            checkpoint's combined state dict before loading it (e.g.
            ``"predictor_module"`` for the DL predictor half of a fused
            native-transform package). Nothing currently reads it — leave it
            unset.
    """

    metadata: TransferLearningMetadata

    model_loader_function: str | None = None
    layer_names_to_inherit: list[str] = field(default_factory=list)
    layer_names_to_inherit_regex: list[str] = field(default_factory=list)
    layer_names_to_freeze: list[str] = field(default_factory=list)
    layer_names_to_freeze_regex: list[str] = field(default_factory=list)
    # Maintainer note: defaults to None here, where internal Michelangelo AI
    # defaults to "predictor_module". Reconcile this divergence deliberately if
    # and when the prefix-stripping behavior is implemented here.
    fused_model_submodule: str | None = None
