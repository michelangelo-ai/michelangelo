# Michelangelo AI

**An end-to-end ML platform for building, training, and registering machine learning models at scale.**

[![Documentation](https://img.shields.io/badge/docs-michelangelo--ai.org-blue)](https://michelangelo-ai.org/docs)
[![GitHub](https://img.shields.io/badge/github-michelangelo--ai%2Fmichelangelo-lightgrey)](https://github.com/michelangelo-ai/michelangelo)

Michelangelo AI gives ML engineers and data scientists a unified Python SDK for the entire model lifecycle — from data preparation and distributed training to model registration and production deployment. Define your ML workflows as Python functions using simple decorators, and Michelangelo AI handles orchestration, caching, and scaling across Ray and Spark clusters.

## Key Features

- **Uniflow Pipeline Framework** — Define ML workflows with `@task` and `@workflow` decorators. Write plain Python functions and Michelangelo AI handles distributed execution, data passing between tasks, and result caching.

- **Distributed Execution** — Scale tasks across Ray or Spark clusters with a single config change. Specify CPU, memory, GPU, and worker resources per task — no changes to your business logic required.

- **Built-in Caching and Resume** — Tasks cache results automatically based on inputs. If a pipeline fails partway through, resume from where it left off instead of rerunning everything.

- **Python API Client** — Programmatically manage projects, pipelines, model registry, and pipeline runs through a gRPC-based Python client.

- **CLI (`ma`)** — Register pipelines, manage triggers, run sandboxes, and interact with the Michelangelo AI platform from your terminal.

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
| `michelangelo[ray-polars]` | Ray, Polars | You read Ray Datasets with nested list/struct columns (Polars fallback for [ray#61675](https://github.com/ray-project/ray/issues/61675)) |
| `michelangelo[vllm]` | vLLM, Ray, PyTorch, Transformers | You're serving or fine-tuning large language models |
| `michelangelo[example]` | All ML libraries for examples | You want to run the included example projects |
| `michelangelo[dev]` | pytest, ruff, pre-commit, Ray | You're contributing to Michelangelo AI itself |

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

### I/O Plugins

Michelangelo AI provides typed I/O handlers for passing data between tasks. The
handler is selected automatically based on the Python type of the value being
written.

| Plugin | Type handled | Import |
|--------|-------------|--------|
| `RayDatasetIO` | `ray.data.Dataset` | `from michelangelo.uniflow.plugins.ray import RayDatasetIO` |
| `PandasIO` | `pandas.DataFrame` | `from michelangelo.uniflow.plugins.pandas import PandasIO` |
| `SparkIO` | `pyspark.sql.DataFrame` | `from michelangelo.uniflow.plugins.spark import SparkIO` |
| `ProtoIO` | `google.protobuf.message.Message` | `from michelangelo.uniflow.plugins.proto import ProtoIO` |

`ProtoIO` serialises protobuf messages as JSON (via `google.protobuf.json_format`)
and stores the message type in the metadata dict for automatic reconstruction on
read. `protobuf` is a core dependency — no extra install required.

For complete working examples, see the [examples directory](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples), including:

- [BERT fine-tuning on CoLA](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/bert_cola) — Text classification with distributed GPU training
- [XGBoost on California Housing](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/california_housing_xgb) — Tabular regression with distributed training
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
export MA_API_SERVER="localhost:12345"
```

## Publishing Evaluation Reports

Evaluation reports capture structured metric charts produced by a training run.
If you've used MLflow or W&B, here's the conceptual mapping:

| Michelangelo AI | MLflow | W&B |
|---|---|---|
| `metadata.namespace` | Experiment name | entity / project |
| `metadata.name` | Run name | Run name |
| Chart with one data point | `log_metric(key, value)` | `wandb.log({key: value})` |

**Push a report via `APIClientEvalReportSink`:**

```python
import os
os.environ["MA_API_SERVER"] = "localhost:50051"

from michelangelo.api.v2 import APIClient
from michelangelo.gen.api.v2.evaluation_report_pb2 import (
    EvaluationReport,
    EvaluationReportSpec,
)
from michelangelo.workflow.tasks.functions.eval_report_sinks import (
    APIClientEvalReportSink,
)

report = EvaluationReport(spec=EvaluationReportSpec(title="Q1 Eval"))
report.metadata.namespace = "my-project"  # analogous to MLflow experiment
report.metadata.name = "q1-eval"

APIClient.set_caller("my-trainer")
sink = APIClientEvalReportSink()
sink.write(report)
```

**Target a different endpoint** (e.g. multi-region, per-worker isolation):

```python
from michelangelo.api.v2 import APIClient
from michelangelo.workflow.tasks.functions.eval_report_sinks import APIClientEvalReportSink

client = APIClient(endpoint="other-server:50051", caller="my-trainer")
sink = APIClientEvalReportSink(svc=client.EvaluationReportService)
sink.write(report)
client.close()
```

**Convert to a flat metrics dict for MLflow / W&B / Comet:**

```python
from michelangelo.workflow.tasks.functions.eval_report_sinks import flatten_report_to_metrics
import mlflow

mlflow.log_metrics(flatten_report_to_metrics(report))
```

For structured pipeline integration see `EvalReportPluginConfig` in the
[ML Pipelines docs](https://michelangelo-ai.org/docs/user-guides/ml-pipelines).

## Documentation

Full documentation is available at **[michelangelo-ai.org/docs](https://michelangelo-ai.org/docs)**.

- [User Guides](https://michelangelo-ai.org/docs/user-guides) — Step-by-step guides for data preparation, training, and deployment
- [ML Pipelines](https://michelangelo-ai.org/docs/user-guides/ml-pipelines) — Deep dive into the Uniflow pipeline framework
- [Set Up Triggers](https://michelangelo-ai.org/docs/user-guides/ml-pipelines/set-up-triggers) — Automate pipeline execution with cron and backfill triggers
- [CLI Reference](https://michelangelo-ai.org/docs/user-guides/reference/cli) — Full command-line interface documentation

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
