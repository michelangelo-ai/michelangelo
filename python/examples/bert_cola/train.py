import logging
import pytorch_lightning

from ray.data import Dataset
from ray.train import CheckpointConfig
from ray.train.lightning import RayFSDPStrategy

from examples.bert_cola.model import SentimentModel
from uber.ai.michelangelo.maf.ray.train import create_run_config, create_scaling_config
from uber.ai.michelangelo.sdk.trainer.torch.pytorch_lightning.lightning_trainer import (
    LightningTrainer,
    LightningTrainerParam,
)

from uber.ai.uniflow.ray_task import Ray
from michelangelo.uniflow import uniflow


log = logging.getLogger(__name__)


def create_model(metric_path: str, metric_config_name: str, lr: float, eps: float) -> pytorch_lightning.LightningModule:
    return SentimentModel(metric_path=metric_path, metric_config_name=metric_config_name, lr=lr, eps=eps)


@uniflow.task(
    config=Ray(
        head_cpu=16,
        head_memory="32Gi",
        head_gpu=1,
        worker_cpu=8,
        worker_memory="16Gi",
        worker_gpu=1,
        worker_instances=3,
    ),
)
def train(
        train_data: Dataset,
        validation_data: Dataset,
        train_loop_config: dict,
):
    scaling_config = create_scaling_config(
        trainer_cpu=2,
        cpu_per_worker=4,
    )
    log.info("scaling_config: %r", scaling_config)

    run_config = create_run_config(
        checkpoint_config=CheckpointConfig(
            num_to_keep=1,  # Save the top-1 checkpoints according to the evaluation metric.
            checkpoint_score_attribute="matthews_correlation",
            checkpoint_score_order="max",
        ),
    )
    log.info("run_config: %r", run_config)

    lightning_trainer_kwargs = {
        "strategy": RayFSDPStrategy(
            sharding_strategy="SHARD_GRAD_OP",
        ),
        "precision": 16,
    }
    if uniflow.is_local_run():
        lightning_trainer_kwargs = None

    trainer_param = LightningTrainerParam(
        create_model,
        {
            "metric_path": train_loop_config["metric_path"],
            "metric_config_name": train_loop_config["metric_config_name"],
            "lr": train_loop_config["lr"],
            "eps": train_loop_config["eps"],
        },
        train_data,
        validation_data,
        batch_size=train_loop_config["batch_size"],
        num_epochs=train_loop_config["max_epochs"],
        lightning_trainer_kwargs=lightning_trainer_kwargs,
    )

    trainer = LightningTrainer(trainer_param)

    return trainer.fit(run_config, scaling_config)
