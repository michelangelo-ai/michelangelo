"""Data preparation module for GPT-OSS-20B fine-tuning
Handles dataset loading, preprocessing, and tokenization
"""

import logging
from typing import Dict, Tuple

import ray
from datasets import Dataset as HFDataset
from datasets import load_dataset
from transformers import AutoTokenizer

import michelangelo.uniflow.core as uniflow
from michelangelo.sdk.workflow.variables import DatasetVariable
from michelangelo.uniflow.plugins.ray import RayTask

log = logging.getLogger(__name__)


@uniflow.task(
    config=RayTask(
        head_cpu=2,
        head_memory="4Gi",
        worker_cpu=2,
        worker_memory="4Gi",
        worker_instances=2,
    )
)
def prepare_finetune_dataset(
    dataset_name: str = "alpaca",
    max_length: int = 2048,
    sample_size: int = 10000,
    model_name: str = "openai/gpt-oss-20b",
) -> Tuple[DatasetVariable, DatasetVariable, DatasetVariable]:
    """Prepare fine-tuning dataset for GPT-OSS-20B

    Args:
        dataset_name: Name of the dataset to use
        max_length: Maximum sequence length
        sample_size: Number of samples to use
        model_name: Model name for tokenizer

    Returns:
        Tuple of (train_dataset, validation_dataset, test_dataset) as DatasetVariables
    """
    log.info(f"Preparing {dataset_name} dataset for GPT-OSS-20B fine-tuning")

    # Load tokenizer
    try:
        tokenizer = AutoTokenizer.from_pretrained(
            model_name, trust_remote_code=True, use_fast=True
        )
        if tokenizer.pad_token is None:
            tokenizer.pad_token = tokenizer.eos_token
        log.info(f"Loaded tokenizer for {model_name}")
    except Exception as e:
        log.warning(f"Failed to load tokenizer for {model_name}, using GPT2: {e}")
        tokenizer = AutoTokenizer.from_pretrained("gpt2")
        tokenizer.pad_token = tokenizer.eos_token

    # Load dataset based on name
    if dataset_name == "alpaca":
        dataset = load_alpaca_dataset(sample_size)
    elif dataset_name == "dolly":
        dataset = load_dolly_dataset(sample_size)
    elif dataset_name == "oasst1":
        dataset = load_oasst1_dataset(sample_size)
    else:
        raise ValueError(f"Unsupported dataset: {dataset_name}")

    log.info(f"Loaded {len(dataset)} samples from {dataset_name}")

    # Preprocess and tokenize
    processed_dataset = preprocess_dataset(dataset, tokenizer, max_length)
    log.info(f"Preprocessed dataset to {len(processed_dataset)} samples")

    # Split into train/validation/test (80/10/10)
    train_size = int(0.8 * len(processed_dataset))
    val_size = int(0.1 * len(processed_dataset))

    train_dataset = processed_dataset.select(range(train_size))
    val_dataset = processed_dataset.select(range(train_size, train_size + val_size))
    test_dataset = processed_dataset.select(
        range(train_size + val_size, len(processed_dataset))
    )

    log.info(
        f"Split into {len(train_dataset)} train, {len(val_dataset)} validation, {len(test_dataset)} test samples"
    )

    # Convert to Ray Datasets
    train_ray_dataset = ray.data.from_pandas(train_dataset.to_pandas())
    val_ray_dataset = ray.data.from_pandas(val_dataset.to_pandas())
    test_ray_dataset = ray.data.from_pandas(test_dataset.to_pandas())

    # Create DatasetVariables
    train_dv = DatasetVariable.create(train_ray_dataset)
    train_dv.save_ray_dataset()

    val_dv = DatasetVariable.create(val_ray_dataset)
    val_dv.save_ray_dataset()

    test_dv = DatasetVariable.create(test_ray_dataset)
    test_dv.save_ray_dataset()

    log.info("✅ Dataset preparation completed")
    return train_dv, val_dv, test_dv


def load_alpaca_dataset(sample_size: int) -> HFDataset:
    """Load Stanford Alpaca dataset"""
    try:
        dataset = load_dataset("tatsu-lab/alpaca", split="train")
        if sample_size < len(dataset):
            dataset = dataset.shuffle(seed=42).select(range(sample_size))
        return dataset
    except Exception as e:
        log.warning(f"Failed to load alpaca dataset: {e}")
        # Fallback to a smaller dataset
        return create_dummy_dataset(sample_size)


def load_dolly_dataset(sample_size: int) -> HFDataset:
    """Load Databricks Dolly dataset"""
    try:
        dataset = load_dataset("databricks/databricks-dolly-15k", split="train")
        if sample_size < len(dataset):
            dataset = dataset.shuffle(seed=42).select(range(sample_size))
        return dataset
    except Exception as e:
        log.warning(f"Failed to load dolly dataset: {e}")
        return create_dummy_dataset(sample_size)


def load_oasst1_dataset(sample_size: int) -> HFDataset:
    """Load OpenAssistant dataset"""
    try:
        dataset = load_dataset("OpenAssistant/oasst1", split="train")
        if sample_size < len(dataset):
            dataset = dataset.shuffle(seed=42).select(range(sample_size))
        return dataset
    except Exception as e:
        log.warning(f"Failed to load oasst1 dataset: {e}")
        return create_dummy_dataset(sample_size)


def create_dummy_dataset(sample_size: int) -> HFDataset:
    """Create a dummy dataset for testing"""
    from datasets import Dataset as HFDataset

    dummy_data = []
    for i in range(min(sample_size, 1000)):  # Cap at 1000 for dummy data
        dummy_data.append(
            {
                "instruction": f"What is the capital of country {i}?",
                "input": "",
                "output": f"The capital of country {i} is City {i}.",
            }
        )

    return HFDataset.from_list(dummy_data)


def preprocess_dataset(dataset: HFDataset, tokenizer, max_length: int) -> HFDataset:
    """Preprocess dataset for GPT-OSS-20B fine-tuning
    Formats data in instruction-following format
    """

    def format_sample(sample):
        """Format sample for instruction fine-tuning"""
        if "instruction" in sample and "output" in sample:
            # Alpaca format
            instruction = sample["instruction"]
            input_text = sample.get("input", "")
            output = sample["output"]

            if input_text:
                prompt = f"### Instruction:\n{instruction}\n\n### Input:\n{input_text}\n\n### Response:\n"
            else:
                prompt = f"### Instruction:\n{instruction}\n\n### Response:\n"

            full_text = prompt + output + tokenizer.eos_token

        elif "question" in sample and "answer" in sample:
            # Q&A format
            full_text = f"Question: {sample['question']}\nAnswer: {sample['answer']}{tokenizer.eos_token}"

        elif "text" in sample:
            # Raw text format
            full_text = sample["text"] + tokenizer.eos_token

        else:
            # Fallback
            full_text = str(sample) + tokenizer.eos_token

        return {"text": full_text}

    def tokenize_sample(sample):
        """Tokenize sample for training"""
        # Tokenize the text
        tokenized = tokenizer(
            sample["text"],
            truncation=True,
            max_length=max_length,
            padding=False,  # Don't pad here, will pad in collator
            return_tensors=None,
        )

        # For causal LM, labels are the same as input_ids (shifted in the model)
        tokenized["labels"] = tokenized["input_ids"].copy()

        return tokenized

    # Apply formatting and tokenization
    log.info("Formatting samples for instruction fine-tuning...")
    formatted_dataset = dataset.map(format_sample, remove_columns=dataset.column_names)

    log.info("Tokenizing samples...")
    tokenized_dataset = formatted_dataset.map(
        tokenize_sample, remove_columns=formatted_dataset.column_names, batched=False
    )

    # Filter out samples that are too short or too long
    def filter_length(sample):
        return 10 <= len(sample["input_ids"]) <= max_length

    filtered_dataset = tokenized_dataset.filter(filter_length)

    log.info(f"Filtered dataset: {len(dataset)} -> {len(filtered_dataset)} samples")

    return filtered_dataset


def create_data_collator(tokenizer, max_length: int):
    """Create data collator for dynamic padding"""
    from transformers import DataCollatorForLanguageModeling

    return DataCollatorForLanguageModeling(
        tokenizer=tokenizer,
        mlm=False,  # Causal LM, not masked LM
        pad_to_multiple_of=8,  # For efficiency
        return_tensors="pt",
    )


def get_dataset_stats(dataset: HFDataset) -> Dict:
    """Get statistics about the dataset"""
    if len(dataset) == 0:
        return {"num_samples": 0}

    sample = dataset[0]
    stats = {
        "num_samples": len(dataset),
        "columns": list(sample.keys()),
        "sample_input_ids_length": len(sample.get("input_ids", [])),
    }

    # Calculate length statistics
    if "input_ids" in sample:
        lengths = [len(sample["input_ids"]) for sample in dataset]
        stats.update(
            {
                "min_length": min(lengths),
                "max_length": max(lengths),
                "avg_length": sum(lengths) / len(lengths),
            }
        )

    return stats
