"""
GPT Lightning Module for distributed training with LoRA support
"""

import logging
import torch
import pytorch_lightning as pl
from transformers import (
    AutoModelForCausalLM,
    AutoTokenizer,
    get_linear_schedule_with_warmup
)
from peft import LoraConfig, get_peft_model, TaskType
from typing import Optional, Dict, Any

log = logging.getLogger(__name__)


class GPTLightningModule(pl.LightningModule):
    """
    Lightning module for GPT training with LoRA support
    Compatible with distributed training using Ray and PyTorch Lightning
    """

    def __init__(
        self,
        model_name: str = "gpt2",
        learning_rate: float = 5e-5,
        use_lora: bool = True,
        lora_rank: int = 16,
        lora_alpha: int = 32,
        lora_dropout: float = 0.1,
        warmup_steps: int = 100,
        **kwargs
    ):
        super().__init__()
        self.save_hyperparameters()

        self.model_name = model_name
        self.learning_rate = learning_rate
        self.use_lora = use_lora
        self.lora_rank = lora_rank
        self.lora_alpha = lora_alpha
        self.lora_dropout = lora_dropout
        self.warmup_steps = warmup_steps

        # Initialize model and tokenizer
        self.setup_model_and_tokenizer()

    def setup_model_and_tokenizer(self):
        """Setup the model and tokenizer"""
        log.info(f"Loading model: {self.model_name}")

        # Load tokenizer
        self.tokenizer = AutoTokenizer.from_pretrained(self.model_name)
        if self.tokenizer.pad_token is None:
            self.tokenizer.pad_token = self.tokenizer.eos_token

        # Load model
        self.model = AutoModelForCausalLM.from_pretrained(
            self.model_name,
            torch_dtype=torch.float16 if torch.cuda.is_available() else torch.float32,
        )

        # Setup LoRA if enabled
        if self.use_lora:
            log.info("Setting up LoRA configuration")
            lora_config = LoraConfig(
                r=self.lora_rank,
                lora_alpha=self.lora_alpha,
                lora_dropout=self.lora_dropout,
                target_modules=["c_attn", "c_proj"],  # GPT-2 specific
                bias="none",
                task_type=TaskType.CAUSAL_LM
            )
            self.model = get_peft_model(self.model, lora_config)

            # Print trainable parameters
            if hasattr(self.model, 'print_trainable_parameters'):
                self.model.print_trainable_parameters()

    def forward(self, input_ids, attention_mask=None, labels=None):
        """Forward pass"""
        return self.model(
            input_ids=input_ids,
            attention_mask=attention_mask,
            labels=labels
        )

    def training_step(self, batch, batch_idx):
        """Training step"""
        outputs = self.forward(
            input_ids=batch["input_ids"],
            attention_mask=batch["attention_mask"],
            labels=batch["labels"]
        )

        loss = outputs.loss

        # Log metrics
        self.log("train_loss", loss, on_step=True, on_epoch=True, prog_bar=True)

        # Log learning rate
        if self.lr_schedulers():
            self.log("lr", self.lr_schedulers().get_last_lr()[0], on_step=True)

        return loss

    def validation_step(self, batch, batch_idx):
        """Validation step"""
        outputs = self.forward(
            input_ids=batch["input_ids"],
            attention_mask=batch["attention_mask"],
            labels=batch["labels"]
        )

        loss = outputs.loss

        # Calculate perplexity
        perplexity = torch.exp(loss)

        # Log metrics
        self.log("val_loss", loss, on_epoch=True, prog_bar=True)
        self.log("val_perplexity", perplexity, on_epoch=True, prog_bar=True)

        return {"val_loss": loss, "val_perplexity": perplexity}

    def configure_optimizers(self):
        """Configure optimizers and schedulers"""
        # Create optimizer
        optimizer = torch.optim.AdamW(
            self.parameters(),
            lr=self.learning_rate,
            weight_decay=0.01
        )

        # Calculate total steps for scheduler
        # This is a rough estimate - in practice you'd get from trainer
        total_steps = self.trainer.estimated_stepping_batches

        # Create scheduler
        scheduler = get_linear_schedule_with_warmup(
            optimizer,
            num_warmup_steps=self.warmup_steps,
            num_training_steps=total_steps
        )

        return {
            "optimizer": optimizer,
            "lr_scheduler": {
                "scheduler": scheduler,
                "interval": "step",
                "frequency": 1
            }
        }

    def get_model_info(self) -> Dict[str, Any]:
        """Get model information for logging"""
        if self.use_lora and hasattr(self.model, 'peft_config'):
            # Get parameter counts for LoRA model
            total_params = sum(p.numel() for p in self.model.parameters())
            trainable_params = sum(p.numel() for p in self.model.parameters() if p.requires_grad)

            return {
                "model_name": self.model_name,
                "use_lora": self.use_lora,
                "lora_rank": self.lora_rank,
                "lora_alpha": self.lora_alpha,
                "total_parameters": total_params,
                "trainable_parameters": trainable_params,
                "trainable_percentage": (trainable_params / total_params) * 100
            }
        else:
            total_params = sum(p.numel() for p in self.model.parameters())
            return {
                "model_name": self.model_name,
                "use_lora": self.use_lora,
                "total_parameters": total_params,
                "trainable_parameters": total_params,
                "trainable_percentage": 100.0
            }


def create_gpt_model(
    model_name: str = "gpt2",
    learning_rate: float = 5e-5,
    use_lora: bool = True,
    lora_rank: int = 16,
    lora_alpha: int = 32,
    lora_dropout: float = 0.1,
    warmup_steps: int = 100,
    **kwargs
) -> pl.LightningModule:
    """
    Factory function to create GPT Lightning module
    Compatible with internal training patterns
    """
    return GPTLightningModule(
        model_name=model_name,
        learning_rate=learning_rate,
        use_lora=use_lora,
        lora_rank=lora_rank,
        lora_alpha=lora_alpha,
        lora_dropout=lora_dropout,
        warmup_steps=warmup_steps,
        **kwargs
    )