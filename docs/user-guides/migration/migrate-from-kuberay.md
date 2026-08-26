---
sidebar_position: 1
sidebar_label: "From KubeRay"
---

# Migrating from KubeRay

This guide is for teams already running Ray on Kubernetes with the KubeRay operator, who are evaluating what changes if they adopt Michelangelo AI.

The short answer is that fewer things change than you might expect. Michelangelo AI does not replace KubeRay — it runs on top of it. The control plane accepts its own `RayCluster` and `RayJob` resources, then creates real `ray.io/v1` resources on a registered compute cluster for the KubeRay operator to reconcile. Your operator, your Ray version, and your cluster topology all survive the move.

:::note
This page maps **resources and manifests**. For concept-level mapping across MLflow, Kubeflow, Ray, and Airflow, see the [glossary's concept mapping](../../getting-started/glossary.md#concept-mapping). For a broader tool-by-tool view, see [ML Workflow Mapping](../../getting-started/overview.md#ml-workflow-mapping) in the overview.
:::

## What does not change

- **You still install and run KubeRay yourself.** The chart's README lists KubeRay as an optional cluster operator to install separately from its upstream chart, required only if your pipelines use Ray tasks. It is not a chart dependency — the local sandbox installs it as its own Helm release into a `ray-system` namespace:

  ```bash
  helm repo add kuberay https://ray-project.github.io/kuberay-helm
  helm install kuberay-operator kuberay/kuberay-operator --namespace ray-system --create-namespace
  ```
- **The resources KubeRay reconciles are still `ray.io/v1`.** Michelangelo AI builds them using the upstream KubeRay Go types, so what lands in your compute cluster is an ordinary `RayCluster` or `RayJob`.
- **Ray itself is unchanged.** Ray Data and Ray Train are used directly — `ray.data.Dataset` is a first-class type in the pipeline IO system, and the bundled Lightning trainer subclasses Ray Train's `TorchTrainer`.

:::warning
There is no documented minimum KubeRay version. The control plane currently compiles against the KubeRay operator's Go types at `v1.2.2`, while the local sandbox installs operator `v1.4.2`. Both are in the `ray.io/v1` API, so an existing install in that range is very likely fine, but no version floor is stated anywhere, so confirm against your own operator version before migrating anything you care about.
:::

## What changes

You stop applying `ray.io/v1` manifests to a cluster directly. Instead you apply a Michelangelo AI resource to the control plane, and it decides which registered compute cluster the workload lands on.

That indirection is the point: it is what enables federated dispatch across multiple compute clusters, and what lets a Ray cluster become one step inside a larger pipeline rather than a standalone object you manage by hand.

## Concept mapping

| Concept | KubeRay | Michelangelo AI |
|---|---|---|
| **Cluster resource** | `RayCluster` (`ray.io/v1`) | `RayCluster` (`michelangelo.api/v2`), translated into a `ray.io/v1` `RayCluster` |
| **Job resource** | `RayJob` (`ray.io/v1`) | `RayJob` (`michelangelo.api/v2`), translated into a `ray.io/v1` `RayJob` |
| **Head node** | `spec.headGroupSpec` | `spec.head` |
| **Worker groups** | `spec.workerGroupSpecs[]` | `spec.workers[]` |
| **Worker group name** | `groupName` | `nodeType`, though the group name on the generated resource is derived from the cluster name rather than from this field |
| **Worker count** | `replicas`, `minReplicas`, `maxReplicas` | `minInstances`, `maxInstances` |
| **Ray start params** | `rayStartParams` | `rayStartParams`, on both head and workers |
| **Ray version** | `spec.rayVersion` | `spec.rayVersion` |
| **Pod customization** | `template` (a `PodTemplateSpec`) | `pod` (a `PodTemplateSpec`), on both head and workers |
| **Which cluster a job runs on** | `clusterSelector` or an inline `rayClusterSpec` | `spec.cluster`, a name and namespace reference |
| **Applying a manifest** | `kubectl apply -f` | `ma ray_cluster apply --file` |
| **Where the workload lands** | The cluster your `kubectl` context points at | A registered compute cluster, chosen by the control plane |
| **Serving** | `RayService` | No equivalent. Online serving is a separate Triton-based stack |

## Your RayCluster, before and after

A minimal KubeRay cluster:

```yaml
apiVersion: ray.io/v1
kind: RayCluster
metadata:
  name: training-cluster
  namespace: my-project
spec:
  rayVersion: "2.9.0"
  headGroupSpec:
    rayStartParams:
      dashboard-host: "0.0.0.0"
    template:
      spec:
        containers:
          - name: ray-head
            image: rayproject/ray:2.9.0
  workerGroupSpecs:
    - groupName: default-worker
      replicas: 1
      minReplicas: 1
      maxReplicas: 2
      template:
        spec:
          containers:
            - name: ray-worker
              image: rayproject/ray:2.9.0
```

The same cluster as a Michelangelo AI resource:

```yaml
apiVersion: michelangelo.api/v2
kind: RayCluster
metadata:
  name: training-cluster
  namespace: my-project
spec:
  user:
    name: your-username
  rayVersion: "2.9.0"
  head:
    rayStartParams:
      dashboard-host: "0.0.0.0"
  workers:
    - nodeType: default-worker
      minInstances: 1
      maxInstances: 2
```

Apply it through the CLI rather than `kubectl`:

```bash
ma ray_cluster apply --file="./training-cluster.yaml"
```

The pod templates are gone from the example because they are optional here, not because they are unsupported — `head.pod` and `workers[].pod` both take a full `PodTemplateSpec` for anything the shorthand fields do not cover, including images, node selectors, and tolerations.

## Your RayJob, before and after

A KubeRay job against an existing cluster:

```yaml
apiVersion: ray.io/v1
kind: RayJob
metadata:
  name: train
  namespace: my-project
spec:
  entrypoint: "python train.py"
  clusterSelector:
    ray.io/cluster: training-cluster
```

The same job here:

```yaml
apiVersion: michelangelo.api/v2
kind: RayJob
metadata:
  name: train
  namespace: my-project
spec:
  user:
    name: your-username
  entrypoint: "python train.py"
  cluster:
    name: training-cluster
    namespace: my-project
```

```bash
ma ray_job apply --file="./train.yaml"
```

:::info
`ray_cluster` and `ray_job` support `get`, `apply`, and `delete`. There is no `run` subcommand for them — applying the `RayJob` is what starts it. See the [CLI reference](../reference/cli.md) for the full resource table.

The `namespace` in these manifests is a project. Projects are not created implicitly, so create yours before applying anything into it — see [Project Management](../getting-started/project-management-for-ml-pipelines.md).
:::

## The other path: let a task own the cluster

If you are willing to change how jobs are authored, you can skip cluster manifests entirely. A pipeline task declares the resources it needs, and the platform creates a cluster before the task runs and tears it down afterwards:

```python
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask

@uniflow.task(
    config=RayTask(
        head_cpu=2,
        head_memory="4Gi",
        worker_cpu=2,
        worker_memory="4Gi",
        worker_instances=2,
    )
)
def train(data_path: str):
    return train_my_model(data_path)
```

This is the closest thing to a KubeRay `RayJob` with an inline `rayClusterSpec`: the cluster is created for one job and does not outlive it. The trade-off is a smaller configuration surface than the CRD path. `RayTask` accepts `head_cpu`, `head_memory`, `head_disk`, `head_gpu`, `head_object_store_memory`, the same five fields for workers, plus `worker_instances`, `breakpoint`, and `runtime_env`.

Two differences worth knowing before you rely on it:

- **The cluster is fixed size.** `worker_instances` sets the minimum and maximum to the same value, so there is no range to scale within.
- **The Ray version is not configurable on this path.** Clusters created by a task use a version hardcoded in the plugin, currently 2.3.1, which is considerably older than what most KubeRay users will be running. If you need a specific Ray version, use the `RayCluster` resource, where `rayVersion` is honored.

For the full task authoring model, see [Running Uniflow Pipelines](../ml-pipelines/running-uniflow.md).

## What does not map yet

These are real gaps, not omissions from this page.

- **No autoscaling.** `minInstances` and `maxInstances` do reach the underlying KubeRay resource as `minReplicas` and `maxReplicas`, but the Ray in-tree autoscaler is never enabled, so a cluster runs at `minInstances` and stays there. Treat the range as a declaration rather than as elasticity.
- **No `RayService` equivalent.** Online serving does not go through Ray. It is a separate stack reached through `InferenceServer` and `Deployment` resources, with pluggable backends — Triton Inference Server for traditional models, vLLM and SGLang for LLM serving. A Ray Serve deployment has to be rebuilt rather than translated.
- **No Ray Tune integration.** There is no hyperparameter tuning plugin, and nothing in the codebase imports Ray Tune. Sweeps are written by hand as pipeline fan-out — see [Workflow Patterns](../ml-pipelines/workflow-patterns.md) for the shape of that.
- **No managed Ray Dashboard.** Reaching the dashboard means finding the service and port-forwarding to it yourself.
- **Effectively one worker group.** The `RayCluster` resource takes a list of worker groups, and a `RayTask` always produces exactly one. Multiple heterogeneous groups are not something this page can recommend yet, since every generated group takes its name from the cluster rather than from its own `nodeType`.

Ray job launch, persistent Ray clusters via the `RayCluster` resource, and federated multi-cluster dispatch are all listed as available on the [roadmap](../../getting-started/roadmap.md). The gaps above are not currently on it, so the honest answer is that they are open rather than scheduled.

## What's next

- [Register a compute cluster](../../operator-guides/setup/register-a-compute-cluster-to-michelangelo-control-plane.md) — required before any Ray workload can be dispatched, and the step most likely to surprise a KubeRay user
- [Running Uniflow Pipelines](../ml-pipelines/running-uniflow.md) — local and remote execution, and how to reach a running cluster for debugging
- [CLI Reference](../reference/cli.md) — the full set of resources and operations
- [Roadmap](../../getting-started/roadmap.md) — what is shipped and what is planned
