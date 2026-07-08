"""Driver-side bridge: ``ExperimentTrackerConfig`` to Lightning logger kwargs.

Pure function with no OSS-infra dependencies (no Ray clusters, no storage
backends) so it can be tested without a Ray session.
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

from michelangelo.workflow.schema.exceptions import ConfigurationError

if TYPE_CHECKING:
    from michelangelo.workflow.schema.tabular_trainer import ExperimentTrackerConfig

_COMET_FACTORY = (
    "michelangelo.lib.trainer.torch.pytorch_lightning._private.util.build_comet_logger"
)
_MLFLOW_FACTORY = (
    "michelangelo.lib.trainer.torch.pytorch_lightning._private.util.build_mlflow_logger"
)


def build_tracker_logger_kwargs(
    config: ExperimentTrackerConfig | None,
) -> dict[str, Any]:
    """Convert ``ExperimentTrackerConfig`` into Lightning logger kwargs.

    Logger construction is deferred to the Ray Train worker (via the
    dotted-path ``logger`` string resolved by ``_resolve_logger``) so that
    per-worker distributed coordination — e.g. Comet's
    ``torch.distributed.barrier()`` — runs correctly inside the worker
    process rather than on the driver.

    Args:
        config: Tracker config, or ``None`` to disable experiment tracking.

    Returns:
        A dict with ``"logger"`` (a dotted import path, or ``None``) and
        ``"logger_kwargs"`` (a dict, or ``None``) suitable for merging into
        ``LightningTrainerKwargs``.

    Raises:
        ConfigurationError: If ``config.tracker`` is a ``TrackerConfig``
            subclass not recognised here. This should not be reachable in
            practice since ``TrackerConfig.__post_init__`` already rejects
            unsupported types at construction time; this guards against
            future subclasses added without a corresponding branch here.
    """
    from michelangelo.workflow.schema.tabular_trainer import (
        CometConfig,
        CustomTrackerConfig,
        MlflowConfig,
    )

    if config is None or config.tracker is None:
        return {"logger": None, "logger_kwargs": None}

    tracker = config.tracker

    if isinstance(tracker, CometConfig):
        return {
            "logger": _COMET_FACTORY,
            "logger_kwargs": {
                "api_key": tracker.api_key,
                "workspace": tracker.workspace,
                "project_name": tracker.project_name,
                "experiment_name": tracker.experiment_name,
                "tags": tracker.tags,
            },
        }

    if isinstance(tracker, MlflowConfig):
        return {
            "logger": _MLFLOW_FACTORY,
            "logger_kwargs": {
                "experiment_name": tracker.experiment_name,
                "tracking_uri": tracker.tracking_uri,
                "run_name": tracker.run_name,
                "tags": tracker.tags,
            },
        }

    if isinstance(tracker, CustomTrackerConfig):
        return {
            "logger": tracker.factory_fn,
            "logger_kwargs": dict(tracker.factory_kwargs),
        }

    raise ConfigurationError(
        f"Unsupported tracker type: {type(tracker).__name__}. "
        "Supported: CometConfig, MlflowConfig, CustomTrackerConfig."
    )
