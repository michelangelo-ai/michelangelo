"""Neural Collaborative Filtering LightningModule for MovieLens-100k."""

from __future__ import annotations

import pytorch_lightning as pl
import torch
import torch.nn.functional as functional
from torch import nn


class NCFLightningModule(pl.LightningModule):
    """A tiny NCF: user/item embeddings -> 2-layer MLP -> sigmoid rating in [0, 1].

    Trained against MovieLens ratings normalized to ``[0, 1]`` with MSE loss.
    The batch dicts produced by ``Dataset.iter_torch_batches`` for this dataset
    contain ``user_idx`` (int64), ``item_idx`` (int64), and ``rating_norm`` (float32).
    """

    def __init__(
        self,
        num_users: int,
        num_items: int,
        embedding_dim: int = 32,
        hidden_dim: int = 64,
        learning_rate: float = 1e-3,
    ) -> None:
        """Initialize the user/item embeddings and the rating-head MLP.

        Args:
            num_users: Number of unique users in the training data.
            num_items: Number of unique items in the training data.
            embedding_dim: Width of the user/item embedding vectors.
            hidden_dim: Width of the two hidden layers in the rating-head MLP.
            learning_rate: Adam learning rate.
        """
        super().__init__()
        self.save_hyperparameters()
        self.user_emb = nn.Embedding(num_users, embedding_dim)
        self.item_emb = nn.Embedding(num_items, embedding_dim)
        self.mlp = nn.Sequential(
            nn.Linear(2 * embedding_dim, hidden_dim),
            nn.ReLU(),
            nn.Linear(hidden_dim, hidden_dim),
            nn.ReLU(),
            nn.Linear(hidden_dim, 1),
        )
        nn.init.normal_(self.user_emb.weight, std=0.01)
        nn.init.normal_(self.item_emb.weight, std=0.01)

    def forward(self, user_idx: torch.Tensor, item_idx: torch.Tensor) -> torch.Tensor:
        """Predict a normalized rating in ``[0, 1]`` for each ``(user, item)`` pair."""
        u = self.user_emb(user_idx)
        i = self.item_emb(item_idx)
        x = torch.cat([u, i], dim=-1)
        logits = self.mlp(x).squeeze(-1)
        return torch.sigmoid(logits)

    def _step(self, batch: dict, stage: str) -> torch.Tensor:
        user_idx = batch["user_idx"].long()
        item_idx = batch["item_idx"].long()
        target = batch["rating_norm"].float()
        preds = self(user_idx, item_idx)
        loss = functional.mse_loss(preds, target)
        # sync_dist=True so the metric is averaged across Ray Train workers.
        self.log(
            f"{stage}_loss",
            loss,
            prog_bar=True,
            on_step=False,
            on_epoch=True,
            sync_dist=True,
        )
        return loss

    def training_step(self, batch, batch_idx):
        """Compute and log the training loss for one batch."""
        del batch_idx
        return self._step(batch, "train")

    def validation_step(self, batch, batch_idx):
        """Compute and log the validation loss for one batch."""
        del batch_idx
        return self._step(batch, "val")

    def configure_optimizers(self):
        """Return an Adam optimizer using ``self.hparams.learning_rate``."""
        return torch.optim.Adam(self.parameters(), lr=self.hparams.learning_rate)


def create_ncf_model(
    num_users: int,
    num_items: int,
    embedding_dim: int = 32,
    hidden_dim: int = 64,
    learning_rate: float = 1e-3,
) -> NCFLightningModule:
    """Factory passed as ``LightningTrainerParam.create_model_fn``.

    The trainer's per-worker loop calls this on each Ray Train worker with the
    kwargs from ``LightningTrainerParam.create_model_fn_kwargs``.
    """
    return NCFLightningModule(
        num_users=num_users,
        num_items=num_items,
        embedding_dim=embedding_dim,
        hidden_dim=hidden_dim,
        learning_rate=learning_rate,
    )
