# Pipeline Plugin for Python/Uniflow

This module provides Python functions for running child pipelines from within Uniflow workflows, matching the functionality of the Go/Starlark pipeline plugin.

## Functions

### `run_pipeline`

Creates and waits for a child pipeline run to complete synchronously.

**Signature:**
```python
def run_pipeline(
    namespace: str,
    pipeline_name: str,
    pipeline_revision: Optional[str] = None,
    environ: Optional[Dict[str, str]] = None,
    args: Optional[List[Any]] = None,
    kwargs: Optional[Dict[str, Any]] = None,
    timeout_seconds: int = 0,
    poll_seconds: int = 10,
    input_data: Optional[Dict[str, Any]] = None,
    actor: Optional[str] = None,
) -> Dict[str, Any]
```

**Parameters:**
- `namespace`: Namespace where the pipeline run will be created (required)
- `pipeline_name`: Name of the pipeline to run (required)
- `pipeline_revision`: Optional git SHA specifying a particular pipeline version for reproducible runs
- `environ`: Optional dictionary of environment variables (map[string]string), typically used for resource configuration
- `args`: Optional list of pipeline-specific arguments
- `kwargs`: Optional dictionary of pipeline-specific keyword configurations (most common way to pass input parameters)
- `timeout_seconds`: Maximum time in seconds to wait for completion (default: 0 = uses default timeout of 10 years)
- `poll_seconds`: Polling interval in seconds (default: 10)
- `input_data`: Optional input parameters for non-Uniflow pipelines. Mutually exclusive with environ/args/kwargs
- `actor`: Optional name of the actor creating the pipeline run (e.g., username or service account)

**Returns:**
```python
{
    "metadata": {
        "name": str,
        "namespace": str
    },
    "status": {
        "state": str  # e.g., "PIPELINE_RUN_STATE_SUCCEEDED"
    }
}
```

**Raises:**
- `ValueError`: If input_data is provided together with environ/args/kwargs, or if required parameters are missing
- `RuntimeError`: If the pipeline run fails or is killed

### `sensor`

Monitors a pipeline run until it reaches a terminal state.

**Signature:**
```python
def sensor(
    namespace: str,
    name: str,
    timeout_seconds: int = 0,
    poll_seconds: int = 10,
) -> Dict[str, Any]
```

**Parameters:**
- `namespace`: Namespace of the pipeline run
- `name`: Name of the pipeline run to monitor
- `timeout_seconds`: Maximum time in seconds to wait for completion (default: 0 = uses default timeout of 10 years)
- `poll_seconds`: Polling interval in seconds (default: 10)

**Returns:**
Same format as `run_pipeline`.

**Raises:**
- `RuntimeError`: If the pipeline run fails or is killed, or if timeout is exceeded

## Examples

### Running a Uniflow pipeline with kwargs

```python
from michelangelo.uniflow.plugins.pipeline import run_pipeline

@uniflow.workflow()
def my_workflow():
    result = run_pipeline(
        namespace="my-project",
        pipeline_name="training-pipeline",
        kwargs={
            "learning_rate": 0.001,
            "batch_size": 32,
            "epochs": 10
        },
        timeout_seconds=3600,  # 1 hour timeout
        poll_seconds=10
    )
    return result
```

### Running with environ for resource configuration

```python
result = run_pipeline(
    namespace="my-project",
    pipeline_name="training-pipeline",
    environ={
        "SPARK_CPU": "4",
        "SPARK_MEMORY": "8Gi"
    },
    kwargs={
        "learning_rate": 0.001,
        "batch_size": 32
    }
)
```

### Running with pipeline revision for reproducibility

```python
result = run_pipeline(
    namespace="my-project",
    pipeline_name="training-pipeline",
    pipeline_revision="abc123def4567890abcdef",
    kwargs={"param": "value"}
)
```

### Running non-Uniflow pipeline with input_data

```python
result = run_pipeline(
    namespace="my-project",
    pipeline_name="eval-pipeline",
    input_data={
        "model_path": "/path/to/model",
        "test_data": "/path/to/test/data",
        "output_path": "/path/to/output"
    }
)
```

### Using sensor separately

```python
from michelangelo.uniflow.plugins.pipeline import run_pipeline, sensor

@uniflow.workflow()
def my_workflow():
    # Create pipeline run
    result = run_pipeline(
        namespace="my-project",
        pipeline_name="training-pipeline",
        kwargs={"param": "value"}
    )
    
    # Do something else...
    
    # Monitor the same run again
    sensor_result = sensor(
        namespace=result["metadata"]["namespace"],
        name=result["metadata"]["name"],
        timeout_seconds=3600,
        poll_seconds=10
    )
    
    return sensor_result
```

## Notes

- This implementation matches the Go/Starlark version exactly in terms of parameters, return types, and behavior
- The function is synchronous and will block until the pipeline run reaches a terminal state
- Failed or killed pipeline runs will raise a `RuntimeError`
- The implementation uses the Michelangelo API client (`APIClient.PipelineRunService`) to create and monitor pipeline runs

