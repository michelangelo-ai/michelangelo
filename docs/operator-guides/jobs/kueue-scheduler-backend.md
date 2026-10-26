# Kueue Scheduler Backend

Michelangelo AI can delegate batch-job admission to [Kueue](https://kueue.sigs.k8s.io/), giving you quota-aware queueing and gang admission on your compute clusters. The backend is opt-in per compute cluster: clusters that don't opt in keep the default immediate-admission behavior, so mixed fleets work throughout a migration.

## How it works

The control plane deliberately does **not** create Kueue `Workload` objects itself. Instead:

1. A compute cluster opts in by setting `scheduler_type: SCHEDULER_TYPE_KUEUE` on its `Cluster` spec.
2. With `jobs.scheduler.backend: kueue`, the control plane validates at enqueue time that a job bound for a Kueue-managed cluster has an existing LocalQueue to land in. Jobs bound for other clusters pass through untouched.
3. When the job is dispatched, the k8s engine stamps exactly one label on the RayCluster it creates: `kueue.x-k8s.io/queue-name`, resolved by the control plane from the job's project. User-supplied labels never cross the cluster boundary, so the queue a job lands in can never be chosen by the job author.
4. Kueue's own [RayCluster integration](https://kueue.sigs.k8s.io/docs/tasks/run/rayclusters/) on the compute cluster suspends the labeled RayCluster, admits it against ClusterQueue quota when capacity is available, and gang-admits all of its pods together.

While a RayCluster waits for admission (KubeRay reports it `suspended`), the controller surfaces a `Queued` condition with reason `AwaitingAdmission`; when the cluster becomes ready, the condition flips to `False` with reason `ClusterAdmitted`.

## Queue topology

The intended topology is **one ClusterQueue per compute cluster, one LocalQueue per project**:

- Each compute cluster enforces its own quota independently (no MultiKueue).
- Each project gets a LocalQueue pointing at the cluster's ClusterQueue, named by the `localQueueTemplate` convention -- `ma-{project}` by default.
- The project identity is the `ma/project-name` label when your deployment stamps it, otherwise the job's namespace (Michelangelo AI jobs run in their project's namespace).

LocalQueues live in the namespace where the control plane dispatches RayClusters on the compute cluster (`default`). In this phase they are operator-managed: create them when you onboard a project, and the control plane validates their existence at dispatch time rather than creating them on demand. A job whose LocalQueue is missing fails visibly (`Enqueued=False`, reason `KueueQueueNotFound`) instead of being dispatched into an unadmittable state.

## Enabling the backend

### 1. Install Kueue on the compute cluster

Kueue comes with the [compute cluster charts](../setup/install-compute-cluster-charts.md). The `michelangelo-compute` chart installs Kueue 0.19.9 when `kueue.enabled` is `true`, and the `michelangelo-compute-queues` chart creates the cluster's quota and the per-project LocalQueues. The chart configures Kueue for this backend: it enables the RayCluster integration and confines Kueue to `default`, the namespace where dispatched RayClusters land. Rendering fails if one of those settings changes. The commands below use the variables from that guide's [Prerequisites](../setup/install-compute-cluster-charts.md#prerequisites).

Set `kueue.enabled: true` in the cluster's values file, then upgrade the release:

```bash
helm upgrade --install michelangelo-compute oci://ghcr.io/michelangelo-ai/michelangelo/charts/michelangelo-compute \
  --version "$VERSION" --kube-context "${COMPUTE_CONTEXT}" \
  --namespace michelangelo-compute --create-namespace \
  --values values-compute.yaml --wait
```

`--wait` returns once Kueue's CRDs are Established and its controller is ready. Kueue's ClusterQueue and ResourceFlavor webhooks fail closed, so create the queues only after this command has returned. With cert-manager (`kueue.enableCertManager`), the webhook certificate arrives later, and the next command may need a retry.

Describe the quota and the LocalQueues in `queues.yaml`:

```yaml
quota:                 # Required: the capacity Michelangelo may use on this cluster
  cpu: 64
  memory: 256Gi
  nvidia.com/gpu: 8
localQueues:           # One per project, named by localQueueTemplate
  - ma-my-project
```

```bash
helm upgrade --install michelangelo-compute-queues oci://ghcr.io/michelangelo-ai/michelangelo/charts/michelangelo-compute-queues \
  --version "$VERSION" --kube-context "${COMPUTE_CONTEXT}" \
  --namespace michelangelo-compute --values queues.yaml --wait
```

To onboard a project, append its LocalQueue to `localQueues` and run the second command again. The chart renders a ResourceFlavor, a ClusterQueue that only LocalQueues in `default` draw on (`BestEffortFIFO`, no cohort, borrowing or preemption), one LocalQueue in `default` per `localQueues` entry, and one WorkloadPriorityClass per `priorityClasses` entry.

| Key | Default | Description |
|-----|---------|-------------|
| `clusterQueueName` | `cluster-queue` | Name of the ClusterQueue that holds the quota |
| `flavorName` | `default-flavor` | Name of the single ResourceFlavor, which has no node labels or taints |
| `quota` | `{}` | Required. One scalar per resource, for example `cpu: 64`. Its keys are the ClusterQueue's covered resources |
| `localQueues` | `[]` | LocalQueue names, created in `default` |
| `priorityClasses` | `[]` | Optional WorkloadPriorityClasses, for example `{name: low, value: 100, description: Batch}` |

- `quota` takes scalars only. On the command line, escape the dots in a resource name, as in `--set quota.nvidia\.com/gpu=8`: unescaped, the nested map fails rendering.
- A workload that requests a resource the quota does not cover is never admitted, so a GPU job needs GPU quota. The exception is `ephemeral-storage`, which Ray pods request when a task sets a disk size: the chart's configuration leaves it out of quota.
- Manage each LocalQueue through `localQueues` or by hand, never both. Helm refuses to create a LocalQueue that already exists without the release's ownership metadata, so adopt it into `michelangelo-compute-queues` first (see [Migrating existing clusters](../setup/install-compute-cluster-charts.md#migrating-existing-clusters)). Helm deletes a LocalQueue once its name leaves the list.
- Kueue does not let a LocalQueue change its ClusterQueue, so changing `clusterQueueName` on an installed release fails on every LocalQueue. To rename it, while no workloads are queued, upgrade once with `localQueues: []`, then again with the new name and the list.

:::caution
Kueue's CRDs belong to the `michelangelo-compute` release. Uninstalling it, or setting `kueue.enabled` back to `false`, deletes them together with every Workload and queue. To remove Kueue, first [unmark the cluster](#2-mark-the-compute-cluster), wait until `kubectl get workloads -n default` returns nothing, and uninstall `michelangelo-compute-queues`. Only then turn Kueue off.
:::

A Kueue that already runs on the cluster, such as a `kueue` release in `kueue-system` from an earlier version of this guide, is kept or replaced:

- **Keep.** Leave `kueue.enabled` at `false`. The Kueue must serve `kueue.x-k8s.io/v1beta2` (0.15 or newer) and carry the chart's configuration. The queues chart works with it: adopt the queue objects it already has into `michelangelo-compute-queues`, or keep them by hand and leave them out of `localQueues` (see [Migrating existing clusters](../setup/install-compute-cluster-charts.md#migrating-existing-clusters)).
- **Replace.** Uninstalling the standalone release deletes its CRDs, and an object that keeps a Kueue finalizer once its controller is gone blocks that deletion. Drain the cluster and delete its queue objects first; the queues chart recreates them:

  1. [Unmark the cluster](#2-mark-the-compute-cluster), so no new workloads are labeled, and wait until `kubectl --context "${COMPUTE_CONTEXT}" get workloads --all-namespaces` returns nothing.
  2. Delete the queue objects while the old controller still runs, LocalQueues first:

     ```bash
     kubectl --context "${COMPUTE_CONTEXT}" delete localqueues --all --all-namespaces --wait
     kubectl --context "${COMPUTE_CONTEXT}" delete clusterqueues,resourceflavors --all --wait
     ```

  3. Uninstall the release, and wait until no `kueue.x-k8s.io` CRD is left:

     ```bash
     helm uninstall kueue --kube-context "${COMPUTE_CONTEXT}" --namespace kueue-system --wait
     kubectl --context "${COMPUTE_CONTEXT}" get crd | grep kueue.x-k8s.io
     ```

  4. Install Kueue and the queues as above, then mark the cluster again.

### 2. Mark the compute cluster

```yaml
apiVersion: michelangelo.api/v2
kind: Cluster
metadata:
  name: my-compute-cluster
  namespace: ma-system
spec:
  schedulerType: SCHEDULER_TYPE_KUEUE
  # ... existing kubernetes/rest connection settings
```

### 3. Enable the backend on the control plane

Helm:

```yaml
controllermgr:
  jobs:
    scheduler:
      backend: kueue
      # Optional:
      # kueue:
      #   localQueueTemplate: "ma-{project}"   # default
      #   localQueueOverrides:
      #     some-project: shared-queue
      #   apiVersion: v1beta2                  # default; v1beta1 for Kueue < 0.15
```

The controllermgr restarts automatically on the config change (checksum annotation). An unrecognized `backend` value fails at startup rather than silently running the default backend.

## Configuration reference

Config path `jobs.scheduler` (Helm: `controllermgr.jobs.scheduler`):

| Key | Default | Meaning |
|-----|---------|---------|
| `backend` | `default` | `default` for immediate admission; `kueue` to validate and label jobs bound for Kueue-managed clusters. |
| `kueue.localQueueTemplate` | `ma-{project}` | LocalQueue naming convention; `{project}` is replaced with the job's project. |
| `kueue.localQueueOverrides` | `{}` | Per-project explicit LocalQueue names; an override wins over the template. |
| `kueue.apiVersion` | `v1beta2` | `kueue.x-k8s.io` API version used for the LocalQueue existence check on compute clusters. |

## Observability

- **Waiting for quota:** the RayCluster carries `Queued=True` (reason `AwaitingAdmission`) while suspended, flipping to `False`/`ClusterAdmitted` on admission. A queued job is therefore distinguishable from one stuck launching.
- **Misconfiguration:** a missing LocalQueue sets `Enqueued=False` with reason `KueueQueueNotFound` and an actionable message; an unreachable compute cluster during validation sets `KueueQueueCheckFailed` and the job is retried.
- **Metrics:** the `kueuescheduler` controller scope emits `kueue.queue_not_found_count` and `kueue.queue_check_failed_count`.
- On the compute cluster, Kueue's own metrics and `kubectl get workloads`/`localqueues` show admission state and quota usage.

## Compatibility

- **Kueue version:** the control plane only performs an existence `GET` against `kueue.x-k8s.io`, so there is no client-library coupling. The default `apiVersion: v1beta2` requires Kueue 0.15+; set `kueue.apiVersion: v1beta1` to run against older Kueue releases.
- **RayJobs:** Michelangelo AI RayJobs target an existing RayCluster via `ClusterSelector`, which Kueue's RayJob integration deliberately skips -- admission is enforced on the RayCluster, which is the resource-owning object. RayJobs never get a queue label.
- **`waitForPodsReady`:** avoid enabling Kueue's `waitForPodsReady` for RayClusters admitted through this backend. A RayCluster's pods can legitimately start over a window (e.g. autoscaling workers), and `waitForPodsReady` can evict and requeue the whole cluster on that window.

## Rollback

Each step is independently reversible:

- Set `jobs.scheduler.backend: default` (or remove it) to stop validating and labeling; already-dispatched suspended clusters still get admitted by Kueue on the compute cluster.
- Remove `scheduler_type` from a `Cluster` spec to return that cluster to immediate admission while the backend stays on for others.
- Uninstalling Kueue from a compute cluster leaves any still-suspended RayClusters suspended: terminate them (or clear `spec.suspend` on the KubeRay object) before removing Kueue.

## Try it in the sandbox

```bash
ma sandbox create
ma sandbox demo kueue
ma sandbox sync --set controllermgr.jobs.scheduler.backend=kueue
```

`ma sandbox demo kueue` sets `kueue.enabled: true` on the cluster's `michelangelo-compute` release, installs `michelangelo-compute-queues` with the demo quota (8 CPU, 16Gi) and the demo project's LocalQueue `ma-ma-dev-test`, and marks the Cluster CR as Kueue-managed. It targets the sandbox cluster itself; pass `--compute-cluster-name michelangelo-compute-0` if you created a dedicated compute cluster. Operators the sandbox was created without stay disabled, and later `ma sandbox sync` runs keep Kueue enabled.

On a sandbox where an earlier version of the demo installed a standalone `kueue` release, the demo replaces it first, as in [step 1](#1-install-kueue-on-the-compute-cluster): it unmarks the cluster, waits until no Kueue workloads remain, deletes the demo queue objects, uninstalls the release and waits until Kueue's CRDs are gone. Each wait gives up after a few minutes with an error that says what to check before you run the demo again.

Submit a Ray task and watch it flow through `Queued` -> admitted. To see a job wait, shrink the quota below its request, from the repository root and with the context of the cluster you passed, then set `quota.cpu=8` again:

```bash
helm upgrade michelangelo-compute-queues helm/michelangelo-compute-queues \
  --kube-context k3d-michelangelo-sandbox --namespace michelangelo-compute \
  --reuse-values --set quota.cpu=1
```
