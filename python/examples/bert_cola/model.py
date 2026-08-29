"""Custom Model Manager wrapper for the fine-tuned BERT CoLA classifier.

Wraps a HuggingFace ``AutoModelForSequenceClassification`` + tokenizer as a
``michelangelo.lib.model_manager.interface.custom_model.Model`` so it can be
packaged via ``CustomTritonPackager``/``custom_assembler``. BERT takes
multiple named tensor inputs (``input_ids``, ``attention_mask``,
``token_type_ids``), which isn't a good TorchScript-tracing fit -- the custom
(Python-backend) path avoids tracing entirely and just calls the HF model
directly in ``predict()``, mirroring
``examples.model_manager.simple_custom.model.DummyEchoModel``.
"""

from __future__ import annotations

import numpy as np
import torch
import transformers

from michelangelo.lib.model_manager.interface.custom_model import Model

__all__ = ["BertColaModel"]


class BertColaModel(Model):
    """Fine-tuned BERT sequence classifier for the CoLA linguistic-acceptability task.

    - **Inputs** (each an int64 array of shape ``[tokenizer_max_length]``):
      - input_ids
      - attention_mask
      - token_type_ids
    - **Outputs**:
      - logits: float32 ``[2]`` (one score per CoLA label)
    """

    def __init__(
        self,
        model: transformers.PreTrainedModel,
        tokenizer: transformers.PreTrainedTokenizerBase,
    ) -> None:
        """Wrap an already-loaded model and tokenizer."""
        self._model = model
        self._tokenizer = tokenizer
        self._model.eval()

    def save(self, path: str) -> None:
        """Persist the model and tokenizer under ``path`` via HF's own format.

        Uses ``save_pretrained()`` rather than ``torch.save``/pickle, per the
        ``Model.save()`` contract's guidance to prefer format-specific
        serialization.
        """
        self._model.save_pretrained(path)
        self._tokenizer.save_pretrained(path)

    @classmethod
    def load(cls, path: str) -> BertColaModel:
        """Load the model and tokenizer from ``path``."""
        model = transformers.AutoModelForSequenceClassification.from_pretrained(path)
        tokenizer = transformers.AutoTokenizer.from_pretrained(path)
        return cls(model, tokenizer)

    def predict(self, inputs: dict[str, np.ndarray]) -> dict[str, np.ndarray]:
        """Run the classifier on tokenized inputs, returning per-class logits."""
        with torch.no_grad():
            model_inputs = {
                name: torch.as_tensor(inputs[name], dtype=torch.long).unsqueeze(0)
                for name in ("input_ids", "attention_mask", "token_type_ids")
                if name in inputs
            }
            logits = self._model(**model_inputs).logits.squeeze(0)
        return {"logits": logits.numpy().astype(np.float32)}
