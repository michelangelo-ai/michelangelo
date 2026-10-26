# Kueue Scheduler Backend

Michelangelo AI can delegate batch-job admission to [Kueue](https://kueue.sigs.k8s.io/), giving you quota-aware queueing and gang admission on your compute clusters. The backend is opt-in per compute cluster: clusters that don't opt in keep the default immediate-admission behavior, so mixed fleets work throughout a migration.

The backend covers RayClusters and SparkJobs. Kueue admits a RayCluster with its default configuration. It does not admit a SparkApplication by default: that integration is off, and the compute cluster needs extra setup described under [Spark jobs](#spark-jobs).

## How it works

The control plane deliberately does **not** create Kueue `Workload` objects itself. Instead:

1. A compute cluster opts in by setting `scheduler_type: SCHEDULER_TYPE_KUEUE` on its `Cluster` spec.
2. With `jobs.scheduler.backend: kueue`, the control plane validates at enqueue time that a job bound for a Kueue-managed cluster has an existing LocalQueue to land in. Jobs bound for other clusters pass through untouched.
3. When the job is dispatched to a Kueue-managed cluster, the k8s engine stamps the `kueue.x-k8s.io/queue-name` label on the RayCluster (or, for a SparkJob, the SparkApplication) it creates, resolved by the control plane from the job's project. The label follows the cluster's `scheduler_type` alone: it is stamped whatever `jobs.scheduler.backend` is. Labels set on the job itself never cross the cluster boundary. The project identity behind the queue name is the `ma/project-name` label when the job has one, otherwise its namespace (see below), so unless your deployment stamps or validates that label on every job, whoever creates a job can set it and pick another project's LocalQueue.
4. Kueue's own [RayCluster integration](https://kueue.sigs.k8s.io/docs/tasks/run/rayclusters/) on the compute cluster suspends the labeled RayCluster, admits it against ClusterQueue quota when capacity is available, and gang-admits all of its pods together.

While a RayCluster waits for admission (KubeRay reports it `suspended`), the controller surfaces a `Queued` condition with reason `AwaitingAdmission`; when the cluster becomes ready, the condition flips to `False` with reason `ClusterAdmitted`.

A SparkJob follows steps 1 to 3 the same way. Step 4 differs: whether anything admits the SparkApplication depends on how Kueue is set up on the compute cluster, and a SparkJob has no `Queued` condition. See [Spark jobs](#spark-jobs).

## Queue topology

The intended topology is **one ClusterQueue per compute cluster, one LocalQueue per project**:

- Each compute cluster enforces its own quota independently (no MultiKueue).
- Each project gets a LocalQueue pointing at the cluster's ClusterQueue, named by the `localQueueTemplate` convention -- `ma-{project}` by default.
- The project identity is the `ma/project-name` label when your deployment stamps it, otherwise the job's namespace (Michelangelo AI jobs run in their project's namespace).

LocalQueues live in the namespace where the control plane dispatches RayClusters and SparkApplications on the compute cluster (`default`). In this phase they are operator-managed: create them when you onboard a project. The control plane does not create them on demand. With `jobs.scheduler.backend: kueue` it checks that the LocalQueue exists before dispatch, and a job whose LocalQueue is missing fails visibly (`Enqueued=False`, reason `KueueQueueNotFound`) instead of being dispatched into an unadmittable state. With the `default` backend nothing checks: the job is dispatched with its queue label all the same, and a RayCluster, or a SparkApplication on a cluster where Kueue manages them, then waits, suspended, until its LocalQueue exists.

## Enabling the backend

### 1. Install Kueue on the compute cluster

Kueue 0.15+ is recommended (its API defaults to `kueue.x-k8s.io/v1beta2`, which matches the control plane's default; see [Compatibility](#compatibility) for older releases). The default Kueue configuration already enables the RayCluster integration:

```bash
helm install kueue oci://registry.k8s.io/kueue/charts/kueue \
  --version 0.15.0 --namespace kueue-system --create-namespace
```

That install admits RayClusters but not SparkApplications. Before you send Spark work to the cluster, see [Spark jobs](#spark-jobs).

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
  namespace: default       # where dispatched RayClusters and SparkApplications land
spec:
  clusterQueue: cluster-queue
```

With the `kueue` backend, the control plane reads a job's LocalQueue on the compute cluster before it dispatches the job there, with the ServiceAccount whose token the cluster's `Cluster` object refers to (through its `tokenTag` Secret). That ServiceAccount needs `get` on `localqueues` in the `kueue.x-k8s.io` API group. Without it the compute cluster refuses every check, and every job bound for the cluster is rejected with `KueueQueueCheckFailed`. The `ray-manager` ClusterRole in [Register a Compute Cluster](../setup/register-a-compute-cluster-to-michelangelo-control-plane.md) grants it, and so does the sandbox's.

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

## Spark jobs

SparkJobs use the same backend as RayClusters. The SparkApplication the control plane creates on a Kueue-managed cluster carries the `kueue.x-k8s.io/queue-name` label whatever the backend, and with `jobs.scheduler.backend: kueue` the SparkJob is also checked against its project's LocalQueue before dispatch. The difference is on the compute cluster: Kueue's RayCluster integration is on by default, and its SparkApplication integration is not.

### Prerequisites on the compute cluster

Kueue admits a SparkJob's SparkApplication only when all of these hold on that cluster:

- **Kueue 0.17 or later.** The SparkApplication integration is alpha and first appeared in 0.17.
- **The `SparkApplicationIntegration` feature gate** is enabled on the Kueue controller manager.
- **`sparkoperator.k8s.io/sparkapplication`** is in `integrations.frameworks` in the Kueue configuration. Add it to the list that is already there and keep the other entries, such as `ray.io/raycluster`.
- **Spark Operator 2.4.0 or later** is installed and watches the namespace the applications land in, which is `default`. In the Spark Operator Helm chart the watched namespaces are `spark.jobNamespaces`, which defaults to `[default]`.
- **The control plane may manage SparkApplications on the cluster.** The ServiceAccount whose token the cluster's `Cluster` object refers to (through its `tokenTag` Secret) needs `create`, `get`, `list`, `watch` and `delete` on `sparkapplications` in the `sparkoperator.k8s.io` API group: the control plane creates, reads and deletes the application of each SparkJob, and watches them all. The `ray-manager` ClusterRole in [Register a Compute Cluster](../setup/register-a-compute-cluster-to-michelangelo-control-plane.md) grants this, and so does the sandbox's.

The driver of every SparkApplication runs as the `spark-operator-spark` service account, which must exist in `default`. The Spark Operator Helm chart names the service account it creates after the release, so it creates `spark-operator-spark` only for a release named `spark-operator`; with any other release name, set `spark.serviceAccount.name: spark-operator-spark`. Kueue also rejects a SparkApplication that sets `spec.dynamicAllocation.enabled: true`. The control plane never sets that field. Kueue's own [Run a SparkApplication](https://kueue.sigs.k8s.io/docs/tasks/run/kubeflow/sparkapplications/) page has the upstream details.

### If a prerequisite is missing

What happens depends on which one is missing:

- **A Kueue prerequisite** (the Kueue version, the feature gate or the framework entry): the queue label is inert. The SparkApplication is created, starts at once, and runs outside any quota. Nothing in Michelangelo AI reports this: the check at enqueue time only looks for the LocalQueue, and it does not read the Kueue configuration of the compute cluster.
- **A Spark Operator that watches `default`:** the SparkApplication is created, but nothing ever processes it. Its state stays empty in `kubectl get sparkapplications -n default`, and the SparkJob never gets an outcome.
- **The SparkApplication API itself**, which the Spark Operator's Helm chart installs as a CRD: the cluster does not serve it, so the SparkJob is never launched. It fails (`Succeeded=False`, reason `SparkApplicationCreationFailed`) with a message that says the cluster does not serve `sparkapplications.sparkoperator.k8s.io` and asks whether the Spark Operator is installed.
- **The control plane's permission to create SparkApplications:** the SparkJob fails the same way, with the compute cluster's Forbidden message.

To confirm that admission works, submit a SparkJob and run `kubectl get workloads -n default` on the compute cluster. With the integration on, there is a Workload owned by the SparkApplication, and Kueue's webhook sets `spec.suspend: true` on the application until the Workload is admitted. Without the integration there is no Workload.

The sandbox is in the first of these states. `ma sandbox demo kueue` installs Kueue 0.15.0, which cannot admit a SparkApplication, so a SparkJob whose LocalQueue exists runs unadmitted on the sandbox's cluster even though its queue label is set. The demo creates the LocalQueue of its demo project only (`ma-ma-dev-test`), and Uniflow's Spark tasks create their SparkJobs in the `default` namespace, so their project is `default`: with the `kueue` backend they fail the enqueue check with `KueueQueueNotFound` until you create a LocalQueue named `ma-default` in `default` on that cluster. A dedicated compute cluster (`ma sandbox create --create-compute-cluster`) has no Spark Operator, so a SparkJob sent there fails at launch as described above.

### Pinning a SparkJob to a cluster

The scheduler picks the compute cluster from the job's affinity and the registered clusters. The size of the job plays no part. A RayCluster states its choice with the `michelangelo/cluster-affinity` metadata label. A SparkJob ignores that label and states it in `spec.affinity`:

```yaml
apiVersion: michelangelo.api/v2
kind: SparkJob
metadata:
  name: my-spark-job
  namespace: my-project
spec:
  affinity:
    resourceAffinity:
      selector:
        matchLabels:
          michelangelo/cluster-affinity: my-compute-cluster
  # ... driver, executor, mainApplicationFile
```

A name that matches no registered cluster is not rejected. The control plane logs that the cluster was not found and assigns the cluster it gives every job without affinity: the first one its cache returns, which can be one that is not ready. That cluster stays the same for the life of a controllermgr process while the registered clusters do not change, but it can be a different one after every restart or failover of the controllermgr. Uniflow's Spark tasks cannot be pinned today: the SparkJobs they create have no `spec.affinity`. So with more than one compute cluster, set the affinity on the SparkJobs you create yourself, and create each project's LocalQueue on every Kueue-managed cluster, not only on the one its jobs land on now.

### What the control plane creates

For each SparkJob it launches, the control plane creates one SparkApplication on the assigned compute cluster:

- It is named after the SparkJob and created in the `default` namespace, whatever namespace the SparkJob is in.
- It carries the control-plane labels `ma/owner-service: michelangelo-spark` (what the control plane's watcher selects on), `ma/control-plane-env` and `ma/project-name` (the SparkJob's namespace), and `ma/job-uid` (the SparkJob's UID). The control plane sets the first three on the driver and executor pod specs too, and Spark Operator 2.x copies the application's labels onto the pods as well, so the pods carry all four.
- On a Kueue-managed cluster it also carries `kueue.x-k8s.io/queue-name` (the project's LocalQueue). The pods must not carry it, or Kueue's pod integration would admit them separately: the control plane leaves it off the pod specs, and Spark Operator 2.4.0 is the first release that does not copy it from the application, which is one reason the [prerequisites](#prerequisites-on-the-compute-cluster) ask for it.
- It sets `timeToLiveSeconds: 86400`, so the Spark Operator deletes it 24 hours after it finishes. Until then the application and its driver logs stay on the compute cluster.

Every application lands in the shared `default` namespace under the SparkJob's name, so two SparkJobs with the same name collide, whichever projects they are in. The `ma/job-uid` label tells the control plane whose application it is looking at: an application is a SparkJob's own only when the label holds that SparkJob's UID. The control plane does not adopt or delete an application that belongs to a different SparkJob, and it does not adopt or delete an application without the label either, such as one created with `kubectl`: the newer SparkJob fails with a conflict error instead. That includes a SparkJob created under the name of one that finished less than 24 hours ago, because the new SparkJob has a new UID. Use names that are unique across projects, and do not reuse a name inside that 24-hour window.

A SparkJob that asks for GPUs gets the `nvidia.com/gpu` resource on the driver or executor that requests them, together with a toleration for the NVIDIA GPU taint (key `nvidia.com/gpu`, operator `Exists`, effect `NoSchedule`). The `gpu_sku` field names a kind of GPU, not a resource, and is never used as a resource name. Kueue's SparkApplication documentation describes CPU and memory accounting only. If a ClusterQueue should also limit GPUs, list `nvidia.com/gpu` in its `coveredResources` and check, on your Kueue version, that Spark jobs are counted against it.

### Status and lifecycle

A watcher on the control plane follows each SparkApplication on its compute cluster and records the result on the SparkJob: `Succeeded=True` when the application completes, `Succeeded=False` when it fails or cannot be submitted. While the application is queued or running, the SparkJob has no outcome yet. Watch events can be missed, for example while the controllermgr is down, so the SparkJob controller also re-checks every launched SparkJob that has no outcome yet every 5 minutes, with one status read on the compute cluster.

Creating the SparkApplication can fail. When it fails for a reason that may pass (the call timed out, the compute cluster could not be reached, was overloaded or answered with a server error, or a proxy in front of it answered with an error page), the control plane tries again, waiting longer after each failure, and the SparkJob carries `Launched=Unknown` with reason `SparkApplicationCreationRetrying` and the failure as its message. A failure that repeats leaves the message as it is, and one that reads differently replaces it only once the message is 5 minutes old. When the creation fails for a reason that will not pass, such as an admission webhook's denial, a missing permission, or an application of the same name that belongs to another SparkJob, the SparkJob records `Launched=False` and `Succeeded=False`, both with reason `SparkApplicationCreationFailed`, and the control plane does not try again.

A compute cluster that does not answer a creation, termination or deletion in time (each call gets 30 seconds), or whose 5-minute re-check fails, is held for 5 minutes. Until the hold ends, the control plane sends that cluster no creation, termination, deletion or re-check of a SparkJob: the SparkJobs on it, a termination you request included, wait for the hold to end. The LocalQueue check before dispatch is not held.

A SparkJob whose driver pod cannot start has no outcome, and nothing on it says why. When the driver's image cannot be pulled (`ErrImagePull`, `ImagePullBackOff`) or a Secret or ConfigMap it needs is missing on the compute cluster (`CreateContainerConfigError`), the Spark Operator keeps reporting the application as `SUBMITTED`, with no error and no timeout. The control plane watches a RayCluster's pods and fails the RayCluster for these reasons, but it does not watch Spark pods yet. If a SparkJob stays without an outcome, look at its driver pod, `<name>-driver` in `default` on the compute cluster, and terminate the SparkJob if the pod cannot start.

If a SparkApplication is deleted from the compute cluster while it is still running, the SparkJob records `Succeeded=False` with reason `SparkApplicationDeleted`. The 5-minute re-check records the same result when it finds the application gone. Neither records a `Killed` condition: Uniflow treats `Killed=True` as "do not retry", so `Killed` is reserved for a termination the user asked for on the SparkJob. An application deleted after it finished keeps its real outcome, provided the control plane saw it finish first: through a watch event, the delete event included, or the 5-minute re-check. One that finished and was deleted while the control plane could not see its cluster, for example during an outage longer than the 24-hour TTL, is recorded as deleted (`Succeeded=False`).

Deleting a SparkJob that has no outcome yet deletes its SparkApplication. Just before it first creates the application, the control plane adds the finalizer `michelangelo/SparkApplicationCleanup` to the SparkJob, so a SparkJob deleted with `kubectl delete` stays, with a deletion timestamp, until its application is deleted from the compute cluster. A delete through the Michelangelo API server deletes the application the same way. If the delete keeps failing, the control plane gives up an hour after the SparkJob's deletion timestamp: it logs an error that names the compute cluster and the application (`default/<name>`), which you then delete by hand, and removes the finalizer. Holds on the cluster put the give-up off, since no delete is attempted while one is on. If the SparkJob's compute cluster is no longer registered, that is, its `Cluster` object is gone, the control plane cannot reach the application: it removes the finalizer without deleting anything, and any application left on that cluster stays there. A cluster whose `Cluster` object still exists is reached through that object, even while the controllermgr has not loaded the cluster yet, as for a while after a controllermgr takes over. A SparkJob loses the finalizer once its outcome is recorded, so deleting a finished SparkJob leaves its application to the 24-hour TTL.

A SparkJob has no `Queued` condition. To see whether Kueue is holding one back, look at its SparkApplication and Workload on the compute cluster, as described above.

## Configuration reference

Config path `jobs.scheduler` (Helm: `controllermgr.jobs.scheduler`):

| Key | Default | Meaning |
|-----|---------|---------|
| `backend` | `default` | `default` for no check before dispatch; `kueue` to check that a job bound for a Kueue-managed cluster has its LocalQueue there. The queue label does not depend on this setting: it follows the cluster's `scheduler_type`. |
| `kueue.localQueueTemplate` | `ma-{project}` | LocalQueue naming convention; `{project}` is replaced with the job's project. |
| `kueue.localQueueOverrides` | `{}` | Per-project explicit LocalQueue names; an override wins over the template. |
| `kueue.apiVersion` | `v1beta2` | `kueue.x-k8s.io` API version used for the LocalQueue existence check on compute clusters. |

## Observability

- **Waiting for quota (RayClusters):** the RayCluster carries `Queued=True` (reason `AwaitingAdmission`) while suspended, flipping to `False`/`ClusterAdmitted` on admission. A queued job is therefore distinguishable from one stuck launching.
- **Waiting for quota (SparkJobs):** a SparkJob has no `Queued` condition. Look at its SparkApplication and Workload on the compute cluster. A SparkApplication with no Workload is not under Kueue at all; see [If a prerequisite is missing](#if-a-prerequisite-is-missing).
- **Launch retries (SparkJobs):** a SparkJob whose SparkApplication cannot be created yet carries `Launched=Unknown` with reason `SparkApplicationCreationRetrying` and the latest recorded failure as its message; see [Status and lifecycle](#status-and-lifecycle).
- **Misconfiguration (with the `kueue` backend):** a missing LocalQueue sets `Enqueued=False` on the RayCluster or SparkJob with reason `KueueQueueNotFound` and an actionable message. When the check cannot reach the compute cluster, or the compute cluster refuses it (see the `get` permission on `localqueues` in [step 1](#1-install-kueue-on-the-compute-cluster)), the job records `KueueQueueCheckFailed` instead and is retried. A SparkJob is retried every 5 minutes after a `KueueQueueCheckFailed` rejection, and every 10 seconds after `KueueQueueNotFound`, so it is dispatched soon after you create its LocalQueue.
- **Metrics:** the `kueuescheduler` controller scope emits `kueue.queue_not_found_count` and `kueue.queue_check_failed_count`.
- On the compute cluster, Kueue's own metrics and `kubectl get workloads`/`localqueues` show admission state and quota usage.

## Compatibility

- **Kueue version:** the control plane only performs an existence `GET` against `kueue.x-k8s.io`, so there is no client-library coupling. The default `apiVersion: v1beta2` requires Kueue 0.15+; set `kueue.apiVersion: v1beta1` to run against older Kueue releases.
- **RayJobs:** Michelangelo AI RayJobs target an existing RayCluster via `ClusterSelector`, which Kueue's RayJob integration deliberately skips -- admission is enforced on the RayCluster, which is the resource-owning object. RayJobs never get a queue label.
- **SparkApplications:** Kueue admits them only under the [Spark prerequisites](#prerequisites-on-the-compute-cluster). Kueue releases before 0.17 have no SparkApplication integration, so the queue label does nothing there.
- **`waitForPodsReady`:** avoid enabling Kueue's `waitForPodsReady` for RayClusters admitted through this backend. A RayCluster's pods can legitimately start over a window (e.g. autoscaling workers), and `waitForPodsReady` can evict and requeue the whole cluster on that window.

## Rollback

Each step is independently reversible:

- Set `jobs.scheduler.backend: default` (or remove it) to stop the LocalQueue check. Jobs bound for clusters that keep `scheduler_type` are still labeled, and already-dispatched suspended clusters still get admitted by Kueue on the compute cluster.
- Remove `scheduler_type` from a `Cluster` spec to stop labeling the jobs bound for it, which returns that cluster to immediate admission while the backend stays on for others.
- Uninstalling Kueue from a compute cluster leaves any still-suspended RayClusters suspended: terminate them (or clear `spec.suspend` on the KubeRay object) before removing Kueue. The same goes for SparkApplications: terminate the SparkJob, or clear `spec.suspend` on the SparkApplication.

Rolling the controllermgr back to a release from before SparkJobs carried the `michelangelo/SparkApplicationCleanup` finalizer (see [Status and lifecycle](#status-and-lifecycle)) leaves that finalizer on the SparkJobs that have it, and the older release never removes it, so deleting one of them never completes. Remove `michelangelo/SparkApplicationCleanup` from the SparkJob's `metadata.finalizers` by hand, for example with `kubectl edit`, leaving any other finalizer in place, and delete its SparkApplication from the compute cluster yourself if it is still there.

## Try it in the sandbox

```bash
ma sandbox create
ma sandbox demo kueue
ma sandbox sync --set controllermgr.jobs.scheduler.backend=kueue
```

`ma sandbox demo kueue` installs Kueue on the sandbox's compute cluster (use `--compute-cluster-name` if you created a dedicated one), creates a demo ClusterQueue and the demo project's LocalQueue, and marks the Cluster CR as Kueue-managed. Submit a Ray task and watch it flow through `Queued` -> admitted; shrink the ClusterQueue quota below the job's request to see it wait. Spark jobs do not queue in the sandbox: it installs Kueue 0.15.0, which cannot admit a SparkApplication (see [If a prerequisite is missing](#if-a-prerequisite-is-missing)). With the `kueue` backend, Uniflow's Spark tasks also need a LocalQueue `ma-default`, which the demo does not create.
