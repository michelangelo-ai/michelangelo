---
sidebar_position: 1
sidebar_label: "Key Concepts and Terms"
---

# Go Key Concepts and Terms

This page is the entry point for contributors working on the Go backend. It maps the codebase structure, names the key types and interfaces you will encounter, and defines the terminology used throughout the code and other guides. Read this before diving into a specific guide or making changes to the API server, controller manager, or worker.

---

## Codebase Role

Go is the primary backend language — approximately 299K lines of the codebase. It implements the three main server-side services: API server, controller manager, and worker. The Python SDK and CLI are thin clients; all persistent state and compute orchestration runs in Go.

---

## Directory Structure

```
go/
├── cmd/                      # Binary entry points — one subdirectory per binary
│   ├── apiserver/            # gRPC API server
│   ├── controllermgr/        # Kubernetes controller manager
│   └── worker/               # Cadence workflow and activity workers
├── components/               # Reusable packages with business logic
│   ├── inferenceserver/      # InferenceServer reconciliation, backends, endpoints
│   │   └── backends/         # Backend interface + registry + implementations (Triton, vLLM)
│   ├── jobs/                 # Ray and Spark job client logic
│   │   └── scheduler/        # Job scheduling abstraction and AssignmentStrategy framework
│   ├── pipelinerun/          # PipelineRun actors and workflow execution
│   └── ingester/             # Data ingestion, finalizer logic
├── storage/                  # Database access (MySQL)
└── worker/
    └── plugins/              # Uniflow worker plugins (Ray, Spark, Pipeline)
```

Other top-level directories include `api/`, `auth/`, `base/`, `kubeproto/`, `logging/`, and `thirdparty/`.

**Rule:** `cmd/` packages wire dependencies and call into `components/`. Business logic belongs in `components/`, not `cmd/`. See [Code Style Guide](code-style.md) for package naming and interface design conventions.

---

## Key Services

### API Server (`go/cmd/apiserver/`)

Central gRPC server that acts as the control plane API. It validates resources, stores them as Kubernetes Custom Resource Definitions (CRDs) via the Kubernetes API, and invokes registered API hooks on mutating operations. All Michelangelo AI resources (InferenceServer, Pipeline, RayJob, SparkJob, etc.) flow through the API server before being persisted.

### Controller Manager (`go/cmd/controllermgr/`)

Runs Kubernetes controllers for each ML resource type: RayCluster, SparkJob, InferenceServer, Deployment, Pipeline, PipelineRun, Ingester, and others. Each controller implements a reconcile loop that continuously brings the actual state of the cluster into alignment with the desired state stored in Kubernetes (etcd) and the metadata store (MySQL). Controllers are the primary place where compute resources are created, updated, and deleted.

### Worker (`go/cmd/worker/`, `go/worker/plugins/`)

Hosts Cadence workflow and activity workers. Uniflow plugins in `go/worker/plugins/` extend the worker with domain-specific Starlark builtins — for example, the Ray plugin exposes Ray cluster lifecycle operations as Starlark functions that can be called from a transpiled Pipeline workflow.

---

## Key Types

| Type | Package / Location | Description |
|------|--------------------|-------------|
| `InferenceServer` | `proto-go/api/v2/` | Protobuf resource for a deployed model serving endpoint |
| `Pipeline` | `proto-go/api/v2/` | Protobuf resource representing a Uniflow workflow definition |
| `PipelineRun` | `proto-go/api/v2/` | Single execution of a Pipeline; created on each submission |
| `RayJob` | `proto-go/api/v2/` | Compute job resource for Ray workloads |
| `SparkJob` | `proto-go/api/v2/` | Compute job resource for Spark workloads |
| `Backend` | `go/components/inferenceserver/backends/` | Interface for an inference framework plugin (Triton, vLLM, etc.) |
| `JobQueue` | `go/components/jobs/scheduler/` | Interface for a job scheduling backend; the real extension point is `AssignmentStrategy` in `go/components/jobs/scheduler/framework/` |
| `IPlugin` | `github.com/cadence-workflow/starlark-worker/service` | Interface for a Uniflow worker plugin (defined in the external dependency); implementations live in `go/worker/plugins/` |

---

## Key Patterns

### Reconcile Loop

Controllers implement `Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error)` from [controller-runtime](https://pkg.go.dev/sigs.k8s.io/controller-runtime). The framework calls this method whenever the observed state may differ from the desired state. Implementations must be **idempotent** — calling `Reconcile` multiple times with the same input must produce the same result without side effects.

A reconcile loop typically has three phases: fetch the current resource, compare to the desired state, and actuate any required changes.

```go
// go/components/<resource>/controller.go
import "sigs.k8s.io/controller-runtime/pkg/log"

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    logger := log.FromContext(ctx).WithValues("resource", req.NamespacedName)
    // fetch, compare, actuate — safe to call repeatedly
    return ctrl.Result{}, nil
}
```

### Interface-at-Consumer

Interfaces are defined in the package that uses them, not the package that implements them. This keeps dependencies pointing inward (consumer → interface, implementation → interface) and makes the consumer independently testable.

```go
// go/components/jobs/scheduler/scheduler.go — defined where it is used
type JobQueue interface {
    Enqueue(ctx context.Context, job matypes.SchedulableJob) error
}
// The Scheduler struct in the same package implements this interface.
// AssignmentStrategy (go/components/jobs/scheduler/framework/interface.go) is the extension point for custom scheduling logic.
```

See [Code Style Guide](code-style.md#interface-design) for full interface design conventions.

### Registry Pattern

Extensible subsystems (backends, schedulers, plugins) use a registry: a map from key → implementation, populated at startup via `fx.Options`. Callers look up the registered implementation by key at runtime rather than importing concrete types.

```go
// Registration at startup (simplified)
// tritonBackend = NewTritonBackend(...)
registry.Register(v2pb.BACKEND_TYPE_TRITON, tritonBackend)

// Lookup at call time
backend, err := registry.GetBackend(inferenceServer.Spec.BackendType)
```

### Starlark Execution Model

Python Uniflow workflows are transpiled to [Starlark](https://pkg.go.dev/go.starlark.net/starlark) (`go.starlark.net/starlark`) at submission time. The worker executes this Starlark bytecode and calls Go plugin builtins for each task step. Plugin builtins are registered as Cadence activities, so the execution is durable and resumable across worker restarts.

See [Uniflow Plugin Guide](../../uniflow-plugin-guide.md) for a full walkthrough of building a new plugin, including how to expose Go functions as Starlark builtins.

---

## Go-Specific Terminology

For platform-wide terms (Pipeline, Workflow, Task, etc.), see [TERMINOLOGY.md](../../TERMINOLOGY.md).

| Term | Meaning |
|------|---------|
| **Uniflow** | The Python-authored workflow framework. Workflows are transpiled to Starlark and executed by the worker. |
| **Reconciler** | A Kubernetes controller that brings actual cluster state to the desired state stored in Kubernetes (etcd) and the metadata store (MySQL). Must be idempotent. |
| **CRD** | Custom Resource Definition — Michelangelo AI resources (InferenceServer, Pipeline, RayJob, SparkJob, etc.) are stored as K8s CRDs. |
| **Finalizer** | A Kubernetes mechanism that prevents object deletion until a cleanup step completes. Used by the ingester to drain in-flight data before a resource is removed. |
| **Activity** | A Cadence unit of work, corresponding to a single task step in a Uniflow workflow. Activities are retried independently on failure. |
| **Starlark** | A deterministic, Python-like scripting language used as the intermediate representation for Uniflow workflows. Determinism guarantees replay correctness. |
| **Plugin** | A Go package in `go/worker/plugins/` that extends the Starlark execution environment with domain-specific builtins (e.g., `ray.create_cluster`). |
| **fx** | Dependency injection framework (`go.uber.org/fx`). Components declare their dependencies via `fx.Provide`, and the framework wires them at startup. |
| **logr** | The logging interface from controller-runtime (`sigs.k8s.io/controller-runtime/pkg/log`). Used in some controllers (e.g., the ingester); other controllers and most components use `go.uber.org/zap`. |
| **mamockgen** | Michelangelo AI's wrapper around `mockgen`. Generates Go mocks from interface definitions. Lives at `tools/mamockgen` in the repo. Invoked via `//go:generate mamockgen Backend` (also accepts multiple names, e.g., `//go:generate mamockgen Publisher Provider`). See [Using Go Mocks in Unit Tests](../../use-go-mocks-in-unit-test.md) for setup. |

---

## Generated Code

The following directories contain auto-generated code. Edits made here will be silently overwritten the next time generation runs — make changes to the source definitions instead.

| Directory | Source | Regenerate with |
|-----------|--------|-----------------|
| `proto-go/` | `.proto` files in `proto/` | `tools/gen-proto-go.sh` |

For adding or modifying proto definitions and regenerating bindings, see [How to Write APIs](../../how-to-write-apis.md).

---

## Related

- [Code Style Guide](code-style.md) — package naming, interface design, logging conventions, test organization
- [Error Handling](error-handling.md) — error wrapping, logging strategy, PR review checklist
- [Uniflow Plugin Guide](../../uniflow-plugin-guide.md) — how to build a new Go worker plugin
- [How to Write APIs](../../how-to-write-apis.md) — proto definitions, Gazelle, gRPC code generation
- [Using Go Mocks in Unit Tests](../../use-go-mocks-in-unit-test.md) — mock generation and usage
- [Managing Go Dependencies](../../manage-go-dependencies.md) — `go mod tidy`, `bazel mod tidy`
- [Building from Source](../../building-michelangelo-ai-from-source.md) — environment setup and build commands
