# Integrate with Your Custom Backend

This guide explains how to extend Michelangelo AI Inference with custom serving backends, model configuration providers, and traffic routing.

## Overview

Michelangelo AI Inference uses a plugin-based architecture with three main extension points:

| Interface | Purpose | Reference Implementation |
| --------- | ------- | ------------------------ |
| `Backend` | Provision inference server infrastructure | Triton |
| `ModelConfigProvider` | Manage model configurations | ConfigMap-based |
| `Manager` | Route traffic to models | Gateway API HTTPRoute |

Each interface is designed to be idempotent—implementations should handle repeated calls gracefully.

---

## 1. Backend Interface

The `Backend` interface abstracts inference server provisioning for different frameworks (Triton, vLLM, TensorRT-LLM, etc.).

**Interface:** [`go/components/inferenceserver/backends/interface.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/inferenceserver/backends/interface.go)

```go
type Backend interface {
    CreateServer(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServer *v2pb.InferenceServer) (*ServerStatus, error)
    GetServerStatus(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) (*ServerStatus, error)
    DeleteServer(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) error
    IsHealthy(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) (bool, error)
    CheckModelStatus(ctx context.Context, logger *zap.Logger, kubeClient client.Client, httpClient *http.Client, inferenceServerName string, namespace string, modelName string) (bool, error)
}
```

**Reference Implementation:** [`go/components/inferenceserver/backends/triton.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/inferenceserver/backends/triton.go)

**Registry:** [`go/components/inferenceserver/backends/registry.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/inferenceserver/backends/registry.go)

### To add a new backend:

1. Create a new file (e.g., `torchserve.go`) implementing the `Backend` interface
2. Register it in the `Registry` with the appropriate `BackendType`

---

## 2. ModelConfigProvider Interface

The `ModelConfigProvider` manages model configurations for inference servers. This enables a sidecar pattern where a sidecar container watches the config and loads/unloads models accordingly.

**Interface:** [`go/components/inferenceserver/modelconfig/interface.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/inferenceserver/modelconfig/interface.go)

```go
type ModelConfigProvider interface {
    CreateModelConfig(ctx context.Context, logger *zap.Logger, kubeclient client.Client, inferenceServerName string, namespace string, labels map[string]string, annotations map[string]string) error
    CheckModelConfigExists(ctx context.Context, logger *zap.Logger, kubeclient client.Client, inferenceServerName string, namespace string) (bool, error)
    DeleteModelConfig(ctx context.Context, logger *zap.Logger, kubeclient client.Client, inferenceServerName string, namespace string) error
    GetModelsFromConfig(ctx context.Context, logger *zap.Logger, kubeclient client.Client, inferenceServerName string, namespace string) ([]ModelConfigEntry, error)
    AddModelToConfig(ctx context.Context, logger *zap.Logger, kubeclient client.Client, inferenceServerName string, namespace string, entry ModelConfigEntry) error
    RemoveModelFromConfig(ctx context.Context, logger *zap.Logger, kubeclient client.Client, inferenceServerName string, namespace string, modelName string) error
}
```

**Reference Implementation:** [`go/components/inferenceserver/modelconfig/provider.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/inferenceserver/modelconfig/provider.go)

### How It Works

The InferenceServer controller creates/deletes the model config, while the Deployment controller adds/removes individual model entries:

- **InferenceServer Controller** → `CreateModelConfig()`, `DeleteModelConfig()`
- **Deployment Controller** → `AddModelToConfig()`, `RemoveModelFromConfig()`

---

## 3. Manager Interface

The `Manager` interface manages traffic routing and routing rules for deployed models.

**Interface:** [`go/components/common/routing/interface.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/common/routing/interface.go)

```go
type Manager interface {
    Create(ctx context.Context, client dynamic.Interface, name, namespace string, config RouteConfig) error
    Exists(ctx context.Context, client dynamic.Interface, name, namespace string) (bool, error)
    Delete(ctx context.Context, client dynamic.Interface, name, namespace string) error
    AddRules(ctx context.Context, client dynamic.Interface, name, namespace string, rules ...Rule) error
    RemoveRules(ctx context.Context, client dynamic.Interface, name, namespace string, matchPaths ...string) error
    RuleExists(ctx context.Context, client dynamic.Interface, name, namespace string, rule Rule) (bool, error)
}
```

**Reference Implementation:** [`go/components/common/routing/gatewayapi/manager.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/common/routing/gatewayapi/manager.go)

### Default Behavior

The default implementation uses Gateway API HTTPRoute resources:

- **`Create`** — creates an HTTPRoute for the given name/namespace using the supplied `RouteConfig`
- **`AddRules` / `RemoveRules`** — incrementally adds or removes path-matching rules on an existing HTTPRoute; `RemoveRules` matches by path prefix
- **`Exists` / `RuleExists`** — idempotency checks used by controllers before creating or adding rules

---

## Best Practices

1. **Idempotency**: All methods should handle repeated calls. Use `errors.IsAlreadyExists` and `client.IgnoreNotFound` appropriately.

2. **Structured Logging**: Use the provided `*zap.Logger` with contextual fields.

3. **Error Handling**: Wrap errors with context using `fmt.Errorf("message: %w", err)`.

4. **Health Checks**: Prefer checking Kubernetes resource status (Deployment conditions) over making HTTP calls when possible.
