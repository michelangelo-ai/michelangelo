"""LLM prediction workflow using vLLM for high-performance inference.

Example workflow demonstrating how to use vLLM for efficient batch inference
with tensor parallelism. Ideal for production deployments requiring high throughput.
"""

import michelangelo.uniflow.core as uniflow
from examples.llm_prediction.data import load_data, write_data
from examples.llm_prediction.vllm_predict import predict
from michelangelo.uniflow.plugins.ray import UF_PLUGIN_RAY_USE_FSSPEC


@uniflow.workflow()
def llm_prediction_workflow(
    data_path: str,
    data_name: str,
    data_slice: str,
    data_predict_column: str,
    data_limit: int,
    batch_size: int,
    model_name: str,
    temperature: float,
    top_p: float,
    max_tokens: int,
    worker_instances: int = 1,
    worker_gpu: int = 0,
    tensor_parallel_size: int = 1,
):
    """LLM prediction workflow using vLLM for high-performance inference.

    Loads data from HuggingFace datasets, runs batch prediction with vLLM,
    and writes results to storage. Supports tensor parallelism for large models.

    Args:
        data_path: HuggingFace dataset path.
        data_name: Dataset configuration name.
        data_slice: Dataset split to use.
        data_predict_column: Column containing text to predict on.
        data_limit: Maximum number of samples to process.
        batch_size: Batch size for prediction.
        model_name: HuggingFace model identifier.
        temperature: Sampling temperature.
        top_p: Nucleus sampling parameter.
        max_tokens: Maximum tokens to generate.
        worker_instances: Number of Ray workers. Defaults to 1.
        worker_gpu: GPUs per worker. Defaults to 0.
        tensor_parallel_size: Number of GPUs for tensor parallelism. Defaults to 1.
    """
    predict_data = load_data(
        path=data_path,
        name=data_name,
        data_slice=data_slice,
        predict_column=data_predict_column,
        limit=data_limit,
    )
    result = predict(
        predict_data,
        tensor_parallel_size=tensor_parallel_size,
        worker_gpu=worker_gpu,
        worker_instances=worker_instances,
        batch_size=batch_size,
        model_name=model_name,
        temperature=temperature,
        top_p=top_p,
        max_tokens=max_tokens,
    )

    if result:
        write_data(
            dataset=result,
            out_path="llm_prediction",
            partitions=1,
        )
    print("ok.")


# For Local Run: poetry run python examples/llm_prediction/vllm_prediction.py
# For Remote Run: poetry run python examples/llm_prediction/vllm_prediction.py remote-run --storage-url <STORAGE_URL> --image <IMAGE>
if __name__ == "__main__":
    ctx = uniflow.create_context()

    # Disable use of fsspec in Ray Plugin. See UF_PLUGIN_RAY_USE_FSSPEC docstring for more information.
    ctx.environ[UF_PLUGIN_RAY_USE_FSSPEC] = "0"
    ctx.environ["PYTORCH_MPS_HIGH_WATERMARK_RATIO"] = "0"
    ctx.environ["MA_NAMESPACE"] = "default"
    # this is example docker image, we don't need to pull it from docker registry
    ctx.environ["IMAGE_PULL_POLICY"] = "Never"

    worker_gpu = 1
    worker_instances = 1
    data_path = "THUDM/LongBench"
    data_name = "2wikimqa"
    data_slice = "test"
    data_predict_column = "input"
    data_limit = 15
    batch_size = 8
    model_name = "Qwen/Qwen2.5-0.5B"
    temperature = 0.95
    top_p = 0.95
    max_tokens = 128
    tensor_parallel_size = 1

    # Run with VLLM if GPU is enabled
    ctx.run(
        llm_prediction_workflow,
        worker_gpu=worker_gpu,
        worker_instances=worker_instances,
        tensor_parallel_size=tensor_parallel_size,
        data_path=data_path,
        data_name=data_name,
        data_slice=data_slice,
        data_predict_column=data_predict_column,
        data_limit=data_limit,
        batch_size=batch_size,
        model_name=model_name,
        temperature=temperature,
        top_p=top_p,
        max_tokens=max_tokens,
    )
