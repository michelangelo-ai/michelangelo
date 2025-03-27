import logging
from ray.data import Dataset
from michelangelo.uniflow.plugins.ray import RayTask
import michelangelo.uniflow.core as uniflow
import numpy as np
from transformers import pipeline

log = logging.getLogger(__name__)


class HFPredictor:
    def __init__(
        self,
        model_name: str,
        temperature: float,
        top_p: float,
        max_tokens: int,
    ):
        self.generator = pipeline(
            "text-generation", model=model_name, device_map="auto"
        )
        self.temperature = temperature
        self.top_p = top_p
        self.max_tokens = max_tokens

    def __call__(self, batch: dict[str, np.ndarray]) -> dict[str, list]:
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
