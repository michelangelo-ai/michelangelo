# Michelangelo Terminology Guide

This glossary defines core concepts used throughout Michelangelo documentation. Use this as the authoritative reference for consistent terminology.

---

## Core Workflow Concepts

### Task
A **task** is a discrete unit of work in a Michelangelo workflow.

- **Definition**: A Python function decorated with `@task` that performs a single, focused operation
- **Execution**: Runs in a container on Kubernetes (using Ray or Spark)
- **Scope**: Data preparation, model training, evaluation, or any other discrete work
- **Key Feature**: Tasks are independently cacheable and retryable
- **Decorator**: Use `@uniflow.task()` or `@task()` (equivalent, but prefer `@uniflow.task()` for consistency)

**Example**:
```python
import michelangelo.uniflow.core as uniflow

@uniflow.task()
def prepare_data(data_path: str):
    """Load and process raw data."""
    # Task implementation
    return processed_data
```

### Workflow
A **workflow** is a Python function that orchestrates one or more tasks.

- **Definition**: A Python function decorated with `@workflow` that controls task execution
- **Execution**: Runs in a Cadence/Temporal worker (compiled to Starlark for deterministic replay)
- **Scope**: Task sequencing, branching logic, conditional execution, and loops
- **Key Feature**: Separates workflow orchestration from task execution (tasks run in containers, workflows run in Temporal)
- **Decorator**: Use `@uniflow.workflow()`

**Example**:
```python
@uniflow.workflow()
def training_pipeline(data_path: str):
    """Orchestrate data prep and model training."""
    train_data, val_data = prepare_data(data_path)
    model = train_model(train_data, val_data)
    return model
```

### Pipeline
A **pipeline** is a deployable, versioned instance of a workflow.

- **Definition**: A concrete deployment configuration bound to a workflow
- **Configuration**: Defined in `pipeline.yaml` or configured via MA Studio UI
- **Lifecycle**: Registered with the `ma` CLI, versioned, and tracked
- **Execution**: Can be triggered manually, via cron schedule, or via API
- **Key Feature**: Separates workflow definition (code) from deployment configuration
- **Note**: The word "pipeline" is also used informally to mean "workflow" in some contexts (this is the ambiguity we're clarifying)

**Configuration Example**:
```yaml
apiVersion: michelangelo.api/v2
kind: Pipeline
metadata:
  name: training-pipeline
  namespace: my-project
spec:
  workflow: training_pipeline  # References the @workflow function
  image: my-training:latest
  resources:
    cpu: "4"
    memory: "16Gi"
```

---

## Execution & Runtime Concepts

### Context
A **context** is the runtime entry point for executing a workflow.

- **Definition**: Object that holds runtime information needed to execute a workflow
- **Contains**: Environment variables, input arguments, configuration parameters
- **Usage**: Workflows receive a context as their entry point
- **Scope**: Isolated per execution (each PipelineRun gets its own context)

**Example**:
```python
@uniflow.workflow()
def training_pipeline(context):
    # Access runtime configuration
    data_path = context.get_param("data_path")
    environment = context.get_env("ENVIRONMENT")
    return ...
```

### PipelineRun
A **PipelineRun** is a single execution instance of a pipeline.

- **Definition**: Concrete execution of a pipeline with specific input parameters and timing
- **Lifecycle**: Created, running, succeeded, failed, or suspended
- **Tracking**: Each PipelineRun has logs, metrics, and can be monitored
- **Resume**: Failed PipelineRuns can be resumed from a specific step
- **Naming**: Automatically generated with timestamp and unique ID

**Example identifiers**:
- `training-pipeline-20260307-abc123`
- `training-pipeline-20260307-def456`

### Trigger / TriggerRun
A **trigger** (or **TriggerRun**) is a scheduling policy bound to a pipeline revision.

- **Definition**: Configuration that automatically creates PipelineRuns on a schedule or on-demand
- **Types**:
  - **Cron Trigger**: Uses cron expressions (e.g., `0 9 * * *` for daily at 9 AM)
  - **Manual Trigger**: One-time execution via API or UI
- **Parameters**: Can pass dynamic parameters to each triggered run
- **Concurrency**: Can limit simultaneous PipelineRuns
- **Key File**: Defined in `trigger.yaml` (TriggerRun Custom Resource)

**Example**:
```yaml
apiVersion: michelangelo.api/v2
kind: TriggerRun
metadata:
  name: daily-training-trigger
spec:
  pipeline:
    name: training-pipeline
    namespace: my-project
  trigger:
    cronSchedule:
      cron: "0 9 * * *"  # Daily at 9 AM
      maxConcurrency: 1
```

---

## Framework & Infrastructure Concepts

### Uniflow
**Uniflow** is the Python-first framework for defining and executing ML workflows in Michelangelo.

- **Definition**: Decorator-based DSL (Domain-Specific Language) for workflow orchestration
- **Core**: Provides `@task` and `@workflow` decorators
- **Execution Modes**: Local (dev machine), Remote (cloud compute), Pipeline (production)
- **Storage**: Checkpoints data between tasks using S3-compatible storage
- **Caching**: Tasks are cached by input hash (blake2b) for 28 days
- **Determinism**: Workflow code is compiled to Starlark for deterministic, replayable execution

**Key Imports**:
```python
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask
from michelangelo.uniflow.plugins.spark import SparkTask
```

### RayTask
A **RayTask** is a task execution configuration for distributed computing using Ray.

- **Definition**: Configuration wrapper that specifies Ray cluster size and resource requirements
- **Usage**: Wrap task with `@uniflow.task(config=RayTask(...))`
- **Resources**: Define head node and worker node CPU/memory/GPU
- **Auto-scaling**: Ray handles worker creation and removal
- **Best For**: Distributed data processing, model training, parallel workloads

**Example**:
```python
@uniflow.task(config=RayTask(
    head_cpu=2,
    head_memory="8Gi",
    worker_cpu=4,
    worker_memory="16Gi",
    worker_instances=4
))
def train_distributed_model(data):
    # This task runs on a 4-worker Ray cluster
    return model
```

### SparkTask
A **SparkTask** is a task execution configuration for distributed computing using Apache Spark.

- **Definition**: Configuration wrapper for Spark cluster execution
- **Usage**: Wrap task with `@uniflow.task(config=SparkTask(...))`
- **Best For**: Large-scale data preprocessing, SQL-like operations

### DatasetVariable
A **DatasetVariable** is Michelangelo's abstraction for passing datasets between tasks.

- **Definition**: Wrapper that handles datasets in different frameworks (Ray, Pandas, Spark)
- **Purpose**: Abstracts away framework differences; manages serialization and storage
- **Usage**: Pass between tasks; automatically checkpointed and cached
- **Supported Formats**:
  - Ray Dataset (distributed)
  - Pandas DataFrame (local/small)
  - Spark DataFrame (large-scale)

**Example**:
```python
from michelangelo.sdk.workflow.variables import DatasetVariable

train_dv = DatasetVariable(value=ray_dataset)
train_dv.save_ray_dataset()  # Checkpoint to storage
# Later, in another task:
train_dv.load_ray_dataset()
df = train_dv.value
```

---

## Data & Model Concepts

### Model Registry
The **Model Registry** is Michelangelo's model versioning and artifact management system.

- **Purpose**: Version, track, and manage trained models
- **Integration**: Built on MLflow with Kubernetes Custom Resources
- **Artifacts**: Stores raw model files and Triton-compatible deployable models
- **Versioning**: Each model can have multiple versions (0, 1, 2, ...)
- **Schema**: Models include input/output schema for validation

### Model Package
A **model package** is a deployable artifact created by registering a model.

- **Formats**: Two versions are created:
  - **Raw Model Format**: Original training files (for fine-tuning, analysis)
  - **Deployable Model Format**: Triton-compatible (for inference)
- **Contents**: Model binaries, schema, metadata, dependencies
- **Storage**: Stored in S3-compatible cloud storage

---

## Execution Modes (Running Patterns)

### Local Run
A **local run** executes a workflow on your development machine.

- **Purpose**: Development, debugging, quick testing
- **Performance**: Fast (instant startup)
- **Resources**: Uses your machine's CPU/memory
- **Best For**: Testing code before cloud deployment
- **Command**: `poetry run python my_workflow.py`

### Remote Run
A **remote run** executes a workflow on cloud compute infrastructure.

- **Purpose**: Testing with larger datasets or compute-intensive operations
- **Performance**: 2-5 minutes to provision
- **Resources**: Cloud-allocated (Ray or Spark cluster)
- **Best For**: Validating scalability before production
- **Command**: `poetry run python my_workflow.py remote-run --storage-url s3://bucket`

### Pipeline Dev Run
A **Pipeline Dev Run** (or **dev-run**) validates the complete pipeline including Docker image building.

- **Purpose**: Full pipeline validation before production deployment
- **Performance**: 20+ minutes (includes Docker build)
- **Resources**: Cloud-allocated
- **Best For**: Pre-production testing, ensuring Dockerfile and dependencies work
- **Command**: `ma pipeline dev-run --pipeline training-pipeline --storage-url s3://bucket`

### Pipeline Run
A **Pipeline Run** is a production execution of a registered pipeline.

- **Purpose**: Production-scale execution
- **Triggering**: Manual, scheduled (cron), or API-driven
- **Monitoring**: Full observability via MA Studio UI
- **Recovery**: Can be paused, resumed, or retried
- **Provisioning Time**: Varies (depends on cluster state and warm-up)

---

## Common Confusions to Clarify

### "Pipeline" Ambiguity
The term **"pipeline"** is used in two contexts:

1. **Informal (Workflow)**: "I'm building a training pipeline" = defining a workflow with tasks
2. **Formal (Deployment)**: "I registered a pipeline" = created a deployable Pipeline resource

**Rule**: Use "workflow" when discussing code logic. Use "pipeline" when discussing deployment/execution.

### Task Decorator Consistency
All of these are equivalent:
```python
# Preferred (most explicit)
@uniflow.task()

# Also acceptable (same thing)
from michelangelo.uniflow.core import task
@task()

# Avoid (less clear origin)
from task import task
@task()
```

**Standard**: Prefer `import michelangelo.uniflow.core as uniflow` and use `@uniflow.task()` throughout documentation.

### Workflow Decorator
```python
# Preferred
@uniflow.workflow()

# Also acceptable
from michelangelo.uniflow.core import workflow
@workflow()
```

**Standard**: Prefer `@uniflow.workflow()` for consistency.

---

## Architecture Layer Mapping

To understand how these concepts fit together:

```
┌─────────────────────────────────────────────────┐
│ Deployment Layer (MA Studio)                    │
│ └─ Pipeline (registered, versioned, triggered) │
│    └─ TriggerRun (schedule or manual)           │
└─────────────────────────────────────────────────┘
                      ↓
┌─────────────────────────────────────────────────┐
│ Execution Layer (Temporal + Ray/Spark)          │
│ └─ PipelineRun (single execution)               │
│    └─ Context (runtime configuration)           │
│    └─ Workflow (orchestration logic)            │
│       └─ Task (work unit)                       │
│          └─ RayTask/SparkTask (compute config)  │
└─────────────────────────────────────────────────┘
                      ↓
┌─────────────────────────────────────────────────┐
│ Data Layer                                      │
│ └─ DatasetVariable (abstracted dataset)        │
│ └─ Model Registry (versioned models)           │
│ └─ S3 Storage (checkpoint & artifact storage)  │
└─────────────────────────────────────────────────┘
```

---

## When to Use Each Term

| Scenario | Correct Term | Example |
|----------|--------------|---------|
| Writing Python code with decorators | Workflow, Task | "Define a `@workflow` that orchestrates data prep and training `@task`s" |
| Talking about registered, versioned deployments | Pipeline | "I registered a pipeline called training-pipeline" |
| Discussing a single execution | PipelineRun | "The PipelineRun failed at the training step" |
| Talking about automatic execution | Trigger / TriggerRun | "Set up a trigger to run daily at 9 AM" |
| Referring to the overall system | Michelangelo + Uniflow | "Uniflow is the framework; Michelangelo is the platform" |
| Selecting compute | RayTask / SparkTask | "Use RayTask for distributed ML training" |
| Passing data between tasks | DatasetVariable | "Pass data as a DatasetVariable for automatic caching" |

---

## Documentation Standards

When writing Michelangelo documentation:

1. **Always clarify context**: If using "pipeline," specify whether you mean workflow or deployment
2. **Use decorators consistently**: `@uniflow.task()` and `@uniflow.workflow()` in all examples
3. **Import pattern**: Prefer `import michelangelo.uniflow.core as uniflow`
4. **Terminology checklist**: Before publishing, verify each term matches this glossary
5. **Link to this guide**: When introducing new concepts, link to this TERMINOLOGY.md

---

## Version History

- **v1.0** (March 6, 2026): Initial terminology glossary created based on Michelangelo documentation audit
