# Type System and Data Serialization

## What you'll learn

* What types Uniflow supports natively
* The 5 codec types and when to use each
* How to serialize custom data types
* Best practices for type safety in workflows
* How to add custom codecs for your types

---

## Overview: Uniflow's Type System

When data flows between tasks, Uniflow automatically **serializes** your Python objects for storage and **deserializes** them when the next task runs. This is powered by a flexible type system supporting 5 built-in codecs.

### The 5 Built-In Codecs

| Codec | Types Supported | Use Case | Example |
|-------|-----------------|----------|---------|
| **Dataclass** | `@dataclass` decorated classes | Lightweight structured data | Configuration objects, metrics |
| **Pydantic** | `BaseModel` subclasses | Validated structured data | API schemas, validated configs |
| **Enum** | `Enum` subclasses | Fixed set of options | Status values, modes |
| **Type** | Basic + container types | Everything else | int, str, list, dict, DataFrame |
| **Bytes** | Binary data | Images, pickles, custom | JPG files, serialized objects |

---

## 1. Basic Types (Type Codec)

The most commonly used codec handles standard Python types automatically:

### Primitive Types

```python
from michelangelo.uniflow.core import task, workflow
from michelangelo.uniflow.plugins.ray import RayTask

@task(config=RayTask(head_cpu=1, head_memory="2Gi"))
def compute_metrics() -> float:
    """Returns a float - automatically serialized"""
    return 0.95

@task(config=RayTask(head_cpu=1, head_memory="2Gi"))
def process_threshold(threshold: float) -> bool:
    """Receives float - automatically deserialized"""
    return threshold > 0.9

@workflow()
def metrics_pipeline():
    score = compute_metrics()
    is_good = process_threshold(score)
    return is_good
```

**Supported primitive types:**
- `int` - Integer numbers
- `float` - Floating point numbers
- `str` - Text strings
- `bool` - True/False values
- `bytes` - Raw binary data

### Collections

```python
@task(config=RayTask(...))
def get_config() -> dict:
    """Returns dictionary with configuration"""
    return {
        "learning_rate": 0.01,
        "batch_size": 32,
        "epochs": 10
    }

@task(config=RayTask(...))
def apply_config(config: dict) -> list:
    """Receives dictionary, returns list"""
    return [config["learning_rate"], config["batch_size"]]

@task(config=RayTask(...))
def process_list(values: list) -> tuple:
    """Receives list, returns tuple"""
    return tuple(v * 2 for v in values)
```

**Supported collections:**
- `list` - Lists of items
- `dict` - Dictionaries with string keys
- `tuple` - Immutable sequences
- `set` - Unique value collections

### Data Science Types

```python
import pandas as pd
import numpy as np

@task(config=RayTask(...))
def load_data() -> pd.DataFrame:
    """Returns Pandas DataFrame"""
    return pd.read_csv("data.csv")

@task(config=RayTask(...))
def numpy_processing(data: pd.DataFrame) -> np.ndarray:
    """Receives DataFrame, returns NumPy array"""
    return data.values

@task(config=RayTask(...))
def process_numpy(arr: np.ndarray) -> float:
    """Receives NumPy array, returns float"""
    return arr.mean()
```

**Supported data science types:**
- `pd.DataFrame` - Pandas DataFrames
- `np.ndarray` - NumPy arrays
- `pa.Table` - PyArrow tables
- `ray.data.Dataset` - Ray Datasets
- `pyspark.sql.DataFrame` - Spark DataFrames

---

## 2. Dataclasses (Dataclass Codec)

Perfect for lightweight, structured data:

```python
from dataclasses import dataclass
from michelangelo.uniflow.core import task, workflow
from michelangelo.uniflow.plugins.ray import RayTask

@dataclass
class ModelMetrics:
    """Simple data container with type hints"""
    accuracy: float
    precision: float
    recall: float
    f1_score: float

@task(config=RayTask(...))
def compute_metrics(predictions, ground_truth) -> ModelMetrics:
    """
    Computes metrics and returns dataclass instance
    Uniflow automatically serializes the entire object
    """
    from sklearn.metrics import accuracy_score, precision_score, recall_score, f1_score

    accuracy = accuracy_score(ground_truth, predictions)
    precision = precision_score(ground_truth, predictions)
    recall = recall_score(ground_truth, predictions)
    f1 = f1_score(ground_truth, predictions)

    return ModelMetrics(
        accuracy=accuracy,
        precision=precision,
        recall=recall,
        f1_score=f1
    )

@task(config=RayTask(...))
def log_metrics(metrics: ModelMetrics) -> bool:
    """
    Receives dataclass instance (automatically deserialized)
    Can access fields directly
    """
    print(f"Accuracy: {metrics.accuracy:.3f}")
    print(f"Precision: {metrics.precision:.3f}")
    print(f"Recall: {metrics.recall:.3f}")
    print(f"F1 Score: {metrics.f1_score:.3f}")

    return metrics.accuracy > 0.9

@workflow()
def evaluation_pipeline(predictions, ground_truth):
    metrics = compute_metrics(predictions, ground_truth)
    success = log_metrics(metrics)
    return success
```

**When to use dataclasses:**
- Lightweight data structures
- Configuration objects
- Metrics and results
- When you don't need validation

**Advantages:**
- Simple and lightweight
- Type hints for IDE support
- Easy to extend

---

## 3. Pydantic Models (Pydantic Codec)

When you need validation and more features:

```python
from pydantic import BaseModel, Field, validator
from michelangelo.uniflow.core import task, workflow
from michelangelo.uniflow.plugins.ray import RayTask

class TrainingConfig(BaseModel):
    """Validated configuration with automatic validation"""
    learning_rate: float = Field(..., gt=0, le=1)  # > 0 and <= 1
    batch_size: int = Field(..., ge=1, le=1024)    # >= 1 and <= 1024
    epochs: int = Field(..., ge=1, le=1000)
    optimizer: str = Field(default="adam")

    @validator('optimizer')
    def validate_optimizer(cls, v):
        allowed = {"adam", "sgd", "rmsprop"}
        if v not in allowed:
            raise ValueError(f"optimizer must be one of {allowed}")
        return v

@task(config=RayTask(...))
def create_config(
    lr: float,
    batch: int,
    epochs: int
) -> TrainingConfig:
    """
    Create validated config
    Pydantic automatically validates all fields
    Raises error if validation fails
    """
    return TrainingConfig(
        learning_rate=lr,
        batch_size=batch,
        epochs=epochs
    )

@task(config=RayTask(...))
def train_model(config: TrainingConfig):
    """
    Receives validated config
    Can be confident all values are valid
    """
    print(f"Training with LR={config.learning_rate}, batch={config.batch_size}")
    # Use validated config values
    return trained_model

@workflow()
def training_pipeline(lr: float, batch: int, epochs: int):
    # If config is invalid, error happens before training
    config = create_config(lr, batch, epochs)
    model = train_model(config)
    return model
```

**When to use Pydantic:**
- Configuration that needs validation
- API request/response schemas
- When you need JSON serialization
- Complex nested models

**Advantages:**
- Automatic validation
- JSON schema support
- Better error messages
- IDE autocompletion

---

## 4. Enums (Enum Codec)

For fixed sets of options:

```python
from enum import Enum
from michelangelo.uniflow.core import task, workflow
from michelangelo.uniflow.plugins.ray import RayTask

class JobStatus(Enum):
    """Fixed set of job status values"""
    PENDING = "pending"
    RUNNING = "running"
    COMPLETED = "completed"
    FAILED = "failed"

class ModelType(Enum):
    """Available model types"""
    LINEAR = "linear"
    TREE = "tree"
    NEURAL = "neural"

@task(config=RayTask(...))
def get_job_status() -> JobStatus:
    """Returns enum value"""
    return JobStatus.COMPLETED

@task(config=RayTask(...))
def check_status(status: JobStatus) -> bool:
    """Receives enum value"""
    return status == JobStatus.COMPLETED

@task(config=RayTask(...))
def select_model(model_type: ModelType):
    """
    Use enum for type-safe selection
    IDE will autocomplete available options
    """
    models = {
        ModelType.LINEAR: linear_model(),
        ModelType.TREE: tree_model(),
        ModelType.NEURAL: neural_model()
    }
    return models[model_type]

@workflow()
def pipeline_with_enums():
    status = get_job_status()
    is_done = check_status(status)

    model = select_model(ModelType.NEURAL)
    return model
```

**When to use Enums:**
- Status/state values
- Choice between fixed options
- Type-safe selection
- Preventing invalid values

**Advantages:**
- Type-safe (IDE catches typos)
- Self-documenting code
- Prevents invalid values

---

## 5. Bytes/Binary Data (Bytes Codec)

For images, files, and custom objects:

```python
from michelangelo.uniflow.core import task, workflow
from michelangelo.uniflow.plugins.ray import RayTask
import pickle

@task(config=RayTask(...))
def save_model_binary(model) -> bytes:
    """
    Serialize model to bytes using pickle
    Uniflow stores and serializes the bytes
    """
    return pickle.dumps(model)

@task(config=RayTask(...))
def load_model_binary(model_bytes: bytes):
    """
    Receive bytes, deserialize back to model
    """
    model = pickle.loads(model_bytes)
    return model

@task(config=RayTask(...))
def process_image(image_path: str) -> bytes:
    """
    Read image file, return as bytes
    """
    with open(image_path, 'rb') as f:
        return f.read()

@task(config=RayTask(...))
def save_image(image_bytes: bytes) -> str:
    """
    Receive image bytes, save to file
    """
    output_path = "/tmp/output.jpg"
    with open(output_path, 'wb') as f:
        f.write(image_bytes)
    return output_path

@workflow()
def image_pipeline(image_path: str):
    # Read image as bytes
    img_bytes = process_image(image_path)

    # Process and save
    output = save_image(img_bytes)
    return output
```

**When to use Bytes:**
- Image/audio/video files
- Custom objects with pickle serialization
- Binary data that can't be represented other ways
- Compatibility with non-Python tools

**Note:** Bytes are base64-encoded for storage, so they're larger than binary files.

---

## Type Safety Best Practices

### 1. Use Type Hints

```python
# ❌ No type hints - unclear what types flow
@task(config=RayTask(...))
def process_data(data):
    return processed

# ✅ Clear type hints - document data flow
@task(config=RayTask(...))
def process_data(data: pd.DataFrame) -> pd.DataFrame:
    """
    Input: Pandas DataFrame with columns [id, value, timestamp]
    Output: Filtered DataFrame with only recent records
    """
    return data[data['timestamp'] > cutoff_date]
```

### 2. Match Input/Output Types

```python
# ❌ Type mismatch - confusing
@task(config=RayTask(...))
def get_data() -> list:
    return [1, 2, 3]

@task(config=RayTask(...))
def process(data: pd.DataFrame):  # Expects DataFrame!
    return data.mean()

# ✅ Types match - clear data flow
@task(config=RayTask(...))
def get_data() -> pd.DataFrame:
    return pd.DataFrame({"values": [1, 2, 3]})

@task(config=RayTask(...))
def process(data: pd.DataFrame) -> float:
    return data.mean()
```

### 3. Validate with Pydantic When Needed

```python
# For critical data flows, use Pydantic for validation
from pydantic import BaseModel

class DataQualityMetrics(BaseModel):
    null_count: int = 0
    duplicate_count: int = 0
    quality_score: float = Field(..., ge=0, le=100)

@task(config=RayTask(...))
def compute_quality(data: pd.DataFrame) -> DataQualityMetrics:
    """Automatically validates before returning"""
    return DataQualityMetrics(
        null_count=data.isnull().sum().sum(),
        duplicate_count=data.duplicated().sum(),
        quality_score=95.5
    )
```

---

## Troubleshooting Type Issues

### Issue: "Type not serializable"

**Cause:** Trying to return a type Uniflow doesn't know about

**Solution:** Use one of the 5 codecs:
1. Wrap in dataclass
2. Wrap in Pydantic model
3. Convert to bytes with pickle
4. Use supported types (list, dict, etc.)

### Issue: Unexpected deserialization error

**Cause:** Data format changed between task versions

**Solution:**
- Use version-aware serialization (Pydantic models)
- Add migration code if format changes
- Test serialization/deserialization

### Issue: Type mismatch between tasks

**Cause:** Task returns type A, next task expects type B

**Solution:** Be explicit with type hints and ensure compatibility:

```python
# Good - explicit types
@task(config=RayTask(...))
def get_data() -> pd.DataFrame:
    return data

@task(config=RayTask(...))
def process(df: pd.DataFrame) -> dict:  # Explicit conversion
    return df.to_dict()

@task(config=RayTask(...))
def use_dict(data: dict) -> None:
    pass
```

---

## Reference: Codec Selection Chart

```
Need to return custom Python object?
├─ Simple data structure?
│  ├─ No validation needed? → Dataclass ✓
│  └─ Validation needed? → Pydantic ✓
├─ Fixed set of options? → Enum ✓
├─ Basic types or collections? → Type Codec ✓
│  (int, float, str, bool, list, dict, tuple)
├─ Data science types? → Type Codec ✓
│  (DataFrame, NumPy, PyArrow, Ray Dataset, Spark DF)
└─ Binary data (image, pickle, etc)? → Bytes ✓
```

---

## Next Steps

- [Reference System Guide](./reference-system.md) - Understand how data flows between tasks
- [Getting Started](./getting-started.md) - See type system in action
- [Caching Guide](./cache-and-pipelinerun-resume-form.md) - Types and caching interaction
