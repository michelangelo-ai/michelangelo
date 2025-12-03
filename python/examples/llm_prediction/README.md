# LLM Batch Prediction Demo

Batch inference with large language models using two execution backends: HuggingFace Transformers and vLLM. Demonstrates distributed inference workflows on Ray with configurable sampling parameters.

## Features

- **Two Backends**: HuggingFace Transformers (CPU/GPU) and vLLM (optimized GPU inference)
- **Distributed Inference**: Ray-based batch processing
- **Tensor Parallelism**: vLLM backend supports multi-GPU tensor parallel execution
- **Configurable Sampling**: Temperature, top-p, and max tokens parameters
- **Dataset Integration**: Load from HuggingFace datasets (default: THUDM/LongBench)

## How to Run

### HuggingFace Transformers (CPU or GPU)

```bash
cd /Users/sally.lee/Uber/michelangelo-ai/michelangelo/python
source .venv/bin/activate
poetry run python examples/llm_prediction/hf_prediction.py
```

### vLLM (GPU optimized)

```bash
cd /Users/sally.lee/Uber/michelangelo-ai/michelangelo/python
source .venv/bin/activate
poetry run python examples/llm_prediction/vllm_prediction.py
```

## Expected Output

```
Loading dataset: THUDM/LongBench (2wikimqa, test split)
Processing 2 samples with batch_size=1
Prediction completed: 2/2 samples
Results written to: llm_prediction/
ok.
```
