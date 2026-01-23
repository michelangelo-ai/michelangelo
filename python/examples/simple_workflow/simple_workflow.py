"""
Simple workflow example for testing DAG Factory to Uniflow conversion.

This module contains simple tasks that demonstrate a typical ML workflow:
1. load_data: Loads and returns array of DataFrames (Spark task)
2. preprocess: Processes each DataFrame in ForEach loop (Spark task)
3. train: Collects all preprocess results and does fake training
4. eval_model: Conditional evaluation based on train results
"""

import logging
import random
from typing import Any, Dict, List

import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.spark import SparkTask
from michelangelo.uniflow.plugins.ray import RayTask

log = logging.getLogger(__name__)


@uniflow.task(
    config=SparkTask(
        driver_cpu=2,
        executor_cpu=1,
    ),
    cache_enabled=True,
)
def load_data(data_source: str = "sample_data", num_partitions: int = 3) -> List[Dict[str, Any]]:
    """
    Load data and return an array of Spark DataFrame references.

    Args:
        data_source: The data source to load from
        num_partitions: Number of data partitions to create

    Returns:
        List of dictionaries representing Spark DataFrames
    """
    log.info(f"Loading data from {data_source} with {num_partitions} partitions")

    # Simulate creating multiple DataFrames
    dataframes = []
    for i in range(num_partitions):
        # Simulate a DataFrame with some metadata
        df_info = {
            "partition_id": i,
            "rows": random.randint(1000, 5000),
            "columns": ["id", "feature1", "feature2", "label"],
            "data_source": data_source,
            "partition_name": f"partition_{i}"
        }
        dataframes.append(df_info)
        log.info(f"Created partition {i}: {df_info['rows']} rows")

    log.info(f"Successfully loaded {len(dataframes)} data partitions")
    return dataframes


@uniflow.task(
    config=SparkTask(
        driver_cpu=1,
        executor_cpu=1,
    ),
    cache_enabled=True,
)
def preprocess(partition_data, normalize: bool = True, remove_nulls: bool = True):
    """Simple preprocess function for workflow testing."""
    log.info(f"Processing: {partition_data}")
    return {
        "processed": True,
        "data": partition_data,
        "processed_rows": 1000,  # Fixed value to make training pass
        "data_quality_score": 0.9  # Fixed value to make training pass
    }


@uniflow.task(
    config=RayTask(
        head_cpu=4,
        head_memory="8Gi",
        worker_cpu=2,
        worker_memory="4Gi",
        worker_instances=2,
    ),
    cache_enabled=True,
)
def train(processed_partitions, model_type: str = "simple_classifier", epochs: int = 10):
    """Simple train function for workflow testing."""
    log.info(f"Starting training")
    log.info(f"Model type: {model_type}, Epochs: {epochs}")
    log.info(f"Processed partitions: {processed_partitions}")

    # Simple success result
    return {
        "success": True,
        "model_type": model_type,
        "training_accuracy": 0.85,
        "total_samples": 3000,
        "data_quality": 0.9,
        "epochs_completed": epochs,
        "partitions_used": 3
    }


@uniflow.task(
    config=RayTask(
        head_cpu=2,
        head_memory="4Gi",
        worker_cpu=1,
        worker_memory="2Gi",
        worker_instances=1,
    ),
    cache_enabled=False,
)
def eval_model(training_result, eval_dataset_size: int = 500):
    """Simple eval_model function for workflow testing."""
    log.info("Starting model evaluation")
    log.info(f"Training result: {training_result}")
    log.info(f"Eval dataset size: {eval_dataset_size}")

    # Simple success result
    return {
        "evaluation_completed": True,
        "model_type": "simple_classifier",
        "eval_accuracy": 0.82,
        "training_accuracy": 0.85,
        "performance_delta": -0.03
    }


@uniflow.workflow()
def simple_workflow_demo():
    """
    Simple workflow demonstration for testing DAG Factory to Uniflow conversion.

    This workflow demonstrates the actual Starlark execution pattern:
    1. load_data: Spark task that loads and partitions data
    2. preprocess: ForEach Spark task processing each partition
    3. train: Ray task that trains on all processed partitions
    4. eval_model: Conditional Ray task that evaluates if training succeeded
    """
    # Task: load_data
    load_data_result = load_data(data_source="demo_data", num_partitions=3)

    # Task: preprocess_partitions (ForEach pattern)
    preprocess_partitions_results = []
    for item_value in load_data_result:
        iteration_result = preprocess(
            partition_data=item_value,
            normalize=True,
            remove_nulls=True
        )
        preprocess_partitions_results.append(iteration_result)

    # Task: train_model
    train_model_result = train(
        processed_partitions=preprocess_partitions_results,
        model_type="simple_classifier",
        epochs=10
    )

    # Task: evaluate_model (conditional)
    if train_model_result.get("success", False):
        eval_model(
            training_result=train_model_result,
            eval_dataset_size=500
        )
    return "done"

if __name__ == "__main__":
    # For Local Run: python3 examples/simple_workflow/simple_workflow.py
    ctx = uniflow.create_context()

    # Set environment variables for demo
    ctx.environ["IMAGE_PULL_POLICY"] = "Never"
    ctx.environ["MA_NAMESPACE"] = "default"

    # Run the workflow
    ctx.run(simple_workflow_demo)