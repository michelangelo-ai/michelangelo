"""
Evaluation module for GPT-OSS-20B fine-tuning
Handles model evaluation with perplexity and generation quality metrics
"""

import logging
import torch
import numpy as np
from transformers import AutoTokenizer
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
    checkpoint_path: str,
    model_name: str = "gpt2",
    use_lora: bool = True,
    lora_rank: int = 16,
    learning_rate: float = 5e-5,
    max_length=512,
    batch_size=1,
    num_samples=100
):
    """
    Evaluate the fine-tuned GPT model

    Args:
        test_dv: Test dataset variable
        checkpoint_path: Ray checkpoint path from training
        model_name: Base model name (e.g., "gpt2")
        use_lora: Whether LoRA was used in training
        lora_rank: LoRA rank used in training
        learning_rate: Learning rate used in training
        max_length: Maximum sequence length for evaluation
        batch_size: Batch size for evaluation
        num_samples: Number of samples to evaluate

    Returns:
        Dictionary with evaluation metrics
    """
    log.info("🔍 EVAL STEP 1: Starting evaluation")
    log.info(f"Checkpoint: {checkpoint_path}")
    log.info(f"Model config: {model_name}, LoRA: {use_lora}, rank: {lora_rank}")

    # Load test dataset
    log.info("🔍 EVAL STEP 2: Loading test dataset")
    test_dv.load_ray_dataset()
    test_data: Dataset = test_dv.value
    log.info("✅ Test dataset loaded successfully")

    # Load Ray checkpoint directly (much simpler!)
    log.info("🔍 EVAL STEP 3: Importing modules")
    from examples.gpt_oss_20b_finetune.model import create_gpt_model
    import os
    import glob
    log.info("✅ Modules imported successfully")

    log.info("🔍 EVAL STEP 4: Finding checkpoint files")
    log.info(f"Looking in directory: {checkpoint_path}")

    # Find checkpoint file
    checkpoint_files = glob.glob(os.path.join(checkpoint_path, "*.ckpt"))
    log.info(f"Found {len(checkpoint_files)} checkpoint files: {checkpoint_files}")

    ckpt_path = checkpoint_files[0]
    log.info(f"Using checkpoint: {ckpt_path}")

    log.info("🔍 EVAL STEP 5: Loading checkpoint data")
    checkpoint_data = torch.load(ckpt_path, map_location="cpu")
    log.info(f"✅ Checkpoint loaded, keys: {list(checkpoint_data.keys())}")

    log.info("🔍 EVAL STEP 6: Creating model")
    model = create_gpt_model(
        model_name=model_name,
        learning_rate=learning_rate,
        use_lora=use_lora,
        lora_rank=lora_rank
    )
    log.info("✅ Model created successfully")

    log.info("🔍 EVAL STEP 7: Loading model weights")
    model.load_state_dict(checkpoint_data["state_dict"])
    log.info("✅ Model weights loaded successfully")

    log.info("🔍 EVAL STEP 8: Extracting base model")
    if hasattr(model, 'model'):
        base_model = model.model
        log.info("✅ Extracted base model from Lightning wrapper")
    else:
        base_model = model
        log.info("✅ Using model directly (no wrapper)")

    log.info("🔍 EVAL STEP 9: Loading tokenizer")
    tokenizer = AutoTokenizer.from_pretrained(model_name)
    if tokenizer.pad_token is None:
        tokenizer.pad_token = tokenizer.eos_token
    log.info("✅ Tokenizer loaded successfully")

    log.info("🔍 EVAL STEP 10: Setting model to eval mode")
    model.eval()
    device = next(model.parameters()).device
    log.info(f"✅ Model ready, device: {device}")

    log.info("🔍 EVAL STEP 11: Converting dataset")
    test_df = test_data.to_pandas()
    test_df = test_df.head(num_samples)
    log.info(f"✅ Dataset ready: {len(test_df)} samples")

    # Evaluation metrics
    perplexities = []
    generation_scores = []

    with torch.no_grad():
        for idx, row in test_df.iterrows():
            if idx % 20 == 0:
                log.info(f"Processing sample {idx}/{len(test_df)}")

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
            outputs = base_model(input_ids=input_ids, labels=labels)
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
                    generated = base_model.generate(
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
        "checkpoint_path": checkpoint_path,
        "device": str(device)
    }

    log.info("✅ Evaluation completed")
    log.info(f"Average Perplexity: {avg_perplexity:.2f}")
    log.info(f"Average Generation Score: {avg_generation_score:.2f}")

    return metrics
