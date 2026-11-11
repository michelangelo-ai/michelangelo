---
sidebar_position: 3
sidebar_label: "From SageMaker or Vertex AI"
---

# Migrating from SageMaker or Vertex AI

This guide is for teams running training and serving on AWS SageMaker or Google Cloud Vertex AI who are evaluating a move to Michelangelo AI — typically to escape per-instance managed-service pricing, to run the same ML platform across clouds, or to own the stack on their own Kubernetes.

The honest framing up front: SageMaker and Vertex are broad managed products, and this platform does not replicate all of them. Training, pipelines, a model registry, and online serving map well. Batch inference, feature stores, hyperparameter tuning, and endpoint traffic splitting do not map today — each is called out below rather than papered over.

:::note
For concept-level mapping across MLflow, Kubeflow, Ray, and Airflow, see the [glossary's concept mapping](../../getting-started/glossary.md#concept-mapping). For a broader tool-by-tool view, see [ML Workflow Mapping](../../getting-started/overview.md#ml-workflow-mapping) in the overview.
:::

## Concept mapping

| Concept | SageMaker | Vertex AI | Michelangelo AI |
|---|---|---|---|
| **Training job** | Estimator | `CustomJob` | A Uniflow task with a `RayTask` config; distributed PyTorch via the bundled Lightning trainer |
| **Pipeline** | SageMaker Pipelines | Vertex Pipelines | `Pipeline` and `PipelineRun` resources over Uniflow workflows |
| **Model registry** | Model Registry | Model Registry | `Model` resources with versioned `Revision`s, addressed as `models:/{namespace}/{name}/{version}` |
| **Online endpoint** | Endpoint | Endpoint | `InferenceServer` plus `Deployment`, applied as YAML through the `ma` CLI |
| **Batch inference** | Batch Transform | Batch Prediction | No dedicated resource — a hand-written pipeline task (see gaps) |
| **Experiment tracking** | SageMaker Experiments | Vertex Experiments | Bring your own — the platform integrates with an external MLflow rather than bundling a tracker |
| **Feature store** | Feature Store | Feature Store | Not available (on the roadmap) |
| **Hyperparameter tuning** | Automatic Model Tuning | Vizier | Not available — sweeps are written by hand as pipeline fan-out |
| **Artifact storage** | S3 | GCS | Any S3-compatible store via MinIO; native GCS is not supported yet (see gaps) |
| **Model grouping** | Model Package Group | — | `ModelFamily`, which groups related models inside a project (platform-specific; no direct cloud equivalent) |

## Your training job, before and after

A typical SageMaker training-plus-registration flow:

```python
from sagemaker.pytorch import PyTorch

estimator = PyTorch(
    entry_point="train.py",
    role="arn:aws:iam::123456789012:role/SageMakerExecutionRole",
    instance_type="ml.p3.2xlarge",
    instance_count=2,
    framework_version="2.1",
)
estimator.fit({"train": "s3://bucket/train"})
model = estimator.create_model()
```

The equivalent here is a pipeline task that declares its resources instead of naming an instance type, and registers the result explicitly:

```python
import michelangelo.uniflow.core as uniflow
from michelangelo.lib.model_manager.registry.api_client import APIRegistryClient
from michelangelo.uniflow.plugins.ray import RayTask


@uniflow.task(
    config=RayTask(
        head_cpu=2,
        head_memory="4Gi",
        worker_cpu=8,
        worker_memory="32Gi",
        worker_gpu=1,
        worker_instances=2,
    )
)
def train(data_path: str):
    model_path = train_my_model(data_path)  # your training code
    # Assumes MA_API_SERVER points at your control plane, or pass an
    # existing client via svc=.
    registry = APIRegistryClient(namespace="my-project")
    return registry.register_model(
        name="my-model",
        artifact_uri=model_path,
        kind="regression",
    )


@uniflow.workflow()
def training_pipeline(data_path: str):
    return train(data_path)
```

Two differences worth internalizing:

- **Resources replace instance types.** There is no `ml.p3.2xlarge` catalog; you ask for CPUs, memory, and GPUs, and the platform places the workload on a registered compute cluster.
- **Registry URIs are three-segment.** A registered model is addressed as `models:/{namespace}/{name}/{version}` — the namespace segment is deliberate, and distinguishes these URIs from both MLflow's two-segment form and the clouds' ARN/resource-name schemes.

## Deployment is a separate, explicit step

There is no `estimator.deploy()` one-liner. Serving is its own pair of resources — an `InferenceServer` (the model server) and a `Deployment` (the rollout onto it) — written as YAML and applied through the `ma` CLI. The full flow, including manifests, is in [Deploy a Model](../train-and-deploy-models/deploy-a-model.md).

If you rely on SageMaker's or Vertex's endpoint conveniences, note the current rollout surface honestly: deployments roll out with a rolling strategy. Canary traffic splitting, shadow routing, and A/B rollouts are on the [roadmap](../../getting-started/roadmap.md) but are not implemented today.

## What does not map yet

These are real gaps, not omissions from this page.

- **No batch inference primitive.** There is no Batch Transform or Batch Prediction equivalent — no batch endpoint resource exists. The working pattern is a pipeline task that loads the model and iterates over your dataset with Ray, which the LLM prediction examples demonstrate. This gap is also not on the roadmap, so treat it as open rather than scheduled.
- **No feature store.** The entire feature-store surface — feature groups, online/offline stores, drift monitoring — is on the roadmap as planned, not shipped. A SageMaker Feature Store or Vertex Feature Store dependency has to be replaced with your own storage for now.
- **No hyperparameter tuning service.** Nothing equivalent to Automatic Model Tuning or Vizier exists, and it is not on the roadmap. Sweeps are hand-written pipeline fan-out — see [Workflow Patterns](../ml-pipelines/workflow-patterns.md).
- **No native GCS backend.** Blob storage supports Azure and any S3-compatible store through MinIO. Vertex users whose artifacts live in GCS currently need MinIO's S3-interoperability mode or a data move; native GCS support is on the roadmap.
- **No endpoint traffic splitting.** Covered above, but it bears repeating in gap form: rolling is the only rollout strategy implemented today.
- **No managed notebooks, no AutoML, no model monitoring dashboards.** None of the studio-adjacent managed conveniences have equivalents here; the platform's Studio UI covers pipeline authoring and tracking, with its deploy-and-predict phase still on the roadmap.

## What's next

- [Running Uniflow Pipelines](../ml-pipelines/running-uniflow.md) — the task and workflow authoring model
- [Deploy a Model](../train-and-deploy-models/deploy-a-model.md) — the full serving flow the deploy step above summarizes
- [Model Registry](../../operator-guides/components/model-registry.md) — how registration and revisions work under the hood
- [MLflow integration](../../operator-guides/integrations/mlflow.md) — the bring-your-own experiment-tracking story
- [Roadmap](../../getting-started/roadmap.md) — what is shipped and what is planned
