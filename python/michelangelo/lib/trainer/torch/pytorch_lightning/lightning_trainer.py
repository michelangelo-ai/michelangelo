"""Public PyTorch Lightning trainer wrapping Ray Train.

This package is a one-time snapshot of an internal trainer used for distributed
PyTorch Lightning training on Ray. Bugs may be patched in OSS, but new features
are not automatically backported from the source. See ``CONTRIBUTING.md`` for
the support policy.

Typical use::

    from michelangelo.lib.trainer.torch.pytorch_lightning import (
        LightningTrainer,
        LightningTrainerParam,
    )

    trainer = LightningTrainer(
        trainer_param=LightningTrainerParam(
            create_model_fn=my_model_factory,
            create_model_fn_kwargs={"hidden_dim": 64},
            train_data=train_ds,
            val_data=val_ds,
            batch_size=256,
        ),
        run_config=ray.train.RunConfig(name="my_run", storage_path="/tmp/runs"),
        scaling_config=ray.train.ScalingConfig(num_workers=1, use_gpu=False),
    )
    result = trainer.train()
"""

from __future__ import annotations

import json
import logging
import os
import uuid
from contextlib import contextmanager
from dataclasses import asdict, dataclass, field
from typing import TYPE_CHECKING, Callable

import ray
import torch
from fsspec.core import url_to_fs
from pytorch_lightning.utilities.deepspeed import (
    convert_zero_checkpoint_to_fp32_state_dict,
)
from ray.train.torch import TorchTrainer

from michelangelo.lib.trainer.torch.pytorch_lightning._private.util import (
    _train_loop_per_worker,
)

if TYPE_CHECKING:
    from michelangelo.lib.trainer.torch.pytorch_lightning.schema import (
        ExperimentStore,
        IncrementalTrainingSpec,
        TrainingObserver,
        TransferLearningSpec,
    )

_logger = logging.getLogger(__name__)
CHECKPOINT_NAME = ray.train.lightning.RayTrainReportCallback.CHECKPOINT_NAME
CHECKPOINT_PATH_KEY = "checkpoint_path"
# Filename Ray Train V2 uses for its on-disk checkpoint manager state, written
# inside each run directory. Mirrors
# ``ray.train.v2._internal.constants.CHECKPOINT_MANAGER_SNAPSHOT_FILENAME``;
# duplicated here because that module is private. It is the only available
# source for locating the latest checkpoint in a run directory, since V2's
# public ``Result.from_path`` is unimplemented and ``can_restore`` /
# ``resume_from_checkpoint`` are deprecated.
_CHECKPOINT_MANAGER_SNAPSHOT_FILENAME = "checkpoint_manager_snapshot.json"
_UNSET = object()


@dataclass
class LightningTrainerParam:
    """Configuration for :class:`LightningTrainer`.

    All callables (``create_model_fn``, ``data_collate_fn``) are invoked inside the
    Ray Train worker. The model is constructed on each worker via
    ``create_model_fn(**create_model_fn_kwargs)`` rather than being pickled across
    process boundaries.

    Attributes:
        create_model_fn: Factory returning a ``pytorch_lightning.LightningModule``.
            Invoked on each worker with ``**create_model_fn_kwargs``.
        create_model_fn_kwargs: Keyword arguments passed to ``create_model_fn``.
        train_data: Training Ray Dataset.
        val_data: Validation Ray Dataset.
        batch_size: Per-worker training batch size.
        num_shuffle_batches: Number of batches kept in the Ray Data local shuffle
            buffer. ``0`` disables shuffling.
        num_epochs: Deprecated; prefer ``lightning_trainer_kwargs={"max_epochs": N}``.
        data_collate_fn: Optional custom collate function passed to
            ``Dataset.iter_torch_batches``; defaults to Ray Data's column-tensor
            output.
        lightning_trainer_kwargs: Extra keyword arguments forwarded verbatim to
            ``pytorch_lightning.Trainer(...)``.
        transfer_learning_spec: Optional warm-start spec describing layer freezing
            patterns.
        incremental_training_spec: Optional spec for continuing from an existing
            run.
        initial_weights_path: Optional path to a state dict file (local, ``s3://``,
            ``gs://``, etc.); loaded on rank 0 and broadcast to other workers.
        training_observer: Optional :class:`~schema.TrainingObserver` that receives
            ``on_result`` (driver-side, after training) and ``on_checkpoint_saved``
            (worker-side, per epoch/step). Must be picklable if per-epoch
            observation is needed.
        experiment_store: Optional :class:`~schema.ExperimentStore` enabling
            opt-in auto-resume. When set (and ``run_config`` carries both a
            ``name`` and a ``storage_path``), the trainer records this run's
            experiment directory on rank 0 and, on a re-run with the same
            identity, seeds training from the previously recorded directory if
            it holds a restorable checkpoint. Note that Ray Train V2 also
            resumes natively from a reused run directory
            (``storage_path/name``); the store additionally covers the case
            where Ray's native checkpoint state is unavailable and provides a
            pluggable, backend-agnostic record of resumable runs. Defaults to
            ``None`` (no tracking, no store-driven resume). Use
            :class:`~experiment_store.FsspecExperimentStore` for the filesystem
            default. Must be picklable (serialized to workers for tracking).
        profiler_sink: Optional callable invoked on each node-local rank 0
            after ``fit()`` returns, as
            ``profiler_sink(profiler, profiler_logs_path, logger)``, where
            ``profiler`` is the profiler built from
            ``lightning_trainer_kwargs["profiler"]``, ``profiler_logs_path`` is
            the directory it wrote to, and ``logger`` is the resolved Lightning
            logger. Use it to ship profiler output to an experiment tracker;
            :func:`~_private.util.comet_profiler_sink` does this for Comet and
            :func:`~_private.util.mlflow_profiler_sink` does this for MLflow.
            Ignored when no profiler is configured or when the profiler config
            sets ``upload_profiler_results: False``. Exceptions raised by the sink are
            logged and swallowed. Must be picklable (serialized to workers).
    """

    create_model_fn: Callable
    create_model_fn_kwargs: dict
    train_data: ray.data.Dataset
    val_data: ray.data.Dataset
    batch_size: int = 8
    num_shuffle_batches: int = (
        10  # By default we reserve 10 batches in ray data shuffle buffer.
    )
    num_epochs: int | None = field(default=_UNSET)  # type: ignore[assignment]  # sentinel replaced in __post_init__
    data_collate_fn: Callable | None = None
    lightning_trainer_kwargs: dict = field(default_factory=dict)

    transfer_learning_spec: TransferLearningSpec | None = None
    incremental_training_spec: IncrementalTrainingSpec | None = None
    initial_weights_path: str | None = None
    training_observer: TrainingObserver | None = None
    experiment_store: ExperimentStore | None = None
    profiler_sink: Callable | None = None
    torch_compile: dict | None = None

    def __post_init__(self):
        """Apply default ``num_epochs`` and warn on the deprecated field usage."""
        if self.num_epochs is _UNSET:
            self.num_epochs = 1
        else:
            _logger.warning(
                "LightningTrainerParam.num_epochs is deprecated. "
                "Use LightningTrainerParam.lightning_trainer_kwargs={'max_epochs': N} instead."
            )


class LightningTrainer(TorchTrainer):
    """Ray ``TorchTrainer`` subclass that runs a PyTorch Lightning training loop."""

    def __init__(
        self,
        trainer_param: LightningTrainerParam,
        run_config: ray.train.RunConfig | None = None,
        scaling_config: ray.train.ScalingConfig | None = None,
    ):
        """Initialize the trainer.

        Args:
            trainer_param: Training configuration (model factory, datasets, etc.).
            run_config: Optional Ray ``RunConfig`` (storage path, run name, ...).
            scaling_config: Optional Ray ``ScalingConfig`` (num_workers, GPU/CPU
                requests, ...).
        """
        self.trainer_param = trainer_param
        _logger.info(
            "LightningTrainer initialized with trainer_param: %r", trainer_param
        )
        train_loop_config = asdict(trainer_param)
        # Unique run id for Comet experiment
        train_loop_config["run_id"] = str(uuid.uuid4())
        # Pop out train and val data since we have to pass them into datasets parameter of TorchTrainer.
        train_data = train_loop_config.pop("train_data")
        val_data = train_loop_config.pop("val_data")

        # A configured profiler needs an estimate of steps-per-epoch to derive
        # (or validate) its sampling schedule, which needs the training row
        # count. Only pay for it when a profiler is actually requested: count()
        # reads Parquet metadata for read_parquet-backed datasets, but any other
        # lineage forces execution of the dataset's lazy transformations.
        # Counted off trainer_param rather than the popped train_data, which
        # asdict() has already deep-copied.
        profiler_config = (train_loop_config.get("lightning_trainer_kwargs") or {}).get(
            "profiler"
        )
        if profiler_config is not None:
            try:
                train_loop_config["train_dataset_num_rows"] = (
                    trainer_param.train_data.count()
                )
            except Exception:
                _logger.warning(
                    "Could not determine the training dataset size; the profiler "
                    "schedule will fall back to its unbounded default.",
                    exc_info=True,
                )
                train_loop_config["train_dataset_num_rows"] = None
        # Pop training_observer — Protocol instances can't survive asdict()
        # because it recursively converts nested objects. We store the original
        # object on self for driver-side use and re-inject it into
        # train_loop_config so it reaches the worker callbacks via Ray
        # serialization (which pickles the config dict to worker processes).
        self._training_observer = trainer_param.training_observer
        train_loop_config.pop("training_observer", None)
        if self._training_observer is not None:
            train_loop_config["training_observer"] = self._training_observer

        # Pop experiment_store for the same asdict()-recursion reason. When set,
        # re-inject it plus the (storage_path, run_name) identity taken from the
        # driver's RunConfig, so the worker can record this run's experiment
        # directory keyed off byte-identical strings a future run resumes on.
        self._experiment_store = trainer_param.experiment_store
        train_loop_config.pop("experiment_store", None)
        if self._experiment_store is not None:
            train_loop_config["experiment_store"] = self._experiment_store
            if run_config is not None:
                train_loop_config["storage_path"] = run_config.storage_path
                train_loop_config["run_name"] = run_config.name

        # Resolve auto-resume at construction time. Ray Train V2 freezes the run
        # context (including run_config) when the base trainer is constructed and
        # resumes natively from a reused run directory; the store-driven seed
        # below is threaded into the worker as a fallback that fires only when
        # Ray has no native checkpoint to restore (see _train_loop_per_worker).
        resume_checkpoint_path = self._resolve_resume_checkpoint(run_config)
        if resume_checkpoint_path is not None:
            train_loop_config["resume_checkpoint_path"] = resume_checkpoint_path

        super().__init__(
            train_loop_per_worker=_train_loop_per_worker,
            train_loop_config=train_loop_config,
            scaling_config=scaling_config,
            run_config=run_config,
            datasets={"train": train_data, "val": val_data},
        )

    def train(
        self,
        run_config: ray.train.RunConfig | None = None,
        scaling_config: ray.train.ScalingConfig | None = None,
    ) -> dict:
        """Run training and return a small result dict.

        Args:
            run_config: Optional override applied before ``fit()``.
            scaling_config: Optional override applied before ``fit()``.

        Returns:
            Dict with ``checkpoint_path`` (path to the latest checkpoint),
            ``path`` (the Ray result path), and ``metrics``.

        Raises:
            Exception: Whatever Ray Train reports in ``result.error``.
        """
        if scaling_config is not None:
            self.scaling_config = scaling_config
        if run_config is not None:
            if self._experiment_store is not None:
                _logger.warning(
                    "run_config passed to train() overrides the construction-time "
                    "RunConfig, but auto-resume was already resolved at __init__ "
                    "from the original RunConfig (Ray Train V2 freezes the run "
                    "context at construction). The override will not re-trigger "
                    "auto-resume; pass run_config to the LightningTrainer "
                    "constructor to control resumption."
                )
            self.run_config = run_config

        result = self.fit()
        if result.error:
            raise result.error

        # The user-supplied LightningModule is captured in result.metrics["config"]
        # and is generally not serializable across worker boundaries. Drop it.
        result.metrics.pop("config", None)
        # Keep the checkpoint object for subclasses that need it (e.g., LightningTrainerWithStateDict)
        self.checkpoint = result.checkpoint

        if self._training_observer is not None:
            self._training_observer.on_result(
                metrics=result.metrics,
                checkpoint_path=result.checkpoint.path if result.checkpoint else None,
            )

        return {
            CHECKPOINT_PATH_KEY: result.checkpoint.path,
            "path": result.path,
            "metrics": result.metrics,
        }

    def _resolve_resume_checkpoint(self, run_config) -> str | None:
        """Resolve the checkpoint to seed auto-resume from, or ``None``.

        No-op unless an ``experiment_store`` is set and ``run_config`` carries
        both a ``name`` and a ``storage_path``. Asks the store for a candidate
        experiment directory, then resolves the latest Ray Train checkpoint
        inside it via :meth:`_latest_checkpoint_in`. The resolved path is
        threaded to the worker (``resume_checkpoint_path`` in
        ``train_loop_config``) where it seeds resumption only if Ray has no
        native checkpoint to restore. A store that raises, a missing candidate,
        an incomplete run identity, or a candidate with no restorable checkpoint
        all fall through to ``None`` (fresh run, or Ray's native resume).

        Args:
            run_config: The ``RunConfig`` passed at construction, carrying the
                ``storage_path`` / ``name`` identity.

        Returns:
            A scheme-qualified checkpoint directory to seed resume from, or
            ``None`` when there is nothing to resume.
        """
        store = self._experiment_store
        if store is None or run_config is None:
            return None
        storage_path = run_config.storage_path
        run_name = run_config.name
        if not storage_path or not run_name:
            _logger.info(
                "experiment_store is set but RunConfig is missing %s; "
                "auto-resume is disabled for this run.",
                "storage_path" if not storage_path else "name",
            )
            return None

        try:
            candidate = store.locate_resumable(
                storage_path=storage_path, run_name=run_name
            )
        except Exception:
            _logger.warning(
                "ExperimentStore.locate_resumable raised; not resuming",
                exc_info=True,
            )
            return None

        if not candidate:
            return None

        checkpoint_path = self._latest_checkpoint_in(candidate)
        if checkpoint_path is None:
            _logger.info(
                "Auto-resume: located experiment dir %s holds no restorable "
                "checkpoint; starting fresh.",
                candidate,
            )
            return None

        _logger.info("Auto-resume: will seed from checkpoint %s", checkpoint_path)
        return checkpoint_path

    @staticmethod
    def _latest_checkpoint_in(experiment_path: str) -> str | None:
        """Return the latest Ray Train checkpoint directory in ``experiment_path``.

        Reads Ray Train V2's ``checkpoint_manager_snapshot.json`` (best-effort)
        and returns its ``latest_checkpoint_result`` directory joined onto
        ``experiment_path``, preserving any URI scheme so the worker can build a
        ``ray.train.Checkpoint`` from it. Returns ``None`` — without raising —
        when the snapshot is missing, unreadable, or records no checkpoint;
        these are the normal "nothing to resume" cases.

        Args:
            experiment_path: The candidate experiment directory returned by
                :meth:`ExperimentStore.locate_resumable`.

        Returns:
            A scheme-qualified checkpoint directory path, or ``None``.
        """
        try:
            fs, root = url_to_fs(experiment_path)
            snapshot = f"{root.rstrip('/')}/{_CHECKPOINT_MANAGER_SNAPSHOT_FILENAME}"
            if not fs.exists(snapshot):
                return None
            with fs.open(snapshot, "r") as f:
                data = json.loads(f.read())
            latest = data.get("latest_checkpoint_result") or {}
            checkpoint_dir_name = latest.get("checkpoint_dir_name")
            if not checkpoint_dir_name:
                return None
            return f"{experiment_path.rstrip('/')}/{checkpoint_dir_name}"
        except Exception:
            _logger.debug(
                "Could not resolve latest checkpoint in %s",
                experiment_path,
                exc_info=True,
            )
            return None


class LightningTrainerWithStateDict(LightningTrainer):
    """LightningTrainer that loads the trained checkpoint into a torch model.

    After ``train()`` completes, callers can pass an initialized ``torch.nn.Module``
    to :meth:`update_model_state_dict` and have it populated from the latest
    checkpoint. Supports both DDP single-file checkpoints and DeepSpeed ZeRO
    sharded directories.
    """

    def _is_deepspeed_strategy(self) -> bool:
        """Return ``True`` if the configured strategy is DeepSpeed."""
        strategy = self.trainer_param.lightning_trainer_kwargs.get("strategy")
        if strategy is None:
            return False

        # DeepSpeed was used if the strategy is "deepspeed" or a RayDeepSpeedStrategy instance
        if isinstance(strategy, str):
            return strategy.lower() == "deepspeed"

        try:
            from ray.train.lightning import RayDeepSpeedStrategy

            return isinstance(strategy, RayDeepSpeedStrategy)
        except ImportError:
            return False

    def update_model_state_dict(self, torch_model: torch.nn.Module) -> None:
        """Populate ``torch_model`` in-place from the latest training checkpoint.

        Args:
            torch_model: Model whose ``state_dict`` will be replaced.

        Raises:
            ValueError: If ``train()`` has not been called yet.
        """
        if not hasattr(self, "checkpoint") or self.checkpoint is None:
            raise ValueError(
                "No checkpoint available. Please call train() first to generate a checkpoint."
            )
        used_deepspeed = self._is_deepspeed_strategy()
        # use the ray checkpoint as_directory() to get the local temp checkpoint directory
        with self.checkpoint.as_directory() as d:
            _logger.info(
                "Saving Ray Checkpoint to local temp Checkpoint directory: %s", d
            )
            data_dir_contents = os.listdir(d)
            _logger.info("Data directory contents: %s", data_dir_contents)
            lightning_ckpt_path = os.path.join(d, CHECKPOINT_NAME)
            if used_deepspeed:
                local_model_path = os.path.join(lightning_ckpt_path, "model.pt")
                # PyTorch 2.6+ defaults weights_only=True, which rejects arbitrary Python classes
                # (LossScaler, DynamicLossScaler, optimizer states, etc.) embedded in DeepSpeed ZeRO
                # checkpoints. The env var reverts the default for any torch.load call that doesn't
                # explicitly pass weights_only, covering both pytorch_lightning and deepspeed internals.
                # TODO: Remove this once we upgrade to Lightning 2.6+ https://github.com/Lightning-AI/pytorch-lightning/pull/21194
                with _torch_weights_only_disabled():
                    model_state_dict = convert_zero_checkpoint_to_fp32_state_dict(
                        lightning_ckpt_path, local_model_path
                    )
                _logger.info(
                    "Loaded DeepSpeed checkpoint from %s to %s",
                    lightning_ckpt_path,
                    local_model_path,
                )
            else:
                # DDP checkpoint
                checkpoint = torch.load(lightning_ckpt_path, map_location="cpu")
                model_state_dict = checkpoint["state_dict"]
                _logger.info("Loaded DDP checkpoint from %s", lightning_ckpt_path)
            torch_model.load_state_dict(model_state_dict, strict=False)
            _logger.info("Updated the state dict of the torch model.")


@contextmanager
def _torch_weights_only_disabled():
    """Force ``torch.load()`` to use ``weights_only=False`` for callers that don't pass it explicitly."""
    key = "TORCH_FORCE_NO_WEIGHTS_ONLY_LOAD"
    old = os.environ.pop(key, None)
    os.environ[key] = "1"
    try:
        yield
    finally:
        if old is not None:
            os.environ[key] = old
        else:
            os.environ.pop(key, None)
