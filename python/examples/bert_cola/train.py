import logging
import mlflow
import mlflow.pytorch  # For logging PyTorch models
from datasets import Dataset as HFDataset
import torch
import transformers
from ray.data import Dataset
from michelangelo.uniflow.plugins.ray import RayTask
import michelangelo.uniflow.core as uniflow
import numpy as np

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
        head_cpu=1,
        head_memory="4Gi",
        worker_cpu=1,
        worker_memory="4Gi",
        worker_instances=1,
        breakpoint=False,
    ),
    cache_enabled=True,
)
def train(
    train_data: Dataset,
    validation_data: Dataset,
    test_data: Dataset,
):
    log.info("Starting training...")

    experiment_name = "bert-cola-experiment"
    artifact_location = "s3://mlflow"

    # Check if experiment already exists
    experiment = mlflow.get_experiment_by_name(experiment_name)

    if experiment is None:
        # Create if it does not exist
        mlflow.create_experiment(
            name=experiment_name, artifact_location=artifact_location
        )
        print(f"Experiment '{experiment_name}' created.")
    else:
        print(f"Experiment '{experiment_name}' already exists.")

    # Set the active experiment
    mlflow.set_experiment(experiment_name)

    # Training configuration
    batch_size = 8
    max_epochs = 1
    lr = 2e-5
    eps = 1e-8
    output_dir = "./bert_cola"

    # Start MLflow experiment
    with mlflow.start_run():
        # Log hyperparameters
        mlflow.log_param("learning_rate", lr)
        mlflow.log_param("eps", eps)
        mlflow.log_param("batch_size", batch_size)
        mlflow.log_param("max_epochs", max_epochs)

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

        # Log metrics
        mlflow.log_metric("train_loss", train_result.training_loss)
        eval_result = trainer.evaluate(validation_data)
        mlflow.log_metric("eval_loss", eval_result["eval_loss"])

        # Get the best checkpoint path
        best_checkpoint = training_args.output_dir + "/checkpoint-best"
        log.info(f"Best checkpoint path: {best_checkpoint}")

        # Register model in MLflow Model Registry
        model_uri = "runs:/{}/bert_model".format(mlflow.active_run().info.run_id)
        mlflow.pytorch.log_model(model, "bert_model")
        mlflow.register_model(model_uri, "BertModelRegistry")

        return model_uri, train_result, best_checkpoint


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
