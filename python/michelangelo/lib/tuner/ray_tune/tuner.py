"""Public Ray Tune helper for hyperparameter search.

Runs a ``ray.tune`` search on the Ray cluster the calling process is already
connected to, exposed with the same shape as the trainers in
``michelangelo.lib.trainer``: a parameter dataclass, a ``tune()`` that returns
a small result dict, and checkpointing handled by Ray's own reporting.

Inside a ``RayTask``-decorated Uniflow task no extra plumbing is needed --
``RayTask.pre_run()`` has already called ``ray.init()`` against the
provisioned cluster by the time the task body runs, so every trial is
scheduled on that cluster. This is the difference from a hand-written sweep
loop, which provisions one Ray cluster per configuration; here one cluster
runs all trials.

Typical use::

    import michelangelo.uniflow.core as uniflow
    from michelangelo.uniflow.plugins.ray import RayTask
    from michelangelo.lib.tuner.ray_tune import TuneParam, tune

    def objective(config: dict) -> None:
        loss = train_once(lr=config["lr"], depth=config["depth"])
        ray.tune.report({"val_loss": loss})

    @uniflow.task(config=RayTask(head_cpu=2, worker_cpu=4, worker_instances=4))
    def tune_step(data_url: str) -> dict:
        return tune(
            TuneParam(
                trainable=objective,
                param_space={
                    "lr": ray.tune.loguniform(1e-4, 1e-1),
                    "depth": ray.tune.randint(2, 10),
                },
                metric="val_loss",
                mode="min",
                num_samples=20,
            )
        )

To persist trial checkpoints to UniFlow-managed storage, pass
``run_config=create_run_config()`` (from
``michelangelo.uniflow.plugins.ray.run_config``), the same helper the
trainers use.
"""

from __future__ import annotations

import logging
from dataclasses import dataclass, field
from typing import Any, Callable

import ray.tune
from ray.tune.schedulers import ASHAScheduler, FIFOScheduler, TrialScheduler

_logger = logging.getLogger(__name__)

BEST_CONFIG_KEY = "best_config"
BEST_METRICS_KEY = "best_metrics"
CHECKPOINT_PATH_KEY = "checkpoint_path"

_SCHEDULERS: dict[str, type[TrialScheduler]] = {
    "asha": ASHAScheduler,
    "fifo": FIFOScheduler,
}

# TuneConfig fields owned by TuneParam's first-class fields; allowing them in
# tune_config_kwargs too would make one of the two values win silently.
_RESERVED_TUNE_CONFIG_KEYS = frozenset(
    {"metric", "mode", "num_samples", "scheduler", "max_concurrent_trials"}
)


@dataclass
class TuneParam:
    """Configuration for :func:`tune`.

    Attributes:
        trainable: Per-trial function. Called once per trial with the sampled
            ``config`` dict; reports metrics via ``ray.tune.report``.
        param_space: Ray Tune search space, passed verbatim to
            ``ray.tune.Tuner`` (for example
            ``{"lr": ray.tune.loguniform(1e-4, 1e-1)}``).
        metric: Metric name to optimize. Must match a key the trainable
            reports.
        mode: ``"min"`` or ``"max"``.
        num_samples: Number of trials to run.
        scheduler: ``"asha"`` (default, early-stops bad trials), ``"fifo"``
            (no early stopping), or any ``ray.tune.schedulers.TrialScheduler``
            instance for full control.
        resources_per_trial: Optional per-trial resource request (for example
            ``{"cpu": 2, "gpu": 1}``), applied with
            ``ray.tune.with_resources``. When ``None``, Ray's default of one
            CPU per trial applies.
        max_concurrent_trials: Optional cap on concurrently running trials.
        run_config: Optional Ray ``RunConfig`` controlling where trial
            results and checkpoints are stored. Defaults to Ray's own storage
            location; pass ``create_run_config()`` to use UniFlow-managed
            storage.
        tune_config_kwargs: Extra keyword arguments forwarded verbatim to
            ``ray.tune.TuneConfig``. Keys owned by the fields above
            (``metric``, ``mode``, ``num_samples``, ``scheduler``,
            ``max_concurrent_trials``) are rejected rather than silently
            overridden.
    """

    trainable: Callable[[dict[str, Any]], Any]
    param_space: dict[str, Any]
    metric: str
    mode: str = "min"
    num_samples: int = 10
    scheduler: str | TrialScheduler = "asha"
    resources_per_trial: dict[str, float] | None = None
    max_concurrent_trials: int | None = None
    run_config: Any | None = None
    tune_config_kwargs: dict[str, Any] = field(default_factory=dict)

    def __post_init__(self) -> None:
        """Reject configurations Ray would fail on later, or silently misuse.

        Raises:
            ValueError: If ``trainable`` is not callable, ``param_space`` is
                empty, ``metric`` is empty, ``mode`` is not ``min``/``max``,
                ``num_samples`` is not positive, ``scheduler`` is an unknown
                name or not a ``TrialScheduler``, or ``tune_config_kwargs``
                carries a reserved key.
        """
        if not callable(self.trainable):
            raise ValueError("trainable must be callable")
        if not self.param_space:
            raise ValueError("param_space must be a non-empty search space")
        if not self.metric:
            raise ValueError("metric must be a non-empty metric name")
        if self.mode not in ("min", "max"):
            raise ValueError(f"mode must be 'min' or 'max', got {self.mode!r}")
        if self.num_samples <= 0:
            raise ValueError(f"num_samples must be positive, got {self.num_samples}")
        if isinstance(self.scheduler, str):
            if self.scheduler not in _SCHEDULERS:
                raise ValueError(
                    f"scheduler must be one of {sorted(_SCHEDULERS)} or a "
                    f"TrialScheduler instance, got {self.scheduler!r}"
                )
        elif not isinstance(self.scheduler, TrialScheduler):
            raise ValueError(
                "scheduler must be a scheduler name or a TrialScheduler "
                f"instance, got {type(self.scheduler).__name__}"
            )
        reserved = _RESERVED_TUNE_CONFIG_KEYS.intersection(self.tune_config_kwargs)
        if reserved:
            raise ValueError(
                f"tune_config_kwargs must not set {sorted(reserved)}; use the "
                "TuneParam fields of the same name instead"
            )


def _resolve_scheduler(scheduler: str | TrialScheduler) -> TrialScheduler:
    """Return the ``TrialScheduler`` instance for a name or passthrough."""
    if isinstance(scheduler, TrialScheduler):
        return scheduler
    return _SCHEDULERS[scheduler]()


def tune(tune_param: TuneParam) -> dict:
    """Run the search and return a small result dict.

    Args:
        tune_param: Search configuration (trainable, space, metric, budget,
            ...).

    Returns:
        Dict with ``best_config`` (the best trial's sampled parameters),
        ``best_metrics`` (its last reported metrics), ``checkpoint_path``
        (path to its checkpoint, or ``None`` if the trainable reported none),
        ``path`` (the best trial's result directory), ``num_trials``, and
        ``num_errors`` (trials that raised; the best result is drawn from the
        trials that completed).

    Raises:
        RuntimeError: From Ray, when no trial completed successfully.
    """
    _logger.info("tune: starting search with tune_param: %r", tune_param)

    trainable = tune_param.trainable
    if tune_param.resources_per_trial:
        trainable = ray.tune.with_resources(trainable, tune_param.resources_per_trial)

    tuner = ray.tune.Tuner(
        trainable,
        param_space=tune_param.param_space,
        tune_config=ray.tune.TuneConfig(
            metric=tune_param.metric,
            mode=tune_param.mode,
            num_samples=tune_param.num_samples,
            scheduler=_resolve_scheduler(tune_param.scheduler),
            max_concurrent_trials=tune_param.max_concurrent_trials,
            **tune_param.tune_config_kwargs,
        ),
        run_config=tune_param.run_config,
    )
    results = tuner.fit()

    if results.errors:
        _logger.warning(
            "tune: %d of %d trials errored; best result is drawn from the "
            "completed trials",
            len(results.errors),
            len(results),
        )

    best = results.get_best_result()
    return {
        BEST_CONFIG_KEY: best.config,
        BEST_METRICS_KEY: best.metrics,
        CHECKPOINT_PATH_KEY: best.checkpoint.path if best.checkpoint else None,
        "path": best.path,
        "num_trials": len(results),
        "num_errors": len(results.errors),
    }
