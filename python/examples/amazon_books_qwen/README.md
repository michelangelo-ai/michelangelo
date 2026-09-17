# Amazon Books Qwen Dual-Encoder Demo

Recommendation system demo using Qwen-based dual-encoder architecture for Amazon Books dataset. Demonstrates feature engineering with Chronon, distributed data processing with Spark, and model training with Ray.

## Features

- **Dual-Encoder Architecture**: Qwen-based model for query and document embeddings
- **Feature Engineering**: Chronon-powered feature computation on Spark
- **Dataset**: Kaggle Amazon Books dataset with reviews
- **Distributed Execution**: Ray-based training with configurable resources
- **Configurable**: Support for both local testing and distributed training

## How to Run

Run from the `python/` directory:

```bash
PYTHONPATH="." poetry run python ./examples/amazon_books_qwen/amazon_books_qwen.py
```

Downloading the dataset requires Kaggle API credentials.

## Expected Output

```
================================================================================
Amazon Books Qwen Dual-Encoder Pipeline
================================================================================
Using smaller dataset
================================================================================
...
Training completed! Final train loss: 0.4132, Val loss: 0.5027
```

Loss values will vary between runs. The local run writes its checkpoint to
`/tmp/qwen_dual_encoder_local.pt`; the distributed path
(`distributed=True`) writes to `/tmp/qwen_dual_encoder_distributed.pt`.
