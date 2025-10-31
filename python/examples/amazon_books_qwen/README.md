# Amazon Books Qwen Dual-Encoder Pipeline

This example demonstrates how to build a **GenRec+Qwen dual-encoder model** for recommendation systems using the Amazon Books dataset from Kaggle.

## Architecture

Following the **N3 - GenRec+Qwen** specifications:

- **Model**: Qwen2.5-1.5B (3B parameters in production)
- **Architecture**: Dual encoder with separate query and document towers
- **Embedding Dimension**: 1536
- **Max Query Length**: 128 tokens
- **Max Document Length**: 512 tokens
- **Training**: InfoNCE contrastive loss
- **Pooling**: Sum pooling over hidden states

## Dataset

Uses the [Amazon Books Reviews dataset](https://www.kaggle.com/datasets/mohamedbakhet/amazon-books-reviews):
- 3 million reviews on 212,404 books
- Covers May 1996 - July 2014
- Includes book metadata from Google Books API

## Pipeline Steps

1. **Data Loading** (`data.py`):
   - Extracts Kaggle ZIP file
   - Loads reviews and book metadata
   - Cleans and preprocesses text

2. **Query-Document Pair Creation**:
   - **Queries**: Book titles, review summaries (≤128 tokens)
   - **Documents**: Book descriptions, full reviews (≤512 tokens)
   - **Positive pairs**: Same book content
   - **Negative pairs**: Different books for contrastive learning

3. **Model Training** (`train.py`):
   - Qwen dual-encoder with InfoNCE loss
   - Separate query and document towers
   - Sum pooling and L2 normalization

## Usage

### Prerequisites

1. Download the Amazon Books dataset from Kaggle:
   ```bash
   # Install Kaggle API
   pip install kaggle

   # Download dataset (requires Kaggle API credentials)
   kaggle datasets download -d mohamedbakhet/amazon-books-reviews
   ```

2. Update the dataset path in `amazon_books_qwen.py`:
   ```python
   dataset_path = "/path/to/amazon-books-reviews.zip"
   ```

### Local Training

```bash
# Navigate to the examples directory
cd python/examples/amazon_books_qwen

# Run the pipeline
python3 amazon_books_qwen.py
```

### Remote Training (with Ray)

```bash
python3 amazon_books_qwen.py remote-run \
    --storage-url s3://your-bucket/qwen-training \
    --image your-docker-image:latest
```

## Configuration

### Environment Variables

- `DATA_SIZE`: Number of samples for development (default: 1000)
- `QWEN_MODEL_SIZE`: Model variant - "0.6B", "1.5B", or "8B" (default: "1.5B")
- `MAX_QUERY_LENGTH`: Maximum query tokens (default: 128)
- `MAX_DOC_LENGTH`: Maximum document tokens (default: 512)
- `ENABLE_BF16`: Enable BF16 training (default: True)

### Model Variants

| Model | Parameters | Qwen Model |
|-------|------------|------------|
| QWEN3_0_6B | 1.2B | Qwen/Qwen2.5-0.6B |
| QWEN1_5B | 3B | Qwen/Qwen2.5-1.5B |
| QWEN3_8B | 16B | Qwen/Qwen2.5-8B |

## Output

The pipeline generates:
- Trained dual-encoder model checkpoint
- Training/validation metrics
- Query-document embeddings for similarity search
- Dataset statistics and configurations

## Production Deployment

For production use:
1. Remove `DATA_SIZE` limitation
2. Increase `global_batch_size` to 512 (with 8 H100s)
3. Enable GPU training in Ray configuration
4. Use the 1.5B model (production standard)
5. Implement proper evaluation metrics (recall@k, MRR, etc.)

## Integration with Michelangelo

This pipeline integrates with the broader Michelangelo ecosystem:
- **DeepCVR V6**: Use embeddings as features
- **AutoRegressive GenRec**: Sequential recommendation enhancement
- **BasicDeepNet**: Multi-task learning integration