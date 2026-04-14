# Core Concepts and Key Terms

## Overview

Michelangelo utilizes a combination of standard industry terms and product-specific naming conventions. This page provides high-level definitions for the platform's most essential and commonly used concepts. It is recommended you familiarize yourself with these concepts as you will encounter them on your ML development journey.

The definitions and examples listed below are organized based on frequency of usage in the documentation and priority for user understanding.

## Quick Reference

Need to quickly look up a term? Here's a summary of the most commonly used concepts:

| Term | What It Is | When You Use It |
|------|-----------|-----------------|
| **Project** | Business use case with trackable metrics | Organizing all ML work for a specific problem (e.g., fraud detection, churn prediction) |
| **Model Family** | Group of related models for one use case | When multiple models solve different aspects of one business problem |
| **Dataset** | Registered data in Michelangelo | Providing training, validation, or prediction input data |
| **Task** | Single unit of computation (function) | Building reusable, modular steps in your ML pipeline |
| **Workflow** | Chain of tasks with dependencies | Orchestrating multi-step ML pipelines (data prep → training → evaluation) |
| **Model & Revision** | Trained model artifact with version number | Tracking different versions of your trained models |
| **Deployment** | Model running in production environment | Making your model available for predictions |
| **Endpoint** | URL/routing for accessing deployed models | Making prediction requests from applications |
| **MA Studio** | No-code UI for ML development | Building models visually without writing code |
| **CanvasFlex** | Code-driven YAML workflows | Advanced customization with best practices and version control |
| **Uniflow** | Python orchestration framework | Writing custom ML pipelines with `@task` and `@workflow` decorators |

---

## System Components
These are the frameworks, interfaces, and compute engines provided by Michelangelo to facilitate development.

### Orchestration & Interfaces

#### MA Studio (No Code UI)

**MA Studio** is Michelangelo's UI environment. The standard, code-free ML development experience guides users through the different phases of the ML development lifecycle. This environment provides all the essential tools which allow ML developers to build, train, deploy, monitor, and debug your machine learning models in a single unified visual interface to boost your productivity. 

Users can use the no-code dev environment to perform standardized ML tasks without writing a single line of code, including:
* Prepare data sources for training models or making batch predictions
* Build and train XGB models, classic ML models, and Deep Learning models

#### CanvasFlex (Code Driven YAML/UI)

**CanvasFlex** is an opinionated predefined ML workflow designed for more advanced tasks with best practices, such as training DL models, setting up customized retraining workflows, building bespoke model performance monitoring workflows. CanvasFlex provides a highly customized, code driven ML development experience by applying software development principles to ML development. Users can create their own dependencies that can be managed in the UI environment.

#### Uniflow (Orchestration Framework)

**Uniflow** is a structured, scalable orchestration framework designed to manage AI/ML pipelines at scale. It enables you to modularize your computation into **tasks**, chain them into **workflows**, and manage input/output artifacts efficiently.

### Execution & Infrastructure

#### Tasks

A **task** is the fundamental unit of computation in Uniflow. Tasks are modular and self-contained, enabling reuse and scalability.

##### Key Features
- **Input and Output Handling**: Tasks process input data and produce outputs.
- **Caching**: Automatically caches results to prevent redundant computations.
- **Retry Mechanism**: Built-in retries for transient failures.
- **Containerized Execution**: Tasks run in isolated environments (Docker, K8s) for scalability.

```python
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask

@uniflow.task(config=RayTask(head_cpu=2, head_memory="4Gi"))
def train():
    print("training")
```

---
#### Workflows

A **workflow** orchestrates multiple tasks, managing dependencies and result passing.

```python
@uniflow.workflow()
def train_workflow(dataset_id: str):
    train_data, valid_data, test_data = load_dataset(dataset_id)
    model = train(train_data, valid_data, test_data)
    metrics = evaluate(model, test_data)
    return metrics
```

To run:

```python
if __name__ == "__main__":
    ctx = uniflow.create_context()
    ctx.run(train_workflow, dataset_id="cola")
```

---

#### Job

A batch job running a ML workload. Currently Michelangelo runs [Spark](https://spark.apache.org/docs/latest/index.html) for data processing and [Ray](https://www.ray.io/) for ML training.

#### Compute Resource

These are hardware resources (CPU, GPU, memory, storage, etc) for running Machine Learning workloads.


#### Inference Server

Inference Server is synonymous with the Online Inference Service, and is essentially the host for use-cases that require online inference.

## ML Concepts
These are the logical entities, data structures, and artifacts that define your machine learning use case.

### Project

A business use case with a set of continuously trackable metrics.

**Familiar Equivalent**: Similar to an MLflow experiment or Weights & Biases project, but encompasses the entire ML lifecycle from data preparation to deployment monitoring. Think of it as the top-level organizational unit for all work related to one business problem.

**Examples**:
-   Predicting customer churn for a subscription service
-   Fraud detection for financial transactions
-   Recommending products on an e-commerce homepage
-   Predicting delivery time estimates for a logistics platform

### Model Family

A Model Family is a group of related ML models within a project that address different aspects of the same use case, each with distinct training features and objectives. Use Model Families when multiple models work together to solve one business problem.

**Familiar Equivalent**: Similar to organizing multiple models within one Kubeflow or SageMaker pipeline, where each model has a specific role in solving the overall problem.

:::warning[Common Confusion]
- A **Model** is a single trained artifact (e.g., one XGBoost classifier)
- A **Model Family** is a group of models solving related sub-problems (e.g., three models for conversion, quality, and fairness in ranking)
:::

**Examples**:
-   Model excellence scores track the quality of each model family
-   A home feed ranking system uses different model families optimizing for conversion rate, content quality, and fairness

### Dataset

A piece of data registered in Michelangelo. Users can set up data pipelines and let Michelangelo manage the dataset, or directly register the dataset in Michelangelo and manage it externally. They can use the dataset for training and evaluation.

**Familiar Equivalent**: Like registering a dataset in a data catalog (e.g., Delta Lake, Data Version Control, or AWS Glue Data Catalog). Michelangelo tracks dataset versions and lineage automatically.

### Feature

An individual measurable property or characteristic of a phenomenon, represented as an attribute in a dataset.

**Familiar Equivalent**: Same as in any ML framework - a column in your training data (e.g., "age", "transaction_amount", "embedding_vector"). Can be managed in external feature stores or within Michelangelo.

### Pipeline

A pipeline is a recipe that runs multiple jobs and creates desired output artifacts.

**Familiar Equivalent**: Similar to Airflow DAGs, Prefect flows, or Kubeflow pipelines - a series of data processing and ML tasks executed in sequence or parallel.

### Model & Revision

As a widely used term, a machine learning model refers to output from a training job over a set of data, providing it an algorithm that it can use to reason over, learn from, and make predictions about that data.

     **model name:**  identifier of a model, it also means a list of models (like a chain) in the incremental training case.

     **revision id:** Revision of the model, for normal model, it will always be revision 0. But for incremental training, the revision id will keep increasing for each iteration of the model training job.

### Evaluation Report

Collection of model metrics. Some examples are model performance report, feature importance report, data quality report, etc.

### Model Excellence Scores

Model Excellent Scores (MES) provide visibility into the ML model quality throughout various stages of a model’s life cycle, such as feature quality, prediction performance, and model freshness.

### Deployment

Runs a set of processes to load a model into a target. Provides a human readable name for accessing a model.

### Endpoint

The routing mechanism for making requests to a group of deployments.


## Output Artifacts

### Task Results

Serialized outputs stored by Uniflow for caching, debugging, or reuse in downstream tasks.

Example:

```json
[
  {
    "url": "s3://default/1a52588fb9774306ab6b112485bdb71e",
    "type": {"path": "ray.data.dataset.Dataset"},
    "__class__": "michelangelo.uniflow.core.ref.Ref"
  }
]
```

Features:
- **Dataset References** with URLs
- **Type Information**
- **Metadata** (optional)

---

### Data Checkpoints

Intermediate datasets are stored using Uniflow's abstract IO layer for:
- Fault tolerance
- Reuse across executions
- Backend flexibility (S3, HDFS, Ray, etc.)

#### Ray-based Implementation Example

```python
from michelangelo.uniflow.core.io_registry import IO
from ray.data import Dataset

class DatasetIO(IO[Dataset]):
    def write(self, url: str, ds: Dataset):
        fs, path = resolve_fs_path(url)
        ds.write_parquet(path, filesystem=fs)

    def read(self, url: str):
        fs, path = resolve_fs_path(url)
        return ray.data.read_parquet(path, filesystem=fs)
```

---

## Supported Data Types

Uniflow tasks support standard Python types plus ML-specific formats:

| Type Category | Supported Types | Use Case |
|---------------|----------------|----------|
| **Primitives** | int, float, str, bool | Simple parameters and return values |
| **Collections** | dict, list, tuple | Multiple values, configurations |
| **Structured** | dataclass, Pydantic models | Complex typed configurations, validation |
| **ML Artifacts** | Ray Datasets, model files via Ref | Large datasets, trained models |
| **Files** | Paths with s3://, hdfs://, file:// | Reading/writing data from storage |
| **Remote References** | Ref pointers | Lightweight references to heavy objects |

**Key Features**:
- **Automatic serialization**: Uniflow handles serialization/deserialization automatically
- **Type safety**: Use Python type hints for better error checking
- **Caching**: Results are cached based on input types and values
- **Protocol support**: Access files via s3://, hdfs://, file:// (via [fsspec](https://filesystem-spec.readthedocs.io/))

**Common Patterns**:
```python
# Simple typed task
@uniflow.task()
def add_numbers(a: int, b: int) -> int:
    return a + b

# Structured config with Pydantic
from pydantic import BaseModel

class ModelConfig(BaseModel):
    learning_rate: float
    batch_size: int

@uniflow.task()
def train(config: ModelConfig):
    # Training code using config.learning_rate, config.batch_size
    pass

# Remote dataset reference (avoids copying large data)
@uniflow.task()
def process_data(dataset_ref: Ref) -> Ref:
    # Process dataset without loading entire thing into memory
    return processed_ref
```

See [Appendix: Data Type Examples](#appendix-uniflow-data-type-examples) for detailed examples of each type.

---

## Logs and Monitoring

- **Pipeline Logs**: Viewable through Kubernetes, `ma`, or Cadence UI.
- **Audit & Debugging**: All execution results and logs can be persisted and traced back.

---

## Example: Build a Pipeline

```python
@uniflow.workflow()
def train_workflow(dataset_id: str):
    train_data, valid_data, test_data = load_dataset(dataset_id)
    model = train(train_data, valid_data, test_data)
    metrics = evaluate(model, test_data)
    return metrics
```

Run it:

```bash
python train_workflow.py
```

---

## Related Modules

- `@uniflow.task`: Define a Uniflow-compatible task
- `@uniflow.workflow`: Declare a Uniflow-managed workflow
- `uniflow.create_context()`: Initialize and run workflows
- `michelangelo.uniflow.core.io_registry`: For registering custom IO handlers

---

## How Concepts Relate

Understanding how Michelangelo's concepts work together:

```
Project (e.g., "Fraud Detection")
├── Model Family 1 (Transaction Scoring)
│   ├── Dataset (Historical Transactions)
│   ├── Workflow (Data Prep → Training → Evaluation)
│   │   ├── Task: prepare_data()
│   │   ├── Task: train_model()
│   │   └── Task: evaluate()
│   ├── Model Revision 0 (XGBoost v1)
│   ├── Model Revision 1 (XGBoost v2 - retrained)
│   ├── Deployment (fraud-scoring-prod)
│   └── Endpoint (https://api.../fraud-scoring)
└── Model Family 2 (User Risk Profiling)
    ├── Dataset (User Behavior)
    ├── Workflow (Feature Engineering → Training)
    └── Model Revision 0 (Random Forest)
```

**Key Relationships**:
- **Project** contains one or more **Model Families**
- **Model Families** use **Datasets** and produce **Models**
- **Workflows** orchestrate **Tasks** to transform data and train models
- **Models** have multiple **Revisions** (versions)
- **Deployments** serve specific **Model Revisions** via **Endpoints**

---

## Common Workflows

### Training Your First Model

1. **Create a Project** for your use case (e.g., "Customer Churn Prediction")
2. **Register your Dataset** in Michelangelo (connect to data warehouse)
3. **Define a Workflow** with training tasks (or use MA Studio UI for no-code approach)
4. **Run the workflow** and track results in Model Registry
5. **Create a Deployment** to serve predictions via an Endpoint

**MA Studio (UI) Path**:
```
Navigate to MA Studio → Create Project → Connect Dataset →
Train Model (select XGBoost) → Evaluate → Deploy
```

**Uniflow (Code) Path**:
```python
@uniflow.task(config=RayTask(head_cpu=2, head_memory="4Gi"))
def train_model(dataset):
    # Your training code
    return model

@uniflow.workflow()
def training_pipeline(dataset_id: str):
    data = load_dataset(dataset_id)
    model = train_model(data)
    return model
```

### Retraining an Existing Model

1. **Update Dataset** with new data (or existing dataset)
2. **Run Workflow** with training workflow
3. **New Revision** created automatically with revision 0
4. **Update Deployment** to new model (instant rollback available if needed)

### Incremental Training an Existing Model

1. **Reference existing Model** by name from your Project
2. **Update Dataset** with new data (or create new dataset version)
3. **Run Workflow** with incremental training enabled
4. **New Revision** created automatically (e.g., revision 0 → revision 1)
5. **Update Deployment** to new revision (instant rollback available if needed)

### Deploying for A/B Testing

1. **Deploy Model Revision 1** to 90% of traffic
2. **Deploy Model Revision 2** to 10% of traffic (same Endpoint)
3. **Monitor metrics** per revision using Model Excellence Scores
4. **Gradually shift traffic** to winning revision
5. **Rollback instantly** if issues detected

---

## Best Practices

- Keep tasks modular and stateless
- Use dataclass or pydantic models for complex input/output
- Leverage caching and checkpointing to reduce compute costs
- Externalize large datasets via Ref to avoid memory bottlenecks
- Use consistent paths and metadata for reproducibility
- Start with MA Studio UI for quick experiments, extend features with Uniflow for the custom needs
- Use Model Families to organize related models solving one business problem
- Always test deployments in sandbox before production

---

## Appendix

### Appendix: Uniflow Data Type Examples

Detailed examples of supported data types in Uniflow tasks.

#### 1. Scalars

```python
@uniflow.task()
def add_numbers(a: int, b: int) -> int:
    return a + b

@uniflow.task()
def format_name(first: str, last: str) -> str:
    return f"{first} {last}"
```

#### 2. Dictionaries

```python
@uniflow.task()
def create_data():
    return {"feature_1": 10, "feature_2": 20}

@uniflow.task()
def process_data(data: dict):
    data["feature_sum"] = data["feature_1"] + data["feature_2"]
    return data
```

#### 3. Lists & Tuples

```python
@uniflow.task()
def get_numbers():
    return [1, 2, 3]

@uniflow.task()
def multiply_numbers(numbers: list):
    return [x * 2 for x in numbers]

@uniflow.task()
def split_dataset(data):
    return (train_data, val_data, test_data)  # tuple
```

#### 4. Dataclasses

```python
from dataclasses import dataclass

@dataclass
class ModelConfig:
    learning_rate: float
    batch_size: int
    epochs: int = 10  # with default

@uniflow.task()
def get_config() -> ModelConfig:
    return ModelConfig(learning_rate=0.01, batch_size=32)

@uniflow.task()
def train_with_config(config: ModelConfig):
    # Access config.learning_rate, config.batch_size, etc.
    pass
```

#### 5. Pydantic Models

```python
from pydantic import BaseModel, Field

class ModelMetrics(BaseModel):
    accuracy: float = Field(ge=0.0, le=1.0)  # with validation
    loss: float
    epoch: int

@uniflow.task()
def compute_metrics() -> ModelMetrics:
    return ModelMetrics(accuracy=0.95, loss=0.05, epoch=10)

@uniflow.task()
def log_metrics(metrics: ModelMetrics):
    print(f"Accuracy: {metrics.accuracy}")
```

#### 6. File & Path Support

```python
@uniflow.task()
def read_file(file_path: str):
    with open(file_path, "r") as f:
        return f.read()

@uniflow.task()
def save_model(model, output_path: str):
    # Supports s3://, hdfs://, file:// protocols
    with open(output_path, "wb") as f:
        pickle.dump(model, f)
```

**Supported protocols**:
- `s3://bucket/path/to/file.parquet`
- `hdfs://namenode/path/to/data`
- `file:///local/path/to/file.csv`

All handled via [fsspec](https://filesystem-spec.readthedocs.io/) for consistent API across storage backends.

#### 7. Remote Object References (Ref)

For large objects like datasets or model weights, use `Ref` to avoid serialization overhead:

```python
from michelangelo.uniflow.core.ref import Ref
import ray.data

@uniflow.task()
def load_large_dataset() -> ray.data.Dataset:
    # Returns a Ref automatically - Uniflow detects large objects
    return ray.data.read_parquet("s3://bucket/huge_dataset.parquet")

@uniflow.task()
def process_dataset(dataset: ray.data.Dataset) -> ray.data.Dataset:
    # Receives Ref, processes without copying
    return dataset.map(lambda x: x * 2)
```

**Internal representation** (you don't create this manually):
```json
{
  "url": "s3://default/1a52588fb9774306ab6b112485bdb71e",
  "type": {"path": "ray.data.dataset.Dataset"},
  "__class__": "michelangelo.uniflow.core.ref.Ref"
}
```

**Benefits**:
- Lightweight pointers to heavy artifacts
- Avoids serialization/deserialization overhead
- Enables distributed processing of large datasets
- Automatic caching and reuse

---

## What's next?

- **Ready to start building?** [Set up your local sandbox](./sandbox-setup.md) and then follow [Getting Started with Pipelines](../user-guides/ml-pipelines/getting-started.md)
- **Want to see complete examples?** Browse [end-to-end tutorials](../user-guides/index.md#learning-by-examples) for XGBoost, BERT, GPT fine-tuning, and recommendation systems
- **Interested in the CLI?** See the [CLI Reference](../user-guides/cli.md) for pipeline and project management commands
