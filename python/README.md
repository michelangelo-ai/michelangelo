# Michelangelo

**An end-to-end ML platform for building, training, and registering machine learning models at scale.**

[![Documentation](https://img.shields.io/badge/docs-michelangelo--ai.github.io-blue)](https://michelangelo-ai.github.io/michelangelo/docs)
[![GitHub](https://img.shields.io/badge/github-michelangelo--ai%2Fmichelangelo-lightgrey)](https://github.com/michelangelo-ai/michelangelo)

Michelangelo gives ML engineers and data scientists a unified Python SDK for the entire model lifecycle — from data preparation and distributed training to model registration and production deployment. Define your ML workflows as Python functions using simple decorators, and Michelangelo handles orchestration, caching, and scaling across Ray and Spark clusters.

## Key Features

- **UniFlow Pipeline Framework** — Define ML workflows with `@task` and `@workflow` decorators. Write plain Python functions and Michelangelo handles distributed execution, data passing between tasks, and result caching.

- **Distributed Execution** — Scale tasks across Ray or Spark clusters with a single config change. Specify CPU, memory, GPU, and worker resources per task — no changes to your business logic required.

- **Built-in Caching and Resume** — Tasks cache results automatically based on inputs. If a pipeline fails partway through, resume from where it left off instead of rerunning everything.

- **Python API Client** — Programmatically manage projects, pipelines, model registry, and pipeline runs through a gRPC-based Python client.

- **CLI (`ma`)** — Register pipelines, manage triggers, run sandboxes, and interact with the Michelangelo platform from your terminal.

- **Flexible Storage** — Read and write data across S3, GCS, HDFS, and local filesystems using the fsspec-based storage layer.

## Installation

Install the core package:

```bash
pip install michelangelo
```

Install with distributed execution plugins (Ray and Spark):

```bash
pip install michelangelo[plugin]
```

### Install Extras

| Extra | What it includes | When to use it |
|-------|-----------------|----------------|
| `michelangelo[plugin]` | Ray, PySpark | You want to run tasks on distributed Ray or Spark clusters |
| `michelangelo[vllm]` | vLLM, Ray, PyTorch, Transformers | You're serving or fine-tuning large language models |
| `michelangelo[example]` | All ML libraries for examples | You want to run the included example projects |
| `michelangelo[dev]` | pytest, ruff, pre-commit, Ray | You're contributing to Michelangelo itself |

## Quickstart

Here's a minimal pipeline that loads data and trains a model using Ray for distributed execution:

```python
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask


@uniflow.task(config=RayTask(head_cpu=1, head_memory="2Gi"))
def load_data(path: str):
    """Load and preprocess data."""
    # Your data loading logic here
    print(f"Loading data from {path}")
    return {"train": [1, 2, 3], "test": [4, 5]}


@uniflow.task(config=RayTask(head_cpu=2, head_memory="4Gi"))
def train_model(data):
    """Train a model on the prepared data."""
    print(f"Training on {len(data['train'])} samples")
    return {"accuracy": 0.95}


@uniflow.workflow()
def training_pipeline(data_path: str):
    """A simple training pipeline."""
    data = load_data(data_path)
    result = train_model(data)
    return result


if __name__ == "__main__":
    ctx = uniflow.create_context()
    ctx.run(training_pipeline, data_path="s3://my-bucket/data")
```

Run locally:

```bash
python my_pipeline.py
```

Want to use Spark instead of Ray? Just swap the task config:

```python
from michelangelo.uniflow.plugins.spark import SparkTask

@uniflow.task(config=SparkTask(driver_cpu=2, executor_cpu=4, executor_instances=3))
def process_data(df):
    # Your Spark processing logic
    return df
```

For complete working examples, see the [examples directory](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples), including:

- [BERT fine-tuning on CoLA](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/bert_cola) — Text classification with distributed GPU training
- [XGBoost on Boston Housing](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/boston_housing_xgb) — Tabular regression with distributed training
- [GPT fine-tuning with LoRA](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/gpt_oss_20b_finetune) — Large language model fine-tuning

## Using the Python API Client

Manage platform resources programmatically:

```python
from michelangelo.api.v2.client import APIClient

APIClient.set_caller("my-client")

# List projects
projects = APIClient.ProjectService.list_project(namespace="default")

# Create a new project
from michelangelo.gen.api.v2.project_pb2 import Project

proj = Project()
proj.metadata.namespace = "default"
proj.metadata.name = "my-project"
proj.spec.description = "My ML project"
APIClient.ProjectService.create_project(proj)
```

Set the API server address via environment variable:

```bash
export MICHELANGELO_API_SERVER="localhost:12345"
```

## Documentation

Full documentation is available at **[michelangelo-ai.github.io/michelangelo/docs](https://michelangelo-ai.github.io/michelangelo/docs)**.

- [User Guides](https://michelangelo-ai.github.io/michelangelo/docs/user-guides) — Step-by-step guides for data preparation, training, and deployment
- [ML Pipelines](https://michelangelo-ai.github.io/michelangelo/docs/user-guides/ml-pipelines) — Deep dive into the UniFlow pipeline framework
- [Set Up Triggers](https://michelangelo-ai.github.io/michelangelo/docs/user-guides/set-up-triggers) — Automate pipeline execution with cron and backfill triggers
- [CLI Reference](https://michelangelo-ai.github.io/michelangelo/docs/user-guides/cli) — Full command-line interface documentation

## Contributing

We welcome contributions! To get started:

```bash
git clone https://github.com/michelangelo-ai/michelangelo.git
cd michelangelo/python
pip install -e ".[dev]"
```

Run the test suite:

```bash
pytest
```

Format your code:

```bash
ruff format .
ruff check .
```

## Requirements

- Python 3.9+

## License

See [LICENSE](https://github.com/michelangelo-ai/michelangelo/blob/main/LICENSE) for details.
