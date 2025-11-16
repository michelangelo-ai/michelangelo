"""
Model packaging module for GPT-OSS-20B fine-tuning
Uses Michelangelo model_manager SDK to package the trained model for deployment
"""

import logging
import os
import torch
import numpy as np
from typing import Dict, Any, Optional
from transformers import AutoModelForCausalLM, AutoTokenizer
from peft import PeftModel

import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask
from michelangelo.lib.model_manager.packager.mlflow import MLflowModelPackager

log = logging.getLogger(__name__)


class GPTFinetunedModel:
    """
    Custom GPT Model class for MLflow packaging and local inference
    Handles model loading, saving, and prediction functionality
    """

    def __init__(self, model_path: str = None, model_name: str = "gpt2"):
        self.model_path = model_path
        self.model_name = model_name
        self.model = None
        self.tokenizer = None
        self.max_length = 512

        if model_path:
            self._load_model()

    def _load_model(self):
        """Load the fine-tuned model and tokenizer"""
        log.info(f"Loading model from {self.model_path}")

        # Load tokenizer
        self.tokenizer = AutoTokenizer.from_pretrained(self.model_name)
        if self.tokenizer.pad_token is None:
            self.tokenizer.pad_token = self.tokenizer.eos_token

        # Load base model
        base_model = AutoModelForCausalLM.from_pretrained(
            self.model_name,
            torch_dtype=torch.float16 if torch.cuda.is_available() else torch.float32,
            device_map="auto" if torch.cuda.is_available() else None
        )

        # Try to load LoRA adapters
        try:
            self.model = PeftModel.from_pretrained(base_model, self.model_path)
            log.info("✅ LoRA adapters loaded successfully")
        except:
            # Fallback to loading full model state
            try:
                checkpoint = torch.load(f"{self.model_path}/pytorch_model.bin", map_location="cpu")
                base_model.load_state_dict(checkpoint, strict=False)
                self.model = base_model
                log.info("✅ Full model state loaded")
            except:
                log.warning("Could not load trained weights, using base model")
                self.model = base_model

        self.model.eval()

    def save(self, path: str):
        """Save the model to the given path"""
        if self.model is None:
            raise ValueError("Model not loaded")

        os.makedirs(path, exist_ok=True)

        # Save the model
        self.model.save_pretrained(path)

        # Save the tokenizer
        self.tokenizer.save_pretrained(path)

        # Save metadata
        metadata = {
            "model_name": self.model_name,
            "max_length": self.max_length,
            "model_type": "gpt_finetuned"
        }

        import json
        with open(os.path.join(path, "model_metadata.json"), "w") as f:
            json.dump(metadata, f)

        log.info(f"Model saved to {path}")

    @classmethod
    def load(cls, path: str) -> "GPTFinetunedModel":
        """Load the model from the given path"""
        # Load metadata
        import json
        with open(os.path.join(path, "model_metadata.json"), "r") as f:
            metadata = json.load(f)

        model = cls()
        model.model_path = path
        model.model_name = metadata.get("model_name", "gpt2")
        model.max_length = metadata.get("max_length", 512)
        model._load_model()

        return model

    def predict(self, inputs: Dict[str, np.ndarray]) -> Dict[str, np.ndarray]:
        """
        Generate text predictions from the fine-tuned GPT model

        Args:
            inputs: Dictionary containing:
                - "prompt": string prompts for generation (shape: [batch_size])
                - "max_new_tokens": optional max tokens to generate (shape: [1])
                - "temperature": optional generation temperature (shape: [1])

        Returns:
            Dictionary containing:
                - "generated_text": generated completions (shape: [batch_size])
                - "input_length": length of input prompts (shape: [batch_size])
                - "output_length": length of generated text (shape: [batch_size])
        """
        if self.model is None or self.tokenizer is None:
            raise ValueError("Model not loaded")

        # Extract inputs
        prompts = inputs["prompt"].astype(str)  # Convert numpy array to string array
        max_new_tokens = int(inputs.get("max_new_tokens", np.array([50]))[0])
        temperature = float(inputs.get("temperature", np.array([0.7]))[0])

        generated_texts = []
        input_lengths = []
        output_lengths = []

        with torch.no_grad():
            for prompt in prompts:
                # Tokenize input
                inputs_tokenized = self.tokenizer(
                    prompt,
                    return_tensors="pt",
                    truncation=True,
                    max_length=self.max_length - max_new_tokens
                )

                input_length = inputs_tokenized["input_ids"].shape[1]

                # Generate text
                outputs = self.model.generate(
                    **inputs_tokenized,
                    max_length=input_length + max_new_tokens,
                    temperature=temperature,
                    do_sample=True if temperature > 0 else False,
                    num_return_sequences=1,
                    pad_token_id=self.tokenizer.eos_token_id,
                    eos_token_id=self.tokenizer.eos_token_id
                )

                # Decode the generated text
                generated_text = self.tokenizer.decode(outputs[0], skip_special_tokens=True)
                generated_only = generated_text[len(prompt):].strip()

                generated_texts.append(generated_text)
                input_lengths.append(input_length)
                output_lengths.append(len(outputs[0]) - input_length)

        # Return as numpy arrays
        return {
            "generated_text": np.array(generated_texts, dtype=object),
            "input_length": np.array(input_lengths, dtype=np.int32),
            "output_length": np.array(output_lengths, dtype=np.int32)
        }


@uniflow.task(
    config=RayTask(
        head_cpu=2,
        head_memory="8Gi",
        worker_cpu=2,
        worker_memory="8Gi",
        worker_instances=1,
        runtime_env={
            "pip": [
                "transformers",
                "torch",
                "peft",
                "numpy",
                "mlflow",
                "boto3",
                "protobuf",
                "grpcio"
            ],
            "env_vars": {"PYTHONPATH": "/Users/weric/works/uber/michelangelo_ai/michelangelo/python"}
        }
    )
)
def package_gpt_model(
    model_path: str,
    model_name: str = "gpt2",
    package_name: str = "gpt_finetuned_model",
    experiment_name: str = "gpt-finetune-experiment",
    namespace: str = "default",
    create_model_cr: bool = True
) -> Dict[str, Any]:
    """
    Package the fine-tuned GPT model using MLflow for deployment and storage

    Args:
        model_path: Path to the trained model
        model_name: Base model name used for training
        package_name: Name for the packaged model in MLflow registry
        experiment_name: MLflow experiment name

    Returns:
        Dictionary with package information including model URI and registry details
    """
    log.info(f"📦 Packaging GPT model from {model_path} using MLflow")

    # Set MA_API_SERVER for Model CR creation
    if create_model_cr and not os.getenv("MA_API_SERVER"):
        os.environ["MA_API_SERVER"] = "localhost:14566"
        log.info(f"🔗 Set MA_API_SERVER to {os.environ['MA_API_SERVER']} for Model CR creation")

    try:
        # Create MLflow packager
        packager = MLflowModelPackager(
            experiment_name=experiment_name,
            artifact_location="s3://mlflow"  # Configure S3 storage for MLflow artifacts
        )

        # Package the model with MLflow
        result = packager.package_gpt_model(
            model_path=model_path,
            model_name=model_name,
            model_registry_name=package_name,
            run_name=f"gpt-finetune-{model_name.replace('/', '-')}",
            description=f"Fine-tuned {model_name} model with LoRA adapters for instruction following",
            tags={
                "model_type": "gpt_finetuned",
                "base_model": model_name,
                "framework": "pytorch",
                "technique": "lora",
                "task": "instruction_following"
            },
            create_model_cr=create_model_cr,
            namespace=namespace
        )

        if result["status"] == "success":
            log.info(f"✅ Model packaged successfully with MLflow!")
            log.info(f"   Model URI: {result['model_uri']}")
            log.info(f"   Registry: {result['model_registry_name']}")
            log.info(f"   Version: {result['model_version']}")
            log.info(f"   Artifact Location: {result['artifact_location']}")

        return result

    except Exception as e:
        log.error(f"❌ Failed to package model with MLflow: {e}")
        return {
            "status": "failed",
            "error": str(e),
            "model_path": model_path
        }


def generate_test_predictions(model_path: str, model_name: str = "gpt2") -> Dict[str, Any]:
    """
    Test the packaged model locally before deployment

    Args:
        model_path: Path to the trained model
        model_name: Base model name

    Returns:
        Dictionary with test predictions
    """
    log.info("Testing model predictions locally...")

    # Load model
    model = GPTFinetunedModel(model_path, model_name)

    # Test inputs
    test_inputs = {
        "prompt": np.array([
            "### Instruction:\nWhat is the capital of France?\n\n### Response:\n",
            "### Instruction:\nExplain deep learning.\n\n### Response:\n"
        ]),
        "max_new_tokens": np.array([50]),
        "temperature": np.array([0.7])
    }

    # Get predictions
    predictions = model.predict(test_inputs)

    log.info("✅ Test predictions generated successfully")

    return {
        "test_inputs": {k: v.tolist() if isinstance(v, np.ndarray) else v for k, v in test_inputs.items()},
        "predictions": {k: v.tolist() if isinstance(v, np.ndarray) else v for k, v in predictions.items()},
        "status": "success"
    }