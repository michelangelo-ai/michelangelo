# Model Registry Guide

Save, version, and manage trained models using Michelangelo's comprehensive model packaging system.

## What Michelangelo Model Packaging Provides

Michelangelo's model packager handles the complete model lifecycle:

* **Creates Model Custom Resource** – Registers model metadata in Kubernetes
* **Stores metadata in ETCD** – Version history, schema, and lineage tracking
* **Dual format packaging** – Raw model files + deployable (serveable) model files
* **Cloud storage integration** – Automatic upload/download to S3/GCS
* **Inference ready** – Fully compatible with Triton Inference Server
* **Model validation** – Built-in schema and inference tests
* **Easy downloading** – Retrieve any version for serving or fine-tuning

# Model Registration

## Register a Model

```py
from michelangelo.lib.model_manager import CustomTritonPackager
import michelangelo.uniflow.core as uniflow

@uniflow.task()
def register_model(model_path: str, model_name: str, package_name: str):
    """Register a trained model"""

    packager = CustomTritonPackager()

    # input and output feature schema for the model
    model_schema = ModelSchema(
        input_schema=[  # list of input features and their schema
            ModelSchemaItem(
                name="feature",
                data_type=DataType.STRING,
                shape=[1],
            ),
        ],
        output_schema=[  # list of output features and their schema
            ModelSchemaItem(
                name="response",
                data_type=DataType.STRING,
                shape=[1],
            ),
        ],
    )

    deployable_model_package_path = packager.create_model_package(
        model_path="/a/b/c",                          # The path of the directory that contains the pretrained raw model binaries, e.g. binaries from torch.save
        model_class="foo.bar.CustomModel",            # The full import path for the model class which contains the inference logics
        model_schema=model_schema,
        model_path_source_type=StorageType.LOCAL,     # or StorageType.TERRABLOB, or StorageType.HDFS,
        include_import_prefixes=["uber"]              # (Optional) Only save the imported modules with the given prefixes in the model package,
                                                      # e.g. ['uber', 'data.michelangelo'] only imports starting with 'uber' or 'data.michelangelo' will be saved in the model package.
                                                      # Default is ['uber'], and if the list is [], save all imports
    )

    raw_model_package_path = packager.create_raw_model_package(
        model_path="/a/b/c",                          # The path of the directory that contains the pretrained raw model binaries, e.g. binaries from torch.save
        model_class="foo.bar.CustomModel",            # The full import path for the model class which contains the inference logics
        model_schema=model_schema,
        sample_data=[                                 # The sample data is a list of example inputs for your model, at least one input is needed
            {"feature": np.array([b"a"])},
            {"feature": np.array([b"b"])},
            ...
        ],
        model_path_source_type=StorageType.LOCAL,     # or StorageType.TERRABLOB, or StorageType.HDFS
        requirements=[                                # (Optional) The third-party libraries the model depends on. This can also be the path to a requirements.txt file.
            "pandas=2.2.2",                           # The requirements will be saved as the "dependencies/requirements.txt" file in the raw model package.
            "scikit-learn",                           # It is recommended to configure this parameter to help future users of the model to recreate the environment needed to load and run the model.
            ...
        ],
        include_import_prefixes=["uber"]              # (Optional) Only save the imported modules with the given prefixes in the model package,
                                                      # e.g. ['uber', 'data.michelangelo'] only imports starting with 'uber' or 'data.michelangelo' will be saved in the model package.
                                                      # Default is ['uber'], and if the list is [], save all imports
    )
```

### What happens during registration

1. A **Model Custom Resource (CR)** is created in the Michelangelo control plane
2. Model **schema & metadata** are stored in ETCD
3. Two artifacts are generated:
   * **Raw model format** – original training files
   * **Deployable model format** – optimized for Triton inference
4. Packager **uploads both artifacts** to cloud storage
5. Model **validation tests** run (load → schema → sample inference)

# Model Formats and Storage

Michelangelo produces **two complementary formats**.

## Raw Model Format (Developer-Facing)

Used for:

* Fine-tuning
* Offline analysis
* Reproducibility

### Directory structure

```sh
model_name/
└── 0/
    ├── metadata/
    │   ├── type.yaml
    │   ├── schema.yaml
    │   └── sample_data.yaml
    ├── model/                        # the model binaries
    └── defs/
        ├── model_class.txt
        ├── michelangelo/...          # runtime code
        └── team/project/models/...   # domain code
```

## Deployable Model Format (Inference-Ready)

*(Updated using your uploaded model.tar)*

The deployable artifact is packaged into a **Triton-compatible model repository**.

### Directory structure

```sh
model_name/
├── 0/
│   ├── model.py
│   ├── user_model.py
│   ├── model_class.txt
│   ├── download.yaml
│   ├── uber/ai/michelangelo/...      # runtime code
│   ├── uber/product/eats/...         # domain code
│   └── ... additional runtime modules ...
└── config.pbtxt
```

### Purpose of key files

| File | Purpose |
| ----- | ----- |
| **model.py** | Triton Python backend entry point |
| **user_model.py** | Your actual model implementation: forward pass & logic |
| **model_class.txt** | Fully-qualified Python class path |
| **download.yaml** | Metadata describing how raw model files were produced |
| **config.pbtxt** | Triton configuration (I/O schema, batching, instances) |
| **uber/**\* | Auto-packaged modules required at inference time |

### Features of Deployable Models

* Fully Triton-compatible
* Batch + real-time inference
* Supports GPU/CPU via Triton instance groups
* Versioned (`0/`, `1/`, …)
* Bundles all Python code required to run inference

Stored in cloud at:

```
s3://<bucket>/models/<model_name>/serve/<version>/
```

# Inference Support

## Online & Offline Inference

```py
from uber.ai.michelangelo.sdk.model_manager.downloader import download_raw_model
from uber.ai.michelangelo.sdk.model_manager.serde.model import load_raw_model

model_path = download_raw_model(
    project_name="ma-dev-test-uber-one",              # The MA Studio Project
    model_name="model-20240913-222032-93cdf484",      # The model name
)

model = load_raw_model(model_path)                    # Load the model, the returned model is a subclass of uber.ai.michelangelo.sdk.model_manager.interface.custom_model.Model

inputs = {                                            # These inputs should match the schema defined in ModelSchema::input_schema
    "feature1": np.array([b"test_feature"]),          # Note: If the schema expects the feature to be DataType.STRING, the model expects the feature to be a ndarray of byte string
    ...
}

result = model.predict(inputs)                        # Outputs should match the schema defined in ModelSchema::output_schema

response = result.get("response")[0]
```

# Loading & Downloading Models

## Load model for serving or analysis

```py
model = packager.load_model("housing-predictor")
model_v2 = packager.load_model("housing-predictor", version="2")
```

## Download for fine-tuning or custom deployment

```py
from uber.ai.michelangelo.sdk.model_manager.downloader import download_raw_model
from uber.ai.michelangelo.sdk.model_manager.serde.model import load_raw_model

model_path = download_raw_model(
    project_name="ma-dev-test-uber-one",              # The MA Studio Project
    model_name="model-20240913-222032-93cdf484",      # The model name
)

model = load_raw_model(model_path)                    # Load the model, the returned model is a subclass of uber.ai.michelangelo.sdk.model_manager.interface.custom_model.Model

inputs = {                                            # These inputs should match the schema defined in ModelSchema::input_schema
    "feature1": np.array([b"test_feature"]),          # Note: If the schema expects the feature to be DataType.STRING, the model expects the feature to be a ndarray of byte string
    ...
}

result = model.predict(inputs)                        # Outputs should match the schema defined in ModelSchema::output_schema

response = result.get("response")[0]
```
