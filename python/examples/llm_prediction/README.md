# LLM Batch Prediction Demo

Batch inference with large language models using two execution backends: HuggingFace Transformers and vLLM. Demonstrates distributed inference workflows on Ray with configurable sampling parameters.

## Features

- **Two Backends**: HuggingFace Transformers (CPU/GPU) and vLLM (optimized GPU inference)
- **Distributed Inference**: Ray-based batch processing
- **Tensor Parallelism**: vLLM backend supports multi-GPU tensor parallel execution
- **Configurable Sampling**: Temperature, top-p, and max tokens parameters
- **Dataset Integration**: Load from HuggingFace datasets (default: THUDM/LongBench)

## How to Run

Both commands are run from the `python/` directory.

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
samples at `batch_size=8`. Predictions are written out as a dataset, logged as
`Wrote <n> items to <path>`.
