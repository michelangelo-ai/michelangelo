# Data Preparation Guide

Learn how to prepare data in Uniflow for the ML pipeline on Michelangelo using Ray's distributed processing capabilities.

## What You'll Learn

* Apply preprocessing at scale with Ray  
* Create train/validation/test splits  
* Handle large datasets efficiently

## Preprocessing Patterns

### Distributed Preprocessing with Ray

```py
import ray.data as rd
from michelangelo.workflow.variables import DatasetVariable

dataset = rd.read_parquet("s3://bucket/data.parquet") \
    .map_batches(clean_missing_values, batch_size=1000) \
    .map_batches(normalize_features) \
    .map_batches(encode_categories)

train_ds, val_ds = dataset.train_test_split(test_size=0.2)
train_dv = DatasetVariable.create(train_ds)
val_dv = DatasetVariable.create(val_ds)
```

### Common Preprocessing Functions

| Task | Implementation Pattern | Notes |
| ----- | ----- | ----- |
| Missing Values | `df.fillna()` or `df.dropna()` | Use inside `map_batches` |
| Normalization | StandardScaler or MinMaxScaler | Apply per batch for efficiency |
| Categorical Encoding | `pd.get_dummies()` or LabelEncoder | Maintain consistent encoding |
| Text Tokenization | HuggingFace tokenizers | For NLP workflows |
| Image Preprocessing | `torchvision.transforms` | For computer vision |

## Data Splitting Strategies

### Random Split

```py
train_ds, temp_ds = dataset.train_test_split(test_size=0.3)
val_ds, test_ds = temp_ds.train_test_split(test_size=0.5)
```

### Temporal Split (Time Series)

```py
train_ds = dataset.filter(lambda x: x["date"] <= "2023-01-01")
val_ds = dataset.filter(lambda x: "2023-01-01" < x["date"] <= "2023-06-01")
```

## DatasetVariable: Michelangelo's Dataset Abstraction

Michelangelo provides `DatasetVariable` to handle datasets across different frameworks with automatic storage and serialization.

### Flexible Dataset Usage

| Framework | Usage | Load Method |
| ----- | ----- | ----- |
| Ray Datasets | `DatasetVariable.create(ray_dataset)` | `load_ray_dataset()` |
| Pandas DataFrames | `DatasetVariable.create(pandas_df)` | `load_pandas_dataframe()` |
| Spark DataFrames | `DatasetVariable.create(spark_df)` | `load_spark_dataframe()` |

### Direct Dataset Usage

| Framework | Direct Usage | When to Use |
| ----- | ----- | ----- |
| Ray Datasets | `rd.read_parquet(...)` | Large-scale processing |
| Pandas DataFrames | `pd.read_csv(...)` | Small datasets |
| Spark DataFrames | `spark.read.parquet(...)` | Large-scale processing |

```py
import michelangelo.uniflow.core as uniflow
import ray.data as rd
from michelangelo.uniflow.plugins.ray import RayTask

@uniflow.task(config=RayTask(head_cpu=2, head_memory="4Gi"))
def process_data_directly(data_path: str):
    dataset = rd.read_parquet(data_path) \
        .map_batches(preprocessing_function) \
        .train_test_split(test_size=0.2)
    return dataset
```

### Creating DatasetVariables

```py
import ray.data as rd
from michelangelo.workflow.variables import DatasetVariable

ray_dataset = rd.read_parquet("s3://bucket/data.parquet")
dataset_var = DatasetVariable.create(ray_dataset)

import pandas as pd
pandas_df = pd.read_csv("local_file.csv")
dataset_var = DatasetVariable.create(pandas_df)

spark_df = spark.read.parquet("s3://bucket/data.parquet")
dataset_var = DatasetVariable.create(spark_df)
```

## Automatic Storage in Uniflow Tasks

```py
import michelangelo.uniflow.core as uniflow
import ray.data as rd
from michelangelo.workflow.variables import DatasetVariable
from michelangelo.uniflow.plugins.ray import RayTask

@uniflow.task(config=RayTask(head_cpu=2, head_memory="4Gi"))
def prepare_training_data(data_path: str):
    dataset = rd.read_parquet(data_path).map_batches(clean_and_normalize)
    train_ds, val_ds = dataset.train_test_split(test_size=0.2)
    train_dv = DatasetVariable.create(train_ds)
    train_dv.save_ray_dataset()
    val_dv = DatasetVariable.create(val_ds)
    val_dv.save_ray_dataset()
    return {
        "train": train_dv,
        "validation": val_dv
    }
```

```py
@uniflow.task(config=RayTask(head_cpu=2, head_memory="4Gi"))
def use_prepared_data(datasets: dict):
    datasets["train"].load_ray_dataset()
    datasets["validation"].load_ray_dataset()
    train_data = datasets["train"].value
    val_data = datasets["validation"].value
```

## Integration with Trainer SDK

```py
trainer_param = LightningTrainerParam(
    create_model=create_model_function,
    model_kwargs=model_config,
    train_data=train_dv.value,
    validation_data=val_dv.value,
    batch_size=32,
    num_epochs=10
)

trainer = LightningTrainer(trainer_param)
result = trainer.train(run_config, scaling_config)
```

## Best Practices

* Use Parquet for large datasets  
* Process in batches  
* Validate data after preprocessing  
* Leverage uniflow tasks for caching and reproducibility

## Next Steps

* Continue to [Model Training guide](./train-and-register-a-model.md)
* Check troubleshooting section

## Common Issues

* Out of memory → Reduce batch size or use Spark  
* Slow preprocessing → Increase `num_cpus`  
* Inconsistent results → Ensure deterministic preprocessing