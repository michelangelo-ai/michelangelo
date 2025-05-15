import logging
import os
import mlflow
import mlflow.pytorch  # For logging PyTorch models
from datasets import Dataset as HFDataset
import torch
import transformers
from ray.data import Dataset
from michelangelo.uniflow.plugins.ray import RayTask
import michelangelo.uniflow.core as uniflow
import numpy as np
import torch
import mlflow.pytorch
from transformers import AutoTokenizer

log = logging.getLogger(__name__)

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
def deploy(

):
    log.info("Starting deploying...")

    model_uri = "models:/BertModelRegistry/latest"
    model = mlflow.pytorch.load_model(model_uri)
    model.eval()

    tokenizer = AutoTokenizer.from_pretrained("bert-base-cased")

    # Example input
    sample_text = "This is a sample input for tracing."
    inputs = tokenizer(sample_text, return_tensors="pt")

    # Ensure correct input keys: input_ids, attention_mask
    traced_model = torch.jit.trace(model, (inputs['input_ids'], inputs['attention_mask']))
    traced_model.save("model.pt")

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
