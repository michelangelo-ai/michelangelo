"""Dispatcher and lightning glue for the tabular trainer workflow task."""

from __future__ import annotations

import contextlib
import dataclasses
import io
import logging
import os
import pickle
import tempfile
import uuid
from typing import TYPE_CHECKING, Callable, Optional

import torch

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
from michelangelo.workflow.variables.metadata import (
    TRAINING_FRAMEWORK_LIGHTNING,
    ModelMetadata,
)
from michelangelo.workflow.variables.types import ModelArtifact

if TYPE_CHECKING:
    import ray
    import ray.train

    from michelangelo.lib.artifact_manager.storage_backend import StorageBackend
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
    storage_backend: StorageBackend | None,
    initial_model: ModelArtifact | None = None,
    run_config: ray.train.RunConfig | None = None,
    scaling_config: ray.train.ScalingConfig | None = None,
    is_local_run: bool = False,
    apply_incremental_training_metadata: (
        ApplyIncrementalTrainingMetadataFn
    ) = _apply_incremental_training_metadata,
) -> ModelArtifact:
    """Run tabular training and return a packaged ``ModelArtifact``.

    Dispatches to the lightning or custom backend based on *config*. The
    resulting torch model is serialised to a temp dir and uploaded via
    *storage_backend*; the returned artifact's ``path`` is the upload URI.

    Args:
        config: ``TabularTrainerConfig`` — exactly one of ``lightning`` or
            ``custom`` must be set.
        train_dataset: Training dataset variable.
        validation_dataset: Validation dataset variable.
        storage_backend: Backend used to upload the trained model and to
            download warm-start weights for *initial_model* (if provided).
            Raises ``ConfigurationError`` when ``None``.
        initial_model: Optional warm-start artifact. Weights are downloaded to
            a local temp dir before training begins; the local path is passed to
            ``LightningTrainerParam.initial_weights_path``.
        run_config: Optional ``ray.train.RunConfig``. When ``None`` a default
            is constructed with a temp ``storage_path``.
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
        A ``ModelArtifact`` with ``assembled=False`` and ``deployable=False``.
        Use the assembler task to produce a serving-ready artifact.

    Raises:
        ConfigurationError: If *storage_backend* is ``None``, or if the
            training dataset is empty.
        NotImplementedError: If ``config.lightning.checkpoint_config
            .save_every_n_steps`` is set, if
            ``config.lightning.transfer_learning_spec`` is set, if
            ``config.lightning.experiment_tracker.mlflow`` is set, or if
            ``config.custom`` is used (custom backend not yet in OSS).
    """
    if storage_backend is None:
        raise ConfigurationError(
            "storage_backend is required for train_tabular. "
            "Provide a StorageBackend instance (e.g. MinioStorageBackend)."
        )

    if config.lightning:
        return _train_lightning(
            config.lightning,
            train_dataset,
            validation_dataset,
            storage_backend=storage_backend,
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
    storage_backend: StorageBackend,
    initial_model: ModelArtifact | None,
    run_config: ray.train.RunConfig | None,
    scaling_config: ray.train.ScalingConfig | None,
    is_local_run: bool,
    apply_incremental_training_metadata: ApplyIncrementalTrainingMetadataFn,
) -> ModelArtifact:
    """Lightning + Ray Train implementation of :func:`train_tabular`."""
    import ray.train

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

    # RunConfig: use injected or build default with temp storage
    if run_config is None:
        _tmp_storage = tempfile.mkdtemp(prefix="michelangelo_train_")
        run_config = ray.train.RunConfig(
            checkpoint_config=checkpoint_config,
            storage_path=_tmp_storage,
        )

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

    # Download warm-start weights, keep dir alive for the duration of training,
    # then clean up. Uses ExitStack for conditional cleanup.
    initial_weights_path: str | None = None
    with contextlib.ExitStack() as stack:
        if initial_model is not None:
            _weights_dir = stack.enter_context(
                tempfile.TemporaryDirectory(prefix="michelangelo_weights_")
            )
            storage_backend.download(initial_model.path, _weights_dir)
            initial_weights_path = os.path.join(_weights_dir, "model.pt")
            _logger.info("Downloaded initial weights to: %s", initial_weights_path)

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

    # Rebuild model from Ray checkpoint (weights dir already cleaned up above)
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

    # Serialise and upload model
    with tempfile.TemporaryDirectory(prefix="michelangelo_upload_") as upload_tmp:
        model_path = f"{upload_tmp}/model.pt"
        torch.save(trained_model.state_dict(), model_path)
        key = f"models/{uuid.uuid4().hex}"
        uri = storage_backend.upload(upload_tmp, key)

    return ModelArtifact(path=uri, metadata=metadata)
