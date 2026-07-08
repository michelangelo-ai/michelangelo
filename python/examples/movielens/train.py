"""End-to-end MovieLens-100k training using the lib/trainer/torch snapshot.

Run from ``python/`` (the OSS Poetry root):

    python -m examples.movielens.train

Trains a tiny NCF on CPU with a single Ray Train worker. Designed as the
smallest viable smoke test for
:class:`michelangelo.lib.trainer.torch.pytorch_lightning.lightning_trainer.LightningTrainer`.

Experiment tracking is optional and picks at most one backend per run:

* Comet — set ``COMET_API_KEY`` and ``COMET_WORKSPACE`` (and optionally
  ``COMET_PROJECT_NAME`` / ``COMET_EXPERIMENT_NAME`` / ``COMET_TAGS``).
* MLflow — set ``MLFLOW_TRACKING_URI`` (and optionally
  ``MLFLOW_EXPERIMENT_NAME`` / ``MLFLOW_RUN_NAME`` / ``MLFLOW_TAGS``).

Comet wins when both env-sets are present. With neither set, the trainer
falls back to Lightning's default local logger.
"""

from __future__ import annotations

import logging
import os

import ray

from examples.movielens.data import load_movielens_100k
from examples.movielens.model import create_ncf_model
from michelangelo.lib.trainer.torch.pytorch_lightning.lightning_trainer import (
    LightningTrainer,
    LightningTrainerParam,
)

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s: %(message)s",
)
log = logging.getLogger("examples.movielens.train")

_STORAGE_DIR = "/tmp/movielens_runs"
_DEFAULT_COMET_PROJECT = "michelangelo-movielens-demo"
_DEFAULT_COMET_EXPERIMENT = "ncf-movielens100k"
_DEFAULT_MLFLOW_EXPERIMENT = "ncf-movielens100k"
_COMET_LOGGER_FACTORY = (
    "michelangelo.lib.trainer.torch.pytorch_lightning._private.util.build_comet_logger"
)


def _build_comet_logger_kwargs() -> dict | None:
    """Build build_comet_logger kwargs from env vars, or None to skip Comet logging.

    Both COMET_API_KEY and COMET_WORKSPACE must be set to enable Comet. The
    other fields fall back to module defaults if unset. Returned as a plain
    dict (rather than a constructed Logger) so the actual CometLogger is
    built inside the Ray Train worker via the dotted-path factory —
    required for the rank-0-creates/others-attach barrier pattern in
    ``build_comet_logger``.
    """
    api_key = os.environ.get("COMET_API_KEY")
    workspace = os.environ.get("COMET_WORKSPACE")
    if not (api_key and workspace):
        return None
    tags_env = os.environ.get("COMET_TAGS", "").strip()
    tags = [t.strip() for t in tags_env.split(",") if t.strip()] or None
    return {
        "api_key": api_key,
        "workspace": workspace,
        "project_name": os.environ.get("COMET_PROJECT_NAME", _DEFAULT_COMET_PROJECT),
        "experiment_name": os.environ.get(
            "COMET_EXPERIMENT_NAME", _DEFAULT_COMET_EXPERIMENT
        ),
        "tags": tags,
    }


def _parse_mlflow_tags(tags_env: str) -> dict | None:
    """Parse ``key1=val1,key2=val2`` into a dict; return None if empty/malformed."""
    if not tags_env.strip():
        return None
    parsed = {}
    for kv in tags_env.split(","):
        if "=" not in kv:
            continue
        k, v = kv.split("=", 1)
        k, v = k.strip(), v.strip()
        if k:
            parsed[k] = v
    return parsed or None


def _build_mlflow_logger():
    """Build an MLFlowLogger from env vars, or return None to skip MLflow logging.

    MLFLOW_TRACKING_URI must be set to enable MLflow. The logger is constructed
    here (not inside the trainer's worker) and forwarded via
    ``lightning_trainer_kwargs["logger"]`` — ``_resolve_logger`` accepts any
    pytorch_lightning ``Logger`` instance.
    """
    tracking_uri = os.environ.get("MLFLOW_TRACKING_URI")
    if not tracking_uri:
        return None
    # Import lazily so an unused MLflow path doesn't force the dependency.
    from pytorch_lightning.loggers import MLFlowLogger

    return MLFlowLogger(
        experiment_name=os.environ.get(
            "MLFLOW_EXPERIMENT_NAME", _DEFAULT_MLFLOW_EXPERIMENT
        ),
        tracking_uri=tracking_uri,
        run_name=os.environ.get("MLFLOW_RUN_NAME"),
        tags=_parse_mlflow_tags(os.environ.get("MLFLOW_TAGS", "")),
    )


def main() -> dict:
    """Run the MovieLens-100k NCF training and return the summary dict."""
    splits = load_movielens_100k()

    # Pick at most one tracking backend. Comet wins if both env-sets are present.
    comet_logger_kwargs = _build_comet_logger_kwargs()
    mlflow_logger = None
    if comet_logger_kwargs is not None:
        log.info(
            "Comet logging enabled (workspace=%s project=%s experiment=%s)",
            comet_logger_kwargs["workspace"],
            comet_logger_kwargs["project_name"],
            comet_logger_kwargs["experiment_name"],
        )
        if os.environ.get("MLFLOW_TRACKING_URI"):
            log.info(
                "MLFLOW_TRACKING_URI is also set but Comet takes precedence; "
                "MLflow logging skipped."
            )
    else:
        mlflow_logger = _build_mlflow_logger()
        if mlflow_logger is not None:
            log.info(
                "MLflow logging enabled (tracking_uri=%s experiment=%s)",
                mlflow_logger._tracking_uri,
                mlflow_logger.experiment_id,
            )
        else:
            log.info(
                "Experiment tracking disabled "
                "(no COMET_* or MLFLOW_TRACKING_URI env vars set)"
            )

    lightning_trainer_kwargs = {
        "max_epochs": 3,
        "log_every_n_steps": 20,
        # Force CPU for this local smoke test.
        #
        # On Apple Silicon, PyTorch Lightning may auto-detect the MPS backend.
        # Ray Train's Lightning integration still uses a DDP-family strategy,
        # and Lightning does not support DDP on MPS. Explicitly pinning the
        # accelerator to CPU keeps this example working on macOS.
        "accelerator": "cpu",
    }
    if comet_logger_kwargs is not None:
        # Dotted-path factory: the CometLogger is constructed inside the Ray
        # Train worker (required for the rank-0-creates/others-attach
        # barrier pattern in build_comet_logger).
        lightning_trainer_kwargs["logger"] = _COMET_LOGGER_FACTORY
        lightning_trainer_kwargs["logger_kwargs"] = comet_logger_kwargs
    elif mlflow_logger is not None:
        # _resolve_logger accepts a pre-built Logger instance and forwards it
        # to the Lightning Trainer unchanged.
        lightning_trainer_kwargs["logger"] = mlflow_logger

    trainer_param = LightningTrainerParam(
        create_model_fn=create_ncf_model,
        create_model_fn_kwargs={
            "num_users": splits.num_users,
            "num_items": splits.num_items,
            "embedding_dim": 32,
            "hidden_dim": 64,
            "learning_rate": 1e-3,
        },
        train_data=splits.train,
        val_data=splits.val,
        batch_size=256,
        num_shuffle_batches=10,
        lightning_trainer_kwargs=lightning_trainer_kwargs,
    )

    os.makedirs(_STORAGE_DIR, exist_ok=True)
    run_config = ray.train.RunConfig(
        name="ncf_movielens100k",
        storage_path=_STORAGE_DIR,
    )
    scaling_config = ray.train.ScalingConfig(
        num_workers=1,
        use_gpu=False,
        resources_per_worker={"CPU": 1},
    )

    trainer = LightningTrainer(
        trainer_param=trainer_param,
        run_config=run_config,
        scaling_config=scaling_config,
    )

    log.info("Starting training...")
    result = trainer.train()
    log.info("Training finished. result=%r", result)
    return result


if __name__ == "__main__":
    main()
