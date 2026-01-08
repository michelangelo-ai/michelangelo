"""Example Python task functions for YAML workflow integration.

These functions show how to write task functions that can be referenced
from YAML workflow configurations. They demonstrate the function signatures
and return types expected by the YAML workflow system.
"""

import time
import random
from typing import List, Dict, Any
import pandas as pd


# Data processing functions
def discover_datasets() -> List[str]:
    """Discover available datasets for training.

    Returns:
        List of dataset identifiers
    """
    print("Discovering available datasets...")
    time.sleep(1)  # Simulate discovery process
    datasets = ["dataset_001", "dataset_002", "dataset_003", "dataset_004"]
    print(f"Found {len(datasets)} datasets: {datasets}")
    return datasets


def preprocess(dataset_id: str) -> Dict[str, Any]:
    """Preprocess a single dataset.

    Args:
        dataset_id: The ID of the dataset to preprocess

    Returns:
        Dictionary containing preprocessed data info
    """
    print(f"Preprocessing dataset: {dataset_id}")
    time.sleep(2)  # Simulate preprocessing

    # Simulate different data sizes
    num_samples = random.randint(1000, 10000)
    num_features = random.randint(50, 200)

    result = {
        "dataset_id": dataset_id,
        "num_samples": num_samples,
        "num_features": num_features,
        "preprocessed_path": f"s3://bucket/preprocessed/{dataset_id}.parquet",
        "status": "completed"
    }
    print(f"Completed preprocessing {dataset_id}: {num_samples} samples, {num_features} features")
    return result


def merge_all(dataset_list: List[Dict[str, Any]]) -> Dict[str, Any]:
    """Merge all preprocessed datasets.

    Args:
        dataset_list: List of preprocessed dataset info dictionaries

    Returns:
        Dictionary containing merged dataset info
    """
    print(f"Merging {len(dataset_list)} preprocessed datasets...")
    time.sleep(1)

    total_samples = sum(d["num_samples"] for d in dataset_list)
    total_features = max(d["num_features"] for d in dataset_list)

    merged_data = {
        "total_samples": total_samples,
        "total_features": total_features,
        "merged_path": "s3://bucket/merged/all_datasets.parquet",
        "source_datasets": [d["dataset_id"] for d in dataset_list],
        "status": "completed"
    }
    print(f"Merged dataset: {total_samples} total samples, {total_features} features")
    return merged_data


# Validation functions
def check_quality(merged_data: Dict[str, Any]) -> Dict[str, Any]:
    """Check the quality of merged dataset.

    Args:
        merged_data: Information about the merged dataset

    Returns:
        Dictionary with quality metrics and score
    """
    print("Checking data quality...")
    time.sleep(1)

    # Simulate quality check based on sample size
    sample_ratio = merged_data["total_samples"] / 50000  # Target 50k samples
    feature_ratio = merged_data["total_features"] / 100   # Target 100 features

    # Calculate quality score (0-1)
    quality_score = min(1.0, (sample_ratio + feature_ratio) / 2)

    quality_result = {
        "quality_score": quality_score,
        "sample_adequacy": sample_ratio,
        "feature_adequacy": feature_ratio,
        "recommendation": "train" if quality_score > 0.8 else "cleanup",
        "merged_data": merged_data
    }

    print(f"Quality check completed: score = {quality_score:.2f}")
    return quality_result


# Training functions
def train_model(training_data: Dict[str, Any], model_type: str = "bert") -> Dict[str, Any]:
    """Train ML model with high-quality data.

    Args:
        training_data: Information about training data
        model_type: Type of model to train

    Returns:
        Dictionary with trained model info
    """
    print(f"Training {model_type} model with {training_data['total_samples']} samples...")
    time.sleep(3)  # Simulate training time

    # Simulate training results
    accuracy = 0.85 + random.random() * 0.1  # 0.85-0.95

    model_result = {
        "model_type": model_type,
        "accuracy": accuracy,
        "model_path": f"s3://bucket/models/{model_type}_model.pkl",
        "training_samples": training_data["total_samples"],
        "training_time_minutes": 45,
        "status": "completed"
    }

    print(f"Model training completed: {model_type} accuracy = {accuracy:.3f}")
    return model_result


def train_with_params(learning_rate: float, batch_size: int = 32) -> Dict[str, Any]:
    """Train model with specific hyperparameters.

    Args:
        learning_rate: Learning rate for training
        batch_size: Batch size for training (default: 32)

    Returns:
        Dictionary with training results
    """
    print(f"Training with lr={learning_rate}, batch_size={batch_size}")
    time.sleep(2)  # Simulate training

    # Simulate accuracy based on hyperparameters (better with moderate values)
    lr_score = 1.0 - abs(learning_rate - 0.01) * 10  # Optimal around 0.01
    batch_score = 1.0 - abs(batch_size - 32) / 64     # Optimal around 32
    accuracy = 0.8 + (lr_score + batch_score) * 0.1
    accuracy = max(0.8, min(0.95, accuracy))  # Clamp between 0.8-0.95

    result = {
        "learning_rate": learning_rate,
        "batch_size": batch_size,
        "accuracy": accuracy,
        "val_loss": 2.5 - accuracy,  # Inverse relationship
        "model_path": f"s3://bucket/models/lr_{learning_rate}_bs_{batch_size}.pkl",
        "status": "completed"
    }

    print(f"Hyperparameter run completed: lr={learning_rate}, bs={batch_size}, acc={accuracy:.3f}")
    return result


# Data cleanup functions
def advanced_cleanup(raw_data: Dict[str, Any]) -> Dict[str, Any]:
    """Perform advanced data cleaning for poor quality data.

    Args:
        raw_data: Information about raw/merged data

    Returns:
        Dictionary with cleaned data info
    """
    print("Performing advanced data cleanup...")
    time.sleep(4)  # Simulate cleanup time

    # Simulate data cleaning (improve sample count)
    improved_samples = int(raw_data["total_samples"] * 1.2)  # 20% improvement

    cleaned_data = {
        "original_samples": raw_data["total_samples"],
        "cleaned_samples": improved_samples,
        "improvement_ratio": 1.2,
        "cleaned_path": "s3://bucket/cleaned/improved_dataset.parquet",
        "cleanup_methods": ["outlier_removal", "data_augmentation", "noise_reduction"],
        "status": "completed"
    }

    print(f"Data cleanup completed: {raw_data['total_samples']} → {improved_samples} samples")
    return cleaned_data


# Evaluation and selection functions
def select_best(collected_results: List[Dict[str, Any]]) -> Dict[str, Any]:
    """Select the best model from hyperparameter search results.

    Args:
        collected_results: List of hyperparameter search results

    Returns:
        Dictionary with best model info
    """
    print(f"Selecting best model from {len(collected_results)} candidates...")

    # Find the model with highest accuracy
    best_model = max(collected_results, key=lambda x: x["accuracy"])

    selection_result = {
        "best_model": best_model,
        "best_accuracy": best_model["accuracy"],
        "best_learning_rate": best_model["learning_rate"],
        "best_batch_size": best_model["batch_size"],
        "total_candidates": len(collected_results),
        "accuracy_range": {
            "min": min(r["accuracy"] for r in collected_results),
            "max": max(r["accuracy"] for r in collected_results),
            "mean": sum(r["accuracy"] for r in collected_results) / len(collected_results)
        },
        "selected_model_path": best_model["model_path"]
    }

    print(f"Best model selected: lr={best_model['learning_rate']}, bs={best_model['batch_size']}, acc={best_model['accuracy']:.3f}")
    return selection_result


# Deployment functions
def deploy_to_production(best_model: Dict[str, Any]) -> Dict[str, Any]:
    """Deploy the best model to production.

    Args:
        best_model: Information about the selected best model

    Returns:
        Dictionary with deployment info
    """
    print("Deploying model to production...")
    time.sleep(2)

    deployment_result = {
        "model_path": best_model["selected_model_path"],
        "model_accuracy": best_model["best_accuracy"],
        "deployment_endpoint": "https://api.ml-service.com/v1/predict",
        "deployment_time": "2024-01-08T10:30:00Z",
        "version": "v1.2.0",
        "status": "deployed",
        "health_check_url": "https://api.ml-service.com/v1/health"
    }

    print(f"Model deployed successfully: {deployment_result['deployment_endpoint']}")
    return deployment_result


if __name__ == "__main__":
    # Test the functions individually
    print("Testing task functions...")

    # Test discovery
    datasets = discover_datasets()

    # Test preprocessing
    preprocessed = [preprocess(ds) for ds in datasets[:2]]  # Test with first 2 datasets

    # Test merge
    merged = merge_all(preprocessed)

    # Test quality check
    quality = check_quality(merged)

    print(f"All tests completed. Final quality score: {quality['quality_score']:.2f}")