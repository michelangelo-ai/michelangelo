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

Kueue 0.15+ is recommended (its API defaults to `kueue.x-k8s.io/v1beta2`, which matches the control plane's default; see [Compatibility](#compatibility) for older releases). The default Kueue configuration already enables the RayCluster integration:

```bash
helm install kueue oci://registry.k8s.io/kueue/charts/kueue \
  --version 0.15.0 --namespace kueue-system --create-namespace
```

Create the cluster's quota and the per-project queues:

```yaml
apiVersion: kueue.x-k8s.io/v1beta2
kind: ResourceFlavor
metadata:
  name: default-flavor
spec: {}
---
apiVersion: kueue.x-k8s.io/v1beta2
kind: ClusterQueue
metadata:
  name: cluster-queue
spec:
  namespaceSelector: {}
  queueingStrategy: BestEffortFIFO
  resourceGroups:
  - coveredResources: ["cpu", "memory"]
    flavors:
    - name: default-flavor
      resources:
      - name: cpu
        nominalQuota: "64"
      - name: memory
        nominalQuota: 256Gi
---
apiVersion: kueue.x-k8s.io/v1beta2
kind: LocalQueue
metadata:
  name: ma-my-project      # localQueueTemplate applied to the project
  namespace: default       # where dispatched RayClusters land
spec:
  clusterQueue: cluster-queue
```

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

`ma sandbox demo kueue` installs Kueue on the sandbox's compute cluster (use `--compute-cluster-name` if you created a dedicated one), creates a demo ClusterQueue and the demo project's LocalQueue, and marks the Cluster CR as Kueue-managed. Submit a Ray task and watch it flow through `Queued` -> admitted; shrink the ClusterQueue quota below the job's request to see it wait.
