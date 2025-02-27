from ray.data import Dataset
from transformers import AutoTokenizer
from pytorch_lightning.strategies import DeepSpeedStrategy
import torch
import michelangelo.uniflow.core as uniflow
from examples.nomic_ai.model import HuggingFaceLightningModel
from michelangelo.uniflow.plugins.ray import RayTask
import pytorch_lightning as pl
from torch.utils.data import DataLoader


@uniflow.task(
    config=RayTask(
        head_cpu=1,
        head_memory="3Gi",
        worker_cpu=1,
        worker_memory="3Gi",
        worker_instances=1,
    ),
)
def train(
        train_data: Dataset,
        validation_data: Dataset,
        test_data: Dataset,
        model_name="nomic-ai/nomic-bert-2048",
) -> dict:
    tokenizer = AutoTokenizer.from_pretrained(model_name)

    train_dataloader = DataLoader(train_data, batch_size=8, shuffle=True)
    val_dataloader = DataLoader(validation_data, batch_size=8)

    model = HuggingFaceLightningModel(model_name)

    use_deepspeed = torch.cuda.is_available()
    strategy = DeepSpeedStrategy(stage=2) if use_deepspeed else "auto"

    trainer = pl.Trainer(
        max_epochs=1,
        precision=16 if torch.cuda.is_available() else 32,
        accelerator="gpu" if torch.cuda.is_available() else "cpu",
        devices=torch.cuda.device_count() if torch.cuda.is_available() else 1,
        strategy=strategy,
        log_every_n_steps=10,
    )

    trainer.fit(model, train_dataloader, val_dataloader)

    model.save_pretrained("./trained_model")
    tokenizer.save_pretrained("./trained_model")

    return {"status": "Training completed successfully"}
