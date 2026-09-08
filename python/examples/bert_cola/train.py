"""Training task for fine-tuning a BERT model on the CoLA dataset."""

import logging

import numpy as np
import torch
import transformers
from datasets import Dataset as HFDataset
from ray.data import Dataset

import michelangelo.uniflow.core as uniflow
from examples.bert_cola.model import BertColaModel
from michelangelo.uniflow.plugins.ray import RayTask
from michelangelo.workflow.variables import ModelVariable

log = logging.getLogger(__name__)


# Model creation function
def create_model(
    lr: float, eps: float
) -> transformers.AutoModelForSequenceClassification:
    """Create BERT model for sequence classification.

    Args:
        lr: Learning rate (unused in current implementation).
        eps: Epsilon parameter (unused in current implementation).

    Returns:
        BERT model initialized for binary classification with 2 labels.
    """
    return transformers.AutoModelForSequenceClassification.from_pretrained(
        "bert-base-cased", num_labels=2
    )


@uniflow.task(
    config=RayTask(
        head_cpu=1,
        head_memory="4Gi",
        worker_cpu=1,
        worker_memory="4Gi",
        worker_instances=1,
        # breakpoint=True,
    ),
)
def train(
    train_data: Dataset,
    validation_data: Dataset,
    test_data: Dataset,
):
    """Fine-tune BERT model on CoLA dataset.

    Trains BERT for sequence classification on the linguistic acceptability task
    using HuggingFace Trainer with automatic mixed precision if GPUs are available.

    Args:
        train_data: Ray Dataset containing training examples.
        validation_data: Ray Dataset containing validation examples.
        test_data: Ray Dataset containing test examples.

    Returns:
        Tuple of ``(train_result, model_variable)``: the HuggingFace
        ``TrainOutput`` from ``trainer.train()``, and a ``ModelVariable``
        wrapping the fine-tuned model and tokenizer, persisted under
        ``UF_STORAGE_URL`` -- this task and the ones downstream of it aren't
        guaranteed to share a filesystem, so the local save path itself
        isn't returned.
    """
    log.info("Starting training...")

    # Training configuration
    batch_size = 8
    max_epochs = 1
    lr = 2e-5
    eps = 1e-8
    output_dir = "./bert_cola"

    # Load model
    model = create_model(lr=lr, eps=eps)

    train_data = HFDataset.from_pandas(train_data.to_pandas())
    validation_data = HFDataset.from_pandas(validation_data.to_pandas())
    test_data = HFDataset.from_pandas(test_data.to_pandas())

    # Define training arguments
    training_args = transformers.TrainingArguments(
        output_dir=output_dir,
        eval_strategy="epoch",
        save_strategy="epoch",
        save_total_limit=1,  # Keep only the best checkpoint
        metric_for_best_model="eval_loss",  # Customize based on your needs
        greater_is_better=False,
        per_device_train_batch_size=batch_size,
        per_device_eval_batch_size=batch_size,
        num_train_epochs=max_epochs,
        learning_rate=lr,
        logging_dir=f"{output_dir}/logs",
        load_best_model_at_end=True,
    )

    tokenizer = transformers.AutoTokenizer.from_pretrained("bert-base-cased")
    trainer = transformers.Trainer(
        model=model,
        args=training_args,
        train_dataset=train_data,
        eval_dataset=validation_data,
        tokenizer=tokenizer,
        compute_metrics=_compute_metrics,
    )

    train_result = trainer.train()

    model_variable = ModelVariable.create(BertColaModel(model, tokenizer))
    model_variable.save()

    log.info("Training complete. Best model uploaded to %s.", model_variable.path)

    return train_result, model_variable


def _compute_metrics(eval_pred):
    """Compute Matthews Correlation Coefficient (MCC) directly using NumPy."""
    logits, labels = eval_pred

    # Ensure logits and labels are NumPy arrays
    if isinstance(logits, torch.Tensor):
        logits = logits.detach().cpu().numpy()
    if isinstance(labels, torch.Tensor):
        labels = labels.detach().cpu().numpy()

    # Convert logits to class predictions
    predictions = np.argmax(logits, axis=-1)

    # Compute MCC manually
    tp = np.sum((predictions == 1) & (labels == 1))
    tn = np.sum((predictions == 0) & (labels == 0))
    fp = np.sum((predictions == 1) & (labels == 0))
    fn = np.sum((predictions == 0) & (labels == 1))

    numerator = (tp * tn) - (fp * fn)
    denominator = np.sqrt((tp + fp) * (tp + fn) * (tn + fp) * (tn + fn))

    mcc = numerator / denominator if denominator != 0 else 0.0

    return {"matthews_correlation": mcc}
