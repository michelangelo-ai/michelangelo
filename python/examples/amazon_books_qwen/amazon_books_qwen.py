"""
Amazon Books Qwen Dual-Encoder Pipeline
Main workflow entry point for training Qwen-based recommendation model
"""

import michelangelo.uniflow.core as uniflow
from examples.amazon_books_qwen.chronon_tasks import compute_chronon_features_with_spark
from examples.amazon_books_qwen.download import download_kaggle_dataset

# Import our local modules using direct file execution to avoid conflicts
# Import workflow functions
from examples.amazon_books_qwen.train import train_dual_encoder
from michelangelo.uniflow.plugins.ray import UF_PLUGIN_RAY_USE_FSSPEC


@uniflow.workflow()
def amazon_books_qwen_workflow(sample_size=100):
    """
    Complete workflow for training Qwen dual-encoder on Amazon Books data
    Following GenRec+Qwen architecture (N3) specifications
    """
    # Step 1: Download dataset (can be cached/reused)
    dataset_config = {
        "max_query_tokens": 128,  # Qwen spec: max query length
        "max_doc_tokens": 512,  # Qwen spec: max document length
        "sample_size": sample_size,  # Small subset for local testing
        "negative_ratio": 1.0,  # 1:1 positive to negative ratio
        "train_split": 0.7,
        "val_split": 0.15,
        "test_split": 0.15,
    }

    books_dv, reviews_dv = download_kaggle_dataset(dataset_config=dataset_config)

    # Step 2: Chronon Feature Engineering and Data Preparation Pipeline
    train_dv, val_dv, test_dv = compute_chronon_features_with_spark(
        dataset_config=dataset_config, books_dv=books_dv, reviews_dv=reviews_dv
    )

    # Step 3: Train dual-encoder model with enhanced data
    model_result = train_dual_encoder(
        train_dv=train_dv,
        val_dv=val_dv,
        test_dv=test_dv,
        embedding_dim=512,  # Start with reasonable size for local testing
        batch_size=16,  # Batch size
        learning_rate=2e-5,
        num_epochs=2,  # 2 epochs for testing
        num_workers=1,  # Local: 1, Distributed: 2+
        use_gpu=False,  # Set to True if GPU available
        distributed=False,  # Set to True for distributed training
    )

    return model_result


# For Local Run from python directory: PYTHONPATH=examples python examples/amazon_books_qwen/amazon_books_qwen.py
# For Remote Run: python examples/amazon_books_qwen/amazon_books_qwen.py remote-run --storage-url <STORAGE_URL> --image <IMAGE>
if __name__ == "__main__":
    print("=" * 80)
    print("Amazon Books Qwen Dual-Encoder Pipeline")
    print("=" * 80)

    ctx = uniflow.create_context()

    # Environment configuration for local testing
    ctx.environ["DATA_SIZE"] = "100"  # Smaller sample for testing
    ctx.environ[UF_PLUGIN_RAY_USE_FSSPEC] = "0"
    ctx.environ["PYTORCH_MPS_HIGH_WATERMARK_RATIO"] = "0"
    ctx.environ["MA_NAMESPACE"] = "default"
    ctx.environ["IMAGE_PULL_POLICY"] = "Never"
    ctx.environ["S3_ALLOW_BUCKET_CREATION"] = "True"

    # Use local model for testing
    ctx.environ["QWEN_MODEL_SIZE"] = "local"  # Use simple local model
    ctx.environ["ENABLE_BF16"] = "False"
    ctx.environ["MAX_QUERY_LENGTH"] = "128"
    ctx.environ["MAX_DOC_LENGTH"] = "512"

    sample_size = 1000
    if ctx.is_local_run():
        print("=" * 80)
        print("Using smaller dataset")
        print("=" * 80)
        sample_size = 100

    # Run the workflow
    result = ctx.run(amazon_books_qwen_workflow, sample_size)
