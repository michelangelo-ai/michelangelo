# BERT CoLA Fine-tuning Demo

Fine-tuning BERT for linguistic acceptability classification using the Corpus of Linguistic Acceptability (CoLA) task from the GLUE benchmark. Demonstrates sequence classification with distributed training on Ray.

## Features

- **Pre-trained Model**: BERT base model fine-tuning
- **GLUE Benchmark**: CoLA task for grammatical acceptability
- **Distributed Training**: Ray-based execution
- **HuggingFace Integration**: Uses transformers and datasets libraries
- **Evaluation Metrics**: Matthews correlation coefficient and accuracy

## How to Run

```bash
cd /Users/sally.lee/Uber/michelangelo-ai/michelangelo/python
source .venv/bin/activate
poetry run python examples/bert_cola/bert_cola.py
```

## Expected Output

```
Loading dataset from GLUE/CoLA...
Training BERT model...
Epoch 1/3: loss=0.512, accuracy=0.85
Epoch 2/3: loss=0.312, accuracy=0.89
Epoch 3/3: loss=0.201, accuracy=0.92
result: {'model_path': '/tmp/bert_cola_model', 'metrics': {...}}
ok.
```
