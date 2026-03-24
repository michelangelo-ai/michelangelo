"""
Simple Flyte Workflow Example for Michelangelo Integration

This example demonstrates how to write a standard Flyte workflow that
can be registered and executed through the Michelangelo platform.
"""

from flytekit import task, workflow, Resources
from flytekit.types.file import FlyteFile
from typing import Dict, Any


@task(
    requests=Resources(cpu="2", mem="4Gi"),
    cache=True,
    cache_version="1.0"
)
def preprocess_data(input_file: FlyteFile, config: Dict[str, Any]) -> FlyteFile:
    """
    Preprocess input data file.

    Args:
        input_file: Input CSV file to preprocess
        config: Configuration parameters

    Returns:
        Preprocessed data file
    """
    import pandas as pd
    import tempfile
    import os

    # Download the input file
    local_input = input_file.download()

    # Load data
    df = pd.read_csv(local_input)

    # Apply preprocessing based on config
    sample_size = config.get("sample_size", len(df))
    if sample_size < len(df):
        df = df.sample(n=sample_size, random_state=42)

    # Feature engineering
    numeric_columns = df.select_dtypes(include=['number']).columns
    for col in numeric_columns:
        # Simple normalization
        df[f"{col}_normalized"] = (df[col] - df[col].mean()) / df[col].std()

    # Save preprocessed data
    output_path = tempfile.mktemp(suffix=".csv")
    df.to_csv(output_path, index=False)

    return FlyteFile(path=output_path)


@task(
    requests=Resources(cpu="4", mem="8Gi", gpu="1"),
    cache=True,
    cache_version="1.0"
)
def train_model(data_file: FlyteFile, model_config: Dict[str, Any]) -> FlyteFile:
    """
    Train a machine learning model.

    Args:
        data_file: Preprocessed training data
        model_config: Model training configuration

    Returns:
        Trained model file
    """
    import pandas as pd
    import joblib
    import tempfile
    from sklearn.ensemble import RandomForestRegressor
    from sklearn.model_selection import train_test_split

    # Load preprocessed data
    local_data = data_file.download()
    df = pd.read_csv(local_data)

    # Prepare features and target
    target_column = model_config.get("target_column", "target")
    if target_column not in df.columns:
        # For demo, use the first numeric column as target
        numeric_cols = df.select_dtypes(include=['number']).columns
        target_column = numeric_cols[0] if len(numeric_cols) > 0 else df.columns[0]

    feature_columns = [col for col in df.columns if col != target_column and col.endswith('_normalized')]
    if not feature_columns:
        # Fall back to numeric columns
        feature_columns = [col for col in df.select_dtypes(include=['number']).columns if col != target_column]

    X = df[feature_columns]
    y = df[target_column]

    # Split data
    X_train, X_test, y_train, y_test = train_test_split(X, y, test_size=0.2, random_state=42)

    # Train model
    model = RandomForestRegressor(
        n_estimators=model_config.get("n_estimators", 100),
        max_depth=model_config.get("max_depth", 10),
        random_state=42
    )
    model.fit(X_train, y_train)

    # Evaluate
    train_score = model.score(X_train, y_train)
    test_score = model.score(X_test, y_test)

    print(f"Model training completed:")
    print(f"  Training score: {train_score:.4f}")
    print(f"  Test score: {test_score:.4f}")
    print(f"  Features used: {len(feature_columns)}")

    # Save model
    output_path = tempfile.mktemp(suffix=".pkl")
    joblib.dump({
        'model': model,
        'feature_columns': feature_columns,
        'target_column': target_column,
        'train_score': train_score,
        'test_score': test_score,
        'config': model_config
    }, output_path)

    return FlyteFile(path=output_path)


@task(
    requests=Resources(cpu="1", mem="2Gi"),
    cache=True,
    cache_version="1.0"
)
def evaluate_model(model_file: FlyteFile, test_data: FlyteFile) -> Dict[str, float]:
    """
    Evaluate the trained model.

    Args:
        model_file: Trained model file
        test_data: Test dataset

    Returns:
        Evaluation metrics
    """
    import pandas as pd
    import joblib
    import numpy as np
    from sklearn.metrics import mean_squared_error, mean_absolute_error, r2_score

    # Load model and test data
    model_data = joblib.load(model_file.download())
    model = model_data['model']
    feature_columns = model_data['feature_columns']
    target_column = model_data['target_column']

    test_df = pd.read_csv(test_data.download())

    # Prepare test features and target
    X_test = test_df[feature_columns]
    y_test = test_df[target_column]

    # Make predictions
    y_pred = model.predict(X_test)

    # Calculate metrics
    metrics = {
        'mse': float(mean_squared_error(y_test, y_pred)),
        'mae': float(mean_absolute_error(y_test, y_pred)),
        'r2': float(r2_score(y_test, y_pred)),
        'rmse': float(np.sqrt(mean_squared_error(y_test, y_pred)))
    }

    print(f"Model evaluation completed:")
    for metric, value in metrics.items():
        print(f"  {metric.upper()}: {value:.4f}")

    return metrics


@workflow
def simple_training_pipeline(
    input_data: FlyteFile,
    preprocessing_config: Dict[str, Any] = None,
    model_config: Dict[str, Any] = None
) -> Dict[str, Any]:
    """
    Simple ML training pipeline workflow.

    This workflow demonstrates a typical machine learning pipeline:
    1. Data preprocessing
    2. Model training
    3. Model evaluation

    Args:
        input_data: Raw input data file
        preprocessing_config: Configuration for data preprocessing
        model_config: Configuration for model training

    Returns:
        Dictionary containing evaluation metrics and model info
    """
    # Set defaults
    if preprocessing_config is None:
        preprocessing_config = {"sample_size": 1000}

    if model_config is None:
        model_config = {
            "n_estimators": 100,
            "max_depth": 10,
            "target_column": "target"
        }

    # Step 1: Preprocess data
    processed_data = preprocess_data(
        input_file=input_data,
        config=preprocessing_config
    )

    # Step 2: Train model
    trained_model = train_model(
        data_file=processed_data,
        model_config=model_config
    )

    # Step 3: Evaluate model (using same processed data for simplicity)
    metrics = evaluate_model(
        model_file=trained_model,
        test_data=processed_data
    )

    # Return results
    return {
        "metrics": metrics,
        "model_config": model_config,
        "preprocessing_config": preprocessing_config
    }


if __name__ == "__main__":
    # This section shows how the workflow could be tested locally
    # In Michelangelo integration, this would not be executed directly

    print("🔍 This is a Flyte workflow for Michelangelo integration")
    print("📝 To register this workflow:")
    print("   mactl flyte register simple_workflow.py --namespace ml-demo --author your-name")
    print("🚀 To execute:")
    print("   mactl flyte execute simple_training_pipeline --input input_data=s3://bucket/data.csv")
    print("📊 To check status:")
    print("   mactl flyte status <execution-id>")
    print("📈 To get outputs:")
    print("   mactl flyte outputs <execution-id>")