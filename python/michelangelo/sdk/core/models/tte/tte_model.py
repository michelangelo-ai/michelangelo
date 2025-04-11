from abc import ABC, abstractmethod
import logging
import os
from typing import Optional, TYPE_CHECKING

import torch
from torch import nn
from torch.nn import functional as F

from michelangelo.sdk.core.lib.utils import get_class

if TYPE_CHECKING:
    from michelangelo.sdk.core.models.tte.text_encoder import TextEncoder

logger = logging.getLogger(__name__)


class AbstractTwoTowerModel(nn.Module, ABC):
    """
    This is a interface for All text based of TTE model.
    It must contain functions: `encode_query`, `encode_item`, `forward`, `get_loss` and `save_model`
    """

    def __init__(
        self, query_text_column, item_text_column, query_text_max_len, item_text_max_len
    ):
        super().__init__()
        self.query_text_column = query_text_column
        self.item_text_column = item_text_column
        self.query_text_max_len = query_text_max_len
        self.item_text_max_len = item_text_max_len

    def encode_query(self, batch: dict[str, torch.Tensor]):
        """Expected to return norm = 1 embeddings for each row in the dataset"""
        raise NotImplementedError

    def encode_item(self, batch: dict[str, torch.Tensor]):
        """Expected to return norm = 1 embeddings for each row in the dataset"""
        raise NotImplementedError

    @abstractmethod
    def forward(self, batch: dict[str, torch.Tensor]) -> torch.Tensor:
        raise NotImplementedError

    @abstractmethod
    def get_loss(
        self, batch: dict[str, torch.Tensor]
    ) -> tuple[torch.Tensor, torch.Tensor, torch.Tensor]:
        raise NotImplementedError

    @abstractmethod
    def get_log_metrics(
        self,
        loss: torch.Tensor,
        predictions: torch.Tensor,
        labels: torch.Tensor,
        training: bool,
    ) -> dict:
        raise NotImplementedError

    @abstractmethod
    def save_model(self, *args, **kwargs):
        raise NotImplementedError


class InBatchAbstractTwoTower(AbstractTwoTowerModel, ABC):
    """
    This is a interface for all InBatch TTE model where we use in-batch negatives to train text model.
    """

    def __init__(
        self,
        query_text_column,
        item_text_column,
        query_text_max_len,
        item_text_max_len,
        softmax_temperature: float = 0.05,
        metrics_k_all: Optional[list[int]] = None,
        item_selection_prob_column: Optional[str] = None,
    ):
        super().__init__(
            query_text_column, item_text_column, query_text_max_len, item_text_max_len
        )
        self.softmax_temperature = softmax_temperature
        self.metrics_k_all = metrics_k_all or []
        self.item_selection_prob_column = item_selection_prob_column
        self.loss = nn.CrossEntropyLoss()

    @staticmethod
    def get_prediction_score(query_emb, item_emb) -> torch.Tensor:
        q_dot_i = query_emb.matmul(item_emb.transpose(0, 1))
        q_dot_q = (query_emb * query_emb).sum(dim=1, keepdim=True)
        i_dot_i = (item_emb * item_emb).sum(dim=1, keepdim=True).transpose(0, 1)
        predictions = -(q_dot_q + i_dot_i - 2.0 * q_dot_i).clamp(min=1e-6).sqrt()
        return predictions

    def forward(self, batch: dict[str, torch.Tensor]) -> torch.Tensor:
        # type hint is necessary, otherwise TorchScript compilation assumes all inputs are torch.Tensor
        """
        Standard forward pass for lightning framework implementation which is used in training_step and one off
        """
        item_embeddings = self.encode_item(batch)
        query_embeddings = self.encode_query(batch)

        # Add prediction_score as an output so that binary classification evaluator works
        # This app will be used to train embeddings optimized on SPR metrics such as cvr, ni
        scores = self.get_prediction_score(query_embeddings, item_embeddings)
        return scores

    def get_loss(self, batch: dict[str, torch.Tensor]):
        scores = self.forward(batch)
        device = scores.device
        batch_size = scores.shape[0]
        softmax_temperature = (
            torch.tensor(self.softmax_temperature).expand(batch_size).to(device)
        )
        item_prob = self.get_item_prob(batch, batch_size, device)
        labels = self.get_labels(scores)

        predictions = self.apply_softmax_temperature(scores, softmax_temperature)
        y_hat = self.apply_log_q_correction(predictions, item_prob)
        loss = self.loss(y_hat.float(), labels)
        return loss, scores, labels

    @staticmethod
    def apply_softmax_temperature(
        predictions: torch.Tensor, softmax_temperature: torch.Tensor
    ) -> torch.Tensor:
        neg_batch_size = predictions.size(1)
        softmax_temperature = softmax_temperature.expand(neg_batch_size, -1).transpose(
            0, 1
        )
        return predictions / softmax_temperature

    @staticmethod
    def get_labels(y_hat: torch.Tensor) -> torch.Tensor:
        # Diagonal entry is the true label for within batch negatives (see
        # https://github.com/tensorflow/recommenders/blob/v0.6.0/tensorflow_recommenders/tasks/retrieval.py#L138)
        batch_size = y_hat.size(0)
        device = y_hat.device
        labels = torch.arange(0, batch_size, device=device)
        return labels

    def get_item_prob(self, batch, batch_size, device):
        if self.item_selection_prob_column:
            return batch[self.item_selection_prob_column].float().to(device)
        return 0.5 * torch.ones((batch_size,), dtype=torch.float, device=device)

    @staticmethod
    def compute_hit_rate(
        predictions: torch.Tensor, labels: torch.Tensor, k: int
    ) -> torch.Tensor:
        _, order = predictions.topk(k, dim=1, sorted=False)
        labels_expanded = labels.expand(k, -1).transpose(0, 1)
        return (1.0 * (order == labels_expanded)).sum(dim=1).mean(dim=0)

    @classmethod
    def log_q_correction_wrapper(
        cls,
        item_prob: torch.Tensor,
    ) -> torch.Tensor:
        """Classmethod introduced so that subclasses can inject their own implementation of log_q_prob"""
        # The `log_q.fill_diagonal_(0.)` step is important as the correction
        # should only be applied to negative logits
        # See Eqn (5) in https://research.google/pubs/pub50257/ which is the
        # latest paper with sampling bias correction from google
        batch_size = item_prob.size(0)
        log_q_raw = torch.log(-torch.expm1(torch.log1p(-item_prob) * batch_size))
        return (
            log_q_raw.unsqueeze(1)
            .permute([1, 0])
            .repeat([batch_size, 1])
            .fill_diagonal_(0.0)
        )

    def apply_log_q_correction(
        self,
        predictions: torch.Tensor,
        item_prob: torch.Tensor,
    ) -> torch.Tensor:
        # if loss is triplet or if selection probabilities are not supplied don't do logQ correction
        if self.item_selection_prob_column is None:
            return predictions
        # We subtract the log of the probability of the item appearing in the same batch
        # Inspired by logQ sampled softmax correction: https://research.google/pubs/pub48840/
        return predictions - 1.0 * self.log_q_correction_wrapper(item_prob)

    def get_log_metrics(
        self,
        loss: torch.Tensor,
        predictions: torch.Tensor,
        labels: torch.Tensor,
        training: bool,
    ) -> dict:
        neg_batch_size = predictions.size(1)
        loss_type = "train" if training else "val"
        log = {f"{loss_type}_loss": loss.detach()}
        for k_ in self.metrics_k_all:
            k = k_ if k_ < neg_batch_size else neg_batch_size
            hit_rate = self.compute_hit_rate(predictions, labels, k)
            log[f"{loss_type}_hit_rate_at_{k_}"] = hit_rate
        return log


class SharedInBatchTwoTower(InBatchAbstractTwoTower):
    """
    This class assume query and item tower shares the same text model (weights is shared.)
    This is also common practise for OSS embedding models.
    """

    def __init__(
        self,
        text_model_class_name: str,
        query_text_column,
        item_text_column,
        query_text_max_len,
        item_text_max_len,
        softmax_temperature: float = 0.05,
        metrics_k_all: Optional[list[int]] = None,
        item_selection_prob_column: Optional[str] = None,
        n_finetune_layers: int = 1,
        **kwargs,
    ):
        super().__init__(
            query_text_column,
            item_text_column,
            query_text_max_len,
            item_text_max_len,
            softmax_temperature=softmax_temperature,
            metrics_k_all=metrics_k_all,
            item_selection_prob_column=item_selection_prob_column,
        )
        model_class = (
            f"michelangelo.sdk.core.models.tte.text_encoder.{text_model_class_name}"
        )
        text_encoder_class = get_class(model_class)
        logger.info(f"Now create {text_model_class_name} object using {kwargs}")
        self.text_model: TextEncoder = text_encoder_class(**kwargs)
        self.text_model.freeze_llm_layers(n_finetune_layers)

    def encode_query(self, batch: dict[str, torch.Tensor]):
        """Expected to return norm = 1 embeddings for each row in the dataset"""
        sentences = batch[self.query_text_column]
        embeddings = self.text_model.encode(
            sentences, max_length=self.query_text_max_len
        )
        embeddings = F.normalize(embeddings, p=2, dim=1)
        return embeddings

    def encode_item(self, batch: dict[str, torch.Tensor]):
        """Expected to return norm = 1 embeddings for each row in the dataset"""
        sentences = batch[self.item_text_column]
        embeddings = self.text_model.encode(
            sentences, max_length=self.item_text_max_len
        )
        embeddings = F.normalize(embeddings, p=2, dim=1)
        return embeddings

    def save_model(self, model_save_local_dir):
        self.text_model.save_model(os.path.join(model_save_local_dir, "text_model"))


class InBatchTwoTower(InBatchAbstractTwoTower, ABC):
    """
    This class allow query and item tower have different text models
    """

    def __init__(
        self,
        text_model_class_name: str,
        query_text_column,
        item_text_column,
        query_text_max_len,
        item_text_max_len,
        softmax_temperature: float = 0.05,
        metrics_k_all: Optional[list[int]] = None,
        item_selection_prob_column: Optional[str] = None,
        n_finetune_layers: int = 1,
        **kwargs,
    ):
        """
        text_model_class_name: class name of the text embedding model
        query_text_column: column name of the query text
        item_text_column: column name of the item text
        query_text_max_len: max length of the tokens in query text
        item_text_max_len: max length of the tokens in item text
        item_selection_prob_column: column name of the popularity probability
        **kwargs: the args for text embedding model
        """
        super().__init__(
            query_text_column,
            item_text_column,
            query_text_max_len,
            item_text_max_len,
            softmax_temperature=softmax_temperature,
            metrics_k_all=metrics_k_all,
            item_selection_prob_column=item_selection_prob_column,
        )
        model_class = (
            f"michelangelo.sdk.core.models.tte.text_encoder.{text_model_class_name}"
        )
        text_encoder_class = get_class(model_class)
        logger.info(f"Now create {text_model_class_name} object using {kwargs}")
        self.query_model: TextEncoder = text_encoder_class(**kwargs)
        self.item_model: TextEncoder = text_encoder_class(**kwargs)
        self.query_model.freeze_llm_layers(n_finetune_layers)
        self.item_model.freeze_llm_layers(n_finetune_layers)

    def encode_query(self, batch: dict[str, torch.Tensor]):
        """Expected to return norm = 1 embeddings for each row in the dataset"""
        sentences = batch[self.query_text_column]
        embeddings = self.query_model.encode(
            sentences, max_length=self.query_text_max_len
        )
        embeddings = F.normalize(embeddings, p=2, dim=1)
        return embeddings

    def encode_item(self, batch: dict[str, torch.Tensor]):
        """Expected to return norm = 1 embeddings for each row in the dataset"""
        sentences = batch[self.item_text_column]
        embeddings = self.item_model.encode(
            sentences, max_length=self.item_text_max_len
        )
        embeddings = F.normalize(embeddings, p=2, dim=1)
        return embeddings

    def save_model(self, model_save_local_dir):
        self.query_model.save_model(os.path.join(model_save_local_dir, "query_model"))
        self.item_model.save_model(os.path.join(model_save_local_dir, "item_model"))


def create_tte_model(tte_class_name, **kwargs) -> AbstractTwoTowerModel:
    """
    Create a TTE model from class name and kwargs
    """
    model_class = f"michelangelo.sdk.core.models.tte.tte_model.{tte_class_name}"
    tte_class = get_class(model_class)
    logger.info(f"Now create {tte_class_name} object using {kwargs}")
    tte_model = tte_class(**kwargs)
    return tte_model
