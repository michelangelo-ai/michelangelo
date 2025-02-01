import logging

import datasets
import pytorch_lightning
import torch
import torch.nn.functional as F
import transformers


from examples.bert_cola.data import tokenizer_path

log = logging.getLogger(__name__)


class SentimentModel(pytorch_lightning.LightningModule):
    def __init__(
        self,
        metric_path: str,
        metric_config_name: str,
        lr=2e-5,
        eps=1e-8,
    ):
        super().__init__()
        self.lr = lr
        self.eps = eps
        self.num_classes = 2
        self.model = transformers.AutoModelForSequenceClassification.from_pretrained(
            tokenizer_path,
            num_labels=self.num_classes,
        )
        self.model.train()
        self.metric = datasets.load_metric(
            path=metric_path,
            config_name=metric_config_name,
        )
        self.predictions = []
        self.references = []

    def forward(self, batch):
        input_ids, attention_mask = batch["input_ids"], batch["attention_mask"]
        outputs = self.model(input_ids, attention_mask=attention_mask)
        return outputs.logits

    def training_step(self, batch, _batch_idx):
        labels = batch["label"]
        logits = self.forward(batch)
        loss = F.cross_entropy(logits.view(-1, self.num_classes), labels)
        self.log("train_loss", loss)
        return loss

    def validation_step(self, batch, _batch_idx):
        labels = batch["label"]
        logits = self.forward(batch)
        preds = torch.argmax(logits, dim=1)
        self.predictions.append(preds)
        self.references.append(labels)

    def on_validation_epoch_end(self):
        predictions = torch.concat(self.predictions).view(-1)
        references = torch.concat(self.references).view(-1)
        # self.metric.compute() returns a dictionary: e.g. {"matthews_correlation": 0.53}
        matthews_correlation = self.metric.compute(
            predictions=predictions,
            references=references,
        )
        self.log_dict(matthews_correlation, sync_dist=True)
        self.predictions.clear()
        self.references.clear()

    def configure_optimizers(self):
        return torch.optim.AdamW(self.parameters(), lr=self.lr, eps=self.eps)
