# Go Code Style Guide

This guide covers Go code conventions used across the Michelangelo codebase. It complements the [error handling guide](error-handling.md) with broader patterns: package structure, interface design, logging, and test organization.

---

## Package Naming and Structure

### Naming

- Package names are lowercase, single-word, no underscores: `backends`, `scheduler`, `ingester`.
- Package names should match the directory name. The directory `go/components/inferenceserver/backends/` contains `package backends`.
- Avoid stutter: a `Backend` type in package `backends` is referenced as `backends.Backend` — not `backends.BackendInterface`.
- Test files for package `foo` use `package foo` (white-box tests) or `package foo_test` (black-box tests). Do not mix both styles in the same directory without good reason.

### Directory Layout

```
go/
├── cmd/                    # Entry points — one binary per subdirectory
│   ├── apiserver/
│   ├── controllermgr/
│   └── worker/
└── components/             # Reusable packages
    ├── inferenceserver/
    │   └── backends/       # Interface + registry + implementations
    ├── scheduler/
    └── jobs/
```

- `cmd/` packages should be thin: they wire dependencies and call into `components/`.
- Business logic belongs in `components/`, not in `cmd/`.
- Generated code lives in `go/gen/` — never edit files there.

---

## Interface Design

The codebase uses interfaces at subsystem boundaries to make components testable and extensible. Follow these patterns when defining a new interface.

### Define interfaces where they are used, not where they are implemented

```go
// ✅ Good: interface defined in the consumer package
// go/components/scheduler/scheduler.go
package scheduler

// JobQueue is the interface the scheduler depends on.
// Implementations live elsewhere (e.g., kueue/, volcano/).
type JobQueue interface {
    Enqueue(ctx context.Context, job *Job) error
    Dequeue(ctx context.Context) (*Job, error)
}
```

```go
// ❌ Bad: defining the interface in the implementation package
// go/components/kueue/interface.go
package kueue

type KueueJobQueue interface { ... }  // Consumer imports kueue — wrong direction
```

### Keep interfaces small

Prefer small, focused interfaces over large ones. A type that satisfies a small interface is easier to mock and test.

```go
// ✅ Good: single responsibility
type ModelConfigProvider interface {
    GetModelConfig(ctx context.Context, name, namespace string) (*ModelConfig, error)
    UpdateModelConfig(ctx context.Context, config *ModelConfig) error
}

// ❌ Bad: unrelated concerns bundled
type ModelManager interface {
    GetModelConfig(ctx context.Context, ...) (*ModelConfig, error)
    UpdateModelConfig(ctx context.Context, ...) error
    DeleteServer(ctx context.Context, ...) error      // unrelated
    IsHealthy(ctx context.Context, ...) (bool, error) // unrelated
}
```

### Document idempotency requirements

Methods that are called by controllers are typically called repeatedly. Document this explicitly:

```go
// Backend abstracts inference server provisioning for different frameworks (Triton, vLLM, etc.).
// All methods must be idempotent.
type Backend interface {
    // CreateServer provisions infrastructure and returns the current state.
    // Safe to call multiple times — must be a no-op if the server already exists.
    CreateServer(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServer *v2pb.InferenceServer) (*ServerStatus, error)
    ...
}
```

### Registry pattern for extensible sets

When a subsystem supports multiple implementations (backends, schedulers), use a registry:

```go
type Registry struct {
    mu       sync.RWMutex
    backends map[v2pb.BackendType]Backend
}

func (r *Registry) Register(backendType v2pb.BackendType, backend Backend) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.backends[backendType] = backend
}

func (r *Registry) GetBackend(backendType v2pb.BackendType) (Backend, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    b, ok := r.backends[backendType]
    if !ok {
        return nil, fmt.Errorf("backend %q not registered", backendType)
    }
    return b, nil
}
```

---

## Logging Conventions

Michelangelo uses two loggers depending on the context:

| Logger | Package | Used in |
|--------|---------|---------|
| `go.uber.org/zap` | `*zap.Logger` | Most components — direct `zap.Logger` field in structs |
| `sigs.k8s.io/controller-runtime/pkg/log` | `logr.Logger` | Kubernetes controllers (controller-runtime convention) |

### Zap usage

Pass the logger as a parameter (do not use a global logger):

```go
// ✅ Good: logger as parameter
func (b *TritonBackend) CreateServer(ctx context.Context, logger *zap.Logger, ...) error {
    logger.Info("creating triton server",
        zap.String("name", inferenceServer.Name),
        zap.String("namespace", inferenceServer.Namespace))
    ...
}

// ❌ Bad: global logger
func (b *TritonBackend) CreateServer(ctx context.Context, ...) error {
    zap.L().Info("creating triton server")  // global — untestable and not contextual
    ...
}
```

**Field names:** use consistent key names across the codebase.

| Field | Zap key |
|-------|---------|
| Error | `zap.Error(err)` |
| Kubernetes resource name | `zap.String("name", ...)` |
| Kubernetes namespace | `zap.String("namespace", ...)` |
| Operation | `zap.String("operation", ...)` |

### controller-runtime logr usage

Controllers use `log.FromContext(ctx)` to get a logger with request context already attached:

```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    logger := log.FromContext(ctx).WithValues("resource", req.NamespacedName)

    if err := r.doSomething(ctx); err != nil {
        logger.Error(err, "failed to do something",
            "operation", "do_something",
            "namespace", req.Namespace,
            "name", req.Name)
        return ctrl.Result{}, err
    }
    return ctrl.Result{}, nil
}
```

See [Error Handling](error-handling.md#pr-review-checklist) for the required log-and-return pattern in controllers.

### Log levels

| Level | When to use |
|-------|-------------|
| `Info` | Normal operations: resource created, task started, status updated |
| `Error` | Actionable failures: operation failed and needs investigation |
| `Debug` | High-frequency detail useful only during active debugging — off by default |

Do not use `Warn`. Use `Info` for expected transient conditions (rate limits, retries) and `Error` for unexpected failures.

---

## TODO Comments

All `TODO` comments must reference a GitHub issue. The CI `TODO` check enforces this:

```go
// ✅ Passes CI
// TODO(#456): switch to batch API once available

// ❌ Fails CI — golangci-lint godox check
// TODO: switch to batch API
// TODO - handle this edge case
```

To add a TODO:
1. Create a GitHub issue describing the work.
2. Reference it: `TODO(#<issue-number>): brief description`.

---

## Test File Organization

### File naming

```
scheduler.go           → scheduler_test.go
registry.go            → registry_test.go
```

Place test files in the same directory as the code under test. Use the same package name (`package scheduler`) for white-box tests that access unexported identifiers, or `package scheduler_test` for black-box tests.

### Test function naming

```go
// Pattern: Test<Type>_<Method>_<Scenario>
func TestRegistry_GetBackend_NotFound(t *testing.T) { ... }
func TestReconciler_Reconcile_ErrorOnGet(t *testing.T) { ... }
```

### Mock generation

Mocks are generated with `mamockgen` (a wrapper around `mockgen`). Add the generate directive at the top of the interface file:

```go
//go:generate mamockgen Backend
```

Then run:

```bash
cd go && go generate ./components/inferenceserver/backends/...
```

Generated mocks land in `<package>/backendsmocks/` (package name + `mocks` suffix). Import them in tests:

```go
import "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends/backendsmocks"
```

See [Using Go Mocks in Unit Tests](../use-go-mocks-in-unit-test.md) for full mock usage patterns.

### Table-driven tests

Prefer table-driven tests for functions with multiple input/output cases:

```go
func TestValidateModelName(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantErr bool
    }{
        {name: "valid", input: "fraud-detector", wantErr: false},
        {name: "empty", input: "", wantErr: true},
        {name: "too long", input: strings.Repeat("a", 256), wantErr: true},
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            err := validateModelName(tc.input)
            if (err != nil) != tc.wantErr {
                t.Errorf("validateModelName(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
            }
        })
    }
}
```

---

## Related

- [Error Handling](error-handling.md) — error wrapping, logging strategy, PR review checklist
- [Using Go Mocks in Unit Tests](../use-go-mocks-in-unit-test.md) — mock generation and usage
- [Managing Go Dependencies](../manage-go-dependencies.md) — `go mod tidy`, `bazel mod tidy`
- [Testing Strategy](../testing.md) — unit, integration, and E2E test expectations
