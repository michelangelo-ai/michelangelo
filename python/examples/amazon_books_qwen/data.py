"""
Data loading and preprocessing for Amazon Books Qwen Pipeline
Handles dataset extraction, text cleaning, and query-document pair creation
"""

import logging
import os
import zipfile
from typing import Dict, Tuple
import pandas as pd
import numpy as np
import re
import json
from pathlib import Path

import ray
from ray.data import Dataset
from michelangelo.uniflow.plugins.ray import RayTask
import michelangelo.uniflow.core as uniflow

log = logging.getLogger(__name__)


@uniflow.task(
    config=RayTask(
        head_cpu=2,
        head_memory="8Gi",
        worker_cpu=2,
        worker_memory="8Gi",
        worker_instances=2,
    )
)
def load_amazon_data(
    dataset_path: str,
    max_query_tokens: int = 128,
    max_doc_tokens: int = 512
) -> Dict[str, Dataset]:
    """
    Load and preprocess Amazon books dataset from Kaggle ZIP file

    Args:
        dataset_path: Path to the downloaded Kaggle ZIP file
        max_query_tokens: Maximum tokens for queries (Qwen spec: 128)
        max_doc_tokens: Maximum tokens for documents (Qwen spec: 512)

    Returns:
        Dictionary containing processed Ray datasets
    """
    log.info(f"Loading Amazon Books dataset from: {dataset_path}")

    # Extract ZIP file
    extract_dir = "/tmp/amazon_books_extracted"
    os.makedirs(extract_dir, exist_ok=True)

    if dataset_path.endswith('.zip'):
        with zipfile.ZipFile(dataset_path, 'r') as zip_ref:
            zip_ref.extractall(extract_dir)
            log.info(f"Extracted dataset to: {extract_dir}")

    # Find CSV files (adjust based on actual file names)
    csv_files = list(Path(extract_dir).glob("*.csv"))
    log.info(f"Found CSV files: {csv_files}")

    # Load main datasets
    reviews_file = next((f for f in csv_files if 'review' in f.name.lower()), csv_files[0])
    books_file = next((f for f in csv_files if 'book' in f.name.lower()), None)

    reviews_df = pd.read_csv(reviews_file)
    log.info(f"Loaded {len(reviews_df)} reviews")

    if books_file:
        books_df = pd.read_csv(books_file)
        log.info(f"Loaded {len(books_df)} books")
        # Merge reviews with book details
        merged_df = reviews_df.merge(books_df, on='Id', how='left', suffixes=('_review', '_book'))
    else:
        merged_df = reviews_df
        log.warning("No separate books file found, using reviews only")

    # Clean and preprocess text
    def clean_text(text: str, max_tokens: int) -> str:
        """Clean and truncate text for Qwen model"""
        if pd.isna(text):
            return ""

        text = str(text)
        # Remove HTML tags and normalize whitespace
        text = re.sub(r'<[^>]+>', '', text)
        text = re.sub(r'\s+', ' ', text)
        text = text.strip()

        # Rough token approximation (4 chars = 1 token)
        if len(text) > max_tokens * 4:
            text = text[:max_tokens * 4]

        return text

    # Apply text cleaning
    text_columns = ['Title', 'review/summary', 'review/text', 'Description']
    for col in text_columns:
        if col in merged_df.columns:
            max_tokens = max_query_tokens if col in ['Title', 'review/summary'] else max_doc_tokens
            merged_df[f'{col}_clean'] = merged_df[col].apply(lambda x: clean_text(x, max_tokens))

    # Create Ray dataset
    dataset = ray.data.from_pandas(merged_df)

    # Sample for development (remove in production)
    data_size = int(os.environ.get("DATA_SIZE", "1000"))
    if data_size > 0:
        dataset = dataset.random_sample(min(data_size / len(merged_df), 1.0), seed=42)
        log.info(f"Sampled dataset to {data_size} records")

    return {"raw_data": dataset}


@uniflow.task(
    config=RayTask(
        head_cpu=4,
        head_memory="16Gi",
        worker_cpu=4,
        worker_memory="16Gi",
        worker_instances=2,
    )
)
def create_training_pairs(
    raw_data: Dict[str, Dataset],
    negative_ratio: float = 1.0,
    train_split: float = 0.7,
    val_split: float = 0.15,
    test_split: float = 0.15
) -> Dict[str, Dataset]:
    """
    Create query-document pairs for Qwen dual-encoder training
    Implements InfoNCE contrastive learning data preparation

    Args:
        raw_data: Raw dataset from load_amazon_data
        negative_ratio: Ratio of negative to positive pairs
        train_split: Training data proportion
        val_split: Validation data proportion
        test_split: Test data proportion

    Returns:
        Dictionary with train/val/test datasets containing query-document pairs
    """
    log.info("Creating query-document pairs for contrastive learning")

    dataset = raw_data["raw_data"]

    def create_pairs_batch(batch: Dict[str, np.ndarray]) -> Dict[str, list]:
        """Create positive and negative query-document pairs from batch"""
        batch_size = len(batch["Id"])
        pairs = {
            "query": [],
            "document": [],
            "label": [],
            "book_id": []
        }

        for i in range(batch_size):
            book_id = batch["Id"][i] if "Id" in batch else f"book_{i}"

            # Create positive pairs (same book)
            queries = []
            documents = []

            # Query candidates (max 128 tokens)
            if "Title_clean" in batch and batch["Title_clean"][i]:
                queries.append(batch["Title_clean"][i])
            if "review/summary_clean" in batch and batch["review/summary_clean"][i]:
                queries.append(batch["review/summary_clean"][i])

            # Document candidates (max 512 tokens)
            if "Description_clean" in batch and batch["Description_clean"][i]:
                documents.append(batch["Description_clean"][i])
            if "review/text_clean" in batch and batch["review/text_clean"][i]:
                documents.append(batch["review/text_clean"][i])

            # Create positive pairs
            for query in queries:
                for doc in documents:
                    if query.strip() and doc.strip():
                        pairs["query"].append(query)
                        pairs["document"].append(doc)
                        pairs["label"].append(1)  # Positive
                        pairs["book_id"].append(str(book_id))

        # Create negative pairs
        num_positives = len(pairs["query"])
        num_negatives = int(num_positives * negative_ratio)

        for _ in range(num_negatives):
            # Random mismatched query-document pairs
            if num_positives > 1:
                query_idx = np.random.randint(0, num_positives)
                doc_idx = np.random.randint(0, num_positives)

                # Ensure different books
                if pairs["book_id"][query_idx] != pairs["book_id"][doc_idx]:
                    pairs["query"].append(pairs["query"][query_idx])
                    pairs["document"].append(pairs["document"][doc_idx])
                    pairs["label"].append(0)  # Negative
                    pairs["book_id"].append(f"neg_{pairs['book_id'][query_idx]}_{pairs['book_id'][doc_idx]}")

        return pairs

    # Create pairs
    pairs_dataset = dataset.map_batches(
        create_pairs_batch,
        batch_format="numpy",
        batch_size=100
    )

    log.info("Created query-document pairs")

    # Split into train/val/test
    # Ray Dataset doesn't have drop(), use limit() and skip() instead
    shuffled = pairs_dataset.random_shuffle(seed=42)

    total_count = shuffled.count()
    train_count = int(total_count * train_split)
    val_count = int(total_count * val_split)

    # Use take() instead of limit() and skip blocks for the remaining data
    train_data = shuffled.take(train_count)
    train_data = ray.data.from_items(train_data)

    remaining_data = shuffled.take_all()[train_count:]  # Skip train data
    val_data = ray.data.from_items(remaining_data[:val_count])
    test_data = ray.data.from_items(remaining_data[val_count:])

    log.info(f"Dataset splits - Train: {len(train_data.take_all()) if hasattr(train_data, 'take_all') else train_count}, Val: {len(val_data.take_all()) if hasattr(val_data, 'take_all') else val_count}, Test: {len(test_data.take_all()) if hasattr(test_data, 'take_all') else 'remaining'}")

    return {
        "train": train_data,
        "validation": val_data,
        "test": test_data
    }