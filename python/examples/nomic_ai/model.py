"""PyTorch Lightning model definitions for Nomic BERT training.

Implements Lightning module wrapper for HuggingFace transformers with
distributed training support via DeepSpeed.
"""

import pytorch_lightning as pl
import torch
from transformers import AutoModel, AutoTokenizer


class HuggingFaceLightningModel(pl.LightningModule):
    """PyTorch Lightning module for training Nomic BERT models.

    Wraps a HuggingFace transformer model for distributed training with
    PyTorch Lightning. Supports DeepSpeed for efficient multi-GPU training
    and automatic mixed precision.

    Attributes:
        model: Pretrained HuggingFace transformer model.
        tokenizer: HuggingFace tokenizer for text encoding.
        learning_rate: Learning rate for AdamW optimizer.
    """

    def __init__(self, model_name: str, learning_rate: float = 2e-5):
        """Initialize the Lightning model.

        Args:
            model_name: HuggingFace model identifier.
            learning_rate: Learning rate for optimizer. Defaults to 2e-5.
        """
        super().__init__()
        self.save_hyperparameters()
        self.model = AutoModel.from_pretrained(model_name, trust_remote_code=True)
        self.tokenizer = AutoTokenizer.from_pretrained(
            model_name, trust_remote_code=True
        )
        self.learning_rate = learning_rate

    def forward(self, input_ids, attention_mask):
        """Forward pass through the model.

        Args:
            input_ids: Input token IDs.
            attention_mask: Attention mask for padding.

        Returns:
            Model outputs with embeddings.
        """
        return self.model(input_ids=input_ids, attention_mask=attention_mask)

    def training_step(self, batch, batch_idx):
        """Execute one training step.

        Args:
            batch: Batch of training data.
            batch_idx: Index of the current batch.

        Returns:
            Training loss for the batch.
        """
        outputs = self(**batch)
        embeddings = outputs.last_hidden_state.mean(dim=1)  # [batch_size, hidden_dim]
        input_embeddings = (
            batch["input_ids"]
            .float()
            .unsqueeze(-1)
            .expand(-1, -1, embeddings.shape[-1])
        )
        loss = torch.nn.functional.mse_loss(embeddings, input_embeddings.mean(dim=1))

        self.log("train_loss", loss)
        return loss

    def validation_step(self, batch, batch_idx):
        """Execute one validation step.

        Args:
            batch: Batch of validation data.
            batch_idx: Index of the current batch.

        Returns:
            Validation loss for the batch.
        """
        outputs = self(**batch)
        embeddings = outputs.last_hidden_state.mean(dim=1)
        input_embeddings = (
            batch["input_ids"]
            .float()
            .unsqueeze(-1)
            .expand(-1, -1, embeddings.shape[-1])
        )
        loss = torch.nn.functional.mse_loss(embeddings, input_embeddings.mean(dim=1))

        self.log("val_loss", loss)
        return loss

    def configure_optimizers(self):
        """Configure the optimizer for training.

        Returns:
            AdamW optimizer with configured learning rate.
        """
        return torch.optim.AdamW(self.parameters(), lr=self.learning_rate)
