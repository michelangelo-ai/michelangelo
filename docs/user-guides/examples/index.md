---
sidebar_position: 5
sidebar_label: "Examples"
---

# Examples

End-to-end ML pipeline examples covering training, inference, recommendation systems, and model packaging. Each example is a complete, working workflow you can run locally or on a Michelangelo AI cluster.

All examples live in [`python/examples/`](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples).

## By Use Case

### Training & Fine-tuning

| Example | Description | Runtime | Difficulty |
|---------|-------------|---------|------------|
| [California Housing (XGBoost)](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/pipelines/california_housing_xgb) | Full pipeline — feature prep, Spark preprocessing, distributed XGBoost training, and pusher step that exports model + eval report to storage and registry. | Ray + Spark | Beginner |
| [BERT Text Classification](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/bert_cola) | Fine-tune BERT for linguistic acceptability classification on the CoLA benchmark (GLUE). Uses HuggingFace Transformers with distributed Ray training. | Ray | Intermediate |
| [GPT Fine-tuning with LoRA](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/gpt_oss_20b_finetune) | Parameter-efficient fine-tuning using LoRA (1.29% trainable params) on the Stanford Alpaca instruction-following dataset. Includes perplexity and generation quality evaluation. | Ray | Advanced |
| [Nomic AI Embedding Training](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/nomic_ai) | Train a long-context Nomic BERT model (2048 tokens) on WikiText using PyTorch Lightning with distributed Ray execution. | Ray | Intermediate |
| [MovieLens Collaborative Filtering](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/movielens) | Neural Collaborative Filtering on MovieLens-100k. Minimal smoke test for the `LightningTrainer` SDK — trains on CPU with a single Ray Train worker. | Ray | Beginner |

### Recommendation Systems

| Example | Description | Runtime | Difficulty |
|---------|-------------|---------|------------|
| [Amazon Books Recommendation](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/amazon_books_qwen) | Dual-encoder recommendation system using Qwen-based architecture for Amazon Books. Demonstrates Chronon feature engineering on Spark and distributed Ray training. | Ray + Spark | Advanced |

### Inference

| Example | Description | Runtime | Difficulty |
|---------|-------------|---------|------------|
| [HuggingFace Batch Inference](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/llm_prediction) | Batch inference with two backends: HuggingFace Transformers (CPU/GPU) and vLLM (optimized GPU with tensor parallelism). Configurable sampling parameters (temperature, top-p, max tokens). | Ray | Intermediate |

### Model Packaging

| Example | Description | Runtime | Difficulty |
|---------|-------------|---------|------------|
| [Custom Model Packaging](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/model_manager/simple_custom) | Package a custom model with `CustomTritonPackager` for Triton Inference Server. Demonstrates the `Model` interface (save/load/predict) with raw and deployable packaging modes. | — | Advanced |
| [Custom PyTorch Model Packaging](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/model_manager/simple_custom_torch) | Package a PyTorch model (`TorchLinearModel`) with `CustomTritonPackager`. Uses numpy arrays for I/O (as required by the Model interface) with internal torch conversion. | — | Advanced |

## Running Examples

Most examples follow the same pattern:

```bash
cd python
poetry install --extras "trainer example"
PYTHONPATH=. poetry run python examples/<example_dir>/<script>.py
```

For remote execution on a Michelangelo AI cluster, append `remote-run`:

```bash
PYTHONPATH=. poetry run python examples/pipelines/california_housing_xgb/california_housing_xgb.py remote-run \
  --project ma-examples \
  --image ghcr.io/michelangelo-ai/examples:main
```

See each example's README for specific prerequisites and run instructions.

## What's Next?

- **New to Michelangelo AI?** Start with [California Housing (XGBoost)](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/pipelines/california_housing_xgb) — it covers the full pipeline end-to-end
- **Want to understand the framework?** Read [Getting Started with Pipelines](../getting-started/getting-started.md) for a guided walkthrough
- **Ready to deploy?** See [Deploy a Model](../train-and-deploy-models/deploy-a-model.md) after training
