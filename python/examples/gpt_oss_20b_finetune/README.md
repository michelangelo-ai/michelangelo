# GPT-OSS-20B Fine-tuning Demo

Simple demo for fine-tuning GPT models using Uniflow, PyTorch, and LoRA. Demonstrates parameter-efficient training with distributed execution and model evaluation.

## Features

- **Parameter-Efficient Training**: LoRA fine-tuning (1.29% trainable parameters)
- **Distributed Execution**: Ray-based workflow with Uniflow
- **Real Dataset**: Stanford Alpaca instruction-following dataset
- **Model Evaluation**: Perplexity and generation quality metrics
- **Scalable**: Tested with GPT-2, designed for GPT-OSS-20B

## How to Run

Run from the `python/` directory:

```bash
PYTHONPATH="." poetry run python ./examples/gpt_oss_20b_finetune/simple_workflow.py
```

## Expected Output

```
============================================================
Simple GPT Fine-tuning Demo
============================================================
trainable params: 1,622,016 || all params: 126,061,824 || trainable%: 1.2867
...
✅ Training completed, MLflow run: <run_id>
```

Between the banner and the completion line, PyTorch Lightning prints its own
progress bars and per-epoch metrics. Parameter counts above are for the
default `gpt2` model.

The training task returns the checkpoint location and MLflow run id:

```python
{"checkpoint_path": "s3://default/ray_checkpoints/...", "mlflow_run_id": "..."}
```