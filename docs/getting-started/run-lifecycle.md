---
sidebar_position: 2.6
title: Trace a Pipeline Run End to End
description: Follow one `ma pipeline run` from the CLI through the API server, controllers, workflow engine, and Ray or Spark, and see which resource and source file handles each hop.
last_verified: 2026-11-25
---

# Trace a Pipeline Run End to End

This page follows a single `ma pipeline run` through Michelangelo AI, one hop at a time. For each hop it names the component, the resource it creates or updates, the code that implements it, and a command you can use to watch it happen. For the big picture of the components and CRDs, read [System Architecture](./architecture.md) first. This page goes one level deeper.

The path has seven hops:

```text
 ma CLI ──gRPC──▶ API server ──creates──▶ PipelineRun (Kubernetes / etcd)
                                               │ watch
                                               ▼
                                   Controller manager
                                   PipelineRun controller
                                   ├─ 1. Source Pipeline
                                   ├─ 2. Image Build
                                   └─ 3. Execute Workflow ──start──▶ Workflow engine
                                                                     (Cadence or Temporal)
                                                                           │ task list
                                                                           ▼
                                                                  Worker (Starlark workflow)
                                                                           │ activities, gRPC
                                                                           ▼
                                                  API server ──creates──▶ RayCluster, RayJob
                                                                           │ watch
                                                                           ▼
                                           Cluster, RayCluster and RayJob controllers
                                                                           │ KubeRay API
                                                                           ▼
                                                           Compute cluster: Ray runs the task
                                                                           │
                          status flows back: Ray job ─▶ RayJob ─▶ workflow ─▶ PipelineRun ─▶ ma CLI
```

:::note
The example below follows a **Ray** task, because that is the path with the most code in this repository. A Spark task takes a shorter route. The worker creates a `SparkJob`, and the SparkJob controller creates a `SparkApplication` directly on the cluster where the controller manager runs, with no scheduler step. See [Spark tasks](#spark-tasks).
:::

## Prerequisites

- A registered pipeline. See [Getting Started with ML Pipelines](../user-guides/getting-started/getting-started.md).
- A running control plane. The [sandbox](./sandbox-setup.md) works, and so does a [Helm install](../operator-guides/helm-chart.md).

The commands below use `<namespace>` for your project namespace and `<pipeline>` for the pipeline name.

## 1. The CLI creates a PipelineRun

```bash
ma pipeline run --namespace="<namespace>" --name="<pipeline>"
```

| | |
|---|---|
| **Component** | `ma` CLI |
| **Resource** | None yet. The CLI builds a `PipelineRun` and sends it in a `CreatePipelineRun` gRPC request. |
| **Code** | [`python/michelangelo/cli/mactl/plugins/entity/pipeline/run.py`](https://github.com/michelangelo-ai/michelangelo/blob/main/python/michelangelo/cli/mactl/plugins/entity/pipeline/run.py) |

The CLI generates a unique run name of the form `run-YYYYMMDD-HHMMSS-<8 hex characters>` and fills in `spec.pipeline` (the pipeline name and namespace) and `spec.actor` (your user name). If you pass `--resume_from`, it also fills in `spec.resume`. If you pass `--notify-slack`, `--notify-email`, or `--notify-on`, it fills in `spec.notifications`, and the API server then leaves the run's notification rules alone in the next hop. It then calls `PipelineRunService/CreatePipelineRun` on the API server, which is `127.0.0.1:15566` by default and can be overridden with `MACTL_ADDRESS`. With `--dry-run`, the server validates the request and rolls it back, so nothing below this hop happens.

`ma pipeline run` has no `--revision` flag. The revision a run uses is chosen on the server in the next hop.

## 2. The API server validates and stores it

| | |
|---|---|
| **Component** | API server (the `michelangelo-apiserver` Deployment) |
| **Resource** | `PipelineRun` is created in Kubernetes (etcd). When metadata storage is on, the API server also adds the ingester finalizer, and the ingester in the controller manager copies the run to MySQL. |
| **Code** | [`proto-go/api/v2/pipeline_run.pb.kubeyarpc.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/proto-go/api/v2/pipeline_run.pb.kubeyarpc.go) (generated handler), [`go/components/pipelinerun/apihook/apihook.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/pipelinerun/apihook/apihook.go) (hook), [`go/api/handler/`](https://github.com/michelangelo-ai/michelangelo/tree/main/go/api/handler) |

The generated `CreatePipelineRun` handler authenticates and authorizes the caller, validates the resource, runs the registered `BeforeCreate` hooks, and then asks the API handler to create the object. The `PipelineRun` hook does five things:

- Sets the `michelangelo/environment` label to the default environment if it is absent.
- Pins the run to a revision. If the pipeline has `status.latestRevision`, the hook copies that revision's pipeline spec into the run. If it has none, the run falls back to the live `Pipeline`.
- Copies the pipeline's notification rules onto the run when the run has none.
- Stamps the owning `Pipeline` as the run's `ownerReference`, which is what lets [cascade delete](../operator-guides/cascade-delete.md) find the run.
- Stamps the pipeline's type as the `michelangelo/SourcePipelineType` label.

For the framework behind this, see [API Framework](../operator-guides/api-framework.md).

**Observe it:**

```bash
ma pipeline_run get --namespace="<namespace>" --name="<run-name>" --output=yaml
```

The run exists but `status` is mostly empty until a controller picks it up. The CLI prints the run name when the create call returns.

## 3. The PipelineRun controller reconciles it

| | |
|---|---|
| **Component** | Controller manager (the `michelangelo-controllermgr` Deployment), PipelineRun controller |
| **Resource** | `PipelineRun`: drain finalizer added, then `status.conditions`, `status.steps`, and `status.state` updated |
| **Code** | [`go/components/pipelinerun/controller.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/pipelinerun/controller.go), [`go/components/pipelinerun/plugin/plugin.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/pipelinerun/plugin/plugin.go) |

The controller watches `PipelineRun` objects. While the run isn't terminal, each reconcile makes sure it has a drain finalizer (`pipelineruns.michelangelo.uber.com/drain`) so a pipeline delete can cancel the workflow before the run is garbage-collected. The `ownerReference` was already set by the hook in hop 2; the controller only fills it in on runs created before that hook existed. It then runs the object through a condition engine with three actors, in order. Each actor records a condition and a matching step in `status.steps`:

| Actor | Step name | What it does | Code |
|-------|-----------|--------------|------|
| `SourcePipelineActor` | `Source Pipeline` | Copies the pipeline into `status.sourcePipeline`: the revision snapshot pinned in hop 2 if there is one (built from the run's inline `spec.pipelineSpec` and annotations), otherwise the live `Pipeline` | [`actors/sourcepipeline.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/pipelinerun/actors/sourcepipeline.go) |
| `ImageBuildActor` | `Image Build` | Reads the image ID from the source pipeline's `michelangelo/uniflow-image` annotation. It does not build anything, and the step fails if the annotation is missing. | [`actors/imagebuild.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/pipelinerun/actors/imagebuild.go) |
| `ExecuteWorkflowActor` | `Execute Workflow` | Starts the workflow, then polls it (hop 4) | [`actors/executeworkflow.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/pipelinerun/actors/executeworkflow.go) |

After the engine runs, the reconciler maps the result to `status.state`: `RUNNING` while any condition is unfinished, `SUCCEEDED` when all are satisfied, `FAILED` otherwise, and `KILLED` if `spec.kill` was set. It persists the status and sends notifications on state changes. When metadata storage is on, a terminal run is marked immutable once `pipelineRun.ttlDays` has passed, and the ingester then moves it to MySQL and deletes it from etcd. The Helm chart doesn't set `ttlDays`, and at its default of `0` this happens as soon as the run finishes. The sandbox turns metadata storage on, so finished runs drop out of `kubectl get pipelineruns` right away, but `ma pipeline_run get` still finds them. With metadata storage off, terminal runs stay in etcd.

**Observe it:**

```bash
kubectl get pipelineruns -n <namespace>
kubectl -n <release-namespace> logs deployment/michelangelo-controllermgr
ma pipeline_run get --namespace="<namespace>" --name="<run-name>" --output=yaml
```

In the YAML, look at `status.steps` for the three steps above and at `status.state`. If the run fails at `Image Build`, check the `michelangelo/uniflow-image` annotation on the run (copied from the revision when one was pinned) or on the pipeline. `status.sourcePipeline.pipeline` shows the pipeline the controller actually used.

## 4. The controller starts a workflow

| | |
|---|---|
| **Component** | Workflow engine, which is **Cadence** or **Temporal** depending on configuration |
| **Resource** | `PipelineRun`: `status.workflowId` and `status.workflowRunId` are set, and the `Execute Workflow` step moves to `RUNNING` with a `logUrl` |
| **Code** | [`go/components/pipelinerun/actors/executeworkflow.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/pipelinerun/actors/executeworkflow.go) (`StartWorkflow`), [`go/base/workflowclient/`](https://github.com/michelangelo-ai/michelangelo/tree/main/go/base/workflowclient) |

The `ExecuteWorkflowActor` reads the pipeline's Uniflow package from the blob store (`spec.manifest.uniflowTar` on the pipeline), assembles the workflow inputs (arguments and environment variables, including `MA_NAMESPACE` and `MA_PIPELINE_RUN_NAME`), and starts a workflow named `starlark-workflow`. The workflow ID is the `PipelineRun` name, and the execution timeout is seven days.

**Which engine?** Both are supported, and one deployment uses one of them. Two settings pick it:

- On the worker, `workflow-engine.provider` is `cadence` or `temporal`. The Helm chart sets it from `workflow.engine`, and `ma sandbox create --workflow=temporal` selects Temporal in the sandbox.
- On the controller manager, `workflowClient.provider` selects the client the controller uses to start and query workflows. The Helm chart renders it for you. In code, the value `Temporal` selects Temporal and anything else selects Cadence.

**Which task list?** The workflow is queued on the task list named by the `michelangelo/worker_queue` annotation on the run's `Project`. If the project has no such annotation, it uses `workflowClient.taskList` (`default` in the Helm chart).

**Observe it:**

- Cadence Web at `http://localhost:8088`, or Temporal Web at `http://localhost:8080`, in the sandbox. Search for a workflow whose ID is the run name. See [Sandbox Ports and Endpoints](./ma-sandbox-ports-and-endpoints.md).
- `status.steps[].logUrl` on the `Execute Workflow` step holds a direct link when `workflowClient.executionUrlFormat` is configured (the Helm chart exposes it as `controllermgr.workflowClient.executionUrlFormat`).

## 5. The worker runs the workflow

| | |
|---|---|
| **Component** | Worker (the `michelangelo-worker` Deployment), an embedded Starlark interpreter, and activities |
| **Resource** | None directly. The worker calls the API server to create and read resources. |
| **Code** | [`go/worker/`](https://github.com/michelangelo-ai/michelangelo/tree/main/go/worker) (`workflowfx/` for the engine connection, `plugins/` for the Starlark modules, `activities/` for the activities), [`python/michelangelo/uniflow/`](https://github.com/michelangelo-ai/michelangelo/tree/main/python/michelangelo/uniflow) (the `.star` task definitions) |

The worker polls the engine's task lists, as configured under `workflow-engine.workers`. The workflow it runs is generic: it interprets the Starlark code from the pipeline's Uniflow package, so changing a pipeline does not require redeploying the worker. Each Uniflow task, such as a `RayTask`, becomes a Starlark function that calls built-in modules, such as `ray`, `spark`, `cachedoutput`, `storage`, `model`, `pipeline`, and `deployment`, registered by [`go/worker/starlark/module.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/worker/starlark/module.go). Those modules call workflow activities, and the activities call the API server over gRPC. The worker does not create Kubernetes resources itself. See the [Starlark worker README](https://github.com/michelangelo-ai/michelangelo/blob/main/go/worker/starlark/README.md).

For each task, the Starlark code first checks the task cache. If caching is enabled and a matching `CachedOutput` exists, the task is skipped. This is also how `--resume_from` works: the run sets `CACHE_ENABLED` and the cache versions of the earlier run's tasks.

**Observe it:**

```bash
kubectl -n <release-namespace> logs deployment/michelangelo-worker --tail=100
```

The workflow's history in Cadence Web or Temporal Web shows each activity as it runs.

## 6. Ray executes the task

For a `RayTask`, [`python/michelangelo/uniflow/plugins/ray/task.star`](https://github.com/michelangelo-ai/michelangelo/blob/main/python/michelangelo/uniflow/plugins/ray/task.star) drives three phases through the `ray` module in [`go/worker/plugins/ray/starlark_module.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/worker/plugins/ray/starlark_module.go): `create_cluster`, `create_job`, and `terminate_cluster`.

| Phase | Resource | Handled by |
|-------|----------|-----------|
| Create a cluster | A `RayCluster` is created through the API server | RayCluster controller, [`go/components/ray/cluster/controller.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/ray/cluster/controller.go) |
| Pick a compute cluster | The controller enqueues the `RayCluster` with the scheduler, which records `status.assignment.cluster` | Scheduler, [`go/components/jobs/scheduler/`](https://github.com/michelangelo-ai/michelangelo/tree/main/go/components/jobs/scheduler); the target is a registered `Cluster` (see [Jobs Overview](../operator-guides/jobs/index.md)) |
| Launch the cluster | The controller creates a KubeRay `RayCluster` (`ray.io/v1`) on the assigned compute cluster, and the state goes to `PROVISIONING` and then `READY` | `federatedClient.CreateJobCluster`, [`go/components/jobs/client/`](https://github.com/michelangelo-ai/michelangelo/tree/main/go/components/jobs/client) |
| Submit the job | A `RayJob` is created through the API server with the entrypoint `python3 -m michelangelo.uniflow.core.run_task --task ...` | RayJob controller, [`go/components/ray/job/controller.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/ray/job/controller.go) |
| Run the job | Once the `RayCluster` is `READY`, the controller creates a KubeRay `RayJob` (`ray.io/v1`) on the compute cluster and polls its status every 10 seconds into `status.state` and `status.dashboardUrl` | `federatedClient.CreateJob` and `GetJobStatus` |
| Clean up | `terminate_cluster` ends the cluster when the job succeeds or fails, and an exit hook ends it if the workflow is interrupted | RayCluster controller |

The Image Build step's output reaches the workflow as the `UF_TASK_IMAGE` environment variable, which the Starlark code uses as the cluster image, and `run_task` runs your Python task function on it. The workflow's `SensorRayJob` activity polls the `RayJob` until it reaches a terminal state, so the workflow waits on this task until then.

**Observe it:**

```bash
# On the control plane, where Michelangelo AI's own resources live
kubectl get rayclusters.michelangelo.api,rayjobs.michelangelo.api -n <namespace>

# On the compute cluster, where KubeRay runs the pods (always in the default namespace)
kubectl --context <compute-context> -n default get rayclusters.ray.io,rayjobs.ray.io
kubectl --context <compute-context> -n default get pods -l ray.io/cluster=<cluster-name>
kubectl --context <compute-context> -n default logs <head-pod-name>
```

Terminal `RayCluster` and `RayJob` objects are moved out of etcd the same way as finished runs, so use these commands while the task is running.

In the sandbox the control plane and the compute cluster can be the same Kubernetes cluster. The `michelangelo.api` and `ray.io` group suffixes keep the two sets of `rayclusters` and `rayjobs` apart. KubeRay's copies use the same names as Michelangelo AI's. `RayJob.status.dashboardUrl` links to the Ray dashboard, which is at `http://localhost:8265` in a sandbox created with `--create-compute-cluster`. See [Troubleshooting](../operator-guides/operations/troubleshooting.md) for more on diagnosing stuck jobs.

### Spark tasks

A `SparkTask` follows the same pattern through the `spark` module ([`go/worker/plugins/spark/starlark_module.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/worker/plugins/spark/starlark_module.go)), but it creates a single `SparkJob` and has no cluster phase. The SparkJob controller ([`go/components/spark/job/controller.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/spark/job/controller.go)) creates a `SparkApplication` through the Spark Operator and copies its state back into the `SparkJob`.

Unlike Ray, Spark work does not go through the scheduler. The SparkJob controller uses the controller manager's own kubeconfig, so the `SparkApplication` lands in the run's namespace on the control-plane cluster:

```bash
kubectl get sparkjobs.michelangelo.api,sparkapplications -n <namespace>
```

Multi-cluster Spark is tracked in [#616](https://github.com/michelangelo-ai/michelangelo/issues/616).

## 7. Outputs and status flow back

**Task outputs.** Each task writes its return value as JSON to object storage, under `UF_METADATA_STORAGE_URL` if set and otherwise `UF_STORAGE_URL`. The controller sets `UF_STORAGE_URL` from the run's `spec.workspaceRootDir`, or `s3://default` when that is empty. When a task succeeds, the workflow records a `CachedOutput` resource that points at the JSON file. `CachedOutput` has no controller. It is what makes caching and `--resume_from` possible.

**Models.** The workflow does not register models. A model reaches the registry when your task code, running inside the Ray or Spark job, calls the Python `APIRegistryClient`, which creates a `Model` resource through the API server. See [Push Models and Artifacts](../user-guides/train-and-deploy-models/push-models-and-artifacts.md) and [Model Registry Guide](../user-guides/train-and-deploy-models/model-registry-guide.md).

**Status.** Status travels back up the same chain in the reverse direction:

1. The Ray job state is copied by the RayJob controller into `RayJob.status`.
2. The Starlark sensor sees the terminal state, and the task records its progress. Progress reporting is the `report_progress` call in [`python/michelangelo/uniflow/commons.star`](https://github.com/michelangelo-ai/michelangelo/blob/main/python/michelangelo/uniflow/commons.star).
3. On each reconcile, the `ExecuteWorkflowActor` reads the workflow's status from the engine and queries the workflow's `task_progress` handler. It writes the result into the `Execute Workflow` step and its `subSteps`, one per task.
4. The reconciler maps the conditions to `status.state` (`PIPELINE_RUN_STATE_RUNNING`, `SUCCEEDED`, `FAILED`, or `KILLED`) and writes the status.
5. `ma pipeline_run get` and Michelangelo AI Studio read that status through the API server.

If you stop a run with `ma pipeline_run kill`, the CLI sets `spec.kill`. The `ExecuteWorkflowActor` then cancels the workflow, the workflow cleans up its Ray or Spark resources, and the run ends as `KILLED`.

**Observe it:**

```bash
ma pipeline_run get --namespace="<namespace>" --name="<run-name>"
ma pipeline_run get --namespace="<namespace>" --name="<run-name>" --output=yaml
```

## Quick reference

| Hop | Component | Resource written | Where to look |
|-----|-----------|------------------|---------------|
| 1 | `ma` CLI | `CreatePipelineRun` request | CLI output |
| 2 | API server | `PipelineRun` | `ma pipeline_run get`, `deployment/michelangelo-apiserver` logs |
| 3 | PipelineRun controller | `PipelineRun.status.conditions`, `status.steps` | `deployment/michelangelo-controllermgr` logs |
| 4 | Workflow engine | `status.workflowId`, `status.workflowRunId` | Cadence Web or Temporal Web |
| 5 | Worker | Calls to the API server | `deployment/michelangelo-worker` logs |
| 6 | Scheduler, Ray controllers, KubeRay | `RayCluster`, `RayJob` (and KubeRay's own `ray.io` copies) | `kubectl get rayclusters.michelangelo.api,rayjobs.michelangelo.api` (live objects only), Ray dashboard |
| 7 | Controllers, workflow, CLI | `CachedOutput`, `Model` (from your code), `status.state` | `ma pipeline_run get` |

## Where to go next

- [System Architecture](./architecture.md): the components and CRDs this page walks through.
- [Running Uniflow Pipelines](../user-guides/ml-pipelines/running-uniflow.md): write and run the pipelines that start this flow.
- [CLI Reference](../user-guides/reference/cli.md): every `ma` command and flag.
- [API Framework](../operator-guides/api-framework.md): how the API server turns gRPC calls into Kubernetes resources.
- [Jobs Overview](../operator-guides/jobs/index.md): how the scheduler assigns Ray work to compute clusters.
- [Troubleshooting](../operator-guides/operations/troubleshooting.md): what to check when a hop stalls.
