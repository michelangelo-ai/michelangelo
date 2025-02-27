import os
import torch
import transformers
import pytorch_lightning as pl
from pytorch_lightning.strategies import DeepSpeedStrategy
from ray.train.lightning import RayDeepSpeedStrategy
from ray.train import CheckpointConfig
from datasets import load_dataset
from transformers import AutoModel, AutoTokenizer
from torch.utils.data import DataLoader
from typing import Dict, Any

# ✅ Correct Model Name
MODEL_NAME = "nomic-ai/nomic-bert-2048"  # A BERT-based model

# ✅ Correct Model Class
class HuggingFaceLightningModel(pl.LightningModule):
    def __init__(self, model_name: str, learning_rate: float = 2e-5):
        super().__init__()
        self.save_hyperparameters()
        self.model = AutoModel.from_pretrained(model_name, trust_remote_code=True)  # ✅ FIXED: Use AutoModel for BERT models
        self.tokenizer = AutoTokenizer.from_pretrained(model_name, trust_remote_code=True)
        self.learning_rate = learning_rate

    def forward(self, input_ids, attention_mask):
        return self.model(input_ids=input_ids, attention_mask=attention_mask)

    def training_step(self, batch, batch_idx):
        outputs = self(**batch)
        embeddings = outputs.last_hidden_state.mean(dim=1)  # [batch_size, hidden_dim]
        input_embeddings = batch["input_ids"].float().unsqueeze(-1).expand(-1, -1, embeddings.shape[-1])
        loss = torch.nn.functional.mse_loss(embeddings, input_embeddings.mean(dim=1))

        self.log("train_loss", loss)
        return loss

    def validation_step(self, batch, batch_idx):
        outputs = self(**batch)
        embeddings = outputs.last_hidden_state.mean(dim=1)
        input_embeddings = batch["input_ids"].float().unsqueeze(-1).expand(-1, -1, embeddings.shape[-1])
        loss = torch.nn.functional.mse_loss(embeddings, input_embeddings.mean(dim=1))

        self.log("val_loss", loss)
        return loss

    def configure_optimizers(self):
        return torch.optim.AdamW(self.parameters(), lr=self.learning_rate)

# ✅ Load and Preprocess Data
def load_data(dataset_name: str = "wikitext", tokenizer=None, max_length: int = 512,  dataset_size=200):
    dataset = load_dataset(dataset_name, "wikitext-2-raw-v1")

    def tokenize_function(examples):
        return tokenizer(examples["text"], padding="max_length", truncation=True, max_length=max_length)

    dataset = dataset.map(tokenize_function, batched=True)

    for split in ["train", "validation", "test"]:
        if split in dataset:
            dataset[split] = dataset[split].select(range(min(dataset_size, len(dataset[split]))))


    dataset.set_format(type="torch", columns=["input_ids", "attention_mask"])
    return dataset

def train():
    tokenizer = AutoTokenizer.from_pretrained(MODEL_NAME)
    dataset = load_data(tokenizer=tokenizer)

    train_dataloader = DataLoader(dataset["train"], batch_size=8, shuffle=True)
    val_dataloader = DataLoader(dataset["validation"], batch_size=8)

    model = HuggingFaceLightningModel(MODEL_NAME)

    use_deepspeed = torch.cuda.is_available()  # ✅ Use DeepSpeed only if GPU is available
    strategy = DeepSpeedStrategy(stage=2) if use_deepspeed else "auto"

    trainer = pl.Trainer(
        max_epochs=1,
        precision=16 if torch.cuda.is_available() else 32,
        accelerator="gpu" if torch.cuda.is_available() else "cpu",
        devices=torch.cuda.device_count() if torch.cuda.is_available() else 1,
        strategy=strategy,  # ✅ Now using DeepSpeedStrategy
        log_every_n_steps=10,
    )

    trainer.fit(model, train_dataloader, val_dataloader)

    # ✅ Save Model
    model.model.save_pretrained("./trained_model")
    tokenizer.save_pretrained("./trained_model")

    print("✅ Training completed successfully.")

if __name__ == "__main__":
    train()
