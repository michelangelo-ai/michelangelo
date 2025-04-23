import json
import logging
import os
import shutil
import tempfile
from typing import Optional

import torch
from torch import nn
from torch.nn import functional as F

from transformers import AutoTokenizer, AutoModel

from michelangelo.canvas.lib.shared.utils import (
    REDUCTION_SCRIP_MODEL_FILE,
    RESHAPE_LAYER_FILE,
)


logger = logging.getLogger(__name__)


class SimpleReductionLayer(nn.Module):
    """
    Simple Reduction Layer is a simple embedding dimension reduction layer.
    """

    def __init__(self, input_dim: int, output_dim):
        super().__init__()
        self.input_dim = input_dim
        self.output_dim = output_dim
        self.linear = nn.Linear(input_dim, output_dim)

    def forward(self, embeddings):
        new_embeddings = torch.nn.functional.silu(embeddings, inplace=False)
        new_embeddings = self.linear(new_embeddings)
        return new_embeddings


class TextEncoder(nn.Module):
    """
    `TextEncoder` class wraps all text embedding model with unified interface for downstream tasks to use.
    It takes text as input and generate fix dimension embedding based on pre-trained text model and some customer settings
    such as: pooling strategy, dimension reduction strategy.
    """

    def __init__(
        self,
        pretrained_text_model_path,
        reshape_layer_input_dim: int = -1,
        reshape_layer_output_dim: int = -1,
        reduction_layer_args: Optional[dict] = None,
        pretrained_reduction_script_file: Optional[str] = None,
        pooling_strategy: str = "mean",
    ):
        super().__init__()
        self.model_path = pretrained_text_model_path
        self.pooling_strategy = pooling_strategy
        self.tokenizer = AutoTokenizer.from_pretrained(
            pretrained_text_model_path, trust_remote_code=True
        )
        self.base_encoder = AutoModel.from_pretrained(
            pretrained_text_model_path, trust_remote_code=True
        )
        self.reshape_layer_input_dim = reshape_layer_input_dim
        self.reshape_layer_output_dim = reshape_layer_output_dim
        self.pretrained_reduction_script_file = pretrained_reduction_script_file
        self.reduction_layer_args = reduction_layer_args

        if self.reshape_layer_input_dim > 0 and self.reshape_layer_output_dim > 0:
            self.reshape_layer = nn.Linear(
                self.reshape_layer_input_dim, self.reshape_layer_output_dim
            )
            reshape_layer_model_path = os.path.join(
                pretrained_text_model_path, RESHAPE_LAYER_FILE
            )
            if os.path.exists(reshape_layer_model_path):
                logger.info(
                    f"Load reshaping layer model from {reshape_layer_model_path}..."
                )
                self.reshape_layer.load_state_dict(torch.load(reshape_layer_model_path))
        else:
            self.reshape_layer = None

        if self.pretrained_reduction_script_file is not None:
            reduction_script_model_path = os.path.join(
                pretrained_text_model_path, pretrained_reduction_script_file
            )
            self.reduction_layer = torch.jit.load(reduction_script_model_path)
        elif self.reduction_layer_args is not None:
            self.reduction_layer = SimpleReductionLayer(**self.reduction_layer_args)
        else:
            self.reduction_layer = None

        if self.pooling_strategy == "mean":
            self.pool_func = self.mean_pool
        elif self.pooling_strategy == "avg":
            self.pool_func = self.average_pool
        elif self.pooling_strategy == "last_token":
            self.pool_func = self.last_token_pool
        else:
            raise NotImplementedError(
                f"Not supported pooling strategy: {self.pooling_strategy}"
            )

    def freeze_llm_layers(self, n_finetune_layers):
        assert n_finetune_layers is not None

    @staticmethod
    def mean_pool(last_hidden_states, attention_mask):
        input_mask_expanded = (
            attention_mask.unsqueeze(-1).expand(last_hidden_states.size()).float()
        )
        return torch.sum(last_hidden_states * input_mask_expanded, 1) / torch.clamp(
            input_mask_expanded.sum(1), min=1e-9
        )

    @staticmethod
    def last_token_pool(
        last_hidden_states: torch.Tensor, attention_mask: torch.Tensor
    ) -> torch.Tensor:
        """
        Most LLM GTE model use the last token embedding as the output embedding. In the following function,
        1. It checks if the last column of the attention_mask is all ones (i.e. left_padding = True). If so, it simply
           takes the last hidden state from each row (last_hidden_states[:, -1]).

        2. Otherwise, it computes the index of the “actual last token” by summing over the attention_mask
           (to find sequence lengths) and then indexing into last_hidden_states at those “last valid token” positions.
        """
        left_padding = attention_mask[:, -1].sum() == attention_mask.shape[0]

        if left_padding:
            return last_hidden_states[:, -1]
        else:
            sequence_lengths = attention_mask.sum(dim=1) - 1
            batch_size = last_hidden_states.shape[0]
            return last_hidden_states[
                torch.arange(batch_size, device=last_hidden_states.device),
                sequence_lengths,
            ]

    @staticmethod
    def average_pool(
        last_hidden_states: torch.Tensor, attention_mask: torch.Tensor
    ) -> torch.Tensor:
        """
        Average pool on last hidden states in all position
        """
        last_hidden = last_hidden_states.masked_fill(
            ~attention_mask[..., None].bool(), 0.0
        )
        return last_hidden.sum(dim=1) / attention_mask.sum(dim=1)[..., None]

    def forward(self, text, max_length: int = 1000):
        tokenized_text = self.tokenizer(
            text,
            padding=True,
            truncation=True,
            return_tensors="pt",
            max_length=max_length,
            return_token_type_ids=False,
        ).to(self.base_encoder.device)
        embeddings = self.pool_func(
            self.base_encoder(**tokenized_text).last_hidden_state,
            tokenized_text["attention_mask"],
        )
        if self.reshape_layer:
            embeddings = torch.nn.functional.silu(embeddings, inplace=False)
            with torch.autocast(device_type="cuda", dtype=embeddings.dtype):
                embeddings = self.reshape_layer(embeddings)

        if self.reduction_layer:
            with torch.autocast(device_type="cuda", dtype=embeddings.dtype):
                embeddings = self.reduction_layer(embeddings)
        return F.normalize(embeddings, p=2, dim=1)

    def encode(self, text, max_length: int = 1000):
        return self.forward(text, max_length)

    def save_model(self, model_save_local_dir):
        model = self.base_encoder
        configs = {
            "pooling_strategy": self.pooling_strategy,
            "reduction_config": {
                "reshape_layer_input_dim": self.reshape_layer_input_dim,
                "reshape_layer_output_dim": self.reshape_layer_output_dim,
                "reduction_layer_args": self.reduction_layer_args,
                "pretrained_reduction_script_file": self.pretrained_reduction_script_file,
            },
        }
        os.makedirs(model_save_local_dir, exist_ok=True)
        with open(model_save_local_dir + "/llm_tunable_model_config.json", "w") as f:
            json.dump(configs, f)

        # Check if huggingface transformer config exists inside dir_path
        if not os.path.exists(os.path.join(model_save_local_dir, "config.json")):
            shutil.copytree(self.model_path, model_save_local_dir, dirs_exist_ok=True)

        # we keeps the original model configs and files except the safetensors that changed during fine-tuning
        extensions = ["model.safetensors.index.json", ".safetensors"]
        with tempfile.TemporaryDirectory() as tmpdirname:
            # By default safe_serialization is True
            model.save_pretrained(tmpdirname)
            for file_name in os.listdir(tmpdirname):
                if any(file_name.endswith(ext) for ext in extensions):
                    source_path = os.path.join(tmpdirname, file_name)
                    destination_path = os.path.join(model_save_local_dir, file_name)
                    shutil.move(source_path, destination_path)
                    print(f"Moved: {file_name}")
        if self.reshape_layer:
            torch.save(
                self.reshape_layer.state_dict(),
                os.path.join(model_save_local_dir, RESHAPE_LAYER_FILE),
            )
        if self.reduction_layer:
            script_model = torch.jit.script(self.reduction_layer)
            torch.jit.save(
                script_model,
                os.path.join(model_save_local_dir, REDUCTION_SCRIP_MODEL_FILE),
            )


class NomicEncoder(TextEncoder):
    """
    A derived class that fit nomic model architecture: https://huggingface.co/nomic-ai/nomic-embed-text-v1.5
    """

    def freeze_llm_layers(self, n_finetune_layers):
        # freeze transformer layers
        for param in self.base_encoder.parameters():
            param.requires_grad = False

        if n_finetune_layers == -1:  # finetune entire model
            for param in self.base_encoder.parameters():
                param.requires_grad = True
        elif n_finetune_layers == 0:  # freeze entire model
            pass

        elif n_finetune_layers > 0:  # set top_n layers to finetune
            if n_finetune_layers > len(self.base_encoder.encoder.layers):
                raise ValueError(
                    "top_n value is greater than the number of layers in the model."
                )
            # unfreeze transformer layers
            for layer in self.base_encoder.encoder.layers[-n_finetune_layers:]:
                for param in layer.parameters():
                    param.requires_grad = True
        else:
            raise ValueError("Input layers value wrong.")


class Me5Encoder(TextEncoder):
    """
    A derived class that fits Me5 architecture: https://huggingface.co/intfloat/multilingual-e5-large
    """

    def freeze_llm_layers(self, n_finetune_layers):
        # freeze transformer layers
        for param in self.base_encoder.parameters():
            param.requires_grad = False

        if n_finetune_layers == -1:  # finetune entire model
            for param in self.base_encoder.parameters():
                param.requires_grad = True
        elif n_finetune_layers == 0:  # freeze entire model
            pass

        elif n_finetune_layers > 0:  # set top_n layers to finetune
            if n_finetune_layers > len(self.base_encoder.encoder.layer):
                raise ValueError(
                    "top_n value is greater than the number of layers in the model."
                )
            # unfreeze transformer layers
            for layer in self.base_encoder.encoder.layer[-n_finetune_layers:]:
                for param in layer.parameters():
                    param.requires_grad = True
        else:
            raise ValueError("Input layers value wrong.")


class QWenEncoder(TextEncoder):
    """
    A derived class that fits QWen architecture: https://huggingface.co/Alibaba-NLP/gte-Qwen2-7B-instruct
    """

    def freeze_llm_layers(self, n_finetune_layers):
        """Finetune the pretrained model with the given top_n layers.
            n_finetune_layers = -1: finetune entire model
            n_finetune_layers = 0: freeze entire model
            n_finetune_layers > 0: finetune top_n layers
        Note: This function assumes query tower and document tower has the same structure.
        """

        # Freeze entire model
        for param in self.base_encoder.parameters():
            param.requires_grad = False

        if n_finetune_layers == -1:  # finetune entire model
            for param in self.base_encoder.parameters():
                param.requires_grad = True
        elif n_finetune_layers == 0:  # freeze entire model
            pass

        elif n_finetune_layers > 0:  # set top_n layers to finetune
            if n_finetune_layers > len(self.base_encoder.layers):
                raise ValueError(
                    "top_n value is greater than the number of layers in the model."
                )
            # unfreeze transformer layers
            for layer in self.base_encoder.layers[-n_finetune_layers:]:
                for param in layer.parameters():
                    param.requires_grad = True
            # unfreeze final layer norm
            for param in self.base_encoder.norm.parameters():
                param.requires_grad = True
        else:
            raise ValueError("Input layers value wrong.")
