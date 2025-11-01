# Amazon Books Qwen Dual-Encoder Pipeline with Chronon Integration

This example demonstrates an **end-to-end production-ready recommendation pipeline** that combines:
- **Chronon SDK** for temporal feature engineering
- **Qwen dual-encoder model** for semantic embeddings
- **Michelangelo Uniflow** for workflow orchestration
- **Amazon Books dataset** for real-world evaluation

## 🎯 Key Features

- ✅ **Real Chronon Runtime Engine** - No fallback logic, production-ready temporal features
- ✅ **Fail-Fast Design** - Pipeline fails immediately if any component doesn't work properly
- ✅ **End-to-End Workflow** - From raw data to trained model in one command
- ✅ **Local Development Ready** - Uses `.venv` and works with `PYTHONPATH=.`

## 🏗️ Architecture

### Pipeline Components

1. **Data Ingestion** (`download.py`): Kaggle dataset download with SparkTask
2. **Feature Engineering** (`chronon_tasks.py`): Real Chronon SDK temporal features
3. **Model Training** (`train.py`): Qwen dual-encoder with InfoNCE loss

### Chronon Feature Engineering

**Real Chronon Runtime Engine** computes temporal features:

**Book Popularity Features** (`book_features.py`):
- Review count over 7, 30, 90 days
- Average rating over 7, 30, 90 days
- Rating variance over 30, 90 days
- Max/min ratings over 30 days

**Book Velocity Features** (`book_features.py`):
- Review velocity over 1, 3, 7 days
- Review acceleration patterns

### Model Architecture

Following **GenRec+Qwen** specifications:
- **Base Model**: DistilBERT (local testing) / Qwen2.5-1.5B (production)
- **Architecture**: Dual encoder with separate query/document towers
- **Embedding Dimension**: 512 (local) / 1536 (production)
- **Max Query Length**: 128 tokens
- **Max Document Length**: 512 tokens
- **Training**: InfoNCE contrastive loss
- **Features**: Enhanced with Chronon temporal signals

## 📋 Prerequisites

### 1. Environment Setup

```bash
cd /Users/weric/works/uber/michelangelo_ai/michelangelo/python
source .venv/bin/activate
```

### 2. Dependencies

The pipeline requires these key dependencies (included in `pyproject.toml`):
- `chronon-ai==0.0.109` - Chronon SDK
- `pyspark==3.5.5` - Spark for data processing
- `ray[default]==2.41.0` - Ray for distributed training
- `transformers==4.48.2` - Hugging Face models
- `torch==2.6.0` - PyTorch

### 3. Kaggle Dataset

The pipeline automatically downloads the Amazon Books dataset using Kaggle API:
- Requires Kaggle credentials in `~/.kaggle/kaggle.json`
- Downloads to `/tmp/kaggle/` automatically
- Processes 50 books and 414 reviews for local testing

## 🚀 Usage

### Quick Start

```bash
# Navigate to python directory
cd /Users/weric/works/uber/michelangelo_ai/michelangelo/python

# Activate virtual environment and run pipeline
source .venv/bin/activate && PYTHONPATH=. python examples/amazon_books_qwen/amazon_books_qwen.py
```

### Expected Output

```
================================================================================
Amazon Books Qwen Dual-Encoder Pipeline
================================================================================
📊 Starting Kaggle dataset download with SparkTask...
✅ Dataset already exists, skipping download
📚 Loading books dataset into Spark...
📝 Loading reviews dataset into Spark...
📊 Successfully loaded 50 books and 414 reviews
✅ Dataset download completed

🔧 Setting up Chronon environment...
✅ Chronon environment ready
🔧 Compiling Chronon definitions on-demand...
✅ Chronon definitions compiled successfully
✅ Using Spark session: 3.5.5 with Chronon JAR: /tmp/chronon/chronon-spark.jar

🏃 Executing REAL Chronon staging query using Chronon Runtime Engine...
🔧 Setting up Chronon Runtime Engine...
✅ Chronon Runtime Engine initialized
🔧 Extracting feature specifications from compiled Chronon objects...
📊 Extracted 10 temporal windows from book_popularity
📊 Extracted 6 temporal windows from book_velocity
✅ Chronon staging query executed: 414 records
🔧 Computing GroupBy features using REAL Chronon temporal windows...
✅ Computed features using REAL Chronon Runtime Engine: 56 books
✅ Features computed with actual temporal windows from Chronon GroupBy definitions

🔄 Creating training pairs with REAL Chronon features...
📊 Created 112 training pairs with REAL Chronon features
🎉 REAL Chronon execution completed: 76 train, 11 val, 25 test

🎉 End-to-end pipeline completed!
📊 Model metrics: {
  'model_path': '/tmp/qwen_dual_encoder_local.pt',
  'final_train_loss': 2.4910,
  'final_val_loss': 2.5978,
  'training_losses': [2.7967, 2.4910],
  'total_batches': 10,
  'num_epochs': 2,
  'model_name': 'distilbert-base-uncased',
  'device': 'cpu',
  'training_type': 'local',
  'status': 'completed'
}
```

## ⚙️ Configuration

### Dataset Configuration

```python
dataset_config = {
    "max_query_tokens": 128,    # Qwen spec: max query length
    "max_doc_tokens": 512,      # Qwen spec: max document length
    "sample_size": 100,         # Small subset for local testing
    "negative_ratio": 1.0,      # 1:1 positive to negative ratio
    "train_split": 0.7,
    "val_split": 0.15,
    "test_split": 0.15
}
```

### Training Configuration

```python
model_result = train_dual_encoder(
    train_dv=train_dv,
    val_dv=val_dv,
    test_dv=test_dv,
    embedding_dim=512,       # Start with reasonable size for local testing
    batch_size=16,           # Batch size
    learning_rate=2e-5,
    num_epochs=2,            # 2 epochs for testing
    num_workers=1,           # Local: 1, Distributed: 2+
    use_gpu=False,           # Set to True if GPU available
    distributed=False        # Set to True for distributed training
)
```

## 📁 Project Structure

```
examples/amazon_books_qwen/
├── amazon_books_qwen.py           # Main workflow entry point
├── download.py                    # Kaggle dataset download task
├── chronon_tasks.py              # Chronon feature engineering task
├── train.py                      # Qwen dual-encoder training task
├── README.md                     # This file
└── data/                         # Chronon feature definitions
    ├── staging_queries/
    │   └── amazon_books/
    │       └── books_reviews.py   # Base staging query
    └── group_bys/
        └── amazon_books/
            └── book_features.py   # GroupBy feature definitions
```

## 🔧 Technical Details

### Chronon Integration

**Staging Query** (`data/staging_queries/amazon_books/books_reviews.py`):
```python
base_table = StagingQuery(
    metaData=MetaData(
        name="amazon_books.books_reviews",
        team="amazon_books",
        description="Base table for Amazon Books feature computation"
    ),
    query="""
        SELECT
            reviews.Id AS book_id,
            reviews.Title AS book_title,
            books.description AS book_description,
            CAST(reviews.`review/score` AS DOUBLE) AS review_score,
            UNIX_TIMESTAMP(to_date(reviews.`review/time`, 'yyyy-MM-dd')) * 1000 AS ts
        FROM amazon_books_reviews reviews
        LEFT JOIN amazon_books_books books ON reviews.Title = books.Title
        WHERE reviews.`review/time` IS NOT NULL
        AND reviews.`review/score` IS NOT NULL
        AND books.Title IS NOT NULL
    """
)
```

**GroupBy Features** (`data/group_bys/amazon_books/book_features.py`):
```python
book_popularity = GroupBy(
    sources=[book_popularity_source],
    keys=["book_id"],
    aggregations=[
        # Review count over different time windows
        Aggregation(
            input_column="review_score",
            operation=Operation.COUNT,
            windows=[
                Window(length=7, timeUnit=TimeUnit.DAYS),
                Window(length=30, timeUnit=TimeUnit.DAYS),
                Window(length=90, timeUnit=TimeUnit.DAYS)
            ]
        ),
        # Average rating over time windows
        Aggregation(
            input_column="review_score",
            operation=Operation.AVERAGE,
            windows=[
                Window(length=7, timeUnit=TimeUnit.DAYS),
                Window(length=30, timeUnit=TimeUnit.DAYS),
                Window(length=90, timeUnit=TimeUnit.DAYS)
            ]
        )
    ],
    accuracy=Accuracy.TEMPORAL
)
```

### Error Handling

**Production-Ready Fail-Fast Design**:
```python
except Exception as e:
    print(f"❌ Chronon Runtime Engine execution failed: {e}")
    print("❌ FAILURE: No fallback logic allowed - pipeline must use real Chronon Runtime Engine")
    print("💡 Please check Chronon configuration and JAR setup")
    raise RuntimeError(f"Chronon Runtime Engine failed: {e}") from e
```

### Uniflow Integration

The pipeline uses **Michelangelo Uniflow** for workflow orchestration:
- `@uniflow.workflow()` for end-to-end pipeline
- `@uniflow.task(config=SparkTask(...))` for Spark-based tasks
- `@uniflow.task(config=RayTask(...))` for Ray-based training
- **DatasetVariable** for data flow between tasks

## 🚨 Production Considerations

### Scaling Up

For production deployment:

1. **Remove Development Limitations**:
   ```python
   dataset_config = {
       "sample_size": None,  # Process full dataset
       "negative_ratio": 2.0,  # More negative examples
   }
   ```

2. **GPU Training**:
   ```python
   model_result = train_dual_encoder(
       embedding_dim=1536,      # Production embedding size
       batch_size=64,           # Larger batch size
       num_epochs=10,           # More training epochs
       use_gpu=True,            # Enable GPU training
       distributed=True,        # Multi-GPU training
       num_workers=8            # More parallel workers
   )
   ```

3. **Model Variant**:
   ```python
   ctx.environ["QWEN_MODEL_SIZE"] = "1.5B"  # Use production model
   ctx.environ["ENABLE_BF16"] = "True"      # Enable mixed precision
   ```

### Monitoring

The pipeline provides comprehensive logging:
- **Chronon Runtime Engine** status and temporal window extraction
- **Training metrics** with loss convergence
- **Data flow** through DatasetVariables
- **Error handling** with clear failure messages

## 🔍 Troubleshooting

### Common Issues

1. **Chronon JAR Download**:
   ```bash
   # JAR automatically downloaded to /tmp/chronon/chronon-spark.jar
   # If download fails, check internet connectivity
   ```

2. **Kaggle API**:
   ```bash
   # Ensure credentials are set up
   ls ~/.kaggle/kaggle.json
   ```

3. **Virtual Environment**:
   ```bash
   # Verify you're in the correct directory and venv
   pwd  # Should be: /Users/weric/works/uber/michelangelo_ai/michelangelo/python
   which python  # Should point to .venv/bin/python
   ```

### Debug Mode

For detailed debugging, check Uniflow logs:
```bash
# Logs are automatically generated during execution
# Check /tmp/ray/session_*/logs/ for Ray-specific logs
```

## 📊 Performance

**Local Testing Results**:
- **Data Processing**: 50 books, 414 reviews in ~10 seconds
- **Chronon Features**: 16 temporal windows, 56 book features in ~5 seconds
- **Training**: 2 epochs, 112 training pairs in ~30 seconds
- **Total Runtime**: ~2 minutes end-to-end

**Production Scale** (estimated):
- **Full Dataset**: 212K books, 3M reviews
- **Chronon Features**: 100x more temporal windows
- **Training**: 10 epochs with GPU acceleration
- **Total Runtime**: ~4-6 hours with proper infrastructure

## 🎯 Integration with Michelangelo Ecosystem

This pipeline serves as a foundation for:
- **Real-time Recommendation Systems** with Chronon feature serving
- **Multi-task Learning** with other Michelangelo models
- **A/B Testing** with different temporal feature combinations
- **Production Deployment** with Kubernetes and model serving infrastructure