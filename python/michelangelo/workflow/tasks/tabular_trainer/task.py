"""Dispatcher and lightning glue for the tabular trainer workflow task.

This task never touches a storage backend — it hands off its trained model
as an intra-pipeline ``ModelVariable``. Only a downstream packaging task
(e.g. the pusher, see ``michelangelo.workflow.tasks.pusher``) owns uploading
into the consolidated model manager/artifact store via a ``StorageBackend``.
"""

from __future__ import annotations

import dataclasses
import io
import logging
import os
import pickle
from typing import TYPE_CHECKING, Callable, Optional

from michelangelo._internal.utils.reflection_utils.module_attr import get_module_attr
from michelangelo.lib.trainer.torch.pytorch_lightning.lightning_trainer import (
    CometParam,
    LightningTrainerParam,
    LightningTrainerWithStateDict,
)
from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.schema.tabular_trainer import (
    IncrementalTrainingModeConfig,
    LightningTrainerConfig,
    TabularTrainerConfig,
)
from michelangelo.workflow.tasks.tabular_trainer._dataset import (
    collate_sample_row,
    construct_read_kwargs,
    get_model_schema,
    get_sample_data,
    raise_lightning_trainer_config_deprecation_warnings,
)
from michelangelo.workflow.variables import ModelVariable
from michelangelo.workflow.variables.metadata import (
    TRAINING_FRAMEWORK_LIGHTNING,
    ModelMetadata,
)
from michelangelo.workflow.variables.types import ModelArtifact

if TYPE_CHECKING:
    import ray
    import ray.train

    from michelangelo.workflow.variables._private.dataset import DatasetVariable

_logger = logging.getLogger(__name__)

ApplyIncrementalTrainingMetadataFn = Callable[
    [ModelMetadata, Optional[ModelArtifact], Optional[IncrementalTrainingModeConfig]],
    None,
]


def _apply_incremental_training_metadata(
    metadata: ModelMetadata,
    initial_model: ModelArtifact | None,
    incremental_training_mode: IncrementalTrainingModeConfig | None,
) -> None:
    """Set incremental training fields on *metadata* based on the training mode.

    Two cases set ``is_incremental_training = True``:

    - **Continuation**: *initial_model* exists and its own
      ``is_incremental_training`` is ``True`` — the chain propagates; the
      original baseline identifier is forwarded unchanged.
    - **BASELINE start**: ``incremental_training_mode == BASELINE`` — marks the
      first link of a new chain.

    An *initial_model* with ``is_incremental_training = False`` represents
    transfer learning and starts a new independent model (no chain propagation).

    When both *initial_model* is incremental **and**
    ``incremental_training_mode == BASELINE``, the continuation branch takes
    priority — the existing chain continues rather than restarting. The BASELINE
    declaration is ignored in this case.

    Args:
        metadata: The ``ModelMetadata`` to mutate in place.
        initial_model: Optional warm-start artifact from a prior run.
        incremental_training_mode: The mode declared in
            ``LightningTrainerConfig.incremental_training_mode``.
    """
    if initial_model is not None and initial_model.metadata.is_incremental_training:
        metadata.is_incremental_training = True
        metadata.baseline_model_identifier = (
            initial_model.metadata.baseline_model_identifier
        )
    elif incremental_training_mode == IncrementalTrainingModeConfig.BASELINE:
        metadata.is_incremental_training = True


def train_tabular(
    config: TabularTrainerConfig,
    train_dataset: DatasetVariable,
    validation_dataset: DatasetVariable,
    *,
    initial_model: ModelArtifact | None = None,
    run_config: ray.train.RunConfig | None = None,
    scaling_config: ray.train.ScalingConfig | None = None,
    is_local_run: bool = False,
    apply_incremental_training_metadata: (
        ApplyIncrementalTrainingMetadataFn
    ) = _apply_incremental_training_metadata,
) -> ModelVariable:
    """Run tabular training and return the trained model as a ``ModelVariable``.

    Dispatches to the lightning or custom backend based on *config*. The
    trained model is an intra-pipeline intermediate, not a registry-ready
    artifact — it is handed off as a ``ModelVariable`` (persisted under
    ``UF_STORAGE_URL``, the same convention ``DatasetVariable`` uses) for a
    downstream task to package and push. Uploading into the consolidated
    model manager/artifact store is a packaging task's job, not the
    trainer's — no such packaging ("assembler") task exists in OSS yet, so
    today the returned ``ModelVariable`` is a stopping point for
    programmatic use of its ``.value`` (the in-memory trained model) until
    that task is added.

    Example:
        >>> config = TabularTrainerConfig(lightning=...)  # doctest: +SKIP
        >>> result = train_tabular(  # doctest: +SKIP
        ...     config, train_dataset, validation_dataset
        ... )
        >>> trained_model = result.value  # in-memory model  # doctest: +SKIP

    Args:
        config: ``TabularTrainerConfig`` — exactly one of ``lightning`` or
            ``custom`` must be set.
        train_dataset: Training dataset variable.
        validation_dataset: Validation dataset variable.
        initial_model: Optional warm-start artifact. ``ModelArtifact.path``
            must point directly to a local state-dict file (e.g. as written
            by ``ModelVariable.save_lightning_model()``) — not a directory.
            No storage backend is involved; the path is passed straight
            through to ``LightningTrainerParam.initial_weights_path``.
        run_config: Optional ``ray.train.RunConfig``. When ``None``, a default
            is built by
            :func:`michelangelo.uniflow.plugins.ray.run_config.create_run_config`,
            which points ``storage_path``/``storage_filesystem`` at
            ``UF_STORAGE_URL`` so worker pods on a multi-node cluster share
            the same checkpoint storage as the head pod. Falls back to a
            local temp dir when ``UF_STORAGE_URL`` is unset (e.g. local runs).
        scaling_config: Optional ``ray.train.ScalingConfig``. When ``None`` a
            default is constructed from ``config.lightning.scaling_config``.
        is_local_run: When ``True``, defaults precision to ``"32"`` (Lightning's
            default) instead of ``"bf16-mixed"``. This only controls the
            *default* — set ``config.lightning.lightning_trainer_kwargs.precision``
            to override regardless of this flag.
        apply_incremental_training_metadata: Callable matching
            ``(ModelMetadata, ModelArtifact | None, IncrementalTrainingModeConfig
            | None) -> None``. Called after training to stamp incremental chain
            metadata onto the result. Defaults to the built-in
            :func:`_apply_incremental_training_metadata`; injectable for testing.

    Returns:
        A ``ModelVariable`` wrapping the trained model, with
        ``metadata.assembled=False`` and ``metadata.deployable=False``.
        There is no OSS packaging ("assembler") task yet to turn this into
        a serving-ready ``ModelArtifact`` — until one exists, use
        ``result.value`` for the in-memory model directly, or
        ``result.save()``/``ModelVariable(path=..., metadata=...)`` to
        persist/reload it across tasks.

    Raises:
        ConfigurationError: If the training dataset is empty, or if
            *initial_model* is provided but ``initial_model.path`` is not a
            file.
        NotImplementedError: If ``config.lightning.checkpoint_config
            .save_every_n_steps`` is set, if
            ``config.lightning.transfer_learning_spec`` is set, if
            ``config.lightning.experiment_tracker.mlflow`` is set, or if
            ``config.custom`` is used (custom backend not yet in OSS).
    """
    if config.lightning:
        return _train_lightning(
            config.lightning,
            train_dataset,
            validation_dataset,
            initial_model=initial_model,
            run_config=run_config,
            scaling_config=scaling_config,
            is_local_run=is_local_run,
            apply_incremental_training_metadata=apply_incremental_training_metadata,
        )

    # config.custom is set (TabularTrainerConfig.__post_init__ ensures exactly one).
    raise NotImplementedError(
        "The custom trainer backend is not yet available in OSS. "
        "Use the lightning backend (config.lightning) instead."
    )


def _train_lightning(
    config: LightningTrainerConfig,
    train_dataset: DatasetVariable,
    validation_dataset: DatasetVariable,
    *,
    initial_model: ModelArtifact | None,
    run_config: ray.train.RunConfig | None,
    scaling_config: ray.train.ScalingConfig | None,
    is_local_run: bool,
    apply_incremental_training_metadata: ApplyIncrementalTrainingMetadataFn,
) -> ModelVariable:
    """Lightning + Ray Train implementation of :func:`train_tabular`."""
    import ray.train

    from michelangelo.uniflow.plugins.ray.run_config import create_run_config

    _logger.info("Starting lightning trainer")

    raise_lightning_trainer_config_deprecation_warnings(config)

    # Load datasets — load_ray_dataset() takes no kwargs in OSS; apply column
    # projection post-load via select_columns.
    train_dataset.load_ray_dataset()
    validation_dataset.load_ray_dataset()
    read_kwargs = construct_read_kwargs(config)
    columns = read_kwargs.get("columns")

    train_data = train_dataset.value
    validation_data = validation_dataset.value

    if columns:
        train_data = train_data.select_columns(columns)
        validation_data = validation_data.select_columns(columns)

    # Model class + kwargs
    create_model_fn = get_module_attr(config.model_class)
    create_model_fn_kwargs: dict = config.model_kwargs or {}

    # Batch / collate resolution (BatchIterConfig > hyperparameters)
    hp = config.hyperparameters or {}
    batch_size = int(hp.get("batch_size", 2))
    num_shuffle_batches = int(hp.get("num_shuffle_batches", 1))
    data_collate_fn = None

    if config.dataloading_config and config.dataloading_config.batch_iter_config:
        dl = config.dataloading_config.batch_iter_config
        for field_name in ("batch_size", "num_shuffle_batches"):
            if field_name in hp:
                _logger.warning(
                    "dataloading_config.batch_iter_config.%s=%r overrides "
                    "hyperparameters.%s=%r",
                    field_name,
                    getattr(dl, field_name),
                    field_name,
                    hp[field_name],
                )
        batch_size = dl.batch_size
        num_shuffle_batches = dl.num_shuffle_batches
        data_collate_fn = get_module_attr(dl.collate_fn) if dl.collate_fn else None

    # Experiment tracking
    comet_param: CometParam | None = None
    if (
        config.experiment_tracker is not None
        and config.experiment_tracker.comet is not None
    ):
        c = config.experiment_tracker.comet
        comet_param = CometParam(
            api_key=c.api_key,
            project_name=c.project_name,
            experiment_name=c.experiment_name,
            workspace=c.workspace,
        )

    # MLflow gate — wiring not yet implemented
    # TODO(#1427): wire MLflow tracking into LightningTrainerParam
    if (
        config.experiment_tracker is not None
        and config.experiment_tracker.mlflow is not None
    ):
        raise NotImplementedError(
            "MLflow experiment tracking is not yet wired in OSS. "
            "Remove mlflow from ExperimentTrackerConfig to proceed, "
            "or use CometML tracking instead."
        )

    # Mid-epoch checkpointing gate
    if config.checkpoint_config.save_every_n_steps is not None:
        raise NotImplementedError(
            "Mid-epoch checkpointing (save_every_n_steps) is not yet available "
            "in OSS. Remove save_every_n_steps from CheckpointConfig to proceed."
        )

    # Transfer-learning spec gate
    if config.transfer_learning_spec is not None:
        raise NotImplementedError(
            "transfer_learning_spec building is not yet available in OSS. "
            "Remove transfer_learning_spec from LightningTrainerConfig to proceed."
        )

    # Ray Train CheckpointConfig
    checkpoint_config = ray.train.CheckpointConfig(
        num_to_keep=config.checkpoint_config.num_to_keep,
        checkpoint_score_attribute=config.checkpoint_config.checkpoint_score_attribute,
        checkpoint_score_order=config.checkpoint_config.checkpoint_score_order.value,
    )

    # ScalingConfig: use injected or build from schema config
    if scaling_config is None:
        if config.scaling_config is not None:
            scaling_config = ray.train.ScalingConfig(
                num_workers=1,
                use_gpu=False,
                resources_per_worker={"CPU": config.scaling_config.cpu_per_worker},
            )
        else:
            scaling_config = ray.train.ScalingConfig(num_workers=1)

    # RunConfig: use injected or build a default via the shared UniFlow helper,
    # which points Ray Train's distributed checkpointing at UF_STORAGE_URL
    # (falling back to a local tempdir if unset).
    if run_config is None:
        run_config = create_run_config(checkpoint_config=checkpoint_config)

    # LightningTrainerKwargs: merge, resolve _count variants, set defaults
    lightning_trainer_kwargs: dict = {}
    if config.lightning_trainer_kwargs is not None:
        lightning_trainer_kwargs = {
            k: v
            for k, v in dataclasses.asdict(config.lightning_trainer_kwargs).items()
            if v is not None
        }

    for split in ("train", "val", "test", "predict"):
        count_key = f"limit_{split}_batches_count"
        if count_key in lightning_trainer_kwargs:
            lightning_trainer_kwargs[f"limit_{split}_batches"] = (
                lightning_trainer_kwargs.pop(count_key)
            )

    lightning_trainer_kwargs.setdefault("max_epochs", int(hp.get("num_epochs", 1)))

    default_precision = "32" if is_local_run else "bf16-mixed"
    lightning_trainer_kwargs.setdefault(
        "precision", hp.get("precision", default_precision)
    )

    # ModelArtifact.path is always a local filesystem path (per its
    # contract) to the packaged state-dict file itself — matching what
    # LightningTrainerParam.initial_weights_path expects (see
    # lightning_trainer.py). No storage backend is involved; weights are
    # read directly.
    initial_weights_path: str | None = None
    if initial_model is not None:
        if not os.path.isfile(initial_model.path):
            raise ConfigurationError(
                f"initial_model.path {initial_model.path!r} is not a file. "
                "ModelArtifact.path for a lightning warm-start must point "
                "directly to the state-dict file (e.g. as written by "
                "ModelVariable.save_lightning_model())."
            )
        initial_weights_path = initial_model.path
        _logger.info("Using initial weights from: %s", initial_weights_path)

    # Build and run trainer
    trainer_param = LightningTrainerParam(
        create_model_fn=create_model_fn,
        create_model_fn_kwargs=create_model_fn_kwargs,
        train_data=train_data,
        val_data=validation_data,
        batch_size=batch_size,
        num_shuffle_batches=num_shuffle_batches,
        data_collate_fn=data_collate_fn,
        lightning_trainer_kwargs=lightning_trainer_kwargs,
        comet_param=comet_param,
        transfer_learning_spec=None,
        initial_weights_path=initial_weights_path,
    )
    trainer = LightningTrainerWithStateDict(
        trainer_param, run_config=run_config, scaling_config=scaling_config
    )
    trainer.train()
    _logger.info("Training complete")

    # Rebuild model from Ray checkpoint
    trained_model = create_model_fn(**create_model_fn_kwargs)
    trainer.update_model_state_dict(trained_model)

    # Schema + sample data
    schema = get_model_schema(config.input_columns, config.output_columns)
    sample_rows = train_data.take(1)
    if not sample_rows:
        raise ConfigurationError(
            "Training dataset produced 0 rows. "
            "At least one row is required to build model sample_data metadata."
        )
    sample_dict = collate_sample_row(
        sample_rows[0], data_collate_fn, config.metadata_columns
    )
    sample_data = get_sample_data(sample_dict, config.input_columns)

    # Build metadata
    metadata = ModelMetadata(
        training_framework=TRAINING_FRAMEWORK_LIGHTNING,
        model_class=config.model_class,
        assembled=False,
        deployable=False,
    )
    metadata.hyperparameters = create_model_fn_kwargs
    metadata._schema = io.BytesIO(pickle.dumps(schema))
    metadata._sample_data = io.BytesIO(pickle.dumps(sample_data[:5]))
    apply_incremental_training_metadata(
        metadata, initial_model, config.incremental_training_mode
    )

    # Hand off as an intra-pipeline ModelVariable — persisted under
    # UF_STORAGE_URL, dispatched to save_lightning_model() via
    # metadata.training_framework. Packaging into a registry-ready
    # ModelArtifact is the assembler's job, not the trainer's.
    model_variable = ModelVariable(value=trained_model, metadata=metadata)
    model_variable.save()
    return model_variable
