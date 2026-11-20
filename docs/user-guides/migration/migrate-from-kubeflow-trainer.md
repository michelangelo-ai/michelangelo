---
sidebar_position: 2
sidebar_label: "From Kubeflow Trainer"
---

# Migrating from Kubeflow Trainer

This guide is for teams running distributed training through Kubeflow Trainer — `PyTorchJob`, `TFJob`, or the v2 `TrainJob` — who are evaluating what changes if they adopt Michelangelo AI.

Unlike the [KubeRay migration](./migrate-from-kuberay.md), this one is a replacement rather than a re-layering. Michelangelo AI has no training-job CRD: there is no resource where you declare per-role replica specs and hand the operator a container to run. Training here is a Python function inside a pipeline, and distribution comes from Ray Train rather than from a framework-specific operator. That is a real authoring change, and this page tries to be precise about what you gain and what you give up.

:::note
This page maps **training jobs**. Kubeflow Pipelines, Katib, and KServe are a different surface — for those, see the [glossary's concept mapping](../../getting-started/glossary.md#concept-mapping) and [ML Workflow Mapping](../../getting-started/overview.md#ml-workflow-mapping) in the overview.
:::

## What changes

A `PyTorchJob` bundles three decisions into one manifest: what code runs, how many replicas of each role run it, and what pod each replica gets. Here those decisions move:

- **What runs** becomes a Uniflow task — a Python function in a pipeline.
- **How it distributes** becomes a Ray Train `ScalingConfig` passed to the bundled Lightning trainer, which subclasses Ray Train's `TorchTrainer`.
- **What it runs on** becomes the task's `RayTask` resource declaration; the platform creates a Ray cluster for the task and tears it down afterwards.

There is no `Master`/`Worker` role split anywhere in that. Ray Train coordinates workers internally, and rank 0 is a runtime concept rather than a differently-shaped Kubernetes object.

## Concept mapping

| Concept | Kubeflow Trainer | Michelangelo AI |
|---|---|---|
| **Training job resource** | `PyTorchJob` / `TFJob` / `TrainJob` (CRDs) | None — a Uniflow task with a `RayTask` config |
| **Distributed PyTorch** | `PyTorchJob` replica specs | `LightningTrainer`, a Ray Train `TorchTrainer` subclass |
| **Distributed XGBoost** | `XGBoostJob` | `XGBoostTrainer`, a Ray Train `XGBoostTrainer` subclass |
| **Distributed TensorFlow** | `TFJob` | Not available — there is no TensorFlow trainer |
| **Replica count** | `replicas` per role | `ScalingConfig(num_workers=...)` plus `RayTask(worker_instances=...)` |
| **Elastic policy** | `elasticPolicy` (min/max replicas) | Not available — clusters are fixed size |
| **Per-role pod templates** | `template` under each replica spec | One resource shape for all workers on a task's cluster |
| **GPU request** | `resources.limits` in the pod template | `RayTask(worker_gpu=...)` |
| **Warm start / fine-tuning from a checkpoint** | Hand-rolled | `IncrementalTrainingSpec` / `TransferLearningSpec` on the Lightning trainer |
| **Job submission** | `kubectl apply -f` | Run the pipeline — see [Running Uniflow Pipelines](../ml-pipelines/running-uniflow.md) |
| **Where it runs** | The cluster your `kubectl` context points at | A registered compute cluster, chosen by the control plane |

## Your PyTorchJob, before and after

A minimal distributed PyTorch job under Kubeflow Trainer:

```yaml
apiVersion: kubeflow.org/v1
kind: PyTorchJob
metadata:
  name: pytorch-dist-train
spec:
  pytorchReplicaSpecs:
    Master:
      replicas: 1
      template:
        spec:
          containers:
            - name: pytorch
              image: my-training-image:latest
              command: ["python", "train.py"]
    Worker:
      replicas: 3
      template:
        spec:
          containers:
            - name: pytorch
              image: my-training-image:latest
              command: ["python", "train.py"]
```

The equivalent here is a pipeline task. The `RayTask` config declares the cluster the task gets, and the trainer's `ScalingConfig` declares how Ray Train spreads work across it:

```python
import michelangelo.uniflow.core as uniflow
import ray.train
from michelangelo.lib.trainer.torch.pytorch_lightning import (
    LightningTrainer,
    LightningTrainerParam,
)
from michelangelo.uniflow.plugins.ray import RayTask


@uniflow.task(
    config=RayTask(
        head_cpu=2,
        head_memory="4Gi",
        worker_cpu=4,
        worker_memory="16Gi",
        worker_gpu=1,
        worker_instances=3,
    )
)
def train(data_path: str):
    trainer = LightningTrainer(
        trainer_param=LightningTrainerParam(
            create_model_fn=my_model_factory,
            create_model_fn_kwargs={},
            train_data=load_train(data_path),
            val_data=load_val(data_path),
            batch_size=256,
        ),
        run_config=ray.train.RunConfig(name="pytorch-dist-train"),
        scaling_config=ray.train.ScalingConfig(num_workers=3, use_gpu=True),
    )
    return trainer.train()


@uniflow.workflow()
def training_pipeline(data_path: str):
    return train(data_path)
```

The `scaling_config` and `run_config` are Ray Train's own `ScalingConfig` and `RunConfig`, constructed directly — there is no wrapper type between you and Ray Train. Note that the worker count appears twice by design: `worker_instances` sizes the Ray cluster, `num_workers` sizes the training group running on it.

## What you gain over a PyTorchJob

- **The job is a pipeline step.** Data preparation, training, evaluation, and registration compose in one workflow instead of a training CRD stitched to everything else by external orchestration.
- **Warm start is built in.** The Lightning trainer accepts `IncrementalTrainingSpec` (resume from a prior checkpoint) and `TransferLearningSpec` (initialize from another model's weights) — the kind of logic Kubeflow Trainer users usually carry in their own training scripts.
- **The cluster is ephemeral by default.** A task's Ray cluster exists for the task and is torn down after it, so there is no standing training infrastructure to keep patched.

## What does not map yet

These are real gaps, not omissions from this page.

- **No TensorFlow trainer.** The trainer library ships PyTorch Lightning and XGBoost trainers; a `TFJob` workload has no drop-in path and would have to be rewritten against Ray Train's TensorFlow support by hand.
- **No elastic training.** A Kubeflow `elasticPolicy` has no equivalent on either path here. A task-created cluster sets its minimum and maximum worker count to the same value, and even on the standing `RayCluster` resource the generated KubeRay resource is pinned to its minimum, with the Ray autoscaler never enabled. Both paths run at a fixed size.
- **No gang scheduling.** Workers are scheduled individually — the plugin compensates by giving workers extra time to connect rather than by co-scheduling pods. `PyTorchJob` has the same property without a scheduler plugin, but if you run Kueue or Volcano with Kubeflow today, be aware there is no hook for that here yet.
- **The Ray version on the task path is fixed.** Clusters created by a `RayTask` use a Ray version hardcoded in the plugin, currently 2.3.1. If your training image needs a specific, newer Ray, use the `RayCluster` resource path described in the [KubeRay guide](./migrate-from-kuberay.md), where `rayVersion` is honored.
- **One homogeneous worker group per cluster.** There is no equivalent of heterogeneous replica specs (parameter servers, differently-sized roles) on a single training job.

None of these are currently on the [roadmap](../../getting-started/roadmap.md) — its distributed-training section lists Ray job launch, persistent Ray clusters, and federated dispatch as available, and does not schedule a training-job CRD, elasticity, or gang scheduling. The honest answer is that they are open rather than planned.

## What's next

- [Running Uniflow Pipelines](../ml-pipelines/running-uniflow.md) — how tasks and workflows execute, locally and remotely
- [Migrating from KubeRay](./migrate-from-kuberay.md) — the standing-cluster path, if you want infrastructure that outlives a single job
- [Register a compute cluster](../../operator-guides/setup/register-a-compute-cluster-to-michelangelo-control-plane.md) — required before any training workload can be dispatched
- [Roadmap](../../getting-started/roadmap.md) — what is shipped and what is planned
