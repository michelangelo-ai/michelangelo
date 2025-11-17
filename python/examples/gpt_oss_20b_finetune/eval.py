"""
Evaluation module for GPT-OSS-20B fine-tuning
Handles model evaluation with perplexity and generation quality metrics
"""

import logging
import torch
import numpy as np
import mlflow.pytorch
from transformers import AutoModelForCausalLM, AutoTokenizer
from peft import PeftModel
from ray.data import Dataset
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask
from michelangelo.sdk.workflow.variables import DatasetVariable

log = logging.getLogger(__name__)


@uniflow.task(
    config=RayTask(
        head_cpu=1,
        head_memory="8Gi",
        worker_cpu=1,
        worker_memory="8Gi",
        worker_instances=1,
    )
)
def evaluate_gpt_model(
    test_dv: DatasetVariable,
    model_uri: str,
    model_name="gpt2",
    max_length=512,
    batch_size=1,
    num_samples=100
):
    """
    Evaluate the fine-tuned GPT model

    Args:
        test_dv: Test dataset variable
        model_uri: MLflow model URI
        model_name: Base model name used for training
        max_length: Maximum sequence length for evaluation
        batch_size: Batch size for evaluation
        num_samples: Number of samples to evaluate

    Returns:
        Dictionary with evaluation metrics
    """
    log.info(f"Starting evaluation with model: {model_name}")
    log.info(f"Model URI: {model_uri}")

    # Load test dataset
    test_dv.load_ray_dataset()
    test_data: Dataset = test_dv.value

    log.info("✅ Test dataset loaded")

    # Load model from MLflow URI
    log.info("Loading model from MLflow...")
    model = mlflow.pytorch.load_model(model_uri)
    log.info("✅ Model loaded from MLflow")

    # Load tokenizer
    tokenizer = AutoTokenizer.from_pretrained(model_name)
    if tokenizer.pad_token is None:
        tokenizer.pad_token = tokenizer.eos_token

    model.eval()
    device = next(model.parameters()).device
    log.info(f"Using device: {device}")

    # Convert Ray dataset to pandas for easier processing
    test_df = test_data.to_pandas()
    test_df = test_df.head(num_samples)  # Limit samples for evaluation

    log.info(f"Evaluating on {len(test_df)} samples")

    # Evaluation metrics
    perplexities = []
    generation_scores = []

    with torch.no_grad():
        for idx, row in test_df.iterrows():
            if idx % 20 == 0:
                log.info(f"Processing sample {idx}/{len(test_df)}")

            try:
                # Get input and target text
                if "input_ids" in row:
                    # If already tokenized
                    input_ids = torch.tensor(row["input_ids"], dtype=torch.long).unsqueeze(0)
                    if "labels" in row:
                        labels = torch.tensor(row["labels"], dtype=torch.long).unsqueeze(0)
                    else:
                        labels = input_ids.clone()
                elif "text" in row:
                    # If raw text
                    text = row["text"]
                    tokens = tokenizer(
                        text,
                        max_length=max_length,
                        truncation=True,
                        padding=False,
                        return_tensors="pt"
                    )
                    input_ids = tokens["input_ids"]
                    labels = input_ids.clone()
                else:
                    continue

                # Move to device
                input_ids = input_ids.to(device)
                labels = labels.to(device)

                # Calculate perplexity
                outputs = model(input_ids=input_ids, labels=labels)
                loss = outputs.loss
                perplexity = torch.exp(loss).item()
                perplexities.append(perplexity)

                # Test generation quality (for instruction following)
                if len(input_ids[0]) > 20:  # Only for longer sequences
                    # Take first half as prompt
                    prompt_length = len(input_ids[0]) // 2
                    prompt_ids = input_ids[:, :prompt_length]

                    # Generate continuation
                    with torch.no_grad():
                        generated = model.generate(
                            prompt_ids,
                            max_length=prompt_length + 50,
                            num_return_sequences=1,
                            temperature=0.7,
                            do_sample=True,
                            pad_token_id=tokenizer.eos_token_id
                        )

                    # Calculate generation score (simple length-based metric)
                    generated_length = len(generated[0]) - prompt_length
                    generation_score = min(generated_length / 50, 1.0)  # Normalize to 0-1
                    generation_scores.append(generation_score)

            except Exception as e:
                log.warning(f"Error processing sample {idx}: {e}")
                continue

    # Calculate final metrics
    avg_perplexity = np.mean(perplexities) if perplexities else float('inf')
    avg_generation_score = np.mean(generation_scores) if generation_scores else 0.0

    # Additional metrics
    metrics = {
        "num_samples_evaluated": len(perplexities),
        "average_perplexity": avg_perplexity,
        "perplexity_std": np.std(perplexities) if perplexities else 0.0,
        "average_generation_score": avg_generation_score,
        "generation_score_std": np.std(generation_scores) if generation_scores else 0.0,
        "model_name": model_name,
        "model_uri": model_uri,
        "device": str(device)
    }

    log.info("✅ Evaluation completed")
    log.info(f"Average Perplexity: {avg_perplexity:.2f}")
    log.info(f"Average Generation Score: {avg_generation_score:.2f}")

    return metrics


def generate_sample_outputs(
    model_path: str,
    model_name="gpt2",
    num_samples=5,
    prompts=None
):
    """
    Generate sample outputs from the fine-tuned model for qualitative evaluation

    Args:
        model_path: Path to the trained model
        model_name: Base model name
        num_samples: Number of samples to generate
        prompts: List of prompts to use (if None, uses default prompts)

    Returns:
        List of generated samples
    """
    if prompts is None:
        prompts = [
            "### Instruction:\nExplain the concept of machine learning.\n\n### Response:\n",
            "### Instruction:\nWhat is the capital of France?\n\n### Response:\n",
            "### Instruction:\nWrite a short story about a robot.\n\n### Response:\n",
            "### Instruction:\nHow do you make chocolate cake?\n\n### Response:\n",
            "### Instruction:\nWhat are the benefits of exercise?\n\n### Response:\n"
        ]

    # Load tokenizer
    tokenizer = AutoTokenizer.from_pretrained(model_name)
    if tokenizer.pad_token is None:
        tokenizer.pad_token = tokenizer.eos_token

    # Load model
    base_model = AutoModelForCausalLM.from_pretrained(
        model_name,
        torch_dtype=torch.float16 if torch.cuda.is_available() else torch.float32,
        device_map="auto" if torch.cuda.is_available() else None
    )

    try:
        model = PeftModel.from_pretrained(base_model, model_path)
        log.info("✅ LoRA adapters loaded for generation")
    except:
        model = base_model
        log.warning("Using base model for generation")

    model.eval()
    device = next(model.parameters()).device

    generated_samples = []

    with torch.no_grad():
        for i, prompt in enumerate(prompts[:num_samples]):
            try:
                # Tokenize prompt
                inputs = tokenizer(prompt, return_tensors="pt").to(device)

                # Generate
                outputs = model.generate(
                    **inputs,
                    max_length=inputs["input_ids"].shape[1] + 150,
                    num_return_sequences=1,
                    temperature=0.7,
                    do_sample=True,
                    pad_token_id=tokenizer.eos_token_id,
                    eos_token_id=tokenizer.eos_token_id
                )

                # Decode
                generated_text = tokenizer.decode(outputs[0], skip_special_tokens=True)

                generated_samples.append({
                    "prompt": prompt,
                    "generated_text": generated_text,
                    "response_only": generated_text[len(prompt):].strip()
                })

            except Exception as e:
                log.warning(f"Error generating sample {i}: {e}")
                continue

    return generated_samples