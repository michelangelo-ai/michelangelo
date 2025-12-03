# GPT-OSS-20B Fine-tuning Demo

Simple demo for fine-tuning GPT models using Uniflow, PyTorch, and LoRA. Demonstrates parameter-efficient training with distributed execution and model evaluation.

## Features

- **Parameter-Efficient Training**: LoRA fine-tuning (1.29% trainable parameters)
- **Distributed Execution**: Ray-based workflow with Uniflow
- **Real Dataset**: Stanford Alpaca instruction-following dataset
- **Model Evaluation**: Perplexity and generation quality metrics
- **Scalable**: Tested with GPT-2, designed for GPT-OSS-20B

## How to Run

```bash
cd michelangelo_ai/michelangelo/python
source .venv/bin/activate
PYTHONPATH=. poetry run python ./examples/gpt_oss_20b_finetune/simple_workflow.py
```

## Expected Output

```
============================================================
Simple GPT Fine-tuning Demo
============================================================
trainable params: 1,622,016 || all params: 126,061,824 || trainable%: 1.2867
{'loss': 3.5656, 'grad_norm': 1.5649, 'learning_rate': 4e-05, 'epoch': 0.22}
...
{'train_runtime': 15.4371, 'train_samples_per_second': 2.915, 'epoch': 1.0}
============================================================
Training completed!
Result: {'model_path': '/tmp/simple_model', 'train_loss': 3.5225, 'training_type': 'simple_local'}
============================================================
```