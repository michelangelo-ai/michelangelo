# Nomic BERT Training Demo

Training Nomic BERT models on WikiText dataset using PyTorch Lightning and Ray. Demonstrates distributed training workflow with long-context BERT architecture.

## Features

- **Nomic BERT**: Long-context BERT model (2048 tokens)
- **WikiText Dataset**: Standard language modeling benchmark
- **PyTorch Lightning**: Training framework with best practices
- **Distributed Execution**: Ray-based workflow
- **Model Checkpoint**: Automatic model saving and evaluation

## How to Run

```bash
cd /Users/sally.lee/Uber/michelangelo-ai/michelangelo/python
source .venv/bin/activate
poetry run python examples/nomic_ai/nomic_ai.py
```

## Expected Output

```
Loading WikiText dataset...
Training Nomic BERT model: nomic-ai/nomic-bert-2048
Epoch 1: loss=3.245, perplexity=25.67
Epoch 2: loss=2.812, perplexity=16.73
Epoch 3: loss=2.534, perplexity=12.60
Training Workflow Result: {'model_path': '/tmp/nomic_bert_model', 'metrics': {...}}
```
