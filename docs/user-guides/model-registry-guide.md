# Model Registry Guide

Save, version, and manage trained models using Michelangelo's model packaging system.

## Overview

Michelangelo's model packager turns your trained model into self-contained, versioned artifacts ready for serving or sharing. The packager handles dependency bundling, schema validation, and Triton configuration generation automatically.

**What the model packager provides:**

* **Dual format packaging** -- deployable (Triton-ready) and raw (developer-facing) artifacts
* **Schema validation** -- input/output contracts enforced at packaging time
* **Dependency bundling** -- auto-packages Python modules your model needs at inference time
* **Built-in testing** -- raw packages are validated with sample data before they are written
* **Triton compatibility** -- deployable packages work directly with NVIDIA Triton Inference Server

## Core Concepts

Before diving in, here are the key components you will work with:

| Concept | What It Is | Module |
|---------|-----------|--------|
| **Model** | Abstract base class your model must implement (`save`, `load`, `predict`) | `michelangelo.lib.model_manager.interface.custom_model` |
| **ModelSchema** | Defines input/output feature names, types, and shapes | `michelangelo.lib.model_manager.schema` |
| **CustomTritonPackager** | Creates deployable and raw model packages | `michelangelo.lib.model_manager.packager.custom_triton` |
| **load_raw_model** | Loads a raw model package for testing or fine-tuning | `michelangelo.lib.model_manager.serde.model` |

The packager produces two complementary artifacts:

| Artifact | Purpose | Created By |
|----------|---------|-----------|
| **Deployable model package** | Triton Inference Server deployment | `create_model_package()` |
| **Raw model package** | Testing, fine-tuning, reproducibility | `create_raw_model_package()` |

## Quick Start: Package Your First Model

This end-to-end walkthrough takes you from a trained model to a verified package in four steps.

### Step 1: Implement the Model interface

Create a class that extends `Model` with three methods: `save`, `load`, and `predict`.

```py
import os
import numpy as np
from michelangelo.lib.model_manager.interface.custom_model import Model


class EchoModel(Model):
    """Minimal model that returns inputs unchanged."""

    def save(self, path: str) -> None:
        os.makedirs(path, exist_ok=True)
        with open(os.path.join(path, "config.txt"), "w") as f:
            f.write("echo-model-v1")

    @classmethod
    def load(cls, path: str) -> "EchoModel":
        with open(os.path.join(path, "config.txt")) as f:
            f.read()
        return cls()

    def predict(self, inputs: dict[str, np.ndarray]) -> dict[str, np.ndarray]:
        a = inputs["a"].astype(np.int32)
        return {"response": a, "doubled": (2 * a).astype(np.int32)}
```

### Step 2: Define the schema

Declare what your model expects as input and what it produces as output.

```py
from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem

schema = ModelSchema(
    input_schema=[
        ModelSchemaItem(name="a", data_type=DataType.INT, shape=[1]),
    ],
    output_schema=[
        ModelSchemaItem(name="response", data_type=DataType.INT, shape=[1]),
        ModelSchemaItem(name="doubled", data_type=DataType.INT, shape=[1]),
    ],
)
```

### Step 3: Package the model

Save your model artifacts, then create both package types.

```py
from michelangelo.lib.model_manager.packager.custom_triton import CustomTritonPackager

# Save model artifacts first
model = EchoModel()
model.save("/tmp/echo-artifacts")

# Create the packager
packager = CustomTritonPackager()

# Deployable package (for Triton serving)
deployable_path = packager.create_model_package(
    model_path="/tmp/echo-artifacts",
    model_class="myproject.models.EchoModel",
    model_schema=schema,
    model_name="echo-model",
    dest_model_path="/tmp/echo-deployable",
)

# Raw package (for testing and fine-tuning)
sample_data = [
    {"a": np.array([1], dtype=np.int32)},
    {"a": np.array([5], dtype=np.int32)},
]

raw_path = packager.create_raw_model_package(
    model_path="/tmp/echo-artifacts",
    model_class="myproject.models.EchoModel",
    model_schema=schema,
    sample_data=sample_data,
    dest_model_path="/tmp/echo-raw",
    requirements=["numpy"],
)
```

### Step 4: Verify the package

Load the raw package and run a prediction to confirm everything works.

```py
from michelangelo.lib.model_manager.serde.model import load_raw_model

loaded = load_raw_model("/tmp/echo-raw")
result = loaded.predict({"a": np.array([42], dtype=np.int32)})
print(result)
# {'response': array([42], dtype=int32), 'doubled': array([84], dtype=int32)}
```

You now have a deployable Triton package at `/tmp/echo-deployable` and a verified raw package at `/tmp/echo-raw`.

## API Reference

### Model Interface

All custom models must extend the `Model` abstract base class:

```py
from michelangelo.lib.model_manager.interface.custom_model import Model
```

| Method | Signature | Description |
|--------|-----------|-------------|
| `save` | `save(self, path: str)` | Serialize model artifacts to a directory |
| `load` | `load(cls, path: str) -> Model` | Class method that loads and returns a ready-to-use model instance |
| `predict` | `predict(self, inputs: dict[str, ndarray]) -> dict[str, ndarray]` | Run inference; keys must match the model schema |

:::tip
Avoid using `pickle` or `torch.save` directly for persistence. Prefer format-specific serialization methods (e.g., `state_dict` for PyTorch, SavedModel for TensorFlow) for better compatibility and security.
:::

### ModelSchema

Defines the contract between your model and the serving infrastructure.

```py
from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem
```

**ModelSchemaItem fields:**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | `str` | (required) | Feature name, used as the key in input/output dictionaries |
| `data_type` | `DataType` | `DataType.UNKNOWN` | The data type of the feature |
| `shape` | `list[int]` | `None` | Shape following NumPy conventions; use `-1` for variable-length dimensions |
| `optional` | `bool` | `None` | If `True`, the feature may be omitted from input data |

**Supported data types:**

| DataType | Python/NumPy Type | Description |
|----------|------------------|-------------|
| `BOOLEAN` | `bool` | Boolean values |
| `STRING` | `str` / `bytes` | Text data (passed as byte strings in NumPy arrays) |
| `BYTE` | `int8` | 8-bit signed integer |
| `CHAR` | `uint8` | 8-bit unsigned integer |
| `SHORT` | `int16` | 16-bit signed integer |
| `INT` | `int32` | 32-bit signed integer |
| `LONG` | `int64` | 64-bit signed integer |
| `FLOAT` | `float32` | 32-bit floating point |
| `DOUBLE` | `float64` | 64-bit floating point |

**Shape examples:**

| Shape | Meaning |
|-------|---------|
| `[1]` | Scalar value |
| `[10]` | 1D array of length 10 |
| `[10, 5]` | 2D array (10 rows, 5 columns) |
| `[-1]` | Variable-length 1D array |

### CustomTritonPackager

```py
from michelangelo.lib.model_manager.packager.custom_triton import CustomTritonPackager
```

**Constructor:**

| Parameter | Default | Description |
|-----------|---------|-------------|
| `custom_batch_processing` | `False` | If `True`, your model handles batching internally |

**`create_model_package()` parameters:**

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `model_path` | Yes | -- | Path to saved model artifacts |
| `model_class` | Yes | -- | Fully qualified Python class name (e.g., `"mypackage.models.MyModel"`) |
| `model_schema` | Yes | -- | `ModelSchema` instance defining inputs and outputs |
| `model_name` | No | Derived from class | Display name in Michelangelo Studio |
| `dest_model_path` | No | Auto temp dir | Output directory for the package |
| `model_revision` | No | `None` | Revision number for versioning |
| `model_path_source_type` | No | `StorageType.LOCAL` | Storage backend type |
| `include_import_prefixes` | No | `None` (all imports) | List of module prefixes to bundle |

**`create_raw_model_package()` parameters:**

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `model_path` | Yes | -- | Path to saved model artifacts |
| `model_class` | Yes | -- | Fully qualified Python class name |
| `model_schema` | Yes | -- | `ModelSchema` instance defining inputs and outputs |
| `sample_data` | Yes | -- | List of sample inputs for validation |
| `dest_model_path` | No | Auto temp dir | Output directory for the package |
| `model_path_source_type` | No | `StorageType.LOCAL` | Storage backend type |
| `requirements` | No | `None` | Dependencies as a list or path to `requirements.txt` |
| `include_import_prefixes` | No | `None` (all imports) | List of module prefixes to bundle |

### load_raw_model

```py
from michelangelo.lib.model_manager.serde.model import load_raw_model

model = load_raw_model("/path/to/raw/package")
```

Returns an instance of your `Model` subclass, fully loaded and ready for inference.

:::note
`load_raw_model` currently supports Custom Python models (`RawModelType.CUSTOM_PYTHON`). Support for additional model types (HuggingFace, PyTorch) is planned for future releases.
:::

## Advanced Topics

### PyTorch Model Example

The packager works with any framework. Here is an example using PyTorch internally while conforming to the numpy-based Model interface:

```py
import numpy as np
import torch
from michelangelo.lib.model_manager.interface.custom_model import Model


class TorchClassifier(Model):
    """PyTorch model with numpy I/O for Model Manager."""

    def __init__(self):
        self.net = torch.nn.Linear(4, 2)

    def save(self, path: str) -> None:
        import os
        os.makedirs(path, exist_ok=True)
        torch.save(self.net.state_dict(), os.path.join(path, "model.pt"))

    @classmethod
    def load(cls, path: str) -> "TorchClassifier":
        import os
        obj = cls()
        state = torch.load(os.path.join(path, "model.pt"), weights_only=True)
        obj.net.load_state_dict(state)
        obj.net.eval()
        return obj

    def predict(self, inputs: dict[str, np.ndarray]) -> dict[str, np.ndarray]:
        x = torch.from_numpy(inputs["x"].astype(np.float32))
        with torch.no_grad():
            out = self.net(x)
        return {"prediction": out.numpy()}
```

Package it with a matching schema:

```py
schema = ModelSchema(
    input_schema=[
        ModelSchemaItem(name="x", data_type=DataType.FLOAT, shape=[1, 4]),
    ],
    output_schema=[
        ModelSchemaItem(name="prediction", data_type=DataType.FLOAT, shape=[1, 2]),
    ],
)

sample_data = [
    {"x": np.array([[1.0, 2.0, 3.0, 4.0]], dtype=np.float32)},
]

packager = CustomTritonPackager()

raw_path = packager.create_raw_model_package(
    model_path="/tmp/torch-artifacts",
    model_class="myproject.models.TorchClassifier",
    model_schema=schema,
    sample_data=sample_data,
    requirements=["numpy", "torch"],
)
```

### Custom Batch Processing

By default, Triton handles batching automatically and your `predict` method receives individual samples. If your model handles batching internally, enable custom batch processing:

```py
packager = CustomTritonPackager(custom_batch_processing=True)
```

When enabled, inputs include an additional leading batch dimension. For example, if the schema specifies shape `[n, m]`, the actual input shape will be `[batch_size, n, m]`.

### Model Package Formats

#### Deployable Format (Triton-Compatible)

```
model_name/
├── 0/
│   ├── model.py                    # Triton Python backend entry point
│   ├── user_model.py               # Your model implementation
│   ├── model_class.txt             # Fully qualified Python class path
│   ├── download.yaml               # Metadata for raw model files
│   └── myproject/models/...        # Auto-packaged runtime dependencies
└── config.pbtxt                    # Triton configuration (I/O schema, batching)
```

| File | Purpose |
|------|---------|
| `model.py` | Triton Python backend entry point |
| `user_model.py` | Your model's forward pass and inference logic |
| `model_class.txt` | Fully qualified Python class path |
| `download.yaml` | Metadata describing how raw model files were produced |
| `config.pbtxt` | Triton configuration (I/O schema, batching, instances) |

#### Raw Format (Developer-Facing)

```
model_name/
└── 0/
    ├── metadata/
    │   ├── type.yaml               # Model type (custom-python, torch, etc.)
    │   ├── schema.yaml             # Input/output schema
    │   └── sample_data.yaml        # Sample data for testing
    ├── model/                      # Model binaries (your saved artifacts)
    └── defs/
        ├── model_class.txt         # Fully qualified class path
        └── myproject/models/...    # Runtime code dependencies
```

## Integration with Uniflow Workflows

Package model registration as a task in your ML pipeline:

```py
import michelangelo.uniflow.core as uniflow
from michelangelo.lib.model_manager.packager.custom_triton import CustomTritonPackager
from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem
from michelangelo.uniflow.plugins.ray import RayTask


@uniflow.task(config=RayTask(head_cpu=2, head_memory="4Gi"))
def package_model(model_path: str, model_class: str):
    """Package a trained model for deployment."""
    packager = CustomTritonPackager()

    schema = ModelSchema(
        input_schema=[
            ModelSchemaItem(name="feature", data_type=DataType.STRING, shape=[1]),
        ],
        output_schema=[
            ModelSchemaItem(name="response", data_type=DataType.STRING, shape=[1]),
        ],
    )

    deployable_path = packager.create_model_package(
        model_path=model_path,
        model_class=model_class,
        model_schema=schema,
    )

    return deployable_path
```

This task can be chained after a training task in a workflow:

```py
@uniflow.workflow()
def train_and_package(dataset_id: str):
    model_path = train_model(dataset_id)
    package_path = package_model(model_path, "myproject.models.MyModel")
    return package_path
```

## Troubleshooting

### `ValueError: model_class is required`

The `model_class` parameter must be a non-empty string containing the fully qualified Python class path (e.g., `"mypackage.models.MyModel"`).

### `ValueError: model_schema is required`

A `ModelSchema` with at least one input and one output `ModelSchemaItem` must be provided.

### Schema validation errors

Ensure your sample data matches the schema exactly:

* Each required input feature must be present in every sample
* Array shapes must match the schema's `shape` field
* Array dtypes must be compatible with the schema's `data_type`

:::warning
When using `DataType.STRING`, pass byte strings in your NumPy arrays (e.g., `np.array([b"hello"])`), not regular Python strings.
:::

### `NotImplementedError: The loader for ... model is not supported yet`

`load_raw_model` currently only supports Custom Python models. HuggingFace and PyTorch loaders are planned for future releases.

### Model class validation fails

Your model class must:

* Be importable from the current Python environment
* Extend `michelangelo.lib.model_manager.interface.custom_model.Model`
* Implement all three abstract methods: `save`, `load`, and `predict`

## Next Steps

* See working examples in [`python/examples/model_manager/`](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/model_manager)
* Learn about [model training](./train-and-register-a-model.md) to prepare models for packaging
* Learn about [data preparation](./prepare-your-data.md) for your training pipeline
