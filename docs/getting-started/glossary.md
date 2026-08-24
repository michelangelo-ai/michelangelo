---
sidebar_position: 4
---

# MLOps Glossary

A concise reference for terms used throughout Michelangelo AI documentation. For a deeper look at how these concepts relate to each other, see [Core Concepts and Key Terms](./core-concepts-and-key-terms.md).

---

## A

### Artifact Store
Persistent storage for ML artifacts (trained models, evaluation reports, datasets). Michelangelo AI uses S3 or MinIO as the default artifact backend.

### Artifact
Any file or object produced or consumed during a pipeline run — model weights, serialized datasets, evaluation reports, or configuration files.

---

## C

### CanvasFlex
A code-driven YAML workflow system for teams that want opinionated structure, version control, and software engineering best practices applied to their ML pipelines. An alternative to building pipelines entirely in Python with Uniflow.

### Caching
Uniflow automatically caches task outputs based on task inputs and code. If a task is re-run with the same inputs, the cached result is returned instead of recomputing. Caching can be disabled per-task.

### Cadence / Temporal
The workflow orchestration engine that powers Uniflow's durable execution model. Workflows compile to Starlark and run in a Cadence/Temporal worker, providing retries, replay, and long-running execution out of the box.

### Checkpoint
An intermediate dataset saved to storage during a pipeline run. Enables fault tolerance and task output reuse across executions.

### Compute Cluster
A pool of hardware resources (CPU, GPU, memory) registered with the Michelangelo AI control plane. Tasks are dispatched to compute clusters via the Ray Operator, Spark Operator, or Kubernetes Batch.

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
The host process that serves online predictions. Michelangelo AI supports Triton Inference Server (for traditional models) and vLLM / SGLang (for LLM serving). Managed by the InferenceServer Controller on the control plane.

### Incremental Training
A training pattern where a new model revision is trained starting from the weights of a prior revision, rather than from scratch. Each iteration produces a new revision (revision 0 → revision 1 → …).

---

## J

### Job
A batch workload running on a compute cluster. Michelangelo AI runs Spark jobs for data processing and Ray jobs for ML training and inference.

---

## M

### MA Studio
The no-code web UI for Michelangelo AI. Provides a visual interface for data preparation, model training (XGBoost, classic ML, deep learning), evaluation, deployment, and monitoring — without writing Python.

### Model
The output artifact of a training job: a serialized set of parameters that a framework can use to make predictions.

### Model Excellence Scores (MES)
Automated quality metrics tracked across the model lifecycle — feature quality, prediction performance, model freshness, and more. MES surfaces model health without requiring manual metric instrumentation.

### Model Family
A group of related models within a project that each address a different sub-problem of the same use case. For example, a ride-pricing project might have separate model families for demand prediction, surge multiplier, and route optimization.

### Model Registry
The versioned catalog of trained models in Michelangelo AI. Each registered model has a name, one or more revisions, associated metadata, and evaluation reports.

### Model Revision
A specific version of a trained model. Revisions are created automatically each time a training job completes. The first revision is revision 0; incremental training increments the revision number.

---

## P

### Pipeline
A registered, versioned deployment configuration for a workflow. Pipelines separate _what_ the workflow does (code) from _how_ it is deployed (schedule, resource config, environment). Managed via the `ma` CLI or MA Studio.

### PipelineRun
A single execution instance of a pipeline. Each run has a unique ID, tracks task statuses, stores outputs, and can be inspected via MA Studio or the CLI.

### Project
The top-level organizational unit in Michelangelo AI — a business use case with a set of continuously trackable ML metrics. A project contains one or more model families, their datasets, workflows, and deployments.

---

## R

### Ref
A lightweight remote reference to a large object (dataset, model file) stored in a backend like S3 or HDFS. Passing a `Ref` between tasks avoids copying large data into memory — Uniflow resolves it on demand.

---

## T

### Task
A Python function decorated with `@uniflow.task()` that performs a single unit of computation. Tasks run in isolated containers on Kubernetes (via Ray or Spark), are independently cacheable and retryable, and are the building blocks of workflows.

### Task Result
The serialized output of a completed task, stored by Uniflow for caching, debugging, and reuse in downstream tasks.

---

## U

### Uniflow
Michelangelo AI's Python orchestration framework. Provides the `@task` and `@workflow` decorators, handles caching, serialization, and distributed execution, and integrates with Ray and Spark compute backends.

---

## W

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
