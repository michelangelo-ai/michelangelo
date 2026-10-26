# Kueue Scheduler Backend

Michelangelo AI can delegate batch-job admission to [Kueue](https://kueue.sigs.k8s.io/), giving you quota-aware queueing and gang admission on your compute clusters. The backend is opt-in per compute cluster: clusters that don't opt in keep the default immediate-admission behavior, so mixed fleets work throughout a migration.

## How it works

The control plane deliberately does **not** create Kueue `Workload` objects itself. Instead:

1. A compute cluster opts in by setting `scheduler_type: SCHEDULER_TYPE_KUEUE` on its `Cluster` spec.
2. With `jobs.scheduler.backend: kueue`, the control plane chooses each job's cluster once, at enqueue. For a Kueue-managed cluster it first checks that the project's LocalQueue exists there, then records the cluster as the job's assignment, so the job is dispatched exactly where it was validated. Jobs bound for other clusters are assigned without the check.
3. When the job is dispatched, the k8s engine stamps exactly one label on the RayCluster it creates: `kueue.x-k8s.io/queue-name`, resolved by the control plane from the job's project (see [Queue topology](#queue-topology)). User-supplied labels never cross the cluster boundary. Whether that makes queues an isolation boundary depends on authentication: under `apiserver.auth.mode: k8s-rbac` only users authorized in a project's namespace can submit into its queues, while under the default `dummy` mode anyone can submit into any namespace, so LocalQueues give attribution and an allowlist, not isolation.
4. Kueue's own [RayCluster integration](https://kueue.sigs.k8s.io/docs/tasks/run/rayclusters/) on the compute cluster suspends the labeled RayCluster, admits it against ClusterQueue quota when capacity is available, and gang-admits all of its pods together.

While Kueue holds a RayCluster (KubeRay reports it `suspended`), the controller sets `Queued=True` with reason `AwaitingAdmission`; when Kueue admits it, the condition flips to `False` with reason `ClusterAdmitted`. The condition is written only when it changes, so its timestamp is the transition time. Clusters that are never held never get the condition.

## Queue topology

The intended topology is **one ClusterQueue per compute cluster, one LocalQueue per project**:

- Each compute cluster enforces its own quota independently (no MultiKueue).
- Each project gets a LocalQueue pointing at the cluster's ClusterQueue, named by the `localQueueTemplate` convention -- `ma-{project}` by default.
- The project identity is the job's namespace (Michelangelo AI jobs run in their project's namespace). The `ma/project-name` label is honored only when `jobs.scheduler.kueue.trustProjectLabel` is `true`. Set that only if your admission path stamps the label and overwrites any value a client supplies. Nothing in Michelangelo AI does that, so with the label trusted any caller could use another project's LocalQueues. Deployments that relied on the label before this setting existed set it to `true` when they upgrade.

LocalQueues live in the namespace where the control plane dispatches RayClusters on the compute cluster (`default`). They are managed outside the control plane, which checks that the queue exists rather than creating it. A job whose LocalQueue is missing waits visibly (`Enqueued=False`, reason `KueueQueueNotFound`) instead of being dispatched into an unadmittable state, and is re-checked until the queue appears or the [admission bound](#observability) expires.

## Object ownership and quota

Every Kueue-managed cluster carries two kinds of Kueue objects, with different owners:

| Object | Cardinality | Owner | Where it comes from |
|---|---|---|---|
| `ResourceFlavor`, `ClusterQueue` (and optional `WorkloadPriorityClass`es) | One set per cluster | Cluster operator (capacity) | The cluster's queue objects, with quota as required input |
| `LocalQueue` `ma-<project>` in namespace `default` | One per project, per cluster the project may run on | Project onboarding | Either the cluster's managed queue list or applied by hand from the project's reference file — exactly one of the two per queue name |

- **Quota is shared per cluster.** Every LocalQueue on a cluster feeds its one ClusterQueue, so projects share that quota in Kueue's order, and a project's LocalQueue only decides whether the project may use the cluster. A shared pool never strands capacity in an idle project's slice. If you need hard per-project limits, author one ClusterQueue per project yourself and point that project's LocalQueue at it. Kueue 0.19 makes a LocalQueue's `spec.clusterQueue` immutable, so re-pointing an existing LocalQueue means deleting and recreating it.
- **Namespace `default`.** Kueue looks up a workload's LocalQueue in the workload's own namespace, and the control plane creates every RayCluster in `default`, so projects' queues are told apart by name.
- **One manager per queue name.** Manage each LocalQueue either through a Helm release or by hand, never both: Helm refuses to adopt an object it did not create and reverts hand edits to objects it did.
- **LocalQueue presence is the allowlist.** A project can run on a Kueue-managed cluster only if its LocalQueue exists there.
- **Reference LocalQueues** for the example projects live in [michelangelo-examples](https://github.com/michelangelo-ai/michelangelo-examples), as `config/localqueue.yaml` beside each project's `config/project.yaml`. ClusterQueues do not, because capacity is a per-cluster decision.
- **Size quota for what Kueue charges.** Kueue charges every container in a Ray pod, so the log-collector sidecar (100m CPU and 128Mi per Ray pod when log persistence is on) counts against quota. The RayJob submitter pod does not, because Kueue admits the RayCluster, not the RayJob.

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

### 2. Grant the control plane read access to Kueue

The control plane reaches each compute cluster as the `ray-manager` ServiceAccount. On a Kueue-managed cluster that identity needs `get`, `list` and `watch` on LocalQueues in `default`, for the enqueue-time check (without it every job waits with `KueueQueueCheckFailed`), and on ClusterQueues, for Kueue-aware placement. It needs nothing else in `kueue.x-k8s.io` and nothing writable. Apply the manifest in [Kueue read access](../setup/register-a-compute-cluster-to-michelangelo-control-plane.md#kueue-read-access-kueue-managed-clusters) from the registration guide.

### 3. Mark the compute cluster

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

### 4. Enable the backend on the control plane

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
      #   admissionTimeout: 24h                # default; bound on the admission wait
      #   trustProjectLabel: false             # default; true only if your admission path stamps ma/project-name
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
| `kueue.admissionTimeout` | `24h` | Bound on a RayCluster's admission wait: time waiting for a placement plus time held by Kueue, measured from creation and again from each eviction. On expiry the RayCluster fails with `AdmissionTimeout` and its compute-side object is deleted. A negative value fails at startup. |
| `kueue.trustProjectLabel` | `false` | When `true`, the `ma/project-name` label overrides the namespace as the job's project. See [Queue topology](#queue-topology). |

## Observability

- **Waiting for quota:** the RayCluster carries `Queued=True` (reason `AwaitingAdmission`) exactly while Kueue holds it, flipping to `False`/`ClusterAdmitted` when Kueue admits it. A queued job is therefore distinguishable from one stuck launching, and the condition's timestamp is when it started or stopped waiting.
- **Waiting for a placement:** a missing LocalQueue sets `Enqueued=False` with reason `KueueQueueNotFound` and a message naming the queue; a compute cluster that cannot be asked sets `KueueQueueCheckFailed`. The job keeps that condition, rewritten only when the reason changes, and is re-checked.
- **Admission bound:** a RayCluster whose admission wait exceeds `kueue.admissionTimeout` fails with `Succeeded=False`, reason `AdmissionTimeout`, and its compute-side RayCluster is deleted, releasing its queue position. The wait counts placement waits and time held by Kueue, from creation and again from each eviction. Uniflow's provisioning budget (1800 s by default) no longer counts time held by Kueue.
- **Dispatch retries.** A create that fails with a transient error, such as Kueue's webhook during a restart, is retried with backoff for up to 10 minutes.
  - While it retries, the RayCluster shows `Launched=False`, reason `ClusterCreationFailed`, with the last error in the message.
  - Validation rejections and denied permissions fail at once.
  - Each create gets 20 seconds, of which at most 5 seconds may go to connecting.
  - A cluster that is not Ready is never dialed.
  - With the Kueue backend, a job is re-placed by a fresh decision, spaced by the same backoff, if its create provably never reached the cluster or was rejected, and its cluster then becomes unready or the 10 minutes run out. The fresh decision skips every cluster the job was already assigned to, unless that leaves none, so a cluster that keeps rejecting is not picked again.
  - A create that may have reached the cluster, for example one that timed out after connecting, is retried in place on the same cluster.
- **Compute-side ownership.** Every RayCluster and RayJob the control plane creates on a compute cluster carries the annotations `ma/owner` (`<namespace>/<name>` of the control-plane object) and `ma/owner-uid`. The control plane deletes only objects whose `ma/owner-uid` matches. A create that finds an object of the same name adopts it only if the object's `ma/owner-uid` is the job's, or the object has no owner annotation, as objects created before this release do. An object owned by another job fails the create at once with `ClusterCreationFailed`, naming that job. Compute-side objects are named after the job alone, in `default`, so same-named jobs in two projects collide, and one of them must be renamed. If the control plane restarts between a create and recording it, and the job is then placed elsewhere, the first object can be left behind. To find such orphans, list the RayClusters in `default` whose `ma/owner-uid` matches no live control-plane RayCluster.
- **Metrics:** the `kueuescheduler` controller scope emits `kueue.queue_not_found_count` and `kueue.queue_check_failed_count`. The controllermgr metrics endpoint serves `michelangelo_jobs_admission_timeouts_total`, the `michelangelo_jobs_admission_wait_seconds` histogram, `michelangelo_jobs_create_retries_total` and `michelangelo_jobs_replacements_total`.
- On the compute cluster, Kueue's own metrics and `kubectl get workloads`/`localqueues` show admission state and quota usage.

## Compatibility

- **Kueue version:** the control plane reads `kueue.x-k8s.io` objects as plain JSON at `kueue.apiVersion`, so there is no client-library coupling. It reads them in two ways: an existence `GET` of a job's LocalQueue when placing the job, and, under `backend: kueue`, a read-only list and watch of each Kueue-managed cluster's ClusterQueues and of its LocalQueues in `default`. The default `apiVersion: v1beta2` requires Kueue 0.15+; set `kueue.apiVersion: v1beta1` to run against older Kueue releases.
- **RayJobs:** Michelangelo AI RayJobs target an existing RayCluster via `ClusterSelector`, which Kueue's RayJob integration deliberately skips -- admission is enforced on the RayCluster, which is the resource-owning object. RayJobs never get a queue label.
- **`waitForPodsReady`:** Kueue 0.19 and later always run it (30 minutes by default), and before 0.19 it is opt-in. Configure it as a backstop rather than a provisioning limit: `timeout: 1h`, above Uniflow's 1800 s provisioning budget, so it never evicts a cluster that is still provisioning; `recoveryTimeout: 0s`, so a running cluster is not evicted when one of its pods restarts; and `requeuingStrategy.backoffLimitCount: 2`, so Kueue deactivates a workload after its third failed start and the admission bound then fails and deletes it.

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
