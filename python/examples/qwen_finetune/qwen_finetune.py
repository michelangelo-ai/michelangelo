import logging
import mlflow
import mlflow.pytorch
import datasets
import ray
import transformers
import torch
import numpy as np
from datasets import Dataset as HFDataset
from ray.data import Dataset
from transformers import AutoTokenizer, AutoModelForCausalLM, TrainingArguments, Trainer
from michelangelo.uniflow.plugins.ray import RayTask
import michelangelo.uniflow.core as uniflow
from examples.qwen_finetune.pusher import pusher
from michelangelo.uniflow.plugins.ray import UF_PLUGIN_RAY_USE_FSSPEC

log = logging.getLogger(__name__)

tokenizer_path = "Qwen/Qwen1.5-1.8B-Chat"


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
def load_and_train(
    dataset_name: str = "squad",
    tokenizer_max_length: int = 512,
    model_name: str = "qwen-1.5-1.8b-chat",
    hf_model_name: str = "Qwen/Qwen1.5-1.8B-Chat",
):
    """Load data and train Qwen model in a single task"""
    log.info("Starting combined data loading and training...")
    
    # Data loading logic
    tokenizer = transformers.AutoTokenizer.from_pretrained(tokenizer_path)
    
    # Add pad token if it doesn't exist
    if tokenizer.pad_token is None:
        tokenizer.pad_token = tokenizer.eos_token

    def format_instruction(example):
        """Format data as instruction-following examples for Qwen"""
        if dataset_name == "squad":
            # Format SQuAD as Q&A
            question = example["question"]
            context = example["context"]
            
            # Handle different answer formats
            answers = example.get("answers", {})
            if isinstance(answers, dict):
                answer_texts = answers.get("text", [])
                if isinstance(answer_texts, list) and len(answer_texts) > 0:
                    answer = answer_texts[0]
                elif isinstance(answer_texts, str):
                    answer = answer_texts
                else:
                    answer = "No answer"
            else:
                answer = "No answer"
            
            instruction = f"Answer the following question based on the context.\n\nContext: {context}\n\nQuestion: {question}\n\nAnswer:"
            response = f" {answer}"
            
        else:
            # Default format for other datasets
            instruction = example.get("instruction", example.get("text", ""))
            response = example.get("response", example.get("label", ""))
        
        return {
            "instruction": instruction,
            "response": response,
            "input_text": instruction + response
        }

    def tokenize_function(batch):
        """Tokenize the formatted examples"""
        # Handle different batch formats
        if isinstance(batch, dict):
            # Extract examples from batch dictionary format
            if "question" in batch and isinstance(batch["question"], list):
                # Batch is in columnar format (question: [q1, q2, ...], context: [c1, c2, ...])
                batch_size = len(batch["question"])
                examples = []
                for i in range(batch_size):
                    example = {}
                    for key in batch:
                        example[key] = batch[key][i]
                    examples.append(example)
            else:
                # Single example in dict format
                examples = [batch]
        else:
            # Batch is a list of examples
            examples = batch
        
        # Format instructions
        formatted_examples = [format_instruction(example) for example in examples]
        
        # Extract input texts
        input_texts = [ex["input_text"] for ex in formatted_examples]
        
        # Tokenize
        tokenized = tokenizer(
            input_texts,
            max_length=tokenizer_max_length,
            truncation=True,
            padding="max_length",
            return_tensors="np",
        )
        
        # For causal LM, labels are the same as input_ids
        tokenized["labels"] = tokenized["input_ids"].copy()
        
        return tokenized

    # Load dataset
    if dataset_name == "squad":
        try:
            data = datasets.load_dataset("squad")
        except Exception as e:
            log.warning(f"Failed to load SQuAD dataset: {e}")
            log.info("Falling back to synthetic data for demonstration")
            # Create synthetic Q&A data for demonstration
            synthetic_data = []
            for i in range(1000):  # Increase sample size
                synthetic_data.append({
                    "question": f"What is example question {i}?",
                    "context": f"This is example context {i} that contains relevant information for the question.",
                    "answers": {"text": [f"answer {i}"], "answer_start": [0]}
                })
            
            # Create a dataset-like structure
            class SyntheticDataset:
                def __init__(self, data):
                    self.data = data
                def __getitem__(self, key):
                    return {"train": self.data, "validation": self.data[:20]}
            
            data = SyntheticDataset(synthetic_data)
    else:
        # Default to a simple text dataset
        data = datasets.load_dataset("wikitext", "wikitext-2-raw-v1")

    def _load_slice(data_slice) -> Dataset:
        try:
            ds = ray.data.from_huggingface(data[data_slice])
        except Exception as e:
            log.warning(f"Failed to load slice '{data_slice}' from HuggingFace: {e}")
            log.info("Using synthetic data instead")
            # Create synthetic data directly as Ray dataset
            synthetic_samples = []
            for i in range(1000):  # Increase sample size for better training
                synthetic_samples.append({
                    "question": f"What is example question {i}?",
                    "context": f"This is example context {i} that contains relevant information for the question.",
                    "answers": {"text": [f"answer {i}"], "answer_start": [0]}
                })
            ds = ray.data.from_items(synthetic_samples)
        
        ds = ds.map_batches(tokenize_function, batch_format="numpy")
        
        # Sample a small subset for demonstration, but ensure we have at least some data
        total_rows = ds.count()
        sample_fraction = max(0.01, 10 / total_rows) if total_rows > 0 else 1.0
        ds = ds.random_sample(min(sample_fraction, 1.0), seed=42)
        
        return ds

    # Handle different dataset splits
    if dataset_name == "squad":
        train_data = _load_slice("train")
        validation_data = _load_slice("validation")
        # SQuAD doesn't have a test set, so we'll use validation for test
        test_data = validation_data
    else:
        train_data = _load_slice("train")
        validation_data = _load_slice("validation")
        test_data = _load_slice("test")

    # Training logic - Configure MLflow for local run
    experiment_name = "qwen-finetune-experiment"
    
    # Set MLflow tracking URI to local file system
    import os
    mlflow_dir = os.path.join(os.getcwd(), "mlruns")
    mlflow.set_tracking_uri(f"file://{mlflow_dir}")
    
    # Check if experiment already exists
    try:
        experiment = mlflow.get_experiment_by_name(experiment_name)
        if experiment is None:
            mlflow.create_experiment(name=experiment_name)
            print(f"Experiment '{experiment_name}' created.")
        else:
            print(f"Experiment '{experiment_name}' already exists.")
    except Exception as e:
        log.warning(f"MLflow experiment setup failed: {e}")
        # Create experiment without checking if it exists
        try:
            mlflow.create_experiment(name=experiment_name)
            print(f"Experiment '{experiment_name}' created.")
        except:
            print(f"Using default experiment")
    
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

        # Load model and tokenizer for training
        model = create_model(hf_model_name)
        train_tokenizer = AutoTokenizer.from_pretrained(hf_model_name, trust_remote_code=True)
        
        # Add pad token if it doesn't exist
        if train_tokenizer.pad_token is None:
            train_tokenizer.pad_token = train_tokenizer.eos_token
            model.resize_token_embeddings(len(train_tokenizer))

        # Convert Ray datasets to HuggingFace datasets
        train_hf_data = HFDataset.from_pandas(train_data.to_pandas())
        validation_hf_data = HFDataset.from_pandas(validation_data.to_pandas())
        test_hf_data = HFDataset.from_pandas(test_data.to_pandas())

        # Check device type for mixed precision training
        use_fp16 = torch.cuda.is_available()  # Only use fp16 on CUDA GPUs
        
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
            fp16=use_fp16,  # Only use mixed precision on CUDA
            remove_unused_columns=False,
        )

        # Create trainer
        trainer = Trainer(
            model=model,
            args=training_args,
            train_dataset=train_hf_data,
            eval_dataset=validation_hf_data,
            tokenizer=train_tokenizer,
            data_collator=transformers.DataCollatorForLanguageModeling(
                tokenizer=train_tokenizer,
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
        eval_result = trainer.evaluate(validation_hf_data)
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


@uniflow.workflow()
def qwen_finetune_workflow():
    """
    Complete workflow for fine-tuning Qwen and deploying with LLM-D
    """
    # Load data and train the model in a single task
    model_name = "qwen-1.5-1.8b-chat"
    hf_model_name = "Qwen/Qwen1.5-1.8B-Chat"
    model_uri, train_result, best_checkpoint = load_and_train(
        dataset_name="squad",  # Using SQuAD for Q&A fine-tuning
        model_name=model_name,
        hf_model_name=hf_model_name,
    )
    
    # Push model for deployment
    model_name = pusher(model_uri, model_name, hf_model_name=hf_model_name)
    
    # Note: After this workflow completes, you can deploy the model using LLM-D backend
    # by creating an InferenceServer resource with backend_type = BACKEND_TYPE_LLM_D
    
    print("Training result:", train_result)
    print(f"Model '{model_name}' ready for LLM-D deployment")
    print("ok.")


# For Local Run: python3 examples/qwen_finetune/qwen_finetune.py
# For Remote Run: python3 examples/qwen_finetune/qwen_finetune.py remote-run --storage-url <STORAGE_URL> --image <IMAGE>
if __name__ == "__main__":
    ctx = uniflow.create_context()

    # Set environment variables
    ctx.environ["DATA_SIZE"] = "10"
    
    # Disable use of fsspec in Ray Plugin
    ctx.environ[UF_PLUGIN_RAY_USE_FSSPEC] = "0"
    ctx.environ["PYTORCH_MPS_HIGH_WATERMARK_RATIO"] = "0"
    ctx.environ["MA_NAMESPACE"] = "default"
    
    # Docker and infrastructure settings
    ctx.environ["IMAGE_PULL_POLICY"] = "Never"
    ctx.environ["S3_ALLOW_BUCKET_CREATION"] = "True"
    ctx.environ["MA_API_SERVER"] = "host.docker.internal:14567"
    # Use local file-based MLflow tracking for local runs
    import os
    mlflow_dir = os.path.join(os.getcwd(), "mlruns")
    ctx.environ["MLFLOW_TRACKING_URI"] = f"file://{mlflow_dir}"
    
    # LLM-D specific settings
    ctx.environ["LLM_D_BACKEND_ENABLED"] = "True"

    ctx.run(qwen_finetune_workflow)