---
last_verified: 2026-09-30
---

# Data Passing and References in Uniflow

## What you'll learn

* How data flows between tasks in Uniflow
* What References are and why they're needed
* How to work with task outputs and inputs
* Automatic serialization and deserialization
* Why data does not convert automatically across framework boundaries, and how to hand it off explicitly

---

## The Problem: Data Between Tasks

When tasks run on distributed clusters, you can't just pass Python objects directly between them. Uniflow solves this with **References** - a smart system that handles data serialization, storage, and retrieval automatically.

```python
# What you write:
@task(config=RayTask(...))
def load_data(file_path: str):
    import pandas as pd
    df = pd.read_csv(file_path)
    return df  # Just return the DataFrame!

@task(config=RayTask(...))
def process_data(data):
    # Receives the DataFrame automatically!
    return data * 2

@workflow()
def my_pipeline(file_path: str):
    data = load_data(file_path)      # Task 1 returns data
    result = process_data(data)       # Task 2 receives data
    return result
```

**Behind the scenes**, Uniflow:
1. Serializes the DataFrame returned by `load_data`
2. Stores it in your configured storage (S3, GCS, etc.)
3. Passes a Reference (a URL + metadata) to `process_data`
4. Automatically deserializes the data when `process_data` runs
5. All of this happens transparently to you!

---

## Understanding References

A **Reference** is Uniflow's internal representation of data that's been stored between tasks. It contains:

| Component | What It Is | Example |
|-----------|-----------|---------|
| **Storage URL** | Where the data is physically stored | `s3://my-bucket/workflows/run-123/task-data/abc123` |
| **Data Type** | What Python type the data is | `pandas.core.frame.DataFrame` |
| **Metadata** | Information about the data | Serialization format, schema, timestamps |

**You don't work with References directly** - they're handled automatically. But understanding them helps you reason about data flow.

---

## How Data Flows Between Tasks

### Example 1: Simple Data Passing

```python
from michelangelo.uniflow.core import task, workflow
from michelangelo.uniflow.plugins.ray import RayTask

@task(config=RayTask(head_cpu=2, head_memory="4Gi"))
def load_data(file_path: str):
    """
    Returns: pandas DataFrame
    Uniflow converts to: Reference pointing to stored DataFrame
    """
    import pandas as pd
    df = pd.read_csv(file_path)
    return df

@task(config=RayTask(head_cpu=2, head_memory="4Gi"))
def clean_data(data):
    """
    Receives: Reference (automatically deserialized to DataFrame)
    Returns: Cleaned DataFrame
    Uniflow converts to: Reference pointing to stored cleaned data
    """
    # data is a real DataFrame, not a Reference object
    cleaned = data.dropna()
    return cleaned

@task(config=RayTask(head_cpu=2, head_memory="4Gi"))
def train_model(data):
    """
    Receives: Reference (automatically deserialized to DataFrame)
    Returns: Trained model object
    """
    import xgboost as xgb
    model = xgb.train(data)
    return model

@workflow()
def training_pipeline(file_path: str):
    """
    Step 1: load_data returns DataFrame → stored as Reference
    Step 2: clean_data receives Reference → deserializes to DataFrame → returns cleaned data
    Step 3: train_model receives Reference → deserializes to DataFrame → returns model
    """
    raw_data = load_data(file_path)
    cleaned_data = clean_data(raw_data)
    model = train_model(cleaned_data)
    return model
```

**Key insight:** Each task receives a Reference but works with the original Python object. Uniflow handles all serialization/deserialization.

---

## Multiple Outputs and Unpacking

Tasks can return multiple values, and they're all handled as References:

```python
from michelangelo.uniflow.core import task, workflow
from michelangelo.uniflow.plugins.ray import RayTask

@task(config=RayTask(head_cpu=2, head_memory="4Gi"))
def split_data(data):
    """
    Returns: Tuple of (train_data, validation_data)
    Uniflow creates: Reference for each element
    """
    from sklearn.model_selection import train_test_split
    train, val = train_test_split(data)
    return train, val

@task(config=RayTask(head_cpu=2, head_memory="4Gi"))
def train_model(train_data):
    """Receives Reference to training data"""
    import xgboost as xgb
    return xgb.train(train_data)

@task(config=RayTask(head_cpu=2, head_memory="4Gi"))
def evaluate_model(model, val_data):
    """Receives References to both model and validation data"""
    return model.evaluate(val_data)

@workflow()
def training_pipeline(data):
    # Unpack multiple outputs - each is a Reference
    train_data, val_data = split_data(data)

    # Pass to different tasks
    model = train_model(train_data)
    metrics = evaluate_model(model, val_data)

    return metrics
```

---

## Cross-Framework Data Passing (Ray to Spark)

Uniflow does **not** convert data between frameworks. A Reference records the type
of the value that was *written*, and `unref` resolves the IO handler from that
recorded type — it has no knowledge of the framework the consuming task runs on.

So if a Ray task returns a `ray.data.Dataset`, a downstream Spark task receives a
`ray.data.Dataset`, not a `pyspark.sql.DataFrame`. Spark DataFrame API calls on it
fail with `AttributeError`.

The good news is that the two handlers already share an interchange format: both
`RayDatasetIO` and `SparkIO` store data as Parquet. Only the automatic dispatch is
missing, so crossing frameworks is a matter of doing the hand-off explicitly.

### The manual step

A consuming task never sees the Reference itself — Uniflow resolves it into a
materialized object before the function body runs. You therefore cannot receive a
Ray task's output and re-read it as Parquet: by the time your code runs, it is
already a Ray `Dataset`.

The producer has to hand over a **location** instead of a dataset. Returning a
`str` is what makes this work: strings are not a Reference-backed type, so the
path passes through to the next task unchanged rather than being materialized.

```python
import os

from michelangelo.uniflow.core import task, workflow
from michelangelo.uniflow.plugins.ray import RayTask
from michelangelo.uniflow.plugins.spark import SparkTask

@task(config=RayTask(head_cpu=2, head_memory="4Gi"))
def load_with_ray(file_path: str) -> str:
    """Load with Ray, write Parquet, and return the location as a string.

    Returning the path (not the Dataset) is the hand-off: a ``str`` is passed
    through as-is, so the Spark task receives a URL it can read itself.
    """
    import ray.data

    dataset = ray.data.read_csv(file_path)
    out_url = os.path.join(os.environ["UF_STORAGE_URL"], "ray_stage")
    dataset.write_parquet(out_url)
    return out_url

@task(config=SparkTask(driver_cpu=2, driver_memory="4Gi"))
def process_with_spark(parquet_url: str) -> str:
    """Read the Parquet written by the Ray task, then write Parquet back out."""
    from pyspark.sql import SparkSession

    spark = SparkSession.builder.getOrCreate()
    df = spark.read.parquet(parquet_url)
    processed = df.filter(df.price > 100)

    out_url = os.path.join(os.environ["UF_STORAGE_URL"], "spark_stage")
    processed.write.parquet(out_url)
    return out_url

@task(config=RayTask(head_cpu=2, head_memory="4Gi"))
def analyze_with_ray(parquet_url: str):
    """Read the Spark output back into Ray."""
    import ray.data

    dataset = ray.data.read_parquet(parquet_url)
    return dataset.groupby("category").mean("price")

@workflow()
def multi_framework_pipeline(file_path: str):
    # Each hop crosses a framework boundary, so each hop passes a path.
    ray_stage = load_with_ray(file_path)
    spark_stage = process_with_spark(ray_stage)
    return analyze_with_ray(spark_stage)
```

Both boundaries need this, not just the first: a Spark task returning a
`DataFrame` produces a Reference typed as a Spark `DataFrame`, which a downstream
Ray task would receive as a Spark `DataFrame`.

### When you don't need any of this

This only applies when data crosses a framework boundary. Passing a
`ray.data.Dataset` from one Ray task to another Ray task — or a Spark `DataFrame`
between two Spark tasks — needs no special handling; return the object directly
and let Uniflow's Reference system do the work, as shown earlier on this page.

---

## Supported Data Types

Uniflow's type system (covered in detail in [Type System Guide](./type-system.md)) supports automatic serialization for:

**Basic types:**
- Integers, floats, strings, booleans
- Lists, tuples, dictionaries

**Data structures:**
- Pandas DataFrames
- Ray Datasets
- Spark DataFrames
- NumPy arrays
- PyArrow tables

**Custom classes:**
- Dataclasses
- Pydantic models
- Enums
- Any type with a registered codec (see Type System Guide)

**Binary data:**
- Bytes (base64 encoded)
- Images, audio, custom serialized objects

---

## Storage and Checkpointing

Understanding where References point helps you debug issues:

```python
@task(config=RayTask(...), cache_enabled=True)
def expensive_computation(data):
    """
    When this task completes:
    1. Result is serialized
    2. Stored in your configured storage (S3/GCS/HDFS)
    3. Reference is created pointing to that location
    4. Reference is passed to next task or returned to user

    If cache_enabled=True:
    - Result stays in storage for 28 days
    - Next run with same inputs skips execution
    - References data from storage location
    """
    result = do_expensive_work(data)
    return result
```

**Storage configuration** is set via `--storage-url` flag:
```bash
# Local storage (development)
poetry run python workflow.py remote-run --storage-url /tmp/workflows

# S3 storage (production)
poetry run python workflow.py remote-run --storage-url s3://my-bucket/workflows

# GCS storage
poetry run python workflow.py remote-run --storage-url gs://my-bucket/workflows
```

---

## Best Practices

### 1. Return Whole Objects, Not Serialized Strings

```python
# ❌ DON'T - Manual serialization
import json
@task(config=RayTask(...))
def process_data(data):
    result = some_computation(data)
    return json.dumps(result)  # Don't do this!

# ✅ DO - Let Uniflow handle it
@task(config=RayTask(...))
def process_data(data):
    result = some_computation(data)
    return result  # Just return the object
```

### 2. Match Data Types Across Tasks

```python
# ❌ AVOID - Type mismatch
@task(config=RayTask(...))
def get_data():
    return [1, 2, 3]  # Returns list

@task(config=RayTask(...))
def process(data):
    return data[0] * 2  # Works, but next task might expect DataFrame

# ✅ GOOD - Consistent types
@task(config=RayTask(...))
def get_data():
    import pandas as pd
    return pd.DataFrame({"values": [1, 2, 3]})

@task(config=RayTask(...))
def process(data):
    return data * 2  # Type matches what downstream expects
```

### 3. Use Appropriate Frameworks

```python
# ✅ GOOD - Use right tool for job
@task(config=RayTask(...))
def distributed_ml_training(data):
    # Ray is great for ML training
    return trained_model

@task(config=SparkTask(...))
def large_scale_etl(data):
    # Spark is great for ETL
    return processed_data

@task(config=RayTask(...))
def gpu_inference(model, data):
    # Ray with GPU for inference
    return predictions
```

Mixing frameworks like this is fine, but note that data does **not** convert
automatically where one framework's output feeds another's input. See
[Cross-Framework Data Passing](#cross-framework-data-passing-ray-to-spark) for the
explicit hand-off those boundaries require.

---

## Debugging Data Flow

### View Storage Locations

When running remotely, References point to real storage locations:

```bash
# Check what was stored
aws s3 ls s3://my-bucket/workflows/run-123/

# Or with Google Cloud Storage
gsutil ls gs://my-bucket/workflows/run-123/
```

### Inspect Reference Details

During development, you can see what's being stored:

```python
@task(config=RayTask(...))
def process_data(data):
    result = do_work(data)
    # In logs, you'll see storage location of result
    print(f"Result stored at: {storage_location}")
    return result
```

### Check Cache Hit/Miss

```python
@task(config=RayTask(...), cache_enabled=True, cache_version="v1")
def expensive_task(input_data):
    # Logs will show:
    # "Cache key: abc123..." (cache miss - computing)
    # OR
    # "Cache hit - loading from storage" (cache hit - skipped)
    return result
```

---

## Common Issues and Solutions

### Issue: "Data type not supported"

**Cause:** You're trying to pass a type that isn't registered with Uniflow

**Solution:** See [Type System Guide](./type-system.md) for supported types and how to add custom types

### Issue: "Reference not found in storage"

**Cause:** Storage location doesn't exist or credentials are wrong

**Solutions:**
1. Check `--storage-url` is correct and accessible
2. Verify cloud credentials (AWS/GCS/etc)
3. For S3: `aws s3 ls s3://bucket/path`
4. For GCS: `gsutil ls gs://bucket/path`

### Issue: Type mismatch between tasks

**Cause:** Task returns DataFrame, next task expects different type

**Solution:** Be explicit about return types and ensure downstream tasks expect that type

```python
@task(config=RayTask(...))
def get_data() -> pd.DataFrame:  # Explicit return type
    return pd.DataFrame(...)

@task(config=RayTask(...))
def process(data: pd.DataFrame) -> pd.DataFrame:  # Expect DataFrame
    return processed_data
```

---

## Next Steps

- [Type System Guide](./type-system.md) - Learn about codecs and data serialization
- [Caching Guide](../ml-pipelines/cache-and-pipelinerun-resume-form.md) - Use References with caching
- [Getting Started](../getting-started/getting-started.md) - See References in action with complete example
