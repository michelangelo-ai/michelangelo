"""End-to-end MovieLens-100k training using the lib/trainer/torch snapshot.

Run from ``python/`` (the OSS Poetry root):

    python -m examples.movielens.train

Trains a tiny NCF on CPU with a single Ray Train worker. Designed as the
smallest viable smoke test for
:class:`michelangelo.lib.trainer.torch.pytorch_lightning.lightning_trainer.LightningTrainer`.
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

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
log = logging.getLogger("examples.movielens.train")

_STORAGE_DIR = "/tmp/movielens_runs"


def main() -> dict:
    splits = load_movielens_100k()

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
        lightning_trainer_kwargs={
            # Don't pass accelerator/devices here: ray.train.lightning.prepare_trainer
            # overrides them based on the worker's resource assignment from ScalingConfig.
            "max_epochs": 3,
            "log_every_n_steps": 20,
        },
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
