---
last_verified: 2026-11-05
---

# Michelangelo AI User Guides

Build, train, and deploy machine learning models at scale using Michelangelo AI's unified ML platform.

## Getting Started

New to Michelangelo AI? Follow this path:

1. **[Getting Started with Pipelines](./getting-started/getting-started.md)** — Build your first pipeline in 30 minutes
2. **[Prepare Your Data](./getting-started/prepare-your-data.md)** — Load, clean, and split datasets using Ray and Spark
3. **[Train & Deploy Models](./train-and-deploy-models/train-and-register-a-model.md)** — Train locally or at scale, then register and serve

---

## Quick Navigation

### ML Pipelines

| Guide | Description |
|-------|-------------|
| [ML Pipelines Overview](./ml-pipelines/index.md) | Tasks, workflows, pipelines, and the Uniflow framework |
| [Running Uniflow Pipelines](./ml-pipelines/running-uniflow.md) | Run pipelines locally and remotely |
| [Pipeline Running Modes](./ml-pipelines/pipeline-running-modes.md) | Local Run, Remote Run, Pipeline Dev Run, and Pipeline Run |
| [Workflow Patterns](./ml-pipelines/workflow-patterns.md) | Sequencing, branching, loops, parallelism, and sharing data between tasks |
| [Caching and Pipeline Run Resume](./ml-pipelines/cache-and-pipelinerun-resume-form.md) | Skip re-execution with task-level caching, and resume a run from a specific step |
| [File Sync](./ml-pipelines/file-sync-testing-flow-runbook.md) | Test local changes on remote infrastructure without rebuilding images |
| [Project Management](./getting-started/project-management-for-ml-pipelines.md) | Organize pipelines, models, and resources into projects |
| [Pipeline Management](./ml-pipelines/pipeline-management.md) | Create, update, and manage pipelines |
| [Set Up Triggers](./ml-pipelines/set-up-triggers.md) | Schedule pipelines on a cron schedule |
| [Backfill Pipelines](./ml-pipelines/backfill.md) | Reprocess historical data windows |
| [Pipeline Notifications](./ml-pipelines/notifications.md) | Email and Slack alerts on pipeline run outcomes |

### Train & Deploy Models

| Guide | Description |
|-------|-------------|
| [Train and Register a Model](./train-and-deploy-models/train-and-register-a-model.md) | Train at scale and register artifacts |
| [Distributed Training](./train-and-deploy-models/distributed-training.md) | Scale `LightningTrainer` across multiple workers or GPUs |
| [Model Registry Guide](./train-and-deploy-models/model-registry-guide.md) | Version, track, and manage trained models |
| [Push Models and Artifacts](./train-and-deploy-models/push-models-and-artifacts.md) | Upload packaged models, datasets, and evaluation reports with `push()` |
| [Deploy a Model](./train-and-deploy-models/deploy-a-model.md) | Bind a registered model to an inference server |
| [Native Feature Transforms](./train-and-deploy-models/native-feature-transforms.md) | Fit and materialize transform layers that run identically at train and serve time |

### Migration

| Guide | Description |
|-------|-------------|
| [Migrating from KubeRay](./migration/migrate-from-kuberay.md) | What changes when a team running Ray with the KubeRay operator adopts Michelangelo AI |

### Reference

| Guide | Description |
|-------|-------------|
| [Type System](./reference/type-system.md) | Uniflow data types and serialization |
| [Data Passing and References](./reference/reference-system.md) | How data flows between pipeline tasks |
| [CLI Reference](./reference/cli.md) | Command-line tools for pipeline and project management |

---

## Examples

Browse the full **[Examples Gallery](./examples/index.md)** — 12 end-to-end workflows covering training, recommendation systems, inference, workflow orchestration, evaluation, and model packaging, organized by use case with difficulty levels.
