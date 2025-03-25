import logging
import os
import ray
from ray.data import Dataset
from michelangelo.uniflow.plugins.ray import RayTask
from ray.util.scheduling_strategies import PlacementGroupSchedulingStrategy
import michelangelo.uniflow.core as uniflow
import numpy as np
from vllm import LLM, SamplingParams

log = logging.getLogger(__name__)


class VLLMPredictor():
    def __init__(
        self,
        model_name: str,
        tensor_parallel_size: int,
        temperature: float,
        top_p: float,
        max_tokens: int,
    ):
        self.tensor_parallel_size = tensor_parallel_size
        self.llm = LLM(
            model=model_name,
            tensor_parallel_size=tensor_parallel_size,
        )
        self.sampling_params = SamplingParams(
            temperature=temperature,
            top_p=top_p,
            max_tokens=max_tokens,
        )

    def __call__(self, batch: dict[str, np.ndarray]) -> dict[str, list]:
        outputs = self.llm.generate(batch["text"], self.sampling_params)
        prompt: list[str] = []
        generated_text: list[str] = []
        for output in outputs:
            prompt.append(output.prompt)
            generated_text.append(' '.join([o.text for o in output.outputs]))
        return {
            "prompt": prompt,
            "generated_text": generated_text,
        }


@uniflow.task(
    config=RayTask(
        head_cpu=1,
        head_memory="4Gi",
        worker_cpu=1,
        worker_memory="16Gi",
        worker_instances=1,   # TODO: make this configurable from workflow after supported is added
        worker_gpu=1,         # TODO: make this configurable from workflow after supported is added 
        # breakpoint=True,
    ),
)
def predict(
        predict_data: Dataset,
        tensor_parallel_size: int,
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
    log.info("Starting offline prediction...")

    if worker_gpu % tensor_parallel_size != 0:
        log.warning(f"worker_gpu {worker_gpu} is indivisible by \
            tensor_parallel_size {tensor_parallel_size}, use all available GPUs instead")
        tensor_parallel_size = worker_gpu
    concurrency = worker_gpu * worker_instances // tensor_parallel_size

    def scheduling_strategy_fn():
        # One bundle per tensor parallel worker
        pg = ray.util.placement_group(
            [{
                "GPU": 1,
                "CPU": 1
            }] * tensor_parallel_size,
            strategy="STRICT_PACK",
        )
        return dict(scheduling_strategy=PlacementGroupSchedulingStrategy(
            pg, placement_group_capture_child_tasks=True))

    resources_kwarg: dict[str, Any] = {}
    if tensor_parallel_size == 1:
        # For tensor_parallel_size == 1, we simply set num_gpus=worker_gpu.
        resources_kwarg["num_gpus"] = worker_gpu
    else:
        # Otherwise, we have to set num_gpus=0 and provide
        # a function that will create a placement group for
        # each instance.
        resources_kwarg["num_gpus"] = 0
        resources_kwarg["ray_remote_args_fn"] = scheduling_strategy_fn

    log.info(f"Starting prediction with batch_size {batch_size} concurrency {concurrency} tensor_parallel_size {tensor_parallel_size}")
    predict_data = predict_data.map_batches(
        VLLMPredictor,
        fn_constructor_kwargs={
            "model_name": model_name,
            "tensor_parallel_size": tensor_parallel_size,
            "temperature": temperature,
            "top_p": top_p,
            "max_tokens": max_tokens,
        },
        concurrency=concurrency,
        batch_size=batch_size,
        **resources_kwarg,
    )
    return predict_data
