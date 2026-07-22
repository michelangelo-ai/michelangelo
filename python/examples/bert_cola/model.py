"""BertColaModel — implements the Michelangelo Model interface for BERT CoLA.

This allows CustomTritonPackager to generate a proper Triton Python backend
package directly from the HuggingFace checkpoint, including config.pbtxt,
model.py, and all dependency bundling.
"""

from __future__ import annotations

import os

import numpy as np
from numpy import ndarray

from michelangelo.lib.model_manager.interface.custom_model import Model


class BertColaModel(Model):
    """BERT fine-tuned on CoLA (linguistic acceptability, binary classification).

    Inputs  (per sample, tokenizer max_length=128):
        input_ids      : int64  [128]
        attention_mask : int64  [128]
        token_type_ids : int64  [128]

    Output:
        logits         : float32 [2]
    """

    def __init__(self, model, tokenizer):
        self._model = model
        self._tokenizer = tokenizer

    def save(self, path: str) -> None:
        self._model.save_pretrained(path)
        self._tokenizer.save_pretrained(path)

    @classmethod
    def load(cls, path: str) -> "BertColaModel":
        import torch
        from transformers import AutoModelForSequenceClassification, AutoTokenizer

        tokenizer = AutoTokenizer.from_pretrained(path)
        model = AutoModelForSequenceClassification.from_pretrained(path)
        model.eval()
        if torch.cuda.is_available():
            model = model.cuda()
        return cls(model, tokenizer)

    def predict(self, inputs: dict[str, ndarray]) -> dict[str, ndarray]:
        import torch

        device = next(self._model.parameters()).device
        input_ids = torch.tensor(inputs["input_ids"], dtype=torch.long).unsqueeze(0).to(device)
        attention_mask = torch.tensor(inputs["attention_mask"], dtype=torch.long).unsqueeze(0).to(device)
        token_type_ids = torch.tensor(inputs["token_type_ids"], dtype=torch.long).unsqueeze(0).to(device)

        with torch.no_grad():
            outputs = self._model(
                input_ids=input_ids,
                attention_mask=attention_mask,
                token_type_ids=token_type_ids,
            )

        return {"logits": outputs.logits.squeeze(0).cpu().numpy().astype(np.float32)}
