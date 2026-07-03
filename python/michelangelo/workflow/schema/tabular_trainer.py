"""Configuration dataclasses for the tabular_trainer workflow task.

These classes are the canonical configuration schema for ``train_tabular()``
and its backends. Mirrors the structure of ``michelangelo.workflow.schema.pusher``
— plain ``@dataclass`` with ``__post_init__`` validation; no Pydantic dependency.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum
from typing import Any, ClassVar

from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.schema.ray_data_io import (
    BatchIterConfig,
    DataloadingConfig,
    ParquetReadConfig,
)

__all__ = [
    "BatchIterConfig",
    "CheckpointConfig",
    "CheckpointScoreOrder",
    "ColumnConfig",
    "CometConfig",
    "CustomTrackerConfig",
    "CustomTrainerConfig",
    "DataloadingConfig",
    "ExperimentTrackerConfig",
    "IncrementalTrainingModeConfig",
    "LightningTrainerConfig",
    "LightningTrainerKwargs",
    "MlflowConfig",
    "ParquetReadConfig",
    "ScalingConfig",
    "TabularTrainerConfig",
    "TrackerConfig",
    "TransferLearningSpecConfig",
]


class CheckpointScoreOrder(str, Enum):
    """Sort order for checkpoint scoring.

    Attributes:
        MAX: Keep the checkpoint with the highest metric value.
        MIN: Keep the checkpoint with the lowest metric value.

    Example:
        >>> CheckpointScoreOrder.MAX.value
        'max'
    """

    MAX = "max"
    MIN = "min"


class IncrementalTrainingModeConfig(str, Enum):
    """Incremental training mode for tabular_trainer.

    Attributes:
        NONE: No incremental training; train from scratch.
        BASELINE: Use the initial model as a warm-start baseline and mark
            the result as incrementally trained.

    Example:
        >>> IncrementalTrainingModeConfig.BASELINE.value
        'BASELINE'
    """

    NONE = "NONE"
    BASELINE = "BASELINE"


@dataclass
class ColumnConfig:
    """Schema descriptor for a single model input, output, or label column.

    Attributes:
        data_type: PyTorch dtype string, e.g. ``"torch.float32"``.
        shape: Tensor shape *excluding* the batch dimension, e.g.
            ``[128]`` for a 128-element embedding. Defaults to ``[]``
            (scalar).

    Example:
        >>> ColumnConfig(data_type="torch.float32", shape=[128])
        ColumnConfig(data_type='torch.float32', shape=[128])
    """

    data_type: str
    shape: list[int] = field(default_factory=list)


@dataclass
class TrackerConfig:
    """Base for all experiment tracker configurations.

    Subclasses that are not yet wired in OSS set ``_oss_supported = False``
    as a class-level default and override :meth:`_check_oss_supported` to
    raise with a specific message; ``__post_init__`` calls it immediately
    at construction time so misconfiguration is caught before training
    starts, not minutes into a run.

    ``_oss_supported`` is a ``ClassVar``, not a dataclass field: these
    configs are serialized through the UniFlow codec (``asdict()`` then
    ``cls(**dct)``), and a field would round-trip into the constructor as
    an unexpected keyword argument.
    """

    _oss_supported: ClassVar[bool] = True

    def __post_init__(self) -> None:
        """Fail fast at construction time for trackers not yet supported in OSS."""
        if not self._oss_supported:
            self._check_oss_supported()

    def _check_oss_supported(self) -> None:
        """Raise ``ConfigurationError``; subclasses override with a specific message."""
        raise ConfigurationError(
            f"{type(self).__name__} is not yet supported in OSS michelangelo."
        )


@dataclass
class CometConfig(TrackerConfig):
    """Comet ML experiment tracking configuration.

    Pass to ``ExperimentTrackerConfig(tracker=CometConfig(...))`` (or the
    legacy ``ExperimentTrackerConfig(comet=CometConfig(...))``) on
    ``LightningTrainerConfig``.

    Attributes:
        api_key: Comet API key.
        workspace: Comet workspace name.
        project_name: Comet project name.
        experiment_name: Comet experiment name.
        tags: Optional tags attached to the Comet experiment.

    Example:
        >>> cfg = CometConfig(
        ...     api_key="key",
        ...     workspace="ws",
        ...     project_name="proj",
        ...     experiment_name="exp",
        ... )
    """

    api_key: str
    workspace: str
    project_name: str
    experiment_name: str
    tags: list[str] = field(default_factory=list)


@dataclass
class MlflowConfig(TrackerConfig):
    """MLflow experiment tracking configuration.

    Pass to ``ExperimentTrackerConfig(mlflow=MlflowConfig(...))`` on
    ``LightningTrainerConfig``.

    Attributes:
        experiment_name: Name of the MLflow experiment. Created automatically
            if it does not exist.
        tracking_uri: MLflow tracking server URI, e.g.
            ``"http://localhost:5000"`` or ``"sqlite:///mlflow.db"``. Falls
            back to the ``MLFLOW_TRACKING_URI`` environment variable when
            ``None``.
        run_name: Optional display name for this training run. Auto-generated
            when ``None``.
        tags: Key-value string tags attached to the MLflow run.

    Raises:
        ConfigurationError: ``MlflowConfig`` itself is not yet wired in OSS
            michelangelo (see GitHub issue #1427). Raised immediately at
            construction time. MLflow is usable today via
            ``CustomTrackerConfig(factory_fn="michelangelo.lib.trainer.torch"
            ".pytorch_lightning._private.util.build_mlflow_logger", ...)``.

    Example:
        Constructing this config always raises ``ConfigurationError`` until
        GitHub issue #1427 is resolved::

            MlflowConfig(experiment_name="tabular-ctr")
            # ConfigurationError: MlflowConfig is not yet wired in OSS
            # michelangelo (see GitHub issue #1427)...

        Use MLflow today via the BYOT escape hatch instead::

            CustomTrackerConfig(
                factory_fn="michelangelo.lib.trainer.torch.pytorch_lightning"
                "._private.util.build_mlflow_logger",
                factory_kwargs={"experiment_name": "tabular-ctr"},
            )
    """

    experiment_name: str
    tracking_uri: str | None = None
    run_name: str | None = None
    tags: dict[str, str] = field(default_factory=dict)
    _oss_supported: ClassVar[bool] = False

    def _check_oss_supported(self) -> None:
        """Raise with a message pointing at the tracking issue and alternatives."""
        raise ConfigurationError(
            "MlflowConfig is not yet wired in OSS michelangelo (see GitHub "
            "issue #1427). MLflow is usable today via CustomTrackerConfig("
            "factory_fn='michelangelo.lib.trainer.torch.pytorch_lightning."
            "_private.util.build_mlflow_logger', ...); this MlflowConfig "
            "convenience wrapper will be enabled once #1427 ships."
        )


@dataclass
class CustomTrackerConfig(TrackerConfig):
    """Bring-your-own experiment tracker via a dotted-path factory function.

    ``factory_fn`` is a dotted import path to a callable that returns a
    ``lightning.pytorch.loggers.Logger`` or ``list[Logger]``. The factory is
    called inside the Ray Train worker with ``**factory_kwargs`` plus an
    optional ``run_id: str | None`` kwarg for distributed correlation.
    Factories that want per-worker run correlation should accept
    ``run_id=None`` as an optional kwarg; factories that don't need it can
    omit the parameter entirely.

    Attributes:
        factory_fn: Dotted import path to the logger factory callable.
        factory_kwargs: Kwargs forwarded to the factory (excluding ``run_id``,
            which is injected automatically when the factory accepts it).

    Example — Weights & Biases:
        >>> # myproject/loggers.py
        >>> def make_wandb_logger(project, run_name, run_id=None):
        ...     from lightning.pytorch.loggers import WandbLogger
        ...     return WandbLogger(project=project, name=run_name, id=run_id)
        >>> CustomTrackerConfig(
        ...     factory_fn="myproject.loggers.make_wandb_logger",
        ...     factory_kwargs={"project": "ctr-model", "run_name": "xgb-v3"},
        ... )
        CustomTrackerConfig(factory_fn='myproject.loggers.make_wandb_logger', ...)
    """

    factory_fn: str
    factory_kwargs: dict[str, Any] = field(default_factory=dict)


@dataclass
class ExperimentTrackerConfig:
    """Experiment tracking backend configuration.

    Two styles accepted, mutually exclusive:

    - **New style** (preferred): ``tracker=CometConfig(...)`` or
      ``tracker=CustomTrackerConfig(...)``.
    - **Legacy style**: ``comet=CometConfig(...)`` or
      ``mlflow=MlflowConfig(...)`` — promoted to ``tracker`` in
      ``__post_init__``. All existing configs continue to work unchanged.

    **Adding a new backend:** define a ``TrackerConfig`` subclass and pass
    it via ``tracker=``. No changes to this class are required — see
    ``CustomTrackerConfig`` for the bring-your-own-tracker escape hatch.

    Attributes:
        tracker: Unified tracker config field (preferred).
        comet: Legacy Comet ML tracker configuration field.
        mlflow: Legacy MLflow tracker configuration field.

    Raises:
        ConfigurationError: If both ``tracker`` and a legacy field are set,
            if more than one legacy field is set, or if the resolved tracker
            type sets ``_oss_supported = False`` (e.g. ``MlflowConfig``,
            which raises when constructed regardless of this class).

    Example — Comet ML:
        >>> ExperimentTrackerConfig(
        ...     tracker=CometConfig(
        ...         api_key="key", workspace="ws",
        ...         project_name="proj", experiment_name="exp",
        ...     )
        ... )

    Example — bring-your-own tracker:
        >>> ExperimentTrackerConfig(
        ...     tracker=CustomTrackerConfig(
        ...         factory_fn="myproject.loggers.make_wandb_logger",
        ...         factory_kwargs={"project": "ctr-model"},
        ...     )
        ... )
    """

    tracker: TrackerConfig | None = None
    comet: CometConfig | None = None
    mlflow: MlflowConfig | None = None

    def __post_init__(self) -> None:
        """Validate exclusivity and promote legacy fields into ``tracker``."""
        legacy = [
            (name, val)
            for name, val in [("comet", self.comet), ("mlflow", self.mlflow)]
            if val is not None
        ]
        if self.tracker is not None and legacy:
            raise ConfigurationError(
                f"Cannot set both 'tracker' and legacy fields "
                f"{[n for n, _ in legacy]}. "
                "Use ExperimentTrackerConfig(tracker=...) alone."
            )
        if len(legacy) > 1:
            raise ConfigurationError(
                f"At most one tracker allowed, got: {[n for n, _ in legacy]}. "
                "Use ExperimentTrackerConfig(tracker=CometConfig(...)) "
                "(preferred) or a single legacy field."
            )
        if self.comet is not None:
            self.tracker = self.comet
        elif self.mlflow is not None:
            self.tracker = self.mlflow


@dataclass
class ScalingConfig:
    """Ray Train scaling configuration for distributed training.

    Attributes:
        cpu_per_worker: Number of CPU cores allocated per Ray Train worker.
            Defaults to ``1``.

    Example:
        >>> ScalingConfig(cpu_per_worker=4)
        ScalingConfig(cpu_per_worker=4)
    """

    cpu_per_worker: int = 1


@dataclass
class CheckpointConfig:
    """Checkpoint retention and scoring configuration.

    Mirrors ``ray.train.CheckpointConfig`` field-for-field with two additions
    for future mid-epoch checkpointing (``save_every_n_steps``,
    ``random_seed``). Both additions are gated in the dispatcher — setting
    ``save_every_n_steps`` raises ``NotImplementedError`` at runtime until
    the mid-epoch checkpoint planner is ported.

    Attributes:
        num_to_keep: Maximum number of checkpoints to retain. ``None`` keeps
            all checkpoints; ``1`` (default) keeps only the best.
        checkpoint_score_attribute: Metric key used to rank checkpoints.
            ``None`` uses the last reported checkpoint.
        checkpoint_score_order: Whether to maximise or minimise the score
            attribute. Defaults to ``MAX``.
        save_every_n_steps: Enable mid-epoch checkpointing every N global
            steps. Must be ``>= 1`` when set. Currently gated — raises
            ``NotImplementedError`` in the dispatcher.
        random_seed: Optional shuffle seed for chunk-based mid-epoch reads.
            Currently gated with ``save_every_n_steps``.

    Raises:
        ConfigurationError: If ``save_every_n_steps`` is set to a value
            less than 1. Set it to ``None`` (the default) to disable
            mid-epoch checkpointing.

    Example:
        >>> CheckpointConfig(num_to_keep=3, checkpoint_score_attribute="val_loss",
        ...                   checkpoint_score_order=CheckpointScoreOrder.MIN)
        CheckpointConfig(num_to_keep=3, ...)
    """

    num_to_keep: int = 1
    checkpoint_score_attribute: str | None = None
    checkpoint_score_order: CheckpointScoreOrder = CheckpointScoreOrder.MAX
    save_every_n_steps: int | None = None
    random_seed: int | None = None

    def __post_init__(self) -> None:
        """Validate save_every_n_steps is a positive integer when set."""
        if self.save_every_n_steps is not None and self.save_every_n_steps < 1:
            raise ConfigurationError(
                f"save_every_n_steps must be >= 1, got {self.save_every_n_steps}."
                " Set it to None to disable mid-epoch checkpointing."
            )


@dataclass
class LightningTrainerKwargs:
    """Passthrough kwargs for ``lightning.pytorch.Trainer.__init__``.

    All fields are optional and default to ``None`` (or the Lightning
    default). Fields forwarded verbatim to the Trainer constructor after
    any ``_count`` → ``limit_*_batches`` merge (int counts take precedence).

    The ``limit_*_batches`` / ``limit_*_batches_count`` pairs are mutually
    exclusive: setting both raises ``ConfigurationError``.

    Attributes:
        strategy: Distributed strategy name, e.g. ``"ddp"``, ``"fsdp"``,
            ``"deepspeed"``.
        strategy_kwargs: Extra kwargs forwarded to the strategy constructor.
        precision: Training precision, e.g. ``"32"``, ``"bf16-mixed"``,
            ``"16-mixed"``.
        logger: Dotted import path to a ``Logger`` class or factory.
        logger_kwargs: kwargs forwarded to the logger constructor.
        callbacks: Dotted import path to a ``Callback`` class or factory
            returning a list of callbacks.
        callback_kwargs: kwargs forwarded to the callback constructor.
        fast_dev_run: Run N batches for fast debugging. ``0`` disables.
        max_epochs: Maximum number of training epochs.
        min_epochs: Minimum number of training epochs.
        max_steps: Maximum number of global training steps. ``-1`` is
            unlimited.
        min_steps: Minimum number of global training steps.
        max_time: Maximum wall-clock time as ``"DD:HH:MM:SS"`` string.
        limit_train_batches: Fraction of training batches to use each epoch.
        limit_train_batches_count: Exact number of training batches per epoch.
        limit_val_batches: Fraction of validation batches.
        limit_val_batches_count: Exact number of validation batches.
        limit_test_batches: Fraction of test batches.
        limit_test_batches_count: Exact number of test batches.
        limit_predict_batches: Fraction of prediction batches.
        limit_predict_batches_count: Exact number of prediction batches.
        overfit_batches: Fraction or count of batches to overfit on.
        val_check_interval: Validation check frequency (fraction or steps).
        check_val_every_n_epoch: Run validation every N epochs.
        num_sanity_val_steps: Number of sanity-check validation steps.
        log_every_n_steps: Logging frequency in global steps.
        enable_progress_bar: Show or hide the training progress bar.
        enable_model_summary: Show or hide the model summary at startup.
        accumulate_grad_batches: Gradient accumulation steps.
        gradient_clip_val: Max gradient norm for clipping.
        gradient_clip_algorithm: Clipping algorithm, e.g. ``"norm"``.
        deterministic: Force deterministic CUDA ops (``"True"`` / ``"False"``
            / ``"warn"``).
        benchmark: Enable cuDNN auto-tuner.
        inference_mode: Use ``torch.inference_mode`` during validation.
        use_distributed_sampler: Wrap the sampler for distributed training.
        detect_anomaly: Enable autograd anomaly detection.
        barebones: Disable all non-essential Lightning features.
        plugins: Dotted import path to a plugin class or factory.
        plugins_kwargs: kwargs forwarded to the plugins constructor.
        sync_batchnorm: Synchronise batch normalisation across devices.
        reload_dataloaders_every_n_epochs: Re-create dataloaders every N
            epochs.
        default_root_dir: Default directory for logs and checkpoints.

    Raises:
        ConfigurationError: If both a ``limit_*_batches`` float field and
            its corresponding ``limit_*_batches_count`` int field are set.

    Example:
        >>> LightningTrainerKwargs(max_epochs=10, precision="bf16-mixed")
        LightningTrainerKwargs(max_epochs=10, precision='bf16-mixed', ...)
    """

    strategy: str | None = None
    strategy_kwargs: dict | None = None
    precision: str | None = None
    logger: str | None = None
    logger_kwargs: dict | None = None
    callbacks: str | None = None
    callback_kwargs: dict | None = None
    fast_dev_run: int = 0
    max_epochs: int | None = None
    min_epochs: int | None = None
    max_steps: int = -1
    min_steps: int | None = None
    max_time: str | None = None
    limit_train_batches: float | None = None
    limit_train_batches_count: int | None = None
    limit_val_batches: float | None = None
    limit_val_batches_count: int | None = None
    limit_test_batches: float | None = None
    limit_test_batches_count: int | None = None
    limit_predict_batches: float | None = None
    limit_predict_batches_count: int | None = None
    overfit_batches: float = 0.0
    val_check_interval: float | None = None
    check_val_every_n_epoch: int | None = 1
    num_sanity_val_steps: int | None = None
    log_every_n_steps: int | None = None
    enable_progress_bar: bool | None = None
    enable_model_summary: bool | None = None
    accumulate_grad_batches: int = 1
    gradient_clip_val: float | None = None
    gradient_clip_algorithm: str | None = None
    deterministic: str | None = None
    benchmark: bool | None = None
    inference_mode: bool = True
    use_distributed_sampler: bool = True
    detect_anomaly: bool = False
    barebones: bool = False
    plugins: str | None = None
    plugins_kwargs: dict | None = None
    sync_batchnorm: bool = False
    reload_dataloaders_every_n_epochs: int = 0
    default_root_dir: str | None = None
    # Note: Lightning's ``profiler`` kwarg is intentionally omitted here.
    # The profiler subsystem will be re-introduced in PR 5 with a pluggable
    # upload sink via TrainingObserver. Use ``profiler=None`` (the Lightning
    # default) until then.

    def __post_init__(self) -> None:
        """Validate mutually exclusive limit_*_batches pairs."""
        pairs = [
            ("limit_train_batches", "limit_train_batches_count"),
            ("limit_val_batches", "limit_val_batches_count"),
            ("limit_test_batches", "limit_test_batches_count"),
            ("limit_predict_batches", "limit_predict_batches_count"),
        ]
        for float_field, count_field in pairs:
            if (
                getattr(self, float_field) is not None
                and getattr(self, count_field) is not None
            ):
                raise ConfigurationError(
                    f"Cannot set both '{float_field}' and"
                    f" '{count_field}' simultaneously."
                    f" Use '{float_field}' (float fraction) or"
                    f" '{count_field}' (int count), not both."
                )


@dataclass
class TransferLearningSpecConfig:
    """Placeholder config for transfer-learning spec building.

    Setting this field on ``LightningTrainerConfig`` raises
    ``NotImplementedError`` in the dispatcher until the spec builder is
    ported from the internal SDK.

    Attributes:
        transfer_learning_spec: Opaque dict forwarded to the internal
            spec builder. Shape is intentionally untyped until the full
            builder is available in OSS.

    Example:
        >>> TransferLearningSpecConfig(transfer_learning_spec={"layers": 3})
        TransferLearningSpecConfig(transfer_learning_spec={'layers': 3})
    """

    transfer_learning_spec: dict | None = None


@dataclass
class CustomTrainerConfig:
    """Configuration for a custom (non-Lightning) trainer backend.

    Using this config raises ``NotImplementedError`` in the dispatcher until
    the custom Ray trainer subsystem is ported from the internal SDK.

    Attributes:
        train_class: Dotted import path to the custom trainer class.
        train_constructor_kwargs: Optional kwargs forwarded to the trainer
            constructor.

    Example:
        >>> CustomTrainerConfig(train_class="myproject.trainers.MyTrainer")
        CustomTrainerConfig(train_class='myproject.trainers.MyTrainer', ...)
    """

    train_class: str
    train_constructor_kwargs: dict | None = None


@dataclass
class LightningTrainerConfig:
    """Configuration for the PyTorch Lightning training backend.

    All column maps (``input_columns``, ``output_columns``, ``labels``) use
    column name as key and ``ColumnConfig`` as value.
    ``metadata_columns`` names columns read from Parquet for logging and
    callbacks but excluded from the model schema.

    Attributes:
        model_class: Dotted import path to a ``LightningModule`` subclass.
        input_columns: Feature columns fed to the model.
        output_columns: Model output columns included in the model schema.
        labels: Target/label columns.
        metadata_columns: Columns read for logging; excluded from schema.
        checkpoint_config: Checkpoint retention settings.
        model_kwargs: Extra kwargs forwarded to ``model_class.__init__``.
        dataloading_config: Parquet read and batch iteration settings.
        scaling_config: Ray Train worker resource allocation.
        lightning_trainer_kwargs: Passthrough kwargs for
            ``lightning.pytorch.Trainer``.
        hyperparameters: Catch-all dict for kwargs not covered by the
            structured fields. Highly discouraged; prefer structured fields.
        experiment_tracker: Experiment tracking backend (Comet ML, or a
            custom tracker via ``CustomTrackerConfig``). ``None`` disables
            experiment tracking. MLflow is not yet wired in OSS — see
            ``MlflowConfig``.
        transfer_learning_spec: Transfer-learning spec config. Setting this
            raises ``NotImplementedError`` until the spec builder is ported.
        incremental_training_mode: Incremental training mode config.

    Example:
        >>> from michelangelo.workflow.schema.tabular_trainer import (
        ...     LightningTrainerConfig, ColumnConfig
        ... )
        >>> cfg = LightningTrainerConfig(
        ...     model_class="myproject.models.TabularNet",
        ...     input_columns={"age": ColumnConfig("torch.float32")},
        ...     output_columns={"score": ColumnConfig("torch.float32")},
        ...     labels={"clicked": ColumnConfig("torch.long")},
        ...     metadata_columns=["user_id"],
        ... )
    """

    model_class: str
    input_columns: dict[str, ColumnConfig]
    output_columns: dict[str, ColumnConfig]
    labels: dict[str, ColumnConfig]
    metadata_columns: list[str]
    checkpoint_config: CheckpointConfig = field(default_factory=CheckpointConfig)
    model_kwargs: dict | None = None
    dataloading_config: DataloadingConfig | None = None
    scaling_config: ScalingConfig | None = None
    lightning_trainer_kwargs: LightningTrainerKwargs | None = None
    hyperparameters: dict | None = None
    experiment_tracker: ExperimentTrackerConfig | None = None
    transfer_learning_spec: TransferLearningSpecConfig | None = None
    incremental_training_mode: IncrementalTrainingModeConfig | None = None


@dataclass
class TabularTrainerConfig:
    """Top-level config for ``train_tabular()``.

    Exactly one of ``lightning`` or ``custom`` must be set.

    Attributes:
        lightning: Config for the PyTorch Lightning backend.
        custom: Config for a custom trainer backend (raises
            ``NotImplementedError`` until ported).

    Raises:
        ConfigurationError: If neither or both backends are set. Set exactly
            one of ``lightning`` or ``custom``.

    Example:
        Minimal config — model class, columns, no optional tuning:

        >>> cfg = TabularTrainerConfig(
        ...     lightning=LightningTrainerConfig(
        ...         model_class="myproject.models.TabularNet",
        ...         input_columns={"age": ColumnConfig("torch.float32"),
        ...                        "income": ColumnConfig("torch.float32")},
        ...         output_columns={"score": ColumnConfig("torch.float32")},
        ...         labels={"clicked": ColumnConfig("torch.long")},
        ...         metadata_columns=["user_id"],
        ...     )
        ... )

        Realistic config — batch size, epochs, checkpointing by val loss:

        >>> cfg = TabularTrainerConfig(
        ...     lightning=LightningTrainerConfig(
        ...         model_class="myproject.models.TabularNet",
        ...         input_columns={"age": ColumnConfig("torch.float32"),
        ...                        "income": ColumnConfig("torch.float32")},
        ...         output_columns={"score": ColumnConfig("torch.float32")},
        ...         labels={"clicked": ColumnConfig("torch.long")},
        ...         metadata_columns=["user_id"],
        ...         dataloading_config=DataloadingConfig(
        ...             batch_iter_config=BatchIterConfig(
        ...                 batch_size=256,
        ...                 num_shuffle_batches=4,
        ...             ),
        ...         ),
        ...         checkpoint_config=CheckpointConfig(
        ...             num_to_keep=3,
        ...             checkpoint_score_attribute="val_loss",
        ...             checkpoint_score_order=CheckpointScoreOrder.MIN,
        ...         ),
        ...         lightning_trainer_kwargs=LightningTrainerKwargs(
        ...             max_epochs=20,
        ...             precision="bf16-mixed",
        ...         ),
        ...         scaling_config=ScalingConfig(cpu_per_worker=4),
        ...         experiment_tracker=ExperimentTrackerConfig(
        ...             tracker=CometConfig(
        ...                 api_key="key", workspace="ws",
        ...                 project_name="tabular-ctr", experiment_name="xgb-v3",
        ...             ),
        ...         ),
        ...     )
        ... )
    """

    lightning: LightningTrainerConfig | None = None
    custom: CustomTrainerConfig | None = None

    def __post_init__(self) -> None:
        """Validate that exactly one backend is configured."""
        if (self.lightning is None) == (self.custom is None):
            raise ConfigurationError(
                "Exactly one of 'lightning' or 'custom' must be set on "
                "TabularTrainerConfig, got "
                f"lightning={'set' if self.lightning else 'None'}, "
                f"custom={'set' if self.custom else 'None'}. "
                "Set TabularTrainerConfig(lightning=LightningTrainerConfig(...)) "
                "for the Lightning backend or "
                "TabularTrainerConfig(custom=CustomTrainerConfig(...)) "
                "for a custom backend."
            )
