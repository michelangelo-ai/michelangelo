# Contributing to Ray Cluster & Ray Job Controllers

> **Prerequisites:** This guide assumes familiarity with Go, controller-runtime, and Kubernetes CRDs.

This guide is for developers contributing to the Ray controller codebase — adding features, fixing bugs, and extending the Ray integration. For deploying and operating Ray clusters, see the operator documentation (coming soon).

## Architecture Overview

Ray integration in Michelangelo spans two execution paths: a **controller path** that manages cluster and job lifecycle via Kubernetes reconciliation, and a **workflow path** where Starlark plugins orchestrate Ray operations through Cadence/Temporal activities.

### Codebase Layout

```
proto/api/v2/
  ray_cluster.proto          # RayCluster CRD definition (states, spec, status)
  ray_job.proto              # RayJob CRD definition (states, spec, status)

go/components/ray/
  module.go                  # FX module — exports cluster.Module + job.Module
  cluster/
    controller.go            # RayCluster reconciler (lifecycle management)
    controller_test.go       # Table-driven reconciler tests
    config.go                # Controller config (QPS/Burst via YAML)
    module.go                # FX wiring — registers reconciler with controller-manager
  job/
    controller.go            # RayJob reconciler (depends on RayCluster)
    controller_test.go       # Table-driven reconciler tests
    module.go                # FX wiring — reuses cluster.Config for QPS/Burst
  kuberay/
    groupversion_info.go     # SchemeGroupVersion (ray.io/v1)
    register.go              # Registers KubeRay CRD types into scheme
    rest_client.go           # REST client for KubeRay API

go/worker/
  plugins/ray/
    plugin.go                # Starlark plugin entry point (ID = "ray")
    starlark_module.go       # Starlark builtins: create_cluster, terminate_cluster, create_job
    starlark_module_test.go  # Starlark plugin tests (Cadence test environment)
  activities/ray/
    ray_activities.go        # Cadence/Temporal activities (CRUD + sensors)

python/michelangelo/uniflow/plugins/ray/
    task.py                  # RayTask dataclass (user-facing config)
    task.star                # Starlark orchestration (cluster lifecycle + job execution)
    io.py                    # RayDatasetIO (Parquet read/write via fsspec or PyArrow)
```

### Dependency Injection

Controllers are wired via [Uber FX](https://github.com/uber-go/fx) modules:

- `go/components/ray/module.go` — combines `cluster.Module` and `job.Module`
- `cluster/module.go` — provides `newConfig` and invokes `register()`, which constructs the `Reconciler` with all dependencies (logger, API handler factory, env, scheduler queue, federated client, cluster cache) and registers it with the controller manager
- `job/module.go` — invokes `register()`, which reuses `cluster.Config` for QPS/Burst settings and constructs the job `Reconciler` with `mgr.GetClient()` (standard controller-runtime client, not the custom API handler)

### States vs Conditions

The Ray controllers track two separate concepts. Confusing them is the most common source of bugs. The tables below are your reference when adding or modifying states. The "Set by" column tells you which method to modify.

**States** are the resource's provisioning/runtime status, defined as proto enums and updated based on external system state (KubeRay).

**Conditions** are the controller's internal lifecycle checkpoints, tracking what the controller has done.

#### RayCluster States

Defined in `proto/api/v2/ray_cluster.proto`:

| State | Value | Set by | Meaning |
|-------|-------|--------|---------|
| `INVALID` | 0 | Default | Zero value, not yet processed |
| `PROVISIONING` | 1 | Cluster controller | Cluster creation initiated via federated client |
| `READY` | 2 | `applyRayClusterStatus` | KubeRay reports cluster is ready |
| `TERMINATING` | 3 | `applyRayClusterStatus` | Cluster is shutting down |
| `TERMINATED` | 4 | `cleanupCluster` | Cluster resources deleted |
| `FAILED` | 5 | `applyRayClusterStatus` | KubeRay reports failure |
| `UNKNOWN` | 6 | `applyRayClusterStatus` | KubeRay state unknown |
| `UNHEALTHY` | 7 | `applyRayClusterStatus` | KubeRay reports unhealthy |

#### RayCluster Conditions

Defined as constants in `cluster/controller.go` (imported from `jobs/common/constants`):

| Condition | Set when | Code path |
|-----------|----------|-----------|
| `Enqueued` | Cluster submitted to scheduler queue | `enqueueIfRequired` |
| `Scheduled` | Scheduler assigns a compute cluster | External scheduler (not in this controller) |
| `Launched` | Federated client creates the cluster | `Reconcile` (after `CreateJobCluster`) |
| `Killing` | Termination initiated | `setClusterKillIfRequired` |
| `Killed` | Cluster resources cleaned up | `cleanupCluster` |
| `Succeeded` | Terminal outcome determined | `setClusterSuccessConditionIfRequired` / `applyRayClusterStatus` |

**How they interact:** The controller sets `state = PROVISIONING` when it sets `Launched = TRUE`. After that, the state transitions independently based on KubeRay status polling via `applyRayClusterStatus`. The `Succeeded` condition (TRUE or FALSE) triggers the termination flow via `shouldTerminateCluster`, regardless of the current state.

#### RayJob States

Defined in `proto/api/v2/ray_job.proto`:

| State | Value | Set by | Meaning |
|-------|-------|--------|---------|
| `INVALID` | 0 | Default | Zero value |
| `INITIALIZING` | 1 | Job controller | Cluster not ready or job just created |
| `RUNNING` | 2 | `applyRayJobStatus` | KubeRay reports job running |
| `SUCCEEDED` | 3 | `applyRayJobStatus` | Job completed successfully |
| `FAILED` | 4 | Job controller / `applyRayJobStatus` | Job failed or cluster not found |
| `KILLED` | 5 | `applyRayJobStatus` | Job was stopped |

The job controller only uses one condition: `Launched` (set to TRUE after `CreateJob` succeeds). Terminal states (`SUCCEEDED`, `FAILED`, `KILLED`) stop requeueing and mark the resource as immutable via `utils.MarkImmutable`.

## Testing Foundations

Both controllers share the same test infrastructure. This section covers shared setup; controller-specific examples appear in each section below.

### Setup Pattern

Every test begins with scheme registration, a fake client, and gomock:

```go
// Scheme — identical in both controller test files
scheme := runtime.NewScheme()
kubescheme.AddToScheme(scheme)
v2pb.AddToScheme(scheme)

// Fake client with status subresource support
objects := tc.setup()
fakeClient := fake.NewClientBuilder().WithScheme(scheme).
    WithObjects(objects...).WithStatusSubresource(objects...).Build()

// gomock for the federated client (generated mock from jobs/client/clientmocks/)
mockCtrl := gomock.NewController(t)
defer mockCtrl.Finish()
mockFedClient := clientmocks.NewMockFederatedClient(mockCtrl)
```

### Shared Test Double: mockClusterCache

Both test files define an identical `mockClusterCache` implementing `RegisteredClustersCache`. Use `addCluster` in `setupMocks` to make clusters available to the reconciler:

```go
// Implements RegisteredClustersCache — use addCluster in setupMocks to populate
type mockClusterCache struct{ clusters map[string]*v2pb.Cluster }
func (m *mockClusterCache) GetCluster(name string) *v2pb.Cluster { return m.clusters[name] }
func (m *mockClusterCache) addCluster(name string, c *v2pb.Cluster) { m.clusters[name] = c }
```

### Table-Driven Test Structure

Both controllers use table-driven tests with these fields:

| Field | Purpose |
|-------|---------|
| `setup` | Create K8s objects (RayCluster, RayJob) at a specific lifecycle stage |
| `setupMocks` | Configure gomock expectations and mock cache entries |
| `expectedState` | Assert the resource's state after reconciliation |
| `postCheck` | Verify requeue behavior (`res.RequeueAfter`) |
| `verifyConditions` | Assert condition updates on the reconciled resource |

The test loop follows Arrange/Act/Assert: build objects and mocks, call `Reconcile`, then assert on the updated resource.

## Ray Cluster Controller

**File:** `go/components/ray/cluster/controller.go`

The cluster controller manages the full lifecycle of RayCluster resources: enqueuing for scheduling, launching via the federated client, monitoring via KubeRay status polling, and termination cleanup.

### Reconciler Dependencies

```go
type Reconciler struct {
    api.Handler                                      // Custom API client (Get, UpdateStatus, etc.)
    logger            logr.Logger
    apiHandlerFactory apiHandler.Factory
    env               env.Context
    schedulerQueue    scheduler.JobQueue              // Enqueue clusters for scheduling
    federatedClient   jobsclient.FederatedClient      // Cross-cluster CRUD operations
    clusterCache      jobscluster.RegisteredClustersCache  // Lookup assigned clusters
}
```

Note: The cluster controller uses `api.Handler` (a custom API client wrapper), not the standard `client.Client`.

### Reconciliation Flow

```
Reconcile()
  ├── shouldTerminateCluster? → processClusterTermination
  │     ├── setClusterSuccessConditionIfRequired
  │     ├── setClusterKillIfRequired
  │     └── cleanupCluster (delete via federated client)
  │
  ├── enqueueIfRequired → schedulerQueue.Enqueue()
  │     └── Sets Enqueued condition = TRUE
  │
  ├── getClusterIfScheduled → clusterCache.GetCluster()
  │     └── Returns nil if not yet scheduled (requeue)
  │
  ├── Not launched? → federatedClient.CreateJobCluster()
  │     ├── Success: state = PROVISIONING, Launched = TRUE
  │     ├── AlreadyExists: treat as launched
  │     └── Error: state = FAILED, Succeeded = FALSE
  │
  ├── getClusterStatus → federatedClient.GetJobClusterStatus()
  │
  └── applyRayClusterStatus (updates state based on KubeRay)
        ├── READY → requeue stops
        ├── FAILED/UNHEALTHY → Succeeded = FALSE (triggers termination)
        └── Other → continue monitoring (requeue after 10s)
```

### Key Patterns

**Status updates** always use `jobsutils.UpdateStatusWithRetries` with a mutation function:

```go
jobsutils.UpdateStatusWithRetries(ctx, r, &rayCluster,
    func(obj client.Object) {
        cluster := obj.(*v2pb.RayCluster)
        // Mutate cluster status here
    },
    &metav1.UpdateOptions{},
)
```

**Termination** is a 3-step process in `processClusterTermination`:
1. Set `Succeeded` condition (TRUE for success, FALSE for failure)
2. Set `Killing` condition to TRUE
3. Clean up resources via federated client, then set `Killed = TRUE` and `state = TERMINATED`

**Requeue** uses a constant 10-second interval: `requeueAfter = time.Second * 10`. Reconciliation stops (no requeue) only when the cluster reaches `READY` or a terminal state.

### Configuration

Controller config is loaded from YAML via `cluster/config.go`:

```go
const configKey = "controllers.rayCluster"

type Config struct {
    QPS   float32 `yaml:"k8sQps"`
    Burst int     `yaml:"k8sBurst"`
}
```

Add new configurable parameters here when your feature needs runtime configuration. The job controller reuses this same `Config` struct.

### Testing Cluster Changes

The cluster controller has two unique test doubles beyond the shared patterns. These examples use the shared mock patterns from [Testing Foundations](#testing-foundations).

**mockSchedulerQueue** — wraps the scheduler queue with a configurable enqueue function:

```go
type mockSchedulerQueue struct {
    enqueueFunc func(ctx context.Context, job matypes.SchedulableJob) error
}
```

**mockAPIHandler** — wraps the fake client to implement `api.Handler` (needed because the cluster controller uses the custom API client, not `client.Client`):

```go
type mockAPIHandler struct {
    client.Client
}

func (m *mockAPIHandler) Get(ctx context.Context, namespace, name string,
    opts *metav1.GetOptions, obj client.Object) error {
    return m.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, obj)
}
// Also implements: UpdateStatus, Update, Create, Delete, List, DeleteCollection, Watch
```

**Example: Testing a new cluster state**

To test a new state returned by KubeRay, add a test case that sets up a launched cluster and configures `GetJobClusterStatus` to return your new state:

```go
{
    name: "Cluster launched - your new state",
    setup: func() []client.Object {
        // Create cluster with Enqueued, Scheduled, Launched conditions all TRUE
        // and Assignment pointing to a cluster
    },
    setupMocks: func(mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache, msq *mockSchedulerQueue) {
        mcc.addCluster(assignedCluster, &v2pb.Cluster{...})
        mfc.EXPECT().GetJobClusterStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(
            &matypes.JobClusterStatus{
                Ray: &v2pb.RayClusterStatus{State: v2pb.YOUR_NEW_STATE},
            }, nil)
    },
    // Assert expected state, requeue behavior, and condition updates
}
```

**Example: Testing termination**

Termination tests set `TerminationSpec` on the cluster spec and verify the 3-step flow:

```go
setup: func() []client.Object {
    cluster := &v2pb.RayCluster{
        Spec: v2pb.RayClusterSpec{
            Termination: &v2pb.TerminationSpec{
                Type:   v2pb.TERMINATION_TYPE_SUCCEEDED,
                Reason: "job completed",
            },
        },
    }
    // ...
},
verifyConditions: func(t *testing.T, cluster *v2pb.RayCluster) {
    // Verify SucceededCondition is TRUE, KilledCondition is TRUE, state is TERMINATED
},
```

## Ray Job Controller

**File:** `go/components/ray/job/controller.go`

The job controller manages RayJob resources. Unlike the cluster controller, it does not handle scheduling or enqueuing — instead, it depends on an existing RayCluster being in `READY` state.

### Reconciler Dependencies

```go
type Reconciler struct {
    client.Client                                    // Standard controller-runtime client
    logger          logr.Logger
    federatedClient jobsclient.FederatedClient
    clusterCache    jobscluster.RegisteredClustersCache
    env             env.Context
}
```

Note: The job controller uses `client.Client` directly, not `api.Handler`. This is a key difference from the cluster controller.

### Reconciliation Flow

```
Reconcile()
  ├── Get RayJob (not found → return, error → requeue)
  │
  ├── Cluster spec nil? → state = FAILED, message = "cluster is not set"
  │
  └── reconcileRayJobWithCluster
        ├── fetchRayCluster → Get referenced RayCluster
        │     └── Not found → state = FAILED
        │
        ├── ensureClusterReady
        │     └── Not READY → state = INITIALIZING (requeue)
        │
        ├── Not launched? → createRayJobIfNotLaunched
        │     ├── getAssignedCluster (from RayCluster status)
        │     ├── federatedClient.CreateJob
        │     └── Set Launched = TRUE, state = INITIALIZING
        │
        └── Launched → updateJobStatusIfLaunched
              ├── federatedClient.GetJobStatus
              └── applyRayJobStatus (maps KubeRay status to RayJob state)
```

### Key Differences from Cluster Controller

| Aspect | Cluster Controller | Job Controller |
|--------|-------------------|----------------|
| Client | `api.Handler` (custom) | `client.Client` (standard) |
| Scheduling | Enqueues to `scheduler.JobQueue` | No scheduling — depends on cluster |
| Conditions | 6 conditions (Enqueued through Killed) | 1 condition (Launched) |
| Condition references | Constants (`EnqueuedCondition`, etc.) | Constants (`constants.LaunchedCondition`) |
| Terminal handling | Termination flow (3 steps) | `MarkImmutable` on terminal state |
| Requeue interval | `requeueAfter = time.Second * 10` | `requeueAfter = time.Second * 10` |
| Status updates | `jobsutils.UpdateStatusWithRetries` | Direct `r.Status().Update()` |

### Terminal State Handling

When a job reaches a terminal state, it stops requeueing and is marked immutable:

```go
func isTerminalRayJobState(state v2pb.RayJobState) bool {
    switch state {
    case v2pb.RAY_JOB_STATE_FAILED, v2pb.RAY_JOB_STATE_SUCCEEDED, v2pb.RAY_JOB_STATE_KILLED:
        return true
    }
    return false
}
```

Before the status update, terminal jobs are marked with `utils.MarkImmutable(&rayJob)` to prevent further modifications.

### Testing Job Changes

Job tests require **two objects** in setup — both the RayJob and its referenced RayCluster. These examples use the shared mock patterns from [Testing Foundations](#testing-foundations).

```go
setup: func() []client.Object {
    rayJob := &v2pb.RayJob{
        Spec: v2pb.RayJobSpec{
            Cluster: &apipb.ResourceIdentifier{
                Name:      "existing-cluster",
                Namespace: testNamespace,
            },
        },
    }
    cluster := &v2pb.RayCluster{
        Status: v2pb.RayClusterStatus{
            State: v2pb.RAY_CLUSTER_STATE_READY,
            Assignment: &v2pb.AssignmentInfo{Cluster: assignedCluster},
        },
    }
    return []client.Object{rayJob, cluster}
}
```

The reconciler is constructed differently from the cluster controller (no `api.Handler`, no scheduler queue):

```go
// Job reconciler — uses standard client.Client, no scheduler queue or api.Handler
r := &Reconciler{
    Client:          fakeClient,       // Standard controller-runtime fake client
    federatedClient: mockFedClient,    // gomock for cross-cluster operations
    clusterCache:    mockCache,        // mockClusterCache from shared pattern
}
```

**Example: Testing cluster dependency**

The "cluster is not ready" test case demonstrates how the job controller handles the cross-resource dependency:

```go
{
    name: "cluster is not ready",
    setup: func() []client.Object {
        // RayJob with cluster reference + RayCluster in PROVISIONING state
    },
    expectedState:   v2pb.RAY_JOB_STATE_INITIALIZING,
    expectedMessage: "cluster default/existing-cluster is not ready",
    postCheck: func(res ctrl.Result) {
        assert.Equal(t, requeueAfter, res.RequeueAfter)  // Will retry
    },
}
```

**Example: Testing terminal states**

Terminal state tests verify that requeueing stops and the immutable marker is applied:

```go
{
    name: "job succeeded",
    setupMocks: func(...) {
        mfc.EXPECT().GetJobStatus(...).Return(&jobtypes.JobStatus{
            Ray: &v2pb.RayJobStatus{
                State:     v2pb.RAY_JOB_STATE_SUCCEEDED,
                JobStatus: "SUCCEEDED",
            },
        }, nil)
    },
    expectedState: v2pb.RAY_JOB_STATE_SUCCEEDED,
    postCheck: func(res ctrl.Result) {
        assert.Equal(t, time.Duration(0), res.RequeueAfter)  // No requeue
    },
}
```

## Starlark Plugin & Activities

The workflow execution path is separate from the controller path. Users write Python workflows with `RayTask` configuration; at submission time, the workflow is transpiled to Starlark and executed on a Cadence/Temporal worker.

### Plugin Entry Point

**File:** `go/worker/plugins/ray/plugin.go`

The plugin implements `service.IPlugin` and exposes three Starlark builtins:

- `ray.create_cluster(spec, timeout_seconds?)` — creates a RayCluster and polls for readiness
- `ray.terminate_cluster(name, namespace, reason, terminateType)` — terminates a cluster
- `ray.create_job(entrypoint, ray_job_namespace?, ray_job_name?)` — submits a job and polls for completion

Each builtin follows the same pattern:
1. Parse args via `starlark.UnpackArgs`
2. Convert Starlark dict to Go proto via `utils.AsGo`
3. Execute a Cadence/Temporal activity via `workflow.ExecuteActivity`
4. Poll with sensor activities using retry policies (`poll = 10` seconds)
5. Convert result back to Starlark via `utils.AsStar`

> **Note:** The plugin has a `TODO` ([#559](https://github.com/michelangelo-ai/michelangelo/issues/559)) indicating it is still partially in progress.

### Activities

**File:** `go/worker/activities/ray/ray_activities.go`

Activities are the bridge between the Starlark plugin and the gRPC services:

| Activity | Service Call | Purpose |
|----------|-------------|---------|
| `CreateRayCluster` | `RayClusterServiceYARPCClient.CreateRayCluster` | Create cluster, return activity ID |
| `CreateRayJob` | `RayJobServiceYARPCClient.CreateRayJob` | Create job |
| `GetRayCluster` | `GetRayCluster` | Retrieve cluster details |
| `GetRayJob` | `GetRayJob` | Retrieve job details |
| `SensorRayClusterReadiness` | `GetRayCluster` | Poll until READY or terminal |
| `SensorRayJob` | `GetRayJob` | Poll until terminal state |
| `TerminateCluster` | `GetRayCluster` + `UpdateRayCluster` | Set termination spec with retry-on-conflict |

Sensor activities use a retry pattern: they return `yarpcerrors.CodeFailedPrecondition` (retryable) when the resource is not yet ready, and `yarpcerrors.CodeInternal` (non-retryable) when it reaches a terminal failure state.

### Python Layer

**`task.py`** — `RayTask` dataclass extends `TaskConfig` with head/worker resource fields (CPU, memory, disk, GPU, instances). The `pre_run()` hook calls `ray.init()` and `post_run()` calls `ray.shutdown()`.

**`io.py`** — `RayDatasetIO` handles Parquet read/write for `ray.data.Dataset` objects. Supports both fsspec and PyArrow filesystem backends, controlled by the `UF_PLUGIN_RAY_USE_FSSPEC` environment variable.

**`task.star`** — Starlark orchestration that calls the Go plugin builtins. Handles cluster spec construction, resource overrides from environment variables, caching, retry logic, and progress reporting. The `task()` function merges user-defined resource fields with environment variable overrides (following the `RAY_OVERRIDE_*` pattern), then `execute_ray_task()` orchestrates the full lifecycle: create the cluster, wait for readiness, submit the job, poll for completion, and report results. This is the most complex file in the plugin.

### Testing Starlark Plugins

Plugin tests use the `service.TestSuite` framework with a Cadence test environment:

```go
func (r *Test) SetupTest() {
    // Create Cadence test environment with the Ray plugin loaded
    r.env = r.NewTestEnvironment(r.T(), &service.TestEnvironmentParams{
        RootDirectory: "testdata",           // Directory containing .star test files
        Plugins: map[string]service.IPlugin{
            "ray": Plugin,                   // Register the Ray Starlark plugin
        },
    })
}
```

Tests register activities, set up mock expectations, and execute Starlark test functions:

```go
// Mock the CreateRayCluster activity to return a test cluster
env.OnActivity(ray.Activities.CreateRayCluster, mock.Anything, mock.Anything).
    Return(func(ctx context.Context, req v2pb.CreateRayClusterRequest) (*ray.CreateRayClusterActivityResponse, error) {
        return &ray.CreateRayClusterActivityResponse{RayCluster: rayCluster, ActivityID: ""}, nil
    })

// Execute the Starlark test function from testdata/test.star
r.env.Cadence.ExecuteFunction("/test.star", "test_create_cluster", nil, nil, nil)
```

## KubeRay Integration

**Directory:** `go/components/ray/kuberay/`

This package provides a thin REST client layer for interacting with KubeRay CRDs on remote clusters. It consists of three small files:

- **`groupversion_info.go`** — defines `SchemeGroupVersion = schema.GroupVersion{Group: "ray.io", Version: "v1"}`
- **`register.go`** — registers `RayCluster` and `RayClusterList` from the `ray-project/kuberay` package into the scheme
- **`rest_client.go`** — creates a Kubernetes REST client configured for the `ray.io` API group

Modify this package to add support for new KubeRay CRD types (e.g., `RayService`). To add a new type, register it in `addKnownTypes` in `register.go`.

## Common Tasks

### Adding a New RayCluster Configuration Option

1. Add the field to `RayClusterSpec` in `proto/api/v2/ray_cluster.proto`
2. Regenerate Go code: `tools/gen-proto-go.sh`
3. Handle the new field in the cluster controller's reconciliation logic
4. If the field needs runtime configuration, add it to `Config` in `cluster/config.go`
5. Add test cases covering the new field
6. If exposed to users via Starlark, update `ray_cluster_spec()` in `task.star` and `RayTask` in `task.py`

### Adding a New RayJob State

1. Add the new state to the `RayJobState` enum in `proto/api/v2/ray_job.proto`
2. Regenerate Go code: `tools/gen-proto-go.sh`
3. Update `isTerminalRayJobState` in `job/controller.go` if the state is terminal
4. Handle the new state in `applyRayJobStatus`
5. If the state is terminal, update `SensorRayJob` in `ray_activities.go` — the sensor has terminal-state early-exit logic that must include the new state
6. Add test cases with `GetJobStatus` returning the new state
7. Update `report_ray_task_result` in `task.star` if the Starlark layer needs to handle it

### Modifying Cluster Provisioning Flow

1. Identify the relevant method in the `Reconcile` flow (see flow diagram above)
2. Modify condition updates using `jobsutils.GetCondition` and `jobsutils.UpdateCondition`
3. Wrap status mutations in `jobsutils.UpdateStatusWithRetries`
4. Add or update test cases — set up the cluster at the right lifecycle stage via conditions in `setup()`
5. Configure `setupMocks` with the appropriate federated client expectations

### Adding a New Starlark Function

1. Add the builtin to `newModule()` in `go/worker/plugins/ray/starlark_module.go`:
   ```go
   "your_function": starlark.NewBuiltin("your_function", m.yourFunction).BindReceiver(m),
   ```
2. Implement the method following the pattern: `UnpackArgs` → `AsGo` → `ExecuteActivity` → `AsStar`
3. If it needs a new activity, add it to `go/worker/activities/ray/ray_activities.go`
4. Add a test in `starlark_module_test.go` using the Cadence test environment
5. Create a test `.star` file in the `testdata/` directory

### Modifying Python Task Resources

1. Add or modify fields in the `RayTask` dataclass in `task.py`
2. Update `ray_cluster_spec()` or `task()` in `task.star` to pass the new field
3. Add environment variable overrides in `task.star` if needed (following the `RAY_OVERRIDE_*` pattern)
4. If the field affects the proto, update `RayClusterSpec` or `RayJobSpec` accordingly

### Adding KubeRay CRD Types

1. Add the new type to `addKnownTypes` in `go/components/ray/kuberay/register.go`:
   ```go
   scheme.AddKnownTypes(SchemeGroupVersion,
       &rayv1.RayCluster{},
       &rayv1.RayClusterList{},
       &rayv1.YourNewType{},      // Add here
       &rayv1.YourNewTypeList{},   // Add here
   )
   ```
2. Ensure the `ray-project/kuberay` dependency includes the type
3. If the type needs a REST client, follow the pattern in `rest_client.go`
4. Add a controller in `go/components/ray/` if the type needs lifecycle management

## Further Reading

- [How to Write APIs](./how-to-write-apis.md) — Proto compilation, gRPC code generation, Bazel builds
- [Developing Uniflow Plugins](./uniflow-plugin-guide.md) — End-to-end plugin development (Go → Starlark → Python)
- [K8s Controller Best Practices](./how-to-write-apis.md#1-ml-entities-in-proto-files) — Links to kubebuilder, K8s API conventions, and controller pitfalls
- [kubebuilder Book](https://book.kubebuilder.io/) — Controller-runtime fundamentals
- [Starlark Language Spec](https://github.com/google/starlark-go/blob/master/doc/spec.md) — Starlark reference
