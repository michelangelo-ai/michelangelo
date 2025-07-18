"""
Training task for fine-tuning a BERT model on the CoLA dataset.
"""

import logging
import os

from datasets import Dataset as HFDataset
import torch
import transformers
from ray.data import Dataset
from michelangelo.uniflow.plugins.ray import RayTask
import michelangelo.uniflow.core as uniflow
import numpy as np

import fsspec

log = logging.getLogger(__name__)


# Model creation function
def create_model(
    lr: float, eps: float
) -> transformers.AutoModelForSequenceClassification:
    return transformers.AutoModelForSequenceClassification.from_pretrained(
        "bert-base-cased", num_labels=2
    )


@uniflow.task(
    config=RayTask(
        head_cpu=2,
        head_memory="8Gi",
        worker_cpu=2,
        worker_memory="8Gi",
        worker_instances=1,
        worker_min_instances=1,
        worker_max_instances=1,
        # breakpoint=True,
    ),
)
def train(
    train_data: Dataset,
    validation_data: Dataset,
    test_data: Dataset,
):
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
        evaluation_strategy="epoch",
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

    trainer = transformers.Trainer(
        model=model,
        args=training_args,
        train_dataset=train_data,
        eval_dataset=validation_data,
        tokenizer=transformers.AutoTokenizer.from_pretrained("bert-base-cased"),
        compute_metrics=_compute_metrics,
    )

    train_result = trainer.train()
    trainer.save_model(output_dir)

    log.info("Training complete. Best model saved.")

    # Get the best checkpoint path
    best_checkpoint = training_args.output_dir + "/checkpoint-best"
    log.info(f"Best checkpoint path: {best_checkpoint}")

    return train_result, best_checkpoint


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
