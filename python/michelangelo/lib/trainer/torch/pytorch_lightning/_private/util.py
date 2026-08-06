"""Internal helpers for the PyTorch Lightning trainer.

This module hosts the per-worker training loop and the strategy / plugin /
logger / callback resolution helpers. Public APIs live in
``michelangelo.lib.trainer.torch.pytorch_lightning.lightning_trainer``.
"""

from __future__ import annotations

import glob
import hashlib
import inspect
import logging
import math
import os
import re
from tempfile import TemporaryDirectory
from typing import TYPE_CHECKING, Any, Union

import pytorch_lightning as pl
import ray
import torch
from fsspec.core import url_to_fs
from pytorch_lightning.callbacks import Callback, ModelCheckpoint
from pytorch_lightning.loggers import CometLogger, Logger
from pytorch_lightning.plugins import (
    CheckpointIO,
    ClusterEnvironment,
    LayerSync,
    Precision,
)
from pytorch_lightning.strategies import Strategy
from ray.train.lightning import (
    RayDDPStrategy,
    RayDeepSpeedStrategy,
    RayFSDPStrategy,
    RayLightningEnvironment,
)

from michelangelo._internal.utils.reflection_utils import get_module_attr
from michelangelo.lib._internal.errors import UserInputError
from michelangelo.lib.trainer.torch.pytorch_lightning._private.callbacks import (
    RayTrainReportCallback,
    RayTrainReportPerNodeCallback,
)

if TYPE_CHECKING:
    from michelangelo.lib.trainer.torch.pytorch_lightning.schema import (
        TrainingObserver,
    )


# Plugin types accepted by the PyTorch Lightning Trainer.
# See: https://github.com/Lightning-AI/pytorch-lightning/blob/2129fdf3622e39ba46be4e1139af408e7e951cf3/src/lightning/pytorch/trainer/trainer.py#L126
_PLUGIN_INPUT = Union[Precision, ClusterEnvironment, CheckpointIO, LayerSync]

CALLBACK_REPORT_PER_NODE = "callback_report_per_node"
CHECKPOINT_FILENAME = "checkpoint.ckpt"

# Upper bounds on the ``torch.profiler`` schedule. Both windows multiply the
# volume of trace data written per worker, and traces are large; these caps
# keep a mis-typed config from filling the run's storage.
_MAX_PROFILER_ACTIVE_STEPS = 10
_MAX_PROFILER_REPEAT = 3

# Shorthand strings accepted in place of a full profiler config dict.
#
# Only ``pytorch`` is supported today — Michelangelo OSS is PyTorch-heavy, and
# the other Lightning profilers (simple / advanced / xla) and a custom-class
# escape hatch add config surface without a concrete user yet. The dict shape
# (``{"pytorch": {...}, "upload_profiler_results": ...}``) is kept as-is so adding a
# second backend later, as we expand to more platforms, is additive rather
# than a breaking config change.
_PROFILER_SHORTHANDS = ("pytorch",)

# Directory (under the worker's cwd) that every profiler writes its output to.
# Fixed rather than temporary so a profiler sink can find the results after
# ``trainer.fit()`` returns.
_PROFILER_LOGS_DIRNAME = "profiler_logs"

_logger = logging.getLogger(__name__)


def _load_weights_from_path(model: torch.nn.Module, path: str) -> None:
    """Download a state-dict file and load it into the model.

    Fetches from any storage scheme supported by ``fsspec`` (local, ``s3://``,
    ``gs://``, ...) and loads it into ``model`` with ``strict=True``.
    """
    fs, fs_path = url_to_fs(path)
    with TemporaryDirectory() as tmp_dir:
        local_path = os.path.join(tmp_dir, "init_weights.pt")
        fs.get(fs_path, local_path)
        # Load to CPU first; DDP/DeepSpeed will move tensors to the correct GPU during broadcast.
        state_dict = torch.load(local_path, map_location="cpu", weights_only=True)
        # strict=True is intentional: initial_weights_path is expected to point to a complete
        # state dict produced upstream for the same model architecture.
        model.load_state_dict(state_dict, strict=True)


def _print_layer_weights(model: torch.nn.Module, limit: int = 50) -> None:
    """Log a summary of each parameter tensor's name, shape, and first ``limit`` chars of weights."""
    _logger.debug("=== Layer weights summary ===")
    for name, param in model.named_parameters():
        weights_str = str(param.data)[:limit]
        _logger.debug(
            "  %s / shape=%s / weights=%s", name, list(param.shape), weights_str
        )
    _logger.debug("============================")


def _apply_layer_freeze(model: torch.nn.Module, transfer_learning_spec: dict) -> None:
    """Re-apply layer freezing from ``transfer_learning_spec`` after loading a state dict.

    ``state_dict`` does not preserve ``requires_grad``, so freezing applied upstream must
    be re-applied in each worker.

    Matching logic:
    - ``layer_names``: substring match (``pattern in layer_name``)
    - ``layer_names_regex``: ``re.search`` (matches anywhere in the string)
    """
    _logger.info(
        "Applying layer freeze based on transfer_learning_spec: %s",
        transfer_learning_spec,
    )
    names_to_freeze = transfer_learning_spec.get("layer_names_to_freeze") or []
    regex_to_freeze = transfer_learning_spec.get("layer_names_to_freeze_regex") or []

    # state_dict().keys() is used intentionally as a superset to show the full model state
    # (parameters + buffers) in debug output. Buffers (e.g., bn.running_mean) may appear
    # in layers_to_freeze but are correctly skipped in the named_parameters() loop below,
    # since buffers have no requires_grad. Actual parameters are always frozen correctly.
    model_layer_names = list(model.state_dict().keys())
    _logger.debug(
        "[freeze] Model layer names (%d): %r", len(model_layer_names), model_layer_names
    )

    layers_to_freeze = set()
    for available_name in model_layer_names:
        for pattern in names_to_freeze:
            if pattern in available_name:
                layers_to_freeze.add(available_name)
        for pattern in regex_to_freeze:
            if re.search(pattern, available_name):
                layers_to_freeze.add(available_name)

    _logger.info(
        "[freeze] Layers to freeze (%d): %r", len(layers_to_freeze), layers_to_freeze
    )

    frozen_count = 0
    for name, param in model.named_parameters():
        if name in layers_to_freeze:
            _logger.info("[freeze] Freezing layer: %r", name)
            param.requires_grad = False
            frozen_count += 1

    rank = ray.train.get_context().get_world_rank()
    _logger.info(
        "[freeze] [Rank %d] Layer freeze re-applied: %d params frozen",
        rank,
        frozen_count,
    )


def _get_comet_logger(
    run_id: str,
    api_key: str,
    workspace: str,
    project_name: str,
    experiment_name: str,
    tags: list[str] | None = None,
) -> CometLogger:
    """Create and return a CometLogger configured for distributed Ray training.

    On rank 0, creates the Comet experiment if it does not already exist, then
    waits for all ranks via a barrier before each worker attaches its own logger
    instance to the shared experiment key derived from ``run_id``.

    ``comet_ml`` is imported lazily so the trainer can be imported without it
    installed; only callers that actually build a Comet logger need it.
    """
    import comet_ml

    experiment_id = hashlib.sha1(run_id.encode("utf-8")).hexdigest()
    os.environ["COMET_EXPERIMENT_KEY"] = experiment_id
    api = comet_ml.API(api_key=api_key)

    # Create experiment only once on rank 0
    if ray.train.get_context().get_world_rank() == 0:
        api_experiment = api.get_experiment_by_key(experiment_id)
        if api_experiment is None:
            # Create an experiment object
            comet_ml.Experiment(
                api_key=api_key, project_name=project_name, workspace=workspace
            )

    torch.distributed.barrier()
    # Attach logger with existing experiment_id
    comet_logger = CometLogger(
        api_key=api_key,
        workspace=workspace,
        project_name=project_name,
        experiment_name=experiment_name,
        experiment_key=experiment_id,
        log_env_details=True,
        log_env_gpu=True,
        log_env_cpu=True,
        log_env_network=True,
    )
    if tags:
        comet_logger.experiment.add_tags(tags)

    # Log cometML URL by head node
    if ray.train.get_context().get_world_rank() == 0:
        _logger.info("Comet experiment URL: %s", comet_logger.experiment.url)
    return comet_logger


def build_comet_logger(
    api_key: str,
    workspace: str,
    project_name: str,
    experiment_name: str,
    tags: list[str] | None = None,
    run_id: str | None = None,
) -> CometLogger:
    """Build a ``CometLogger`` inside a Ray Train worker.

    Dotted-path factory target for ``LightningTrainerKwargs.logger``,
    resolved by :func:`_resolve_logger`. ``run_id`` is injected automatically
    when set on the driver, providing cross-worker experiment correlation.

    Args:
        api_key: Comet API key.
        workspace: Comet workspace name.
        project_name: Comet project name.
        experiment_name: Comet experiment name.
        tags: Optional tags attached to the Comet experiment.
        run_id: Cross-worker correlation id; required for the barrier-based
            rank-0 experiment creation in :func:`_get_comet_logger`.

    Returns:
        A ``CometLogger`` attached to the shared experiment for this run.
    """
    return _get_comet_logger(
        run_id=run_id or "",
        api_key=api_key,
        workspace=workspace,
        project_name=project_name,
        experiment_name=experiment_name,
        tags=tags,
    )


def build_mlflow_logger(
    experiment_name: str,
    tracking_uri: str | None = None,
    run_name: str | None = None,
    tags: dict[str, str] | None = None,
    run_id: str | None = None,
) -> Logger:
    """Build an ``MLFlowLogger`` for a Ray Train worker.

    Dotted-path factory target for ``LightningTrainerKwargs.logger``.
    Unlike Comet, constructing an ``MLFlowLogger`` is process-safe and
    requires no distributed barrier — each worker builds its own logger
    independently. This does not extend to the ``.experiment`` property,
    which Lightning decorates with the same rank-zero restriction as Comet's;
    see :func:`mlflow_profiler_sink` for the implication on profiler export.
    ``run_id`` lets all workers attach to the same MLflow run for per-rank
    metric correlation; when ``None``, Lightning's default (rank-0-only
    logging) applies.

    Args:
        experiment_name: Name of the MLflow experiment. Created
            automatically if it does not exist.
        tracking_uri: MLflow tracking server URI. Falls back to the
            ``MLFLOW_TRACKING_URI`` environment variable when ``None``.
        run_name: Optional display name for this training run.
        tags: Key-value string tags attached to the MLflow run.
        run_id: Cross-worker correlation id for shared-run logging.

    Returns:
        An ``MLFlowLogger`` instance.
    """
    from pytorch_lightning.loggers import MLFlowLogger

    return MLFlowLogger(
        experiment_name=experiment_name,
        tracking_uri=tracking_uri,
        run_name=run_name,
        tags=tags or {},
        run_id=run_id,
    )


def _resolve_strategy(
    strategy: str | Strategy | None = None,
    strategy_kwargs: dict[str, Any] | None = None,
) -> Strategy:
    """Factory to create the correct Ray/Lightning strategy based on strategy name or instance."""
    if strategy is not None and not isinstance(strategy, (str, Strategy)):
        raise TypeError(
            f"strategy must be a str, Strategy instance, or None, got {type(strategy)!r}"
        )
    if strategy_kwargs is not None and not isinstance(strategy_kwargs, dict):
        raise TypeError(
            f"strategy_kwargs must be a dict or None, got {type(strategy_kwargs)!r}"
        )

    if isinstance(strategy, Strategy):
        return strategy

    strategy_kwargs = strategy_kwargs or {}

    if strategy is None or strategy.lower() == "ddp":
        return RayDDPStrategy(**strategy_kwargs)
    elif strategy.lower() == "deepspeed":
        return RayDeepSpeedStrategy(**strategy_kwargs)
    elif strategy.lower() == "fsdp":
        return RayFSDPStrategy(**strategy_kwargs)
    else:
        raise ValueError(
            f"Unsupported strategy: {strategy!r}; expected 'ddp', 'deepspeed', 'fsdp', or None"
        )


def _resolve_plugins(
    plugins: str | list | _PLUGIN_INPUT | None = None,
    plugins_kwargs: dict[str, Any] | None = None,
) -> list:
    """Resolve plugins for the Lightning Trainer, always ensuring RayLightningEnvironment is present."""
    if plugins is not None and not isinstance(
        plugins, (str, list, tuple, *_PLUGIN_INPUT.__args__)
    ):
        raise TypeError(
            f"plugins must be a str import path, a plugin instance, a list of plugin instances, or None; got {type(plugins)!r}"
        )
    if plugins_kwargs is not None and not isinstance(plugins_kwargs, dict):
        raise TypeError(
            f"plugins_kwargs must be a dict or None, got {type(plugins_kwargs)!r}"
        )
    if plugins_kwargs is not None and not isinstance(plugins, str):
        raise TypeError(
            "plugins_kwargs can only be used when plugins is a str import path"
        )

    plugin_kwargs = plugins_kwargs or {}

    if plugins is None:
        result = []
    elif isinstance(plugins, str):
        # Create the plugin instances from the provided plugins function
        plugins_fn = get_module_attr(plugins)
        plugin_instances = plugins_fn(**plugin_kwargs)
        result = (
            list(plugin_instances)
            if isinstance(plugin_instances, (list, tuple))
            else [plugin_instances]
        )
    elif isinstance(plugins, (list, tuple)):
        result = list(plugins)
    else:
        result = [plugins]

    invalid = [p for p in result if not isinstance(p, _PLUGIN_INPUT.__args__)]
    if invalid:
        raise TypeError(
            f"All plugins must be instances of {[t.__name__ for t in _PLUGIN_INPUT.__args__]}; got invalid types: {[type(p).__name__ for p in invalid]}"
        )

    # We always need to use the RayLightningEnvironment plugin for lightning training with Ray Train
    if not any(isinstance(p, RayLightningEnvironment) for p in result):
        result.append(RayLightningEnvironment())

    return result


def _resolve_logger(
    logger: str | bool | Logger | list[Logger] | None = None,
    logger_kwargs: dict[str, Any] | None = None,
    run_id: str | None = None,
) -> bool | Logger | list[Logger] | None:
    """Resolve the logger for the Lightning Trainer.

    When *logger* is a dotted import path, *run_id* is injected into the
    kwargs passed to the factory (unless already present in *logger_kwargs*)
    so factories can opt into cross-worker run correlation by declaring a
    ``run_id: str | None = None`` parameter — see
    ``build_comet_logger``/``build_mlflow_logger``. The factory's signature
    is inspected first, so factories that genuinely don't accept ``run_id``
    (and don't take ``**kwargs``) are called without it instead of raising
    ``TypeError`` for an unexpected keyword argument.
    """
    if logger_kwargs is not None and not isinstance(logger_kwargs, dict):
        raise TypeError(
            f"logger_kwargs must be a dict or None, got {type(logger_kwargs)!r}"
        )
    if logger_kwargs is not None and not isinstance(logger, str):
        raise TypeError(
            "logger_kwargs can only be used when logger is a str import path"
        )

    if isinstance(logger, bool):
        return logger
    if isinstance(logger, Logger):
        return logger
    if isinstance(logger, (list, tuple)):
        if any(not isinstance(elem, Logger) for elem in logger):
            raise TypeError(
                f"All elements of logger list must be Logger instances, got {logger!r}"
            )
        return list(logger)
    if isinstance(logger, str):
        logger_fn = get_module_attr(logger)
        kwargs = dict(logger_kwargs or {})
        if run_id is not None and _accepts_run_id(logger_fn):
            kwargs.setdefault("run_id", run_id)
        result = logger_fn(**kwargs)
        return list(result) if isinstance(result, (list, tuple)) else result
    if logger is not None:
        raise TypeError(
            f"logger must be a str, bool, Logger instance, list of Logger instances, or None, got {type(logger)!r}"
        )
    return None


def _accepts_run_id(fn: Any) -> bool:
    """Return True if calling *fn* with a ``run_id=`` kwarg would not raise.

    True when *fn* declares a ``run_id`` parameter, or accepts ``**kwargs``.
    Used to make ``run_id`` injection in :func:`_resolve_logger` opt-in for
    custom tracker factories that don't need cross-worker correlation.
    """
    try:
        params = inspect.signature(fn).parameters
    except (TypeError, ValueError):
        # Signature introspection can fail for builtins/some C extensions;
        # fail open and let the injected run_id surface any real mismatch.
        return True
    return "run_id" in params or any(
        p.kind is inspect.Parameter.VAR_KEYWORD for p in params.values()
    )


def _resolve_callbacks(
    callbacks: str | Callback | list[Callback] | None = None,
    callback_kwargs: dict[str, Any] | None = None,
    per_node_callback_kwargs: dict[str, Any] | None = None,
    strategy: Strategy | None = None,
    training_observer: TrainingObserver | None = None,
) -> tuple[list[Callback], bool]:
    """Build callback list for the Lightning Trainer.

    A RayTrainReportCallback or RayTrainReportPerNodeCallback is always appended to the list.
    """
    if callbacks is not None and not isinstance(
        callbacks, (str, Callback, list, tuple)
    ):
        raise TypeError(
            f"callbacks must be a str import path, a Callback instance, a list of Callback instances, or None; got {type(callbacks)!r}"
        )
    if callback_kwargs is not None and not isinstance(callback_kwargs, dict):
        raise TypeError(
            f"callback_kwargs must be a dict or None, got {type(callback_kwargs)!r}"
        )
    if per_node_callback_kwargs is not None and not isinstance(
        per_node_callback_kwargs, dict
    ):
        raise TypeError(
            f"per_node_callback_kwargs must be a dict or None, got {type(per_node_callback_kwargs)!r}"
        )

    callback_kwargs = callback_kwargs or {}
    resolved_callbacks: list[Callback] = []

    if isinstance(callbacks, str):
        # Import the callable and invoke it — may be a Callback class or a factory returning one or more.
        fn = get_module_attr(callbacks)
        result = fn(**callback_kwargs)
        if isinstance(result, (list, tuple)):
            for obj in result:
                if not isinstance(obj, Callback):
                    raise TypeError(
                        f"Expected Callback instances from {callbacks!r}, got {type(obj)!r}"
                    )
                resolved_callbacks.append(obj)
        elif isinstance(result, Callback):
            resolved_callbacks.append(result)
        else:
            raise TypeError(
                f"Expected a Callback instance or list of Callback instances from {callbacks!r}, got {type(result)!r}"
            )
    elif isinstance(callbacks, (list, tuple)):
        for obj in callbacks:
            if not isinstance(obj, Callback):
                raise TypeError(
                    f"All callbacks must be Callback instances, got {type(obj)!r}"
                )
            resolved_callbacks.append(obj)
    elif callbacks is not None:
        resolved_callbacks.append(callbacks)

    has_model_checkpoint = any(
        isinstance(c, ModelCheckpoint) for c in resolved_callbacks
    )

    # Always append a callback that calls ray.train.report() to report metrics and checkpoint.
    # Per-node reporting is required for model-parallel strategies (DeepSpeed ZeRO, FSDP) because
    # each node holds shards of the model and must upload its own checkpoint shard.
    _use_per_node = per_node_callback_kwargs is not None or isinstance(
        strategy, (RayDeepSpeedStrategy, RayFSDPStrategy)
    )
    if _use_per_node:
        per_node_callback_kwargs = per_node_callback_kwargs or {}
        resolved_callbacks.append(
            RayTrainReportPerNodeCallback(
                **per_node_callback_kwargs, training_observer=training_observer
            )
        )
    else:
        resolved_callbacks.append(
            RayTrainReportCallback(training_observer=training_observer)
        )

    return resolved_callbacks, has_model_checkpoint


def _compute_steps_per_epoch(
    trainer_kwargs: dict,
    dataset_num_rows: int | None,
    batch_size: int,
    world_size: int = 1,
) -> int | None:
    """Estimate the number of training steps per epoch, per worker.

    The estimate starts at ``ceil(dataset_num_rows / world_size / batch_size)``
    and then applies Lightning's batch-limiting arguments in the same order
    Lightning itself does:

    1. ``limit_train_batches`` — a float in ``[0.0, 1.0]`` scales the epoch
       proportionally; an int ``>= 1`` caps it at that many batches.
    2. ``max_steps`` — when set, training stops there regardless of epoch
       length, so the estimate is capped at ``max_steps *
       accumulate_grad_batches`` (``max_steps`` counts optimizer steps, the
       estimate counts mini-batches).

    The division by ``world_size`` assumes each worker iterates roughly
    ``1 / world_size`` of the rows. That holds for every configuration this
    trainer supports: the split is performed by Ray Train's default
    ``DataConfig`` (``datasets_to_split="all"``), which shards the dataset
    evenly across workers before the Lightning strategy sees it, so it is
    unaffected by the choice of DDP / FSDP / DeepSpeed. A caller supplying a
    custom ``Strategy`` under which several workers form one model replica and
    must therefore see identical batches would break the assumption, and the
    estimate would then be low by the replication factor.

    Args:
        trainer_kwargs: Keyword arguments destined for ``pl.Trainer``. Read
            only; ``limit_train_batches``, ``max_steps``, and
            ``accumulate_grad_batches`` are consulted if present.
        dataset_num_rows: Total rows in the (unsharded) training dataset, or
            ``None`` when the row count could not be determined.
        batch_size: Per-worker batch size.
        world_size: Number of training workers the dataset is sharded across.

    Returns:
        The estimated number of steps in one epoch on one worker, or ``None``
        when ``dataset_num_rows`` is ``None``.

    Raises:
        ValueError: If ``batch_size`` or ``world_size`` is not positive, or if
            ``limit_train_batches`` / ``accumulate_grad_batches`` hold values
            Lightning would not accept.
    """
    if dataset_num_rows is None:
        return None
    if batch_size <= 0:
        raise ValueError(f"batch_size must be a positive integer, got {batch_size}.")
    if world_size <= 0:
        raise ValueError(f"world_size must be a positive integer, got {world_size}.")
    if dataset_num_rows == 0:
        _logger.warning("dataset_num_rows is 0; steps_per_epoch will be 0.")

    steps_per_epoch = math.ceil(dataset_num_rows / world_size / batch_size)

    limit_train_batches = trainer_kwargs.get("limit_train_batches")
    if limit_train_batches is not None:
        if isinstance(limit_train_batches, float) and 0.0 <= limit_train_batches <= 1.0:
            steps_per_epoch = int(steps_per_epoch * limit_train_batches)
        elif type(limit_train_batches) is int and limit_train_batches >= 1:
            steps_per_epoch = min(steps_per_epoch, limit_train_batches)
        else:
            raise ValueError(
                f"Invalid limit_train_batches: {limit_train_batches!r}. "
                "Must be a float in [0.0, 1.0] or an int >= 1."
            )

    max_steps = trainer_kwargs.get("max_steps", -1)
    if max_steps is not None and max_steps > 0:
        accumulate_grad_batches = trainer_kwargs.get("accumulate_grad_batches", 1)
        if not (
            isinstance(accumulate_grad_batches, int) and accumulate_grad_batches >= 1
        ):
            raise ValueError(
                f"Invalid accumulate_grad_batches: {accumulate_grad_batches!r}. "
                "Must be an int >= 1."
            )
        return min(max_steps * accumulate_grad_batches, steps_per_epoch)

    return steps_per_epoch


def _compute_default_schedule(steps_per_epoch: int | None = None) -> dict:
    """Compute a default ``torch.profiler`` schedule for an epoch of *n* steps.

    ``wait`` scales with the step count so profiling starts once throughput has
    stabilized. ``warmup`` and ``active`` are small fixed windows (2 and 3
    steps), shrunk only when the epoch is too short to fit them after the wait.

    ==========================  ====  ======  ======
    steps_per_epoch             wait  warmup  active
    ==========================  ====  ======  ======
    ``None`` or 1               0     0       1
    2                           1     0       1
    5                           1     2       2
    N (N >= 7)                  N//3  2       3
    ==========================  ====  ======  ======

    Args:
        steps_per_epoch: Estimated steps in one epoch on one worker, or
            ``None`` when unknown.

    Returns:
        A kwargs dict for ``torch.profiler.schedule`` with ``skip_first``,
        ``wait``, ``warmup``, ``active``, and ``repeat``.
    """
    n = steps_per_epoch
    if n is None or n <= 1:
        return {"skip_first": 0, "wait": 0, "warmup": 0, "active": 1, "repeat": 1}
    if n == 2:
        return {"skip_first": 0, "wait": 1, "warmup": 0, "active": 1, "repeat": 1}
    # Wait out the first third of the epoch so the profile reflects steady state.
    wait = n // 3
    remaining_steps = n - wait
    warmup = min(2, remaining_steps - 1)  # leave at least 1 step for active
    active = min(3, remaining_steps - warmup)
    return {
        "skip_first": 0,
        "wait": wait,
        "warmup": warmup,
        "active": active,
        "repeat": 1,
    }


def _validate_profiler_schedule(
    schedule_config: dict, steps_per_epoch: int | None
) -> None:
    """Validate that a user-supplied profiler schedule fits within one epoch.

    Args:
        schedule_config: Kwargs for ``torch.profiler.schedule``; recognizes
            ``skip_first``, ``wait``, ``warmup``, ``active``, and ``repeat``.
        steps_per_epoch: Estimated steps in one epoch on one worker. When
            ``None`` the per-epoch bounds check is skipped and only the
            individual field ranges are enforced.

    Returns:
        ``None``.

    Raises:
        UserInputError: If any field is out of range, or if the schedule would
            need more steps than a single epoch provides.
    """
    skip_first = schedule_config.get("skip_first", 0)
    wait = schedule_config.get("wait", 0)
    warmup = schedule_config.get("warmup", 0)
    active = schedule_config.get("active", 1)
    repeat = schedule_config.get("repeat", 1)

    for name, value in (
        ("skip_first", skip_first),
        ("wait", wait),
        ("warmup", warmup),
    ):
        if value < 0:
            raise UserInputError(
                f"Profiler schedule {name!r} must be non-negative, got {value}."
            )

    if not 1 <= active <= _MAX_PROFILER_ACTIVE_STEPS:
        raise UserInputError(
            f"Profiler schedule 'active' must be between 1 and "
            f"{_MAX_PROFILER_ACTIVE_STEPS}, got {active}. Large active windows "
            "can generate extremely large profiler trace files."
        )
    if not 1 <= repeat <= _MAX_PROFILER_REPEAT:
        raise UserInputError(
            f"Profiler schedule 'repeat' must be between 1 and "
            f"{_MAX_PROFILER_REPEAT}, got {repeat}. Large repeat values can "
            "generate extremely large profiler trace files; a repeat of 1 is "
            "recommended."
        )

    cycle_length = wait + warmup + active
    total_profiled_steps = cycle_length * repeat + skip_first
    if steps_per_epoch is not None and total_profiled_steps > steps_per_epoch:
        raise UserInputError(
            f"Profiler schedule requests {total_profiled_steps} steps "
            f"(({wait} wait + {warmup} warmup + {active} active) * {repeat} "
            f"repeat + {skip_first} skip_first) but each epoch only has "
            f"{steps_per_epoch} steps. Reduce wait, warmup, active, and/or "
            "repeat so that ((wait + warmup + active) * repeat + skip_first) "
            f"<= {steps_per_epoch}."
        )


def _build_pytorch_profiler(
    dirpath: str,
    filename: str,
    kwargs: dict,
    steps_per_epoch: int | None = None,
) -> pl.profilers.Profiler:
    """Build a ``PyTorchProfiler`` with a validated schedule and trace handler.

    Args:
        dirpath: Directory the profiler writes traces to.
        filename: Base filename for this worker's trace.
        kwargs: The ``pytorch`` sub-config. ``schedule`` (a
            ``torch.profiler.schedule`` kwargs dict) and ``on_trace_ready`` (a
            dotted path to a ``fn(prof) -> None`` callable) are consumed here;
            everything else is forwarded to ``PyTorchProfiler``. Not mutated.
        steps_per_epoch: Estimated steps per epoch, used to validate an
            explicit schedule or to derive the default one.

    Returns:
        A configured ``PyTorchProfiler``.

    Raises:
        UserInputError: If an explicit ``schedule`` does not fit in one epoch.
    """
    from pytorch_lightning.profilers import PyTorchProfiler
    from torch.profiler import schedule, tensorboard_trace_handler

    kwargs = dict(kwargs)
    schedule_kwargs = kwargs.pop("schedule", None) or {}
    if schedule_kwargs:
        _validate_profiler_schedule(schedule_kwargs, steps_per_epoch)
    else:
        schedule_kwargs = _compute_default_schedule(steps_per_epoch)

    # on_trace_ready must resolve to a callable with signature
    # fn(prof: torch.profiler.profile) -> None.
    on_trace_ready_path = kwargs.pop("on_trace_ready", None)
    on_trace_ready_fn = (
        get_module_attr(on_trace_ready_path)
        if on_trace_ready_path
        else tensorboard_trace_handler(dir_name=dirpath)
    )

    return PyTorchProfiler(
        dirpath=dirpath,
        filename=filename,
        schedule=schedule(**schedule_kwargs),
        on_trace_ready=on_trace_ready_fn,
        **kwargs,
    )


def _build_profiler(
    profiler_config: dict | str | None,
    steps_per_epoch: int | None = None,
) -> tuple[pl.profilers.Profiler | None, str]:
    """Build a Lightning profiler from a user-supplied profiler config.

    Only the ``pytorch`` profiler is currently supported (Michelangelo OSS is
    PyTorch-heavy); more backends can be added as the platform expands.

    Every profiler writes into a fixed ``profiler_logs`` directory under the
    worker's current working directory, so a profiler sink can locate the
    results after training. Traces are named per world rank; Lightning's
    ``Profiler`` base class appends the local rank.

    Args:
        profiler_config: One of

            * ``None`` — no profiler.
            * The shorthand string ``"pytorch"``, equivalent to
              ``{"pytorch": {}}``.
            * A dict setting ``pytorch`` to a kwargs dict, handled by
              :func:`_build_pytorch_profiler`.

            ``upload_profiler_results`` is reserved metadata read by
            :func:`_resolve_profiler` and is never forwarded to a profiler
            constructor.
        steps_per_epoch: Estimated steps per epoch, used to derive or
            validate the profiler's schedule.

    Returns:
        A ``(profiler, profiler_logs_path)`` tuple. Both are ``None`` / ``""``
        when ``profiler_config`` is ``None``.

    Raises:
        TypeError: If ``profiler_config`` is neither a str, dict, nor ``None``.
        ValueError: If a shorthand string is not a recognized profiler name.
        UserInputError: If the dict sets zero or more than one profiler.
    """
    if profiler_config is None:
        return None, ""
    if isinstance(profiler_config, str):
        if profiler_config not in _PROFILER_SHORTHANDS:
            raise ValueError(
                f"Invalid profiler type: {profiler_config!r}. Valid types are: "
                f"{', '.join(_PROFILER_SHORTHANDS)}."
            )
        profiler_config = {profiler_config: {}}
    elif not isinstance(profiler_config, dict):
        raise TypeError(
            f"profiler_config must be a str, dict, or None, got {type(profiler_config)!r}"
        )

    # Exactly one profiler flavor must be selected.
    selected = [
        name for name in _PROFILER_SHORTHANDS if profiler_config.get(name) is not None
    ]
    if not selected:
        raise UserInputError(
            "Profiler config must set exactly one of: "
            f"{', '.join(_PROFILER_SHORTHANDS)}. None were set."
        )
    if len(selected) > 1:
        raise UserInputError(
            "Profiler config must set exactly one of: "
            f"{', '.join(_PROFILER_SHORTHANDS)}. Multiple were set: {selected}."
        )

    profiler_type = selected[0]
    profiler_kwargs = profiler_config[profiler_type]

    profiler_logs_path = os.path.join(os.getcwd(), _PROFILER_LOGS_DIRNAME)
    os.makedirs(profiler_logs_path, exist_ok=True)
    world_rank = ray.train.get_context().get_world_rank()
    _logger.info(
        "[profiler] [Rank %d] Writing profiler logs to %s",
        world_rank,
        profiler_logs_path,
    )
    # Lightning's Profiler base class appends the local rank to this filename.
    filename = f"profile-world-rank-{world_rank}-local-rank"

    return _build_pytorch_profiler(
        profiler_logs_path, filename, profiler_kwargs, steps_per_epoch
    ), profiler_logs_path


def _resolve_profiler(
    profiler_config: dict | str | None,
    trainer_kwargs: dict,
    dataset_num_rows: int | None,
    batch_size: int,
    world_sz: int,
    rank: int,
) -> tuple[pl.profilers.Profiler | None, str, bool]:
    """Resolve the profiler for the Lightning Trainer.

    Args:
        profiler_config: The ``profiler`` entry popped from
            ``lightning_trainer_kwargs``; see :func:`_build_profiler`.
        trainer_kwargs: Remaining trainer kwargs. Read only, to estimate the
            number of steps per epoch.
        dataset_num_rows: Total rows in the training dataset, or ``None``.
        batch_size: Per-worker batch size.
        world_sz: Total number of training workers.
        rank: This worker's global rank (logging only).

    Returns:
        A ``(profiler, profiler_logs_path, upload_results)`` tuple.
        ``upload_results`` mirrors the config's ``upload_profiler_results`` flag and
        gates the post-``fit`` ``profiler_sink`` call; it defaults to ``True``
        so callers must opt *out* of exporting results.
    """
    if profiler_config is None:
        return None, "", False

    upload_results = (
        profiler_config.get("upload_profiler_results", True)
        if isinstance(profiler_config, dict)
        else True
    )
    steps_per_epoch = _compute_steps_per_epoch(
        trainer_kwargs, dataset_num_rows, batch_size, world_sz
    )
    _logger.info("[profiler] [Rank %d] Steps per epoch: %s", rank, steps_per_epoch)
    profiler, profiler_logs_path = _build_profiler(profiler_config, steps_per_epoch)
    return profiler, profiler_logs_path, upload_results


def _profiler_output(
    profiler: pl.profilers.Profiler, profiler_logs_path: str
) -> tuple[str, list[str]] | None:
    """Classify a profiler's output as a directory or a set of text files.

    Shared by :func:`comet_profiler_sink` and :func:`mlflow_profiler_sink`;
    the classification is backend-independent, only the upload call differs.

    Returns:
        ``("dir", [profiler_logs_path])`` for ``PyTorchProfiler``
        (TensorBoard-format traces), ``("files", txt_paths)`` for
        ``SimpleProfiler``/``AdvancedProfiler`` text summaries (``None`` if
        the directory has no ``.txt`` files), or ``None`` for any other
        profiler type, which has no supported export format.
    """
    from pytorch_lightning.profilers import (
        AdvancedProfiler,
        PyTorchProfiler,
        SimpleProfiler,
    )

    if isinstance(profiler, PyTorchProfiler):
        return "dir", [profiler_logs_path]

    if isinstance(profiler, (SimpleProfiler, AdvancedProfiler)):
        txt_files = glob.glob(os.path.join(profiler_logs_path, "*.txt"))
        if not txt_files:
            _logger.warning("No profiler .txt files found in %s", profiler_logs_path)
            return None
        return "files", txt_files

    _logger.info(
        "Uploading results for %s is not supported; skipping.",
        type(profiler).__name__,
    )
    return None


def comet_profiler_sink(
    profiler: pl.profilers.Profiler,
    profiler_logs_path: str,
    logger: Any,
) -> None:
    """Upload profiler output to a Comet experiment.

    Ready-made ``LightningTrainerParam.profiler_sink`` for runs that log to
    Comet via :func:`build_comet_logger`. ``PyTorchProfiler`` traces are in
    TensorBoard format and go through ``log_tensorflow_folder``;
    ``SimpleProfiler`` / ``AdvancedProfiler`` write text summaries that are
    uploaded individually as assets. ``XLAProfiler`` and custom profilers are
    not supported and are skipped with a log message.

    Every upload is best-effort: a failure is logged and swallowed so a broken
    export never fails an otherwise-successful training run.

    Note:
        Lightning decorates ``CometLogger.experiment`` with
        ``@rank_zero_experiment``, so on any worker whose *global* rank is not 0
        the property yields a no-op dummy experiment and uploads are silently
        dropped. In a multi-node run this means only the node holding global
        rank 0 actually exports its profile. Pass a custom sink that resolves
        its own Comet experiment if per-node profiles are required.

    Args:
        profiler: The profiler built for this worker.
        profiler_logs_path: Directory the profiler wrote its output to.
        logger: The resolved Lightning logger. Must expose an ``experiment``
            attribute holding a Comet experiment; anything else is skipped.

    Returns:
        ``None``.
    """
    experiment = getattr(logger, "experiment", None)
    if experiment is None:
        _logger.warning(
            "comet_profiler_sink: logger %r has no Comet experiment; "
            "skipping profiler upload.",
            type(logger).__name__,
        )
        return

    classified = _profiler_output(profiler, profiler_logs_path)
    if classified is None:
        return
    kind, paths = classified

    if kind == "dir":
        try:
            _logger.info("Uploading PyTorch profiler traces from %s to Comet", paths[0])
            experiment.log_tensorflow_folder(paths[0])
        except Exception:
            _logger.warning(
                "Failed to upload PyTorch profiler traces to Comet", exc_info=True
            )
        return

    for txt_file in paths:
        try:
            _logger.info("Uploading profiler log %s to Comet", txt_file)
            experiment.log_asset(txt_file)
        except Exception:
            _logger.warning(
                "Failed to upload profiler log %s to Comet",
                txt_file,
                exc_info=True,
            )


def mlflow_profiler_sink(
    profiler: pl.profilers.Profiler,
    profiler_logs_path: str,
    logger: Any,
) -> None:
    """Upload profiler output to an MLflow run.

    Ready-made ``LightningTrainerParam.profiler_sink`` for runs that log to
    MLflow via :func:`build_mlflow_logger`. ``PyTorchProfiler`` traces are
    uploaded as a directory via ``MlflowClient.log_artifacts``;
    ``SimpleProfiler``/``AdvancedProfiler`` text summaries are uploaded
    per-file via ``MlflowClient.log_artifact``. Unlike Comet's
    ``log_tensorflow_folder``, MLflow has no TensorBoard-aware upload: traces
    are stored verbatim under an ``artifact_path="profiler"`` prefix and must
    be downloaded to view in a trace viewer (e.g. ``chrome://tracing``).
    ``XLAProfiler`` and custom profilers are not supported and are skipped
    with a log message.

    Every upload is best-effort: a failure is logged and swallowed so a
    broken export never fails an otherwise-successful training run.

    Note:
        Lightning decorates ``MLFlowLogger.experiment`` with
        ``@rank_zero_experiment``, identically to ``CometLogger.experiment``.
        On any worker whose *global* rank is not 0 the property yields a
        no-op dummy client (``_DummyExperiment``) and uploads are silently
        dropped. ``experiment is None`` is therefore not a reliable
        "can I upload?" check on any Lightning logger, including this one —
        a non-zero-rank worker gets a dummy object, not ``None``. Pass a
        custom sink built on a directly-constructed ``MlflowClient`` if
        per-node profiles are required.

    Args:
        profiler: The profiler built for this worker.
        profiler_logs_path: Directory the profiler wrote its output to.
        logger: The resolved Lightning logger. Must expose ``experiment``
            (an ``MlflowClient``) and ``run_id``; anything else is skipped.

    Returns:
        ``None``.
    """
    client = getattr(logger, "experiment", None)
    run_id = getattr(logger, "run_id", None)
    if client is None or run_id is None:
        _logger.warning(
            "mlflow_profiler_sink: logger %r has no MLflow client or run id; "
            "skipping profiler upload.",
            type(logger).__name__,
        )
        return

    classified = _profiler_output(profiler, profiler_logs_path)
    if classified is None:
        return
    kind, paths = classified

    for path in paths:
        try:
            _logger.info("Uploading profiler output %s to MLflow", path)
            if kind == "dir":
                client.log_artifacts(run_id, path, artifact_path="profiler")
            else:
                client.log_artifact(run_id, path, artifact_path="profiler")
        except Exception:
            _logger.warning(
                "Failed to upload profiler output %s to MLflow", path, exc_info=True
            )


def _maybe_track_experiment(train_loop_config: dict, rank: int) -> None:
    """Record this run's experiment directory via the configured ExperimentStore.

    Best-effort and rank-0-only: called once near the start of the worker loop so
    a future re-run with the same ``RunConfig(storage_path=..., name=...)`` can
    locate and resume this run's Ray Train experiment directory. A non-rank-0
    worker or a missing store is skipped silently; an incomplete
    ``(storage_path, run_name)`` identity is logged once (auto-resume needs both)
    and skipped; any failure (including a misbehaving custom store) is caught and
    logged — tracking must never fail an otherwise-successful training run.

    The recorded ``experiment_path`` is reconstructed from the driver-provided
    ``storage_path`` (scheme-qualified, e.g. ``s3://bucket/runs``) rather than
    the storage context's scheme-stripped ``storage_fs_path``, so a later resume
    can address the directory on remote filesystems.

    Args:
        train_loop_config: The per-worker config dict. May carry
            ``experiment_store`` plus the ``storage_path`` / ``run_name``
            identity injected by :class:`LightningTrainer` when a store and a
            ``RunConfig`` are both set.
        rank: This worker's global rank; tracking runs only on rank 0.
    """
    if rank != 0:
        return
    store = train_loop_config.get("experiment_store")
    if store is None:
        return
    storage_path = train_loop_config.get("storage_path")
    run_name = train_loop_config.get("run_name")
    if not storage_path or not run_name:
        _logger.info(
            "experiment_store is set but the run identity is incomplete "
            "(storage_path=%r, run_name=%r); skipping experiment tracking and "
            "auto-resume.",
            storage_path,
            run_name,
        )
        return
    try:
        storage_context = ray.train.get_context().get_storage()
        experiment_path = (
            f"{storage_path.rstrip('/')}/{storage_context.experiment_dir_name}"
        )
        store.track(
            storage_path=storage_path,
            run_name=run_name,
            experiment_path=experiment_path,
        )
    except Exception:
        _logger.warning("experiment_store.track failed", exc_info=True)


# Training loop.
def _train_loop_per_worker(train_loop_config):
    """Execute one Lightning training run on a single Ray Train worker.

    This function is passed to ray.train.torch.TorchTrainer as the per-worker
    training loop. It reads all configuration from train_loop_config, sets up
    the Lightning Trainer, handles checkpoint restoration from a previous run,
    and calls trainer.fit.
    """
    if torch.cuda.is_available():
        _logger.info(
            "CUDA is available with torch, training on GPU with CUDA version: %s",
            torch.version.cuda,
        )
    else:
        _logger.info("CUDA is not available with torch, training on CPU.")

    rank = ray.train.get_context().get_world_rank()
    world_sz = ray.train.get_context().get_world_size()
    _logger.info("rank: %d, world_sz: %d", rank, world_sz)

    _maybe_track_experiment(train_loop_config, rank)

    # Read configurations.
    batch_size = train_loop_config["batch_size"]
    # num_epochs is kept here because callers can use LightningTrainer directly without
    # setting lightning_trainer_kwargs["max_epochs"]; we apply this as a default below.
    num_epochs = train_loop_config["num_epochs"]
    num_shuffle_batches = train_loop_config["num_shuffle_batches"]

    create_model_fn = train_loop_config["create_model_fn"]
    create_model_fn_kwargs = train_loop_config["create_model_fn_kwargs"]
    # If collate_fn_to_torch is None, return a dictionary of column-tensors.
    # https://docs.ray.io/en/latest/data/api/doc/ray.data.DataIterator.iter_torch_batches.html#ray.data.DataIterator.iter_torch_batches
    collate_fn_to_torch = train_loop_config["data_collate_fn"]

    # Fetch dataset.
    train_dataset_shard = ray.train.get_dataset_shard("train")
    val_dataset_shard = ray.train.get_dataset_shard("val")

    # Create data loader.
    # We need to adjust 'local_shuffle_buffer_size' in Ray Data.
    train_dataloader = train_dataset_shard.iter_torch_batches(
        batch_size=batch_size,
        collate_fn=collate_fn_to_torch,
        local_shuffle_buffer_size=None
        if num_shuffle_batches == 0
        else num_shuffle_batches * batch_size,
    )
    val_dataloader = val_dataset_shard.iter_torch_batches(
        batch_size=batch_size,
        collate_fn=collate_fn_to_torch,
    )

    model = create_model_fn(**create_model_fn_kwargs)

    # =========================================================
    # Initial weights loading (Rank 0 only) + layer freeze re-application
    # When an upstream task saves a state_dict to storage and passes the path
    # via initial_weights_path, only Rank 0 downloads from storage; other
    # workers receive weights via RayDDPStrategy broadcast (NCCL).
    # Layer freeze (requires_grad=False) is not preserved in state_dict, so
    # it must be re-applied here using transfer_learning_spec.
    # =========================================================
    initial_weights_path = train_loop_config.get("initial_weights_path")
    _logger.info(
        "[init_weights] [Rank %d] Initial weights path: %r", rank, initial_weights_path
    )
    if initial_weights_path:
        if rank == 0:
            _logger.info(
                "[init_weights] [Rank 0] Loading initial weights from: %r",
                initial_weights_path,
            )
            try:
                _load_weights_from_path(model, initial_weights_path)
                _logger.info("[init_weights] [Rank 0] Weights loaded successfully.")
                _print_layer_weights(model)
            except Exception as e:
                msg = f"[init_weights] [Rank 0] Failed to load initial weights from {initial_weights_path!r}: {e!r}"
                _logger.error(msg)
                raise UserInputError(msg) from e
        else:
            _logger.info(
                "[init_weights] [Rank %d] Waiting for broadcast from Rank 0...", rank
            )

    transfer_learning_spec = train_loop_config.get("transfer_learning_spec")
    if transfer_learning_spec:
        _apply_layer_freeze(model, transfer_learning_spec)

    # Set defaults for values that differ from Lightning's Trainer defaults.
    trainer_kwargs = dict(train_loop_config.get("lightning_trainer_kwargs") or {})
    trainer_kwargs.setdefault("max_epochs", num_epochs if num_epochs is not None else 1)
    trainer_kwargs.setdefault("num_sanity_val_steps", 0)
    trainer_kwargs.setdefault("enable_progress_bar", False)

    if "enable_checkpointing" in trainer_kwargs:
        _logger.warning(
            "enable_checkpointing in lightning_trainer_kwargs is ignored; its value is determined by the presence of a ModelCheckpoint callback."
        )

    # Convert values from trainer_kwargs to their corresponding arguments for the Lightning Trainer.
    # We pop the values from trainer_kwargs to avoid passing invalid values to the Lightning Trainer.
    strategy = _resolve_strategy(
        trainer_kwargs.pop("strategy", None),
        trainer_kwargs.pop("strategy_kwargs", None),
    )
    plugins = _resolve_plugins(
        trainer_kwargs.pop("plugins", None), trainer_kwargs.pop("plugins_kwargs", None)
    )
    logger = _resolve_logger(
        trainer_kwargs.pop("logger", None),
        trainer_kwargs.pop("logger_kwargs", None),
        train_loop_config.get("run_id"),
    )
    callbacks, has_model_checkpoint = _resolve_callbacks(
        trainer_kwargs.pop("callbacks", None),
        trainer_kwargs.pop("callback_kwargs", None),
        trainer_kwargs.pop(CALLBACK_REPORT_PER_NODE, None),
        strategy,
        training_observer=train_loop_config.get("training_observer"),
    )
    profiler, profiler_logs_path, upload_profiler_results = _resolve_profiler(
        trainer_kwargs.pop("profiler", None),
        trainer_kwargs,
        train_loop_config.get("train_dataset_num_rows"),
        batch_size,
        world_sz,
        rank,
    )

    # Update trainer_kwargs with the resolved arguments for the Lightning Trainer.
    trainer_kwargs["strategy"] = strategy
    trainer_kwargs["plugins"] = plugins
    trainer_kwargs["logger"] = logger
    trainer_kwargs["callbacks"] = callbacks
    trainer_kwargs["profiler"] = profiler
    trainer_kwargs["enable_checkpointing"] = (
        has_model_checkpoint  # enable_checkpointing must be set to True if a ModelCheckpoint callback is used
    )

    trainer = pl.Trainer(
        **trainer_kwargs,
    )
    trainer = ray.train.lightning.prepare_trainer(trainer)

    checkpoint = ray.train.get_checkpoint()
    if checkpoint is None:
        # Ray Train V2 resumes natively from a reused run directory; when it has
        # no checkpoint to restore, fall back to the checkpoint the driver
        # located via the ExperimentStore (if any). Native restoration always
        # takes priority.
        seed_path = train_loop_config.get("resume_checkpoint_path")
        if seed_path:
            _logger.info(
                "No native Ray checkpoint found; seeding auto-resume from "
                "store-located checkpoint: %s",
                seed_path,
            )
            checkpoint = ray.train.Checkpoint(seed_path)
    _logger.info(
        "Resuming from checkpoint.path=%s", checkpoint.path if checkpoint else None
    )

    # Download checkpoint locally to support both DDP and DeepSpeed strategies.
    # DDP checkpoints are single files; DeepSpeed ZeRO checkpoints are sharded directories.
    # Using to_directory() handles both cases by downloading the full checkpoint to a local path.
    ckpt_path = None
    if checkpoint:
        local_ckpt_dir = checkpoint.to_directory()
        ckpt_path = os.path.join(local_ckpt_dir, CHECKPOINT_FILENAME)
    trainer.fit(
        model,
        train_dataloaders=train_dataloader,
        val_dataloaders=val_dataloader,
        ckpt_path=ckpt_path,
    )

    _maybe_export_profiler_results(
        profiler,
        profiler_logs_path,
        logger,
        upload_profiler_results,
        train_loop_config.get("profiler_sink"),
    )


def _maybe_export_profiler_results(
    profiler: pl.profilers.Profiler | None,
    profiler_logs_path: str,
    logger: Any,
    upload_profiler_results: bool,
    profiler_sink: Any,
) -> None:
    """Hand this worker's profiler output to the configured sink, if any.

    Called once per worker after ``trainer.fit()`` returns. Runs only on
    node-local rank 0: every profiler writes into a ``profiler_logs`` directory
    under the worker's cwd, which is shared by all workers on a node, so one
    call per node exports that node's whole directory exactly once.

    Best-effort — a sink that raises is logged and swallowed, since profiling
    is an observability feature and must not fail a completed training run.

    Args:
        profiler: The profiler built for this worker, or ``None``.
        profiler_logs_path: Directory the profiler wrote its output to.
        logger: The resolved Lightning logger, forwarded to the sink so it can
            attach results to the active experiment.
        upload_profiler_results: The config's ``upload_profiler_results`` flag; when
            ``False`` the sink is not called.
        profiler_sink: The user-supplied
            ``Callable[[Profiler, str, logger], None]``, or ``None``.

    Returns:
        ``None``.
    """
    if profiler is None or not upload_profiler_results or profiler_sink is None:
        return
    if ray.train.get_context().get_local_rank() != 0:
        return
    try:
        profiler_sink(profiler, profiler_logs_path, logger)
    except Exception:
        _logger.warning(
            "profiler_sink raised; profiler results not exported", exc_info=True
        )
