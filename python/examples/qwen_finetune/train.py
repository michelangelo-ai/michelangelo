import logging
import mlflow
import mlflow.pytorch
from datasets import Dataset as HFDataset
import torch
import transformers
from ray.data import Dataset
from michelangelo.uniflow.plugins.ray import RayTask
import michelangelo.uniflow.core as uniflow
import numpy as np
from transformers import AutoTokenizer, AutoModelForCausalLM, TrainingArguments, Trainer

log = logging.getLogger(__name__)


def create_model(model_name: str = "Qwen/Qwen1.5-1.8B-Chat") -> transformers.AutoModelForCausalLM:
    """Create Qwen model for fine-tuning"""
    model = AutoModelForCausalLM.from_pretrained(
        model_name,
        torch_dtype=torch.float16,
        device_map="auto",
        trust_remote_code=True
    )
    
    # Enable gradient checkpointing to save memory
    model.gradient_checkpointing_enable()
    
    return model


@uniflow.task(
    config=RayTask(
        head_cpu=2,
        head_memory="8Gi",
        head_gpu=1,
        worker_cpu=2,
        worker_memory="8Gi",
        worker_gpu=1,
        worker_instances=1,
    ),
    cache_enabled=True,
)
def train(
    train_data: Dataset,
    validation_data: Dataset,
    test_data: Dataset,
    model_name: str = "Qwen/Qwen1.5-1.8B-Chat",
):
    """Fine-tune Qwen model"""
    log.info("Starting Qwen fine-tuning...")

    experiment_name = "qwen-finetune-experiment"
    artifact_location = "s3://mlflow"

    # Check if experiment already exists
    experiment = mlflow.get_experiment_by_name(experiment_name)
    if experiment is None:
        mlflow.create_experiment(
            name=experiment_name, artifact_location=artifact_location
        )
        print(f"Experiment '{experiment_name}' created.")
    else:
        print(f"Experiment '{experiment_name}' already exists.")

    mlflow.set_experiment(experiment_name)

    # Training configuration
    batch_size = 4
    max_epochs = 1
    lr = 5e-5
    output_dir = "./qwen_finetune"

    # Start MLflow experiment
    with mlflow.start_run():
        # Log hyperparameters
        mlflow.log_param("model_name", model_name)
        mlflow.log_param("learning_rate", lr)
        mlflow.log_param("batch_size", batch_size)
        mlflow.log_param("max_epochs", max_epochs)

        # Load model and tokenizer
        model = create_model(model_name)
        tokenizer = AutoTokenizer.from_pretrained(model_name, trust_remote_code=True)
        
        # Add pad token if it doesn't exist
        if tokenizer.pad_token is None:
            tokenizer.pad_token = tokenizer.eos_token
            model.resize_token_embeddings(len(tokenizer))

        # Convert Ray datasets to HuggingFace datasets
        train_data = HFDataset.from_pandas(train_data.to_pandas())
        validation_data = HFDataset.from_pandas(validation_data.to_pandas())
        test_data = HFDataset.from_pandas(test_data.to_pandas())

        # Define training arguments
        training_args = TrainingArguments(
            output_dir=output_dir,
            evaluation_strategy="epoch",
            save_strategy="epoch",
            save_total_limit=1,
            metric_for_best_model="eval_loss",
            greater_is_better=False,
            per_device_train_batch_size=batch_size,
            per_device_eval_batch_size=batch_size,
            num_train_epochs=max_epochs,
            learning_rate=lr,
            warmup_steps=100,
            logging_dir=f"{output_dir}/logs",
            load_best_model_at_end=True,
            gradient_accumulation_steps=4,
            dataloader_pin_memory=False,
            fp16=True,  # Use mixed precision training
            remove_unused_columns=False,
        )

        # Create trainer
        trainer = Trainer(
            model=model,
            args=training_args,
            train_dataset=train_data,
            eval_dataset=validation_data,
            tokenizer=tokenizer,
            data_collator=transformers.DataCollatorForLanguageModeling(
                tokenizer=tokenizer,
                mlm=False,  # Causal LM, not masked LM
            ),
        )

        # Train the model
        train_result = trainer.train()
        trainer.save_model(output_dir)

        log.info("Training complete. Model saved.")

        # Log metrics
        mlflow.log_metric("train_loss", train_result.training_loss)
        
        # Evaluate on validation set
        eval_result = trainer.evaluate(validation_data)
        mlflow.log_metric("eval_loss", eval_result["eval_loss"])
        mlflow.log_metric("eval_perplexity", np.exp(eval_result["eval_loss"]))

        # Get the best checkpoint path
        best_checkpoint = training_args.output_dir
        log.info(f"Best checkpoint path: {best_checkpoint}")

        # Register model in MLflow Model Registry
        model_uri = "runs:/{}/qwen_model".format(mlflow.active_run().info.run_id)
        mlflow.pytorch.log_model(model, "qwen_model")
        mlflow.register_model(model_uri, "QwenModelRegistry")

        return model_uri, train_result, best_checkpoint