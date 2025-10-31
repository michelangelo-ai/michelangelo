"""
Amazon Books Qwen Dual-Encoder Pipeline
Main workflow entry point for training Qwen-based recommendation model
"""

import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import UF_PLUGIN_RAY_USE_FSSPEC

# Import our local modules
import sys
import os
from pathlib import Path

# Add current directory to path for local imports
current_dir = Path(__file__).parent
sys.path.insert(0, str(current_dir))

from data import load_amazon_data, create_training_pairs
from train import train_dual_encoder  # Use unified training function


@uniflow.workflow()
def amazon_books_qwen_workflow():
    """
    Complete workflow for training Qwen dual-encoder on Amazon Books data
    Following GenRec+Qwen architecture (N3) specifications
    """

    # Step 1: Load and preprocess Amazon books dataset
    dataset_path = "/tmp/amazon-books-reviews.zip"  # Update with actual path

    raw_data = load_amazon_data(
        dataset_path=dataset_path,
        max_query_tokens=128,    # Qwen spec: max query length
        max_doc_tokens=512       # Qwen spec: max document length
    )

    # Step 2: Create query-document pairs for contrastive learning
    training_data = create_training_pairs(
        raw_data=raw_data,
        negative_ratio=1.0,      # 1:1 positive to negative ratio
        train_split=0.7,
        val_split=0.15,
        test_split=0.15
    )

    # Step 3: Train dual-encoder model with unified training function
    model_result = train_dual_encoder(
        train_data=training_data["train"],
        val_data=training_data["validation"],
        test_data=training_data["test"],
        embedding_dim=1536,      # Qwen spec: 1536 dimensions
        batch_size=32,           # Batch size
        learning_rate=2e-5,
        num_epochs=3,
        num_workers=1,           # Local: 1, Distributed: 2+
        use_gpu=False,           # Set to True if GPU available
        distributed=False        # Set to True for distributed training
    )

    print("Training completed!")
    print(f"Model metrics: {model_result}")
    return model_result


# For Local Run: python3 examples/amazon_books_qwen/amazon_books_qwen.py
# For Remote Run: python3 examples/amazon_books_qwen/amazon_books_qwen.py remote-run --storage-url <STORAGE_URL> --image <IMAGE>
if __name__ == "__main__":
    ctx = uniflow.create_context()

    # Environment configuration
    ctx.environ["DATA_SIZE"] = "1000"  # Number of samples for testing
    ctx.environ[UF_PLUGIN_RAY_USE_FSSPEC] = "0"
    ctx.environ["PYTORCH_MPS_HIGH_WATERMARK_RATIO"] = "0"
    ctx.environ["MA_NAMESPACE"] = "default"
    ctx.environ["IMAGE_PULL_POLICY"] = "Never"
    ctx.environ["S3_ALLOW_BUCKET_CREATION"] = "True"

    # Qwen model specific environment
    ctx.environ["QWEN_MODEL_SIZE"] = "1.5B"  # Options: 0.6B, 1.5B, 8B
    ctx.environ["ENABLE_BF16"] = "True"
    ctx.environ["MAX_QUERY_LENGTH"] = "128"
    ctx.environ["MAX_DOC_LENGTH"] = "512"

    ctx.run(amazon_books_qwen_workflow)