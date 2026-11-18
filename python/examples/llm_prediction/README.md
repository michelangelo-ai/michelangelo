# LLM Batch Prediction Demo

Batch inference with large language models using two execution backends: HuggingFace Transformers and vLLM. Demonstrates distributed inference workflows on Ray with configurable sampling parameters.

## Features

- **Two Backends**: HuggingFace Transformers (CPU/GPU) and vLLM (optimized GPU inference)
- **Distributed Inference**: Ray-based batch processing
- **Tensor Parallelism**: vLLM backend supports multi-GPU tensor parallel execution
- **Configurable Sampling**: Temperature, top-p, and max tokens parameters
- **Dataset Integration**: Load from HuggingFace datasets (default: THUDM/LongBench)

## Prerequisites

- **Network access**: both backends download the dataset (`THUDM/LongBench`, config `2wikimqa`) and the model (`Qwen/Qwen2.5-0.5B`) from the HuggingFace Hub.
- **HuggingFace backend**: no GPU required. The entrypoint defaults to `worker_gpu = 0`; set it to the number of GPUs per worker to use a GPU.
- **vLLM backend**: requires a CUDA-compatible GPU. The entrypoint defaults to `worker_gpu = 1` and `tensor_parallel_size = 1`, and `worker_gpu` should be divisible by `tensor_parallel_size`.
- **Sandbox**: not required for local runs. For remote runs, build the image and use the `remote-run` commands in [`../README.md`](../README.md#remote-run).

## How to Run

Both commands are run from the `python/` directory. Model, dataset and sampling settings are variables in each script's `__main__` block; edit them there.

### HuggingFace Transformers (CPU or GPU)

Requires the `example` extra (`poetry install -E example`):

```bash
PYTHONPATH="." poetry run python ./examples/llm_prediction/hf_prediction.py
```

### vLLM (GPU optimized)

Requires the `vllm` extra (`poetry install -E vllm`), which only supports
AMD64 machines with a CUDA-compatible GPU:

```bash
PYTHONPATH="." poetry run python ./examples/llm_prediction/vllm_prediction.py
```

## Expected Output

Both workflows log their progress and print `ok.` on success. The HuggingFace
path defaults to 2 samples at `batch_size=1`; the vLLM path defaults to 15
samples at `batch_size=8`. Predictions are written as CSV files to a
`llm_prediction` output directory (relative path) and logged as
`Wrote <n> items to llm_prediction`.
