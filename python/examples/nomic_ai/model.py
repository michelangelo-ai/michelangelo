import pytorch_lightning as pl
import torch
from transformers import AutoModel, AutoTokenizer


class HuggingFaceLightningModel(pl.LightningModule):
    def __init__(self, model_name: str, learning_rate: float = 2e-5):
        super().__init__()
        self.save_hyperparameters()
        self.model = AutoModel.from_pretrained(model_name, trust_remote_code=True)
        self.tokenizer = AutoTokenizer.from_pretrained(
            model_name, trust_remote_code=True
        )
        self.learning_rate = learning_rate

    def forward(self, input_ids, attention_mask):
        return self.model(input_ids=input_ids, attention_mask=attention_mask)

    def training_step(self, batch, batch_idx):
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
        return torch.optim.AdamW(self.parameters(), lr=self.learning_rate)
