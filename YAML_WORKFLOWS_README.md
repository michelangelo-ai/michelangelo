# Uniflow YAML Workflows

Uniflow now supports dynamic workflow definition using YAML configurations, inspired by DAG Factory patterns. This enables configuration-driven workflow development with support for foreach/expand loops, conditional logic, and complex task dependencies.

## Quick Start

### 1. Define Tasks in Python

```python
# src/data.py
def discover_datasets() -> List[str]:
    """Find available datasets."""
    return ["dataset_001", "dataset_002", "dataset_003"]

def preprocess(dataset_id: str) -> Dict[str, Any]:
    """Preprocess a single dataset."""
    return {"dataset_id": dataset_id, "status": "completed"}
```

### 2. Define Workflow in YAML

```yaml
# workflow.yml
metadata:
  name: "data_pipeline"
  version: "1.0"

defaults:
  storage_url: "s3://bucket/storage"
  image_spec: "ml-base:v1.0"

tasks:
  discover_datasets:
    function: "src.data.discover_datasets"
    config:
      type: "RayTask"
      resources:
        cpu: 2

  preprocess_data:
    function: "src.data.preprocess"
    expand:
      dataset_id: "+discover_datasets"  # Foreach over previous task output
    config:
      type: "SparkTask"
    dependencies: ["discover_datasets"]
```

### 3. Execute Workflow

```bash
# Local execution
python -c "import michelangelo.uniflow as uf; ctx = uf.create_context()" yaml-local-run workflow.yml

# Remote execution
python -c "import michelangelo.uniflow as uf; ctx = uf.create_context()" yaml-remote-run workflow.yml \
    --storage-url s3://bucket/storage \
    --image ml-pipeline:v1.0
```

## YAML Schema Reference

### Basic Structure

```yaml
metadata:
  name: "workflow_name"          # Required: Workflow identifier
  description: "Description"     # Optional: Workflow description
  version: "1.0"                 # Optional: Version string
  author: "team@company.com"     # Optional: Author information

defaults:
  storage_url: "s3://bucket"     # Optional: Default storage location
  image_spec: "image:tag"        # Optional: Default container image
  cache_enabled: true            # Optional: Enable caching by default
  cache_version: "v1"            # Optional: Cache version identifier
  retry_attempts: 2              # Optional: Default retry count

environment:
  variables:                     # Optional: Environment variables
    MODEL_TYPE: "bert"
    BATCH_SIZE: 32
  secrets:                       # Optional: Required secrets
    - "API_KEY"
    - "DATABASE_URL"

tasks:
  # Task definitions here
```

### Task Configuration

```yaml
tasks:
  task_name:
    function: "module.path.function_name"    # Required: Python function to call
    description: "What this task does"       # Optional: Task description

    config:                                  # Optional: Task configuration
      type: "RayTask"                       # Required: RayTask, SparkTask
      resources:                            # Optional: Resource requirements
        cpu: 4
        memory: "8GB"
        gpu: 1
        executor_instances: 3               # Spark only

    inputs:                                 # Optional: Static inputs
      param_name: "static_value"
      data_ref: "+previous_task"            # Reference to previous task output

    dependencies: ["task1", "task2"]        # Optional: Task dependencies
    cache_enabled: true                     # Optional: Enable caching
    retry_attempts: 3                       # Optional: Retry attempts
    image_spec: "custom:image"              # Optional: Custom container image
```

## Dynamic Workflow Patterns

### 1. Foreach/Expand Pattern

Process each item in a list with parallel tasks:

```yaml
tasks:
  # Generate list of items
  generate_items:
    function: "src.data.get_file_list"
    config:
      type: "RayTask"

  # Process each item in parallel
  process_items:
    function: "src.processing.process_file"
    expand:
      filename: "+generate_items"          # Expand over previous task output
      # OR static list:
      # filename: ["file1.txt", "file2.txt", "file3.txt"]
    config:
      type: "SparkTask"
    dependencies: ["generate_items"]
```

### 2. Conditional Logic

Execute different tasks based on runtime conditions:

```yaml
tasks:
  # Task that produces data for condition evaluation
  data_check:
    function: "src.validation.check_data_quality"
    config:
      type: "RayTask"

  # Conditional execution based on check result
  quality_gate:
    function: "src.validation.quality_gate"
    condition:
      field: "quality_score"              # Field to check in task output
      operator: ">"                       # Comparison operator
      value: 0.8                          # Threshold value
      on_true: "train_model"              # Task to execute if condition true
      on_false: "clean_data"              # Task to execute if condition false
    dependencies: ["data_check"]

  # Executed if quality is good
  train_model:
    function: "src.training.train"
    when: "quality_gate.condition_result == true"

  # Executed if quality is poor
  clean_data:
    function: "src.data.cleanup"
    when: "quality_gate.condition_result == false"
```

### 3. Collect/Aggregate Pattern

Gather and aggregate results from multiple dynamic tasks:

```yaml
tasks:
  # Dynamic task that produces multiple results
  parallel_training:
    function: "src.training.train_model"
    expand:
      learning_rate: [0.001, 0.01, 0.1]
      batch_size: [16, 32, 64]
    config:
      type: "RayTask"

  # Collect and aggregate all results
  select_best:
    function: "src.evaluation.select_best_model"
    collect:
      from: "+parallel_training"           # Collect from dynamic task
      strategy: "max"                      # Aggregation strategy: list, sum, max, min
      field: "accuracy"                    # Field to aggregate on
    dependencies: ["parallel_training"]
```

### 4. Complex Expressions

Use complex conditional expressions:

```yaml
tasks:
  complex_condition:
    function: "src.logic.complex_check"
    condition:
      expression: "(data_quality > 0.8 AND sample_size > 1000) OR force_training == true"
      variables:
        data_quality: "+validation.quality_score"
        sample_size: "+data_stats.count"
        force_training: "${FORCE_TRAINING_FLAG}"
```

## Task Reference System

### Output References

Reference outputs from previous tasks using the `+task_name` syntax:

```yaml
tasks:
  load_data:
    function: "src.data.load"

  transform:
    function: "src.transform.apply"
    inputs:
      raw_data: "+load_data"              # Reference to load_data output

  analyze:
    function: "src.analysis.run"
    expand:
      dataset: "+transform"               # Expand over transform outputs
```

### Environment Variables

Reference environment variables using `${VAR_NAME}` syntax:

```yaml
tasks:
  train:
    function: "src.training.train"
    inputs:
      model_type: "${MODEL_TYPE}"         # From environment.variables
      api_key: "${API_KEY}"               # From environment.secrets
```

## Resource Configuration

### Ray Tasks

```yaml
tasks:
  ray_task:
    function: "src.processing.compute"
    config:
      type: "RayTask"
      resources:
        cpu: 4                            # Number of CPUs
        memory: "8GB"                     # Memory allocation
        gpu: 1                            # Number of GPUs (optional)
```

### Spark Tasks

```yaml
tasks:
  spark_task:
    function: "src.processing.big_data"
    config:
      type: "SparkTask"
      resources:
        cpu: 2                            # Driver CPU
        memory: "4GB"                     # Driver memory
        executor_instances: 10            # Number of executors
        executor_cores: 4                 # Cores per executor
```

## CLI Commands

### Local Execution

```bash
# Basic local run
python -c "import michelangelo.uniflow as uf; ctx = uf.create_context()" yaml-local-run workflow.yml

# With environment variables
python -c "import michelangelo.uniflow as uf; ctx = uf.create_context()" yaml-local-run workflow.yml \
    --env MODEL_TYPE=bert \
    --env BATCH_SIZE=32
```

### Remote Execution

```bash
# Basic remote run
python -c "import michelangelo.uniflow as uf; ctx = uf.create_context()" yaml-remote-run workflow.yml \
    --storage-url s3://bucket/storage \
    --image ml-pipeline:v1.0

# With additional options
python -c "import michelangelo.uniflow as uf; ctx = uf.create_context()" yaml-remote-run workflow.yml \
    --storage-url s3://bucket/storage \
    --image ml-pipeline:v1.0 \
    --workflow temporal \
    --cron "0 2 * * *" \
    --file-sync \
    --yes
```

### Validation

```bash
# Validate YAML syntax
python -c "import michelangelo.uniflow as uf; uf.validate_yaml_workflow('workflow.yml')"
```

## Python API

### Direct Usage

```python
import michelangelo.uniflow as uniflow

# Load and execute YAML workflow
workflow_func = uniflow.load_yaml_workflow("workflow.yml")
result = workflow_func()

# Validate YAML workflow
uniflow.validate_yaml_workflow("workflow.yml")
```

### Context Usage

```python
import michelangelo.uniflow as uniflow

# Create context and run
ctx = uniflow.create_context()  # Automatically detects mode from CLI args
result = ctx.run("workflow.yml")  # Pass YAML file path
```

## Migration from Python Workflows

### Before (Python-first)

```python
import michelangelo.uniflow as uniflow
from michelangelo.uniflow.plugins.ray import RayTask

@uniflow.task(config=RayTask(head_cpu=2))
def process_data(input_file):
    return {"processed": input_file}

@uniflow.expand_task(over=["file1", "file2", "file3"])
@uniflow.task(config=RayTask())
def process_files(filename):
    return process_file(filename)

@uniflow.workflow()
def my_workflow():
    result = process_files()
    return result
```

### After (YAML-driven)

```yaml
# workflow.yml
metadata:
  name: "my_workflow"

tasks:
  process_files:
    function: "src.processing.process_file"
    expand:
      filename: ["file1", "file2", "file3"]
    config:
      type: "RayTask"
```

```python
# src/processing.py
def process_file(filename):
    return {"processed": filename}
```

## Best Practices

### 1. Task Function Design

- **Pure functions**: Make tasks stateless and deterministic
- **Clear signatures**: Use type hints for inputs and outputs
- **Error handling**: Let uniflow handle retries, focus on business logic
- **Serializable outputs**: Return JSON-serializable data structures

```python
def process_data(input_path: str, config: Dict[str, Any]) -> Dict[str, Any]:
    """Process data with clear input/output types."""
    # Business logic here
    return {
        "output_path": "s3://bucket/output.parquet",
        "record_count": 1000,
        "status": "completed"
    }
```

### 2. YAML Organization

- **Modular design**: Break complex workflows into smaller, reusable tasks
- **Clear naming**: Use descriptive task and parameter names
- **Documentation**: Add descriptions to workflows and tasks
- **Resource tuning**: Match resource allocation to task requirements

### 3. Dependency Management

- **Explicit dependencies**: Always specify task dependencies clearly
- **Reference validation**: Use `validate_yaml_workflow()` during development
- **Circular detection**: The parser automatically detects circular dependencies

### 4. Environment Management

- **Externalize config**: Use environment variables for configuration
- **Secret management**: Leverage the secrets system for sensitive data
- **Environment isolation**: Use different YAML files for dev/staging/prod

## Examples

See the included example files:
- `example_workflow.yml` - Complete ML pipeline with dynamic patterns
- `example_tasks.py` - Python task functions for the YAML workflow
- `yaml_workflow_demo.py` - CLI demo script showing usage

## Advanced Features

### Custom Aggregation Functions

```python
# Define custom aggregation in Python
def custom_aggregator(results: List[Any]) -> Any:
    return {"custom_metric": sum(r["score"] for r in results)}

# Reference in YAML
tasks:
  aggregate:
    function: "src.aggregation.custom_aggregator"
    collect:
      from: "+parallel_tasks"
      strategy: "custom"
```

### Nested Dynamic Tasks

```yaml
tasks:
  cross_validation:
    function: "src.cv.run_fold"
    expand:
      fold_id: [1, 2, 3, 4, 5]
      model_type: ["bert", "roberta", "gpt"]
    config:
      type: "RayTask"
```

### Complex Task Graphs

```yaml
tasks:
  # Parallel data loading
  load_train:
    function: "src.data.load_train_data"
  load_val:
    function: "src.data.load_val_data"

  # Preprocessing (depends on both)
  preprocess:
    function: "src.data.preprocess_both"
    inputs:
      train_data: "+load_train"
      val_data: "+load_val"
    dependencies: ["load_train", "load_val"]

  # Parallel training experiments
  train_models:
    function: "src.training.train_model"
    expand:
      config: "+generate_configs"
    dependencies: ["preprocess", "generate_configs"]
```

## Troubleshooting

### Common Issues

1. **Import errors**: Ensure Python task functions are importable
2. **Reference errors**: Check that referenced tasks exist and are spelled correctly
3. **Type errors**: Verify function signatures match YAML input/output specifications
4. **Resource errors**: Adjust resource allocations for your cluster configuration

### Debugging Tips

1. **Validate first**: Always run `validate_yaml_workflow()` during development
2. **Local testing**: Test workflows locally before remote deployment
3. **Incremental development**: Build workflows incrementally, testing each part
4. **Log inspection**: Check logs for detailed error messages and execution flow

### Migration Path

1. Start with simple static workflows
2. Add dynamic patterns incrementally
3. Convert existing Python workflows one task at a time
4. Use mixed Python/YAML workflows during transition

## Performance Considerations

- **Caching**: Enable caching for expensive, deterministic tasks
- **Parallelism**: Use expand patterns for CPU-bound parallel work
- **Resource allocation**: Match task resources to computational requirements
- **Storage optimization**: Use appropriate storage formats for data passing