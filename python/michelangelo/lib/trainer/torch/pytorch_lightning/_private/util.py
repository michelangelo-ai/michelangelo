"""Internal helpers for the PyTorch Lightning trainer.

This module hosts the per-worker training loop and the strategy / plugin /
logger / callback resolution helpers. Public APIs live in
``michelangelo.lib.trainer.torch.pytorch_lightning.lightning_trainer``.
"""

from __future__ import annotations

import hashlib
import importlib
import logging
import os
import re
from tempfile import TemporaryDirectory
from typing import Any, Union

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

from michelangelo.lib.trainer.torch.pytorch_lightning._private.callbacks import (
    RayTrainReportCallback,
    RayTrainReportPerNodeCallback,
)


class UserInputError(Exception):
    """Raised when a user-supplied input or path causes training to fail."""


def _get_module_attr(module_attr: str) -> Any:
    """Resolve a dotted ``module.attribute`` path to the attribute object."""
    module_def, _, attr_def = module_attr.rpartition(".")
    module = importlib.import_module(module_def)
    return getattr(module, attr_def)


# Plugin types accepted by the PyTorch Lightning Trainer.
# See: https://github.com/Lightning-AI/pytorch-lightning/blob/2129fdf3622e39ba46be4e1139af408e7e951cf3/src/lightning/pytorch/trainer/trainer.py#L126
_PLUGIN_INPUT = Union[Precision, ClusterEnvironment, CheckpointIO, LayerSync]

CALLBACK_REPORT_PER_NODE = "callback_report_per_node"
CHECKPOINT_FILENAME = "checkpoint.ckpt"

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
    installed; only callers that actually pass a ``comet_param`` need it.
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
        plugins_fn = _get_module_attr(plugins)
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
    comet_param: dict[str, Any] | None = None,
    run_id: str | None = None,
) -> bool | Logger | list[Logger] | None:
    """Resolve the logger for the Lightning Trainer."""
    if logger_kwargs is not None and not isinstance(logger_kwargs, dict):
        raise TypeError(
            f"logger_kwargs must be a dict or None, got {type(logger_kwargs)!r}"
        )
    if logger_kwargs is not None and not isinstance(logger, str):
        raise TypeError(
            "logger_kwargs can only be used when logger is a str import path"
        )
    if comet_param is not None and not isinstance(comet_param, dict):
        raise TypeError(
            f"comet_param must be a dict or None, got {type(comet_param)!r}"
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
        logger_fn = _get_module_attr(logger)
        result = logger_fn(**(logger_kwargs or {}))
        return list(result) if isinstance(result, (list, tuple)) else result
    if logger is not None:
        raise TypeError(
            f"logger must be a str, bool, Logger instance, list of Logger instances, or None, got {type(logger)!r}"
        )
    if comet_param and run_id:
        return _get_comet_logger(
            run_id,
            api_key=comet_param["api_key"],
            workspace=comet_param["workspace"],
            project_name=comet_param["project_name"],
            experiment_name=comet_param["experiment_name"],
            tags=comet_param.get("tags"),
        )
    return None


def _resolve_callbacks(
    callbacks: str | Callback | list[Callback] | None = None,
    callback_kwargs: dict[str, Any] | None = None,
    per_node_callback_kwargs: dict[str, Any] | None = None,
    strategy: Strategy | None = None,
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
        fn = _get_module_attr(callbacks)
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
            RayTrainReportPerNodeCallback(**per_node_callback_kwargs)
        )
    else:
        resolved_callbacks.append(RayTrainReportCallback())

    return resolved_callbacks, has_model_checkpoint


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
        train_loop_config.get("comet_param"),
        train_loop_config.get("run_id"),
    )
    callbacks, has_model_checkpoint = _resolve_callbacks(
        trainer_kwargs.pop("callbacks", None),
        trainer_kwargs.pop("callback_kwargs", None),
        trainer_kwargs.pop(CALLBACK_REPORT_PER_NODE, None),
        strategy,
    )

    # Update trainer_kwargs with the resolved arguments for the Lightning Trainer.
    trainer_kwargs["strategy"] = strategy
    trainer_kwargs["plugins"] = plugins
    trainer_kwargs["logger"] = logger
    trainer_kwargs["callbacks"] = callbacks
    trainer_kwargs["enable_checkpointing"] = (
        has_model_checkpoint  # enable_checkpointing must be set to True if a ModelCheckpoint callback is used
    )

    trainer = pl.Trainer(
        **trainer_kwargs,
    )
    trainer = ray.train.lightning.prepare_trainer(trainer)

    checkpoint = ray.train.get_checkpoint()
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
