# Nomic BERT Training Demo

Training Nomic BERT models on WikiText dataset using PyTorch Lightning and Ray. Demonstrates distributed training workflow with long-context BERT architecture.

## Features

- **Nomic BERT**: `nomic-ai/nomic-bert-2048`, tokenized at 512 tokens by default
- **WikiText Dataset**: Standard language modeling benchmark (`wikitext-2-raw-v1`)
- **PyTorch Lightning**: Training framework with best practices
- **Distributed Execution**: Ray-based workflow
- **Model Checkpoint**: Automatic model saving

## How to Run

Run from the `python/` directory, with the `example` extra installed
(`poetry install -E example`):

```bash
PYTHONPATH="." poetry run python ./examples/nomic_ai/nomic_ai.py
```

## Expected Output

PyTorch Lightning prints its own progress bars and per-epoch train/validation
loss. The trained model is saved to `./nomic_ai`, and the workflow returns:

```python
{'status': 'Training completed successfully'}
```
