"""PyTorch Lightning regression model for the California Housing Lightning workflow.

A minimal multi-layer perceptron regressor. This is the first
``pytorch_lightning.LightningModule`` subclass with an example/e2e caller
anywhere in this repo — ``tabular_trainer``'s existing tests use mocked
models only (see ``workflow/tasks/tabular_trainer/tests/fixtures.py``).
"""

from __future__ import annotations

import torch
import torch.nn.functional as F  # noqa: N812
from pytorch_lightning import LightningModule
from torch import nn

__all__ = ["TorchRegressionModel"]


class TorchRegressionModel(LightningModule):
    """Small MLP regressor trained via ``train_tabular()``'s Lightning backend.

    The Ray Data batches passed to ``training_step``/``validation_step`` are
    dicts of column-name -> tensor (Ray's default ``iter_torch_batches``
    output when no custom collate function is configured). This model reads
    ``feature_columns`` and ``label_column`` from its own constructor
    arguments — set via ``LightningTrainerConfig.model_kwargs`` — to know
    which batch keys to assemble into the input tensor and which key holds
    the regression target.

    Attributes:
        feature_columns: Ordered list of input feature column names.
        label_column: Name of the regression target column.
        hidden_dim: Width of the first hidden layer. The second hidden layer
            is half this width.
        learning_rate: Adam learning rate.

    Example:
        >>> model = TorchRegressionModel(
        ...     feature_columns=["MedInc", "HouseAge"],
        ...     label_column="target",
        ... )
    """

    def __init__(
        self,
        feature_columns: list[str],
        label_column: str,
        hidden_dim: int = 64,
        learning_rate: float = 1e-3,
    ):
        """Build the MLP and store constructor args as hyperparameters."""
        super().__init__()
        self.save_hyperparameters()
        self.net = nn.Sequential(
            nn.Linear(len(feature_columns), hidden_dim),
            nn.ReLU(),
            nn.Linear(hidden_dim, hidden_dim // 2),
            nn.ReLU(),
            nn.Linear(hidden_dim // 2, 1),
        )

    def forward(self, x: torch.Tensor) -> torch.Tensor:
        """Return the model's scalar prediction for a batch of feature vectors."""
        return self.net(x)

    def _assemble_batch(
        self, batch: dict[str, torch.Tensor]
    ) -> tuple[torch.Tensor, torch.Tensor]:
        """Stack the configured feature columns and extract the label column."""
        x = torch.stack(
            [batch[c].float() for c in self.hparams.feature_columns], dim=1
        )
        y = batch[self.hparams.label_column].float().view(-1, 1)
        return x, y

    def training_step(
        self, batch: dict[str, torch.Tensor], batch_idx: int
    ) -> torch.Tensor:
        """Compute and log the MSE training loss for one batch."""
        x, y = self._assemble_batch(batch)
        loss = F.mse_loss(self(x), y)
        self.log("train_loss", loss, on_step=False, on_epoch=True)
        return loss

    def validation_step(
        self, batch: dict[str, torch.Tensor], batch_idx: int
    ) -> torch.Tensor:
        """Compute and log the MSE validation loss for one batch."""
        x, y = self._assemble_batch(batch)
        loss = F.mse_loss(self(x), y)
        self.log("val_loss", loss, on_step=False, on_epoch=True)
        return loss

    def configure_optimizers(self) -> torch.optim.Optimizer:
        """Return the Adam optimizer configured with ``learning_rate``."""
        return torch.optim.Adam(self.parameters(), lr=self.hparams.learning_rate)
