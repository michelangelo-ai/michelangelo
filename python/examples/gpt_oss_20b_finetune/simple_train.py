"""Distributed training for GPT-OSS-20B fine-tuning using Ray Lightning."""

import logging
import os
from typing import TYPE_CHECKING

import mlflow
from pytorch_lightning.loggers import MLFlowLogger
from ray.train import CheckpointConfig
from ray.train.lightning import RayFSDPStrategy

import michelangelo.uniflow.core as uniflow
from examples.gpt_oss_20b_finetune.model import create_gpt_model
from michelangelo.maf.ray.train import create_run_config, create_scaling_config
from michelangelo.sdk.trainer.torch.pytorch_lightning.lightning_trainer import (
    LightningTrainer,
    LightningTrainerParam,
)
from michelangelo.sdk.workflow.variables import DatasetVariable
from michelangelo.uniflow.plugins.ray import RayTask

if TYPE_CHECKING:
    from ray.data import Dataset

log = logging.getLogger(__name__)


def log_checkpoint_to_mlflow(checkpoint_path: str, run_id: str) -> str:
    """Log checkpoint to MLflow artifacts (automatically saved to S3).

    Args:
        checkpoint_path: Local path to the checkpoint directory or file.
        run_id: MLflow run ID to log artifacts to.

    Returns:
        MLflow artifact URI for the checkpoint.

    Raises:
        Exception: If MLflow logging fails.
    """
    log.info(f"Logging checkpoint to MLflow artifacts: {checkpoint_path}")
    log.info(f"Using MLflow run ID: {run_id}")

    # Use MLflow client to log to specific run

    # Log checkpoint as MLflow artifact (goes to S3 automatically)
    if os.path.isdir(checkpoint_path):
        # Log entire directory
        with mlflow.start_run(run_id=run_id):
            mlflow.log_artifacts(checkpoint_path, "checkpoint")
        artifact_path = "checkpoint"
    else:
        # Log single file
        with mlflow.start_run(run_id=run_id):
            mlflow.log_artifact(checkpoint_path, "checkpoint")
        artifact_path = f"checkpoint/{os.path.basename(checkpoint_path)}"

    # Get MLflow artifact URI
    artifact_uri = f"runs:/{run_id}/{artifact_path}"

    log.info(f"Checkpoint logged to MLflow: {artifact_uri}")
    return artifact_uri


@uniflow.task(
    config=RayTask(
        head_cpu=2,
        head_memory="8Gi",
        worker_cpu=2,
        worker_memory="8Gi",
        worker_instances=2,
    )
)
def simple_train_gpt(
    train_dv: DatasetVariable,
    validation_dv: DatasetVariable,
    model_name: str = "gpt2",
    num_epochs: int = 1,
    batch_size: int = 1,
    learning_rate: float = 5e-5,
    use_lora: bool = True,
    lora_rank: int = 16,
    num_workers: int = 2,
    use_gpu: bool = True,
):
    """Distributed training function using Ray Lightning.

    Args:
        train_dv: Training dataset variable.
        validation_dv: Validation dataset variable.
        model_name: Base model name (e.g., "gpt2").
        num_epochs: Number of training epochs.
        batch_size: Training batch size.
        learning_rate: Learning rate for optimization.
        use_lora: Whether to use LoRA (Low-Rank Adaptation).
        lora_rank: LoRA rank for parameter-efficient fine-tuning.
        num_workers: Number of worker nodes for distributed training.
        use_gpu: Whether to use GPU acceleration.

    Returns:
        Dictionary with checkpoint path and MLflow run ID.
    """
    log.info(f"Starting distributed training with model: {model_name}")
    log.info(f"Training with {num_workers} workers, use_gpu: {use_gpu}")

    train_dv.load_ray_dataset()
    train_data: Dataset = train_dv.value

    validation_dv.load_ray_dataset()
    validation_data: Dataset = validation_dv.value

    # Detect accelerator type for proper configuration
    import torch

    # Detect accelerator type and choose appropriate strategy
    if torch.backends.mps.is_available():
        # Apple Silicon - use CPU to avoid MPS/FSDP conflicts
        use_gpu = False
        log.info("Detected Apple Silicon (MPS) - using CPU for compatibility")
    elif not torch.cuda.is_available():
        use_gpu = False
        log.info("No CUDA available - using CPU")

    # Create scaling configuration for Ray
    scaling_config = create_scaling_config(
        trainer_cpu=2,
        cpu_per_worker=4,
        num_workers=num_workers,
        use_gpu=use_gpu,
    )
    log.info("scaling_config: %r", scaling_config)

    # Setup MLflow environment for Ray Train

    # Create run configuration with checkpointing (no MLflow callback needed)
    run_config = create_run_config(
        name=f"gpt-distributed-{model_name.replace('/', '-')}",
        checkpoint_config=CheckpointConfig(
            num_to_keep=1,
            checkpoint_score_attribute="val_loss",
            checkpoint_score_order="min",
        ),
    )
    log.info("run_config: %r", run_config)

    # Setup MLflow logger for Lightning training
    experiment_name = "gpt-finetune-experiment"

    # Create MLflow logger that will handle run creation automatically
    mlflow_logger = MLFlowLogger(
        experiment_name=experiment_name,
        tracking_uri=os.getenv("MLFLOW_TRACKING_URI", "http://localhost:5001"),
        run_name=f"training-{model_name}",
        tags={
            "model_name": model_name,
            "use_lora": str(use_lora),
            "lora_rank": str(lora_rank),
            "training_type": "distributed",
        },
    )

    # Log hyperparameters to MLflow
    mlflow_logger.log_hyperparams(
        {
            "model_name": model_name,
            "learning_rate": learning_rate,
            "use_lora": use_lora,
            "lora_rank": lora_rank,
            "batch_size": batch_size,
            "num_epochs": num_epochs,
        }
    )

    # Lightning trainer configuration for distributed training
    if torch.backends.mps.is_available():
        # Apple Silicon - use CPU strategy to avoid MPS/FSDP conflicts
        lightning_trainer_kwargs = {
            "accelerator": "cpu",
            "precision": 32,  # MPS doesn't support mixed precision well
            "log_every_n_steps": 10,
            "val_check_interval": 0.25,
            "logger": mlflow_logger,
        }
    elif use_gpu and torch.cuda.is_available():
        # CUDA GPU - use FSDP
        lightning_trainer_kwargs = {
            "strategy": RayFSDPStrategy(
                sharding_strategy="SHARD_GRAD_OP",
            ),
            "precision": "16-mixed",
            "log_every_n_steps": 10,
            "val_check_interval": 0.25,
            "logger": mlflow_logger,
        }
    else:
        # CPU fallback
        lightning_trainer_kwargs = {
            "accelerator": "cpu",
            "precision": 32,
            "log_every_n_steps": 10,
            "val_check_interval": 0.25,
            "logger": mlflow_logger,
        }

    # Create Lightning trainer parameters
    trainer_param = LightningTrainerParam(
        create_model=create_gpt_model,
        model_kwargs={
            "model_name": model_name,
            "learning_rate": learning_rate,
            "use_lora": use_lora,
            "lora_rank": lora_rank,
        },
        train_data=train_data,
        validation_data=validation_data,
        batch_size=batch_size,
        num_epochs=num_epochs,
        lightning_trainer_kwargs=lightning_trainer_kwargs,
    )

    # Create Lightning trainer
    trainer = LightningTrainer(trainer_param)

    # Log model information
    log.info("Model configuration:")
    log.info(f"  Model name: {model_name}")
    log.info(f"  Learning rate: {learning_rate}")
    log.info(f"  Use LoRA: {use_lora}")
    log.info(f"  LoRA rank: {lora_rank}")
    log.info(f"  Batch size: {batch_size}")
    log.info(f"  Number of epochs: {num_epochs}")

    # Start distributed training (MLflow logger will handle run creation)
    log.info("Starting distributed Lightning training...")
    result = trainer.train(run_config, scaling_config)
    log.info("Distributed training completed successfully")

    # Get the MLflow run ID from the logger
    run_id = mlflow_logger.run_id
    log.info(f"✅ Training completed, MLflow run: {run_id}")

    # Return checkpoint path for evaluation (much simpler!)
    checkpoint = (
        result.get_best_checkpoint(metric="val_loss", mode="min") or result.checkpoint
    )

    # Convert Checkpoint object to directory path
    if hasattr(checkpoint, "as_directory"):
        # If it's a Checkpoint object, get the directory path
        checkpoint_path = checkpoint.path
    else:
        # If it's already a path string
        checkpoint_path = checkpoint

    # Log checkpoint to MLflow artifacts (automatically saved to S3)
    log.info("🔄 Logging checkpoint to MLflow...")
    mlflow_artifact_uri = log_checkpoint_to_mlflow(checkpoint_path, run_id)
    log.info(f"✅ Checkpoint logged to MLflow: {mlflow_artifact_uri}")

    return {
        "checkpoint_path": mlflow_artifact_uri,
        "mlflow_run_id": run_id,
    }
