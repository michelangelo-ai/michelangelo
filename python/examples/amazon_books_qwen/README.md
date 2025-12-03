# Amazon Books Qwen Dual-Encoder Demo

Recommendation system demo using Qwen-based dual-encoder architecture for Amazon Books dataset. Demonstrates feature engineering with Chronon, distributed data processing with Spark, and model training with Ray.

## Features

- **Dual-Encoder Architecture**: Qwen-based model for query and document embeddings
- **Feature Engineering**: Chronon-powered feature computation on Spark
- **Dataset**: Kaggle Amazon Books dataset with reviews
- **Distributed Execution**: Ray-based training with configurable resources
- **Configurable**: Support for both local testing and distributed training

## How to Run

```bash
cd michelangelo-ai/michelangelo/python
source .venv/bin/activate
PYTHONPATH=examples poetry run python examples/amazon_books_qwen/amazon_books_qwen.py
```

## Expected Output

```
================================================================================
Amazon Books Qwen Dual-Encoder Pipeline
================================================================================
Using smaller dataset
================================================================================
...
Training completed with model saved to: /tmp/qwen_dual_encoder_model
```
