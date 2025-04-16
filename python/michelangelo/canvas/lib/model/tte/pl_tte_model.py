import logging
import os
import tempfile

import torch.optim as optim
import pytorch_lightning as pl
from pytorch_lightning.utilities.deepspeed import (
    convert_zero_checkpoint_to_fp32_state_dict,
)

from michelangelo.canvas.lib.model.tte.tte_model import (
    AbstractTwoTowerModel,
    create_tte_model,
)

logger = logging.getLogger(__name__)
# setting environment variable for deepspeed optimizer,
# see issue: https://github.com/deepspeedai/DeepSpeed/issues/6486 and
# https://discuss.huggingface.co/t/accelerate-deepspeed-cache-mount/63284
os.environ["TRITON_CACHE_DIR"] = tempfile.TemporaryDirectory().name


class TwoTowerPLModule(pl.LightningModule):
    """
    A wrapper class to wrap torch nn.Module into torch lightening Module so that we could use ray torch lightening trainer
    to train the model.
    We defined TTE specific loss and metrics, and also enable deepspeed optimizer for efficient training
    In the implementation, we overwrites  `configure_optimizers` so that it supports deepspeed cpu offload optimizer
    We also overwrite `training_step` to make the lightening module to fit TTE specific training.

    """

    def __init__(
        self,
        tte_class_name,
        learning_rate: float = 0.00005,
        optimizer_str: str = "adam",
        **kwargs,
    ):
        super().__init__()
        # Important: This property activates manual optimization.
        self.automatic_optimization = False
        self.tte_model: AbstractTwoTowerModel = create_tte_model(
            tte_class_name, **kwargs
        )
        self.optimizer_str = optimizer_str
        self.learning_rate = learning_rate

    # None of the below needs to be changed in the subclass

    def _train_val_step(self, batch, training: bool):
        """
        we write TTE specific forward and loss computation
        """
        loss, scores, labels = self.tte_model.get_loss(batch)
        logs = self.tte_model.get_log_metrics(loss, scores, labels, training)
        self.log_metrics(logs, training)
        return loss

    def training_step(self, batch):
        self.train()
        optimizer = self.optimizers()
        optimizer.zero_grad()
        loss = self._train_val_step(batch, training=True)
        self.manual_backward(loss)
        optimizer.step()

    def validation_step(self, batch):
        """
        Standard pytorch lightning validation_step for a batch
        """
        self.eval()
        self._train_val_step(batch, training=False)
        self.train()

    def test_step(self, batch):
        """
        Standard pytorch lightning validation_step for a batch
        """
        self.eval()
        self._train_val_step(batch, training=False)
        self.train()

    def configure_optimizers(self):
        if self.optimizer_str == "adam":
            optimizer = optim.Adagrad(self.parameters(), lr=self.learning_rate)
        elif self.optimizer_str == "deepspeed":
            from deepspeed.ops.lamb import FusedLamb

            optimizer = FusedLamb(self.parameters(), lr=self.learning_rate)
        elif self.optimizer_str == "deepspeed_cpu_offload":
            from deepspeed.ops.adam import DeepSpeedCPUAdam

            optimizer = DeepSpeedCPUAdam(self.parameters(), lr=self.learning_rate)
        else:
            optimizer = optim.SGD(self.parameters(), lr=self.learning_rate)
        return [optimizer], []

    def log_metrics(self, log, training: bool) -> None:
        """
        log: dict from metric name to metric value
        """
        for metric in log:
            if training:
                self.log(
                    metric, log[metric], prog_bar=True, on_step=True, on_epoch=True
                )
            else:
                self.log(
                    metric, log[metric], prog_bar=True, on_step=False, on_epoch=True
                )

    def save_final_model(self, model_save_local_dir):
        tte_model_dir = self.tte_model.save_model(model_save_local_dir)
        return tte_model_dir


def create_model(**kwargs) -> pl.LightningModule:
    return TwoTowerPLModule(**kwargs)


def load_deepspeed_model_from_checkpoint(lightening_ckt_file, create_model_kwargs):
    """
    lightening_ckt_file: a model director output by ray trained using deepspeed.
    """
    model_state_dict_file = os.path.join(lightening_ckt_file, "model.pt")
    logger.info(
        f"Loading deepspeed model from {lightening_ckt_file} to {model_state_dict_file}!"
    )
    model_state_dict = convert_zero_checkpoint_to_fp32_state_dict(
        lightening_ckt_file, model_state_dict_file
    )
    assert model_state_dict
    tower_pl_model = TwoTowerPLModule.load_from_checkpoint(
        model_state_dict_file,
        **create_model_kwargs,
    )
    return tower_pl_model
