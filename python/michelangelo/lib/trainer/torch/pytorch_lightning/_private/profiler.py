"""Profiler helpers for the PyTorch Lightning trainer."""

from __future__ import annotations

import glob
import logging
import math
import os
from typing import TYPE_CHECKING, Any

import ray

if TYPE_CHECKING:
    import pytorch_lightning as pl

from michelangelo._internal.utils.reflection_utils import get_module_attr
from michelangelo.lib._internal.errors import UserInputError

_logger = logging.getLogger(__name__)

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
