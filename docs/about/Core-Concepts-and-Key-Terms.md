# Core Concepts and Key Terms

## overview

Michelangelo utilizes a combination of standard industry terms and product-specific naming conventions. This page provides high-level definitions for the platform's most essential and commonly used concepts. It is recommended you familiarize yourself with these concepts as you will encounter them on your ML development journey.

The definitions and examples listed below are organized based on frequency of usage in the documentation and priority for user understanding.

## Core Concepts

### Uniflow

**Uniflow** is a structured, scalable orchestration framework designed to manage AI/ML pipelines. It enables you to modularize your computation into **tasks**, chain them into **workflows**, and manage input/output artifacts efficiently.

### MA Studio

**MA Studio** is Michelangelo's UI environment. The standard, code-free ML development experience guides users through the different phases of the ML development lifecycle. Uber’s internal term for the UI is MA Studio (you may see it appear on this site in screenshots of the UI). This environment provides all the essential tools which allows ML developers to build, train, deploy, monitor, and debug your machine learning models in a single unified visual interface to boost your productivity. **_(Currently, Preparing and Training models are available for open source users. More features will be made available soon.)_**

Users can use the no-code dev environment to perform standardized ML tasks without writing a single line of code, including:
* Prepare data sources for training models or making batch predictions
* Build and train XGB models, classic ML models, and Deep Learning models

### CanvasFlex

**CanvasFlex** is an opinionated predefined ML workflow designed for more advanced tasks, such as training DL models, setting up customized retraining workflows, building bespoken model performance monitoring workflows. CanvasFlex provides a highly customized, code driven ML development experience by applying software development principles to ML development. Users can create their own dependencies that can be managed in the UI environment.


### Tasks

A **task** is the fundamental unit of computation in Uniflow. Tasks are modular and self-contained, enabling reuse and scalability.

#### Key Features
- **Input and Output Handling**: Tasks process input data and produce outputs.
- **Caching**: Automatically caches results to prevent redundant computations.
- **Retry Mechanism**: Built-in retries for transient failures.
- **Containerized Execution**: Tasks run in isolated environments (Docker, K8s) for scalability.

```python
import michelangelo.uniflow.core as uniflow

@uniflow.task()
def train():
    print("training")
```

### **Project**

A business use case with a set of continuously trackable metrics.

-   Uber-specific examples include predicting cancellation rate on rides dispatch and the ranking of restaurants on the UberEats home feed.

### **Compute Resource**

These are hardware resources (CPU, GPU, memory, storage, etc) for running Machine Learning workloads.

### **Dataset**

A piece of data registered in Michelangelo. Users can set up data pipelines and let Michelangelo manage the dataset, or directly register the dataset in Michelangelo and manage it externally. They can can use the dataset for training and evaluation.

### **Feature**

An individual measurable property or characteristic of a phenomenon, represented as an attribute in a dataset.

### **Job**

A batch job running a ML workload. Currently Michelangelo runs [Spark](https://spark.apache.org/docs/latest/index.html) for data processing and [Ray](https://www.ray.io/) for ML training.

### **Pipeline**

A pipeline is a recipe that runs multiple jobs and creates desired output artifacts.

### **Model**

As a widely used term, a machine learning model refers to output from a training job over a set of data, providing it an algorithm that it can use to reason over, learn from, and make predictions about that data.

     **model name:**  identifier of a model, it also means a list of models (like a chain) in the incremental training case.

     **revision id:** Revision of the model, for normal model, it will always be revision 0. But for incremental training, the revision id will keep increasing for each iteration of the model training job.

### **Model Excellence Scores**

Model Excellent Scores (MES) provide visibility into the ML model quality throughout various stages of a model’s life cycle, such as feature quality, prediction performance, and model freshness.

### **Model Family**

Model Family is a sub problem using different ML model especially training features and objectives within a project (use case). This is used when there are multiple models supporting one use case.

-   Model excellence scores track the quality on each model family.
-   For example, UberEats home feed ranking is determined by different models optimizing conversion rate, net inflow, service quality and fairness.

### **Inference Server**

Inference Server is synonymous with the Online Prediction Service, and is essentially the host for use-cases that require online prediction.

### **Deployment**

Runs a set of processes to load a model into a target. Provides a human readable name for accessing a model.

### **Endpoint**

The routing mechanism for making requests to a group of deployments.

### **Target**

Target is the physical destination of a model.

### **Evaluation Report**

Collection of model metrics. Some examples are model performance report, feature importance report, data quality report, etc.



---

### Workflows

A **workflow** orchestrates multiple tasks, managing dependencies and result passing.

```python
@uniflow.workflow
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

Uniflow supports multiple data types as task input/output:

### 1. Scalars

```python
@uniflow.task()
def add_numbers(a: int, b: int) -> int:
    return a + b
```

### 2. Dictionaries

```python
@uniflow.task()
def create_data():
    return {"feature_1": 10, "feature_2": 20}

@uniflow.task()
def process_data(data: dict):
    data["feature_sum"] = data["feature_1"] + data["feature_2"]
    return data
```

### 3. Lists & Tuples

```python
@uniflow.task()
def get_numbers():
    return [1, 2, 3]

@uniflow.task()
def multiply_numbers(numbers: list):
    return [x * 2 for x in numbers]
```

### 4. Dataclasses

```python
from dataclasses import dataclass

@dataclass
class ModelConfig:
    learning_rate: float
    batch_size: int

@uniflow.task()
def get_config() -> ModelConfig:
    return ModelConfig(learning_rate=0.01, batch_size=32)
```

### 5. Pydantic Models

```python
from pydantic import BaseModel

class ModelMetrics(BaseModel):
    accuracy: float
    loss: float

@uniflow.task()
def compute_metrics() -> ModelMetrics:
    return ModelMetrics(accuracy=0.95, loss=0.05)
```

### 6. File & Path Support

```python
@uniflow.task()
def read_file(file_path: str):
    with open(file_path, "r") as f:
        return f.read()

# Example call:
# read_file("file://path/to/data.txt")
```

Supported protocols:
- `s3://`
- `hdfs://`
- `file://`
- `tb://` (Terrablob)

Handled via [`[fsspec](https://filesystem-spec.readthedocs.io/)`](https://filesystem-spec.readthedocs.io/)

### 7. Remote Object References

```json
[
  {
    "url": "s3://default/1a52588fb9774306ab6b112485bdb71e",
    "type": {"path": "ray.data.dataset.Dataset"},
    "__class__": "michelangelo.uniflow.core.ref.Ref"
  }
]
```

References are lightweight pointers to heavy artifacts (e.g., datasets, model weights).

---

## Logs and Monitoring

- **Pipeline Logs**: Viewable through Kubernetes, `mactl`, or Cadence UI.
- **Audit & Debugging**: All execution results and logs can be persisted and traced back.

---

## Example: Build a Pipeline

```python
@uniflow.workflow
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

## Best Practices

- Keep tasks modular and stateless
- Use dataclass or pydantic models for complex input/output
- Leverage caching and checkpointing to reduce compute costs
- Externalize large datasets via Ref to avoid memory bottlenecks
- Use consistent paths and metadata for reproducibility
