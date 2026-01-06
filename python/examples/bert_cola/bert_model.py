"""Custom BERT CoLA model for Triton deployment."""

import json
import os

import numpy as np
import torch
import transformers
from numpy import ndarray

from michelangelo.lib.model_manager.interface.custom_model import Model


class BertColaModel(Model):
    """BERT model for CoLA linguistic acceptability classification.

    This model wraps a fine-tuned BERT model for binary classification
    on the CoLA (Corpus of Linguistic Acceptability) dataset.
    """

    def __init__(self, model=None):
        """Initialize the model.

        Args:
            model: The model instance.
        """
        self.model = model
        self.device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
        if self.model is not None:
            self.model.to(self.device)
            self.model.eval()

    def save(self, path: str):
        """Save the model to the given path.

        Args:
            path: Directory path to save the model artifacts.
        """
        os.makedirs(path, exist_ok=True)

        # Save model state dict
        torch.save(self.model.state_dict(), os.path.join(path, "model.pt"))

        # Save config
        config = {
            "model_name": "bert-base-cased",
            "num_labels": 2,
        }
        with open(os.path.join(path, "config.json"), "w") as f:
            json.dump(config, f)

    @classmethod
    def load(cls, path: str) -> "BertColaModel":
        """Load the model from the given path.

        Args:
            path: Directory path containing the saved model artifacts.

        Returns:
            Loaded BertColaModel instance.
        """
        # Load config
        with open(os.path.join(path, "config.json")) as f:
            config = json.load(f)

        # Initialize model architecture
        model = transformers.AutoModelForSequenceClassification.from_pretrained(
            config["model_name"],
            num_labels=config["num_labels"],
        )

        # Load weights
        state_dict = torch.load(
            os.path.join(path, "model.pt"),
            map_location="cpu",
        )
        model.load_state_dict(state_dict)

        return cls(model=model)

    def predict(self, inputs: dict[str, ndarray]) -> dict[str, ndarray]:
        """Run inference on tokenized input.

        Args:
            inputs: Dictionary with 'input_ids' and 'attention_mask' arrays.
                    - input_ids: shape [batch_size, seq_len] or [seq_len]
                    - attention_mask: shape [batch_size, seq_len] or [seq_len]

        Returns:
            Dictionary with 'logits' predictions.
                    - logits: shape [batch_size, 2] (binary classification)
        """
        input_ids = inputs["input_ids"]
        attention_mask = inputs["attention_mask"]

        # Ensure batch dimension
        if input_ids.ndim == 1:
            input_ids = input_ids[np.newaxis, :]
            attention_mask = attention_mask[np.newaxis, :]

        # Convert to tensors
        input_ids_tensor = torch.tensor(input_ids, dtype=torch.long).to(self.device)
        attention_mask_tensor = torch.tensor(attention_mask, dtype=torch.long).to(
            self.device
        )

        # Inference
        with torch.no_grad():
            outputs = self.model(
                input_ids=input_ids_tensor,
                attention_mask=attention_mask_tensor,
            )
            logits = outputs.logits

        return {
            "logits": logits.cpu().numpy().astype(np.float32),
        }
