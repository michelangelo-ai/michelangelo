---
sidebar_position: 4
---

# MLOps Glossary

A concise reference for terms used throughout Michelangelo AI documentation. For a deeper look at how these concepts relate to each other, see [Core Concepts and Key Terms](./core-concepts-and-key-terms.md).

---

## A

### API Server
The gRPC server in front of Michelangelo AI's Kubernetes custom resources (`go/cmd/apiserver`). The `ma` CLI, the UI, the worker and the Python SDK all talk to it. The UI reaches it through an Envoy proxy that transcodes JSON to gRPC. See [API Framework](../operator-guides/api-framework.md).

### Artifact Store
Persistent storage for ML artifacts (trained models, evaluation reports, datasets). Michelangelo AI uses S3-compatible object storage; the sandbox runs SeaweedFS by default. See [Object Store](#object-store).

### Artifact
Any file or object produced or consumed during a pipeline run — model weights, serialized datasets, evaluation reports, or configuration files.

---

## B

### Backfill
A one-off TriggerRun that replays a pipeline over a past time range. When both `start_timestamp` and `end_timestamp` are set and `catchup` is not, the trigger runs once for each schedule occurrence in that range and then stops. See [Set Up Triggers](../user-guides/ml-pipelines/set-up-triggers.md) and [Backfill Pipelines](../user-guides/ml-pipelines/backfill.md).

---

## C

### CanvasFlex
A code-driven YAML workflow system for teams that want opinionated structure, version control, and software engineering best practices applied to their ML pipelines. An alternative to building pipelines entirely in Python with Uniflow.

### Caching
Uniflow can cache a task's output, keyed on the task's inputs and a cache version, and return the cached result instead of recomputing when the task runs again with the same inputs. Caching is opt-in: pass `cache_enabled=True` to `@uniflow.task()`. When `cache_version` is not set, the version is derived from the container image.

### Cadence / Temporal
The workflow orchestration engine that powers Uniflow's durable execution model. Workflows compile to Starlark and run in a Cadence/Temporal worker, providing retries, replay, and long-running execution out of the box. Cadence is the default in the Helm chart and the sandbox.

### Cascade Delete
Deleting a Pipeline also deletes its PipelineRuns, TriggerRuns and pipeline Revisions, after their in-flight workflows are cancelled. This is the default; delete with the `orphan` propagation policy to keep the children. See [Pipeline Cascade Delete](../operator-guides/cascade-delete.md).

### Catch-up
An option on a cron TriggerRun (`catchup: true`) that replays every schedule occurrence missed since `start_timestamp`, once each and in order, before the schedule continues normally. It requires the Temporal engine; Cadence rejects it. A catch-up that would replay more than 500 occurrences is rejected.

### Checkpoint
An intermediate dataset saved to storage during a pipeline run. Enables fault tolerance and task output reuse across executions.

### Compute Cluster
A pool of hardware resources (CPU, GPU, memory) registered with the Michelangelo AI control plane. Tasks are dispatched to compute clusters via the Ray Operator, Spark Operator, or Kubernetes Batch.

### Control Plane
The Michelangelo AI services that manage resources and run workflows: the [API Server](#api-server), the [Controller Manager](#controller-manager) and the [Worker](#worker), together with Kubernetes and the workflow engine.

### Controller Manager
The process that runs Michelangelo AI's Kubernetes controllers (`go/cmd/controllermgr`): pipelines, pipeline runs, triggers, Ray and Spark jobs, deployments, inference servers, and the ingester.

### Cron Trigger
A TriggerRun that starts a pipeline run on a cron schedule, for example `0 9 * * *` for every day at 09:00. See [Set Up Triggers](../user-guides/ml-pipelines/set-up-triggers.md).

### Custom Resource (CRD)
Michelangelo AI stores its objects as Kubernetes custom resources. There are 15 kinds: Project, Pipeline, PipelineRun, TriggerRun, Revision, Model, ModelFamily, EvaluationReport, Deployment, InferenceServer, CachedOutput, Cluster, RayCluster, RayJob and SparkJob.

---

## D

### DatasetVariable
A cross-backend dataset reference that allows tasks on different compute backends (Spark and Ray) to pass datasets to each other. A Spark task wraps its output in a `DatasetVariable` and saves it; a Ray task receives the reference and loads it in its own format.

### Deployment
A running instance of a model revision, loaded into a target inference environment. A deployment provides a human-readable name for accessing a specific model version and is rolled out with a rolling strategy; additional rollout strategies are on the roadmap.

---

## E

### Endpoint
The routing layer that directs prediction requests to one or more deployments. Endpoints route requests by inference server and deployment name and are the entry point for online inference; traffic splitting across model revisions (e.g., for A/B testing) is on the roadmap.

### Evaluation Report
A structured collection of model metrics produced after a training or evaluation run. Common report types include model performance, feature importance, and data quality reports.

---

## F

### Feature
An individual measurable property used as input to a model — a column in your training data (e.g., `age`, `transaction_amount`, `embedding_vector`).

### Future
A handle returned by `concurrent_run()` representing an in-progress task. The workflow continues executing while the task runs in the background. Call `.result()` to block and retrieve the value.

---

## I

### Inference Server
The host process that serves online predictions, managed by Michelangelo AI's InferenceServer controller. The API defines Triton, LLM-D, Dynamo and TorchServe backend types; the open-source controller registers the Triton backend. vLLM is also available (`pip install michelangelo[vllm]`) for calling directly from your own Ray/Uniflow tasks, such as for offline batch inference.

### Incremental Training
A training pattern where a new model revision is trained starting from the weights of a prior revision, rather than from scratch. Each iteration produces a new revision (revision 0 → revision 1 → …).

### Ingester
A controller that watches every Michelangelo AI custom resource and syncs it into MySQL, so the API server can query metadata without relying only on etcd. It uses a finalizer so MySQL is updated before an object is removed. See [Ingester Internals](../contributing/ingester-internals.md).

---

## J

### Job
A batch workload running on a compute cluster. Michelangelo AI runs Spark jobs for data processing and Ray jobs for ML training and inference.

---

## L

### LightningTrainer
A Ray `TorchTrainer` subclass that runs a PyTorch Lightning training loop across Ray Train workers, configured with `LightningTrainerParam`. See [Distributed Training](../user-guides/train-and-deploy-models/distributed-training.md).

### Local Run
Running a workflow script directly on your machine (`python my_workflow.py`). Tasks run in-process, with no cluster or image build. The fastest way to iterate. See [Pipeline Running Modes](../user-guides/ml-pipelines/pipeline-running-modes.md).

---

## M

### `ma` CLI
Michelangelo AI's command-line tool, installed with the Python package. It manages resources such as projects, pipelines and trigger runs (`ma pipeline apply`, `ma trigger_run get`), and creates the local sandbox (`ma sandbox create`). See the [CLI Reference](../user-guides/reference/cli.md).

### MA Studio
The no-code web UI for Michelangelo AI. Provides a visual interface for data preparation, model training (XGBoost, classic ML, deep learning), evaluation, deployment, and monitoring — without writing Python.

### Model
The output artifact of a training job: a serialized set of parameters that a framework can use to make predictions.

### Model Excellence Scores (MES)
Automated quality metrics tracked across the model lifecycle — feature quality, prediction performance, model freshness, and more. MES surfaces model health without requiring manual metric instrumentation.

### Model Family
A group of related models within a project that each address a different sub-problem of the same use case. For example, a ride-pricing project might have separate model families for demand prediction, surge multiplier, and route optimization.

### Model Packager
Turns a trained model into a package that can be served. `CustomTritonPackager` packages any model that implements the `Model` interface (`save`, `load`, `predict`), either as a deployable Triton package (`create_model_package`) or as a raw package for testing and fine-tuning (`create_raw_model_package`). See the [Model Registry Guide](../user-guides/train-and-deploy-models/model-registry-guide.md).

### Model Registry
The versioned catalog of trained models in Michelangelo AI. Each registered model has a name, one or more revisions, associated metadata, and evaluation reports.

### Model Revision
A specific version of a trained model. Revisions are created automatically each time a training job completes. The first revision is revision 0; incremental training increments the revision number.

---

## N

### Native Feature Transform
A PyTorch feature-transform layer from `michelangelo.lib.native_transform` that runs the same way at training time and at serving time, so the code that scaled, bucketized or tokenized a column in training also runs inside the served model. Transforms are declared as a `TransformSpec`. See [Native Feature Transforms](../user-guides/train-and-deploy-models/native-feature-transforms.md).

---

## O

### Object Store
The S3-compatible storage that holds workflow data, task results and artifacts. New sandboxes use SeaweedFS by default; MinIO is still selectable with `ma sandbox create --object-store minio` but is deprecated.

---

## P

### Pipeline
A registered, versioned deployment configuration for a workflow. Pipelines separate _what_ the workflow does (code) from _how_ it is deployed (schedule, resource config, environment). Managed via the `ma` CLI or MA Studio.

### Pipeline Dev Run
Running a pipeline without registering it (`ma pipeline dev-run`). It builds the container image and runs the full pipeline, so it validates the Dockerfile and dependencies before you register. See [Pipeline Running Modes](../user-guides/ml-pipelines/pipeline-running-modes.md).

### PipelineRun
A single execution instance of a pipeline. Each run has a unique ID, tracks task statuses, stores outputs, and can be inspected via MA Studio or the CLI.

### Project
The top-level organizational unit in Michelangelo AI — a business use case with a set of continuously trackable ML metrics. A project contains one or more model families, their datasets, workflows, and deployments.

### Pusher
The `push()` task that exports workflow outputs: models to the model registry and storage, datasets to data sinks, and evaluation reports to report sinks. Each artifact is handled by a pusher plugin. See [Push Models and Artifacts](../user-guides/train-and-deploy-models/push-models-and-artifacts.md).

---

## R

### Ref
A lightweight remote reference to a large object (dataset, model file) stored in a backend like S3 or HDFS. Passing a `Ref` between tasks avoids copying large data into memory — Uniflow resolves it on demand.

### Remote Run
Running a workflow on a Michelangelo AI cluster straight from your script, without registering a pipeline: `python my_workflow.py remote-run --image <image> --storage-url <url>`. Tasks run as Ray or Spark jobs. See [Pipeline Running Modes](../user-guides/ml-pipelines/pipeline-running-modes.md).

---

## S

### Sandbox
A complete Michelangelo AI control plane running on a local k3d Kubernetes cluster, created with `ma sandbox create`. It uses Cadence by default (`--workflow temporal` to switch) and SeaweedFS as its object store. See [Sandbox Setup](./sandbox-setup.md).

### Starlark
A deterministic, Python-like language. In remote runs, the body of a `@uniflow.workflow()` function is transpiled to Starlark and executed by the worker, which makes workflows replayable. This is why workflow bodies can't use `import`, f-strings or `try`/`except`; task bodies are unrestricted Python. See [Workflow Patterns](../user-guides/ml-pipelines/workflow-patterns.md).

### Storage Backend
Where `push()` uploads model files and other artifacts. `StorageBackend` is the interface; `LocalStorageBackend` copies to a local directory. See [Push Models and Artifacts](../user-guides/train-and-deploy-models/push-models-and-artifacts.md).

---

## T

### Task
A Python function decorated with `@uniflow.task()` that performs a single unit of computation. Tasks run in isolated containers on Kubernetes (via Ray or Spark), are independently cacheable and retryable, and are the building blocks of workflows.

### Task Config
The `config` argument of `@uniflow.task()`, which picks the compute backend and its resources: `RayTask` (for example `head_cpu`, `worker_instances`) or `SparkTask` (for example `driver_cpu`, `executor_instances`).

### Task Result
The serialized output of a completed task, stored by Uniflow for caching, debugging, and reuse in downstream tasks.

### TriggerRun
The resource that runs a pipeline automatically, either on a [cron schedule](#cron-trigger) or as a one-off [backfill](#backfill). A TriggerRun is paused, resumed or killed through its `action` field. See [Set Up Triggers](../user-guides/ml-pipelines/set-up-triggers.md).

### Triton Inference Server
NVIDIA's model server, and the serving backend the open-source InferenceServer controller registers. Models packaged with `CustomTritonPackager` are served by Triton. See [Serving](../operator-guides/serving/index.md).

---

## U

### Uniflow
Michelangelo AI's Python orchestration framework. Provides the `@task` and `@workflow` decorators, handles caching, serialization, and distributed execution, and integrates with Ray and Spark compute backends.

---

## W

### Worker
The Cadence/Temporal worker process (`go/cmd/worker`) that executes transpiled Starlark workflows and the plugins they call to start Ray and Spark jobs, read and write models, and create deployments.

### Workflow
A Python function decorated with `@uniflow.workflow()` that orchestrates one or more tasks. Workflow functions control task sequencing, branching, loops, and concurrency. They compile to Starlark and run in Cadence/Temporal for durable, replayable execution.

---

## Concept mapping

If you are coming to Michelangelo AI from another ML platform, the table below maps familiar concepts to their Michelangelo AI equivalents. This is a rough translation — the implementations differ in important ways, so follow the links to read the full definitions.

| Concept | MLflow | Kubeflow | Ray | Airflow |
|---|---|---|---|---|
| Unit of computation | Step / Component | Pipeline Component | Remote function / Actor | Task / Operator |
| Workflow definition | `mlflow.projects` | KFP Pipeline | Ray Workflow | DAG |
| Experiment tracking | MLflow Experiment | Katib | Ray Tune | — |
| Model versioning | MLflow Model Registry | — | — | — |
| Online serving | MLflow Serving | KServe | Ray Serve | — |
| Dataset passing between steps | `mlflow.log_artifact` | PipelineChannel | ObjectRef | XCom |
| Scheduled runs | MLflow Projects (external scheduler) | Recurring Run | — | DAG schedule |

**In Michelangelo AI terms:**

| Other platform concept | Michelangelo AI equivalent |
|---|---|
| Experiment | Project |
| Run | PipelineRun |
| Registered model | Model + Model Registry |
| Model version | Model Revision |
| Serving endpoint | Endpoint |
| Pipeline component / step | Task |
| Pipeline | Workflow (code) + Pipeline (deployment config) |
| Dataset artifact | DatasetVariable / Ref |

---

## Related pages

- [Core Concepts and Key Terms](./core-concepts-and-key-terms.md) — deeper explanations with code examples
- [Getting Started with ML Pipelines](../user-guides/getting-started/getting-started.md) — build your first pipeline
- [Workflow Patterns](../user-guides/ml-pipelines/workflow-patterns.md) — sequencing, branching, loops, and DatasetVariable
- [CLI Reference](../user-guides/reference/cli.md) — pipeline and project management commands
