"""HuggingFace-based prediction module for LLM inference.

This module implements distributed batch inference using HuggingFace transformers
with Ray for parallel processing. Supports both CPU and GPU inference.
"""

import logging

import numpy as np
from ray.data import Dataset
from transformers import pipeline

import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask

log = logging.getLogger(__name__)


class HFPredictor:
    """HuggingFace text generation predictor for batch inference.

    Uses HuggingFace transformers pipeline for distributed text generation with Ray.
    Automatically handles device placement and supports both CPU and GPU inference.

    Attributes:
        generator: HuggingFace text-generation pipeline.
        temperature: Sampling temperature for generation diversity.
        top_p: Nucleus sampling threshold.
        max_tokens: Maximum length for generated sequences.
    """

    def __init__(
        self,
        model_name: str,
        temperature: float,
        top_p: float,
        max_tokens: int,
    ):
        """Initialize HuggingFace predictor.

        Args:
            model_name: HuggingFace model identifier.
            temperature: Sampling temperature for generation.
            top_p: Nucleus sampling parameter.
            max_tokens: Maximum tokens to generate.
        """
        self.generator = pipeline(
            "text-generation", model=model_name, device_map="auto"
        )
        self.temperature = temperature
        self.top_p = top_p
        self.max_tokens = max_tokens

    def __call__(self, batch: dict[str, np.ndarray]) -> dict[str, list]:
        """Generate text for a batch of prompts.

        Args:
            batch: Batch dictionary with 'text' key containing prompts.

        Returns:
            Dictionary with 'prompt' and 'generated_text' lists.
        """
        prompt: list[str] = batch["text"].tolist()
        generated_text: list[str] = [
            self.generator(
                p,
                max_length=self.max_tokens,
                temperature=self.temperature,
                top_p=self.top_p,
                num_return_sequences=1,
                truncation=True,
            )[0]["generated_text"]
            for p in prompt
        ]

        return {
            "prompt": prompt,
            "generated_text": generated_text,
        }


@uniflow.task(
    config=RayTask(
        head_cpu=1,
        head_memory="4Gi",
        worker_cpu=1,
        worker_memory="4Gi",
        worker_instances=1,  # TODO: make this configurable from workflow after supported is added
        worker_gpu=0,  # TODO: make this configurable from workflow after supported is added
        # breakpoint=True,
    ),
)
def predict(
    predict_data: Dataset,
    worker_instances: int,
    worker_gpu: int,
    batch_size: int,
    # LLM parameters
    model_name: str,
    # SamplingParams parameters
    temperature: float,
    top_p: float,
    max_tokens: int,
) -> Dataset:
    """Run distributed batch prediction using HuggingFace models.

    Args:
        predict_data: Ray Dataset containing input prompts.
        worker_instances: Number of concurrent Ray workers.
        worker_gpu: Number of GPUs per worker.
        batch_size: Batch size for prediction.
        model_name: HuggingFace model identifier.
        temperature: Sampling temperature.
        top_p: Nucleus sampling parameter.
        max_tokens: Maximum tokens to generate.

    Returns:
        Ray Dataset with generated text results.
    """
    log.info("Starting offline prediction with HFPredictor...")
    log.info(
        f"Starting prediction with batch_size {batch_size} concurrency {worker_instances}"
    )
    predict_data = predict_data.map_batches(
        HFPredictor,
        fn_constructor_kwargs={
            "model_name": model_name,
            "temperature": temperature,
            "top_p": top_p,
            "max_tokens": max_tokens,
        },
        concurrency=worker_instances,
        batch_size=batch_size,
        num_gpus=worker_gpu,
    )
    return predict_data
