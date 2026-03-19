# Integrate with Your Custom Backend

This guide explains how to extend Michelangelo Inference with custom serving backends, model configuration providers, and traffic routing.

## Overview

Michelangelo Inference uses a plugin-based architecture with three main extension points:

| Interface | Purpose | Reference Implementation |
| --------- | ------- | ------------------------ |
| `Backend` | Provision inference server infrastructure | Triton |
| `ModelConfigProvider` | Manage model configurations | ConfigMap-based |
| `RouteProvider` | Route traffic to models | Gateway API HTTPRoute |

Each interface is designed to be idempotent—implementations should handle repeated calls gracefully.

---

## 1. Backend Interface

The `Backend` interface abstracts inference server provisioning for different frameworks (Triton, vLLM, TensorRT-LLM, etc.).

**Interface:** [`go/components/inferenceserver/backends/interface.go`](../../../go/components/inferenceserver/backends/interface.go)

```go
type Backend interface {
    CreateServer(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServer *v2pb.InferenceServer) (*ServerStatus, error)
    GetServerStatus(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) (*ServerStatus, error)
    DeleteServer(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) error
    IsHealthy(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) (bool, error)
    CheckModelStatus(ctx context.Context, logger *zap.Logger, kubeClient client.Client, httpClient *http.Client, inferenceServerName string, namespace string, modelName string) (bool, error)
}
```

**Reference Implementation:** [`go/components/inferenceserver/backends/triton.go`](../../../go/components/inferenceserver/backends/triton.go)

**Registry:** [`go/components/inferenceserver/backends/registry.go`](../../../go/components/inferenceserver/backends/registry.go)

### To add a new backend:

1. Create a new file (e.g., `torchserve.go`) implementing the `Backend` interface
2. Register it in the `Registry` with the appropriate `BackendType`

---

## 2. ModelConfigProvider Interface

The `ModelConfigProvider` manages model configurations for inference servers. This enables a sidecar pattern where a sidecar container watches the config and loads/unloads models accordingly.

**Interface:** [`go/components/inferenceserver/modelconfig/interface.go`](../../../go/components/inferenceserver/modelconfig/interface.go)

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

**Reference Implementation:** [`go/components/inferenceserver/modelconfig/provider.go`](../../../go/components/inferenceserver/modelconfig/provider.go)

### How It Works

The InferenceServer controller creates/deletes the model config, while the Deployment controller adds/removes individual model entries:

- **InferenceServer Controller** → `CreateModelConfig()`, `DeleteModelConfig()`
- **Deployment Controller** → `AddModelToConfig()`, `RemoveModelFromConfig()`

---

## 3. RouteProvider Interface

The `RouteProvider` manages traffic routing to deployed models.

**Interface:** [`go/components/deployment/route/interface.go`](../../../go/components/deployment/route/interface.go)

```go
type RouteProvider interface {
    EnsureDeploymentRoute(ctx context.Context, logger *zap.Logger, client dynamic.Interface, deploymentName string, namespace string, inferenceServerName string, modelName string) error
    CheckDeploymentRouteStatus(ctx context.Context, logger *zap.Logger, client dynamic.Interface, deploymentName string, namespace string, inferenceServerName string, modelName string) (bool, error)
    DeploymentRouteExists(ctx context.Context, logger *zap.Logger, client dynamic.Interface, deploymentName string, namespace string) (bool, error)
    DeleteDeploymentRoute(ctx context.Context, logger *zap.Logger, client dynamic.Interface, deploymentName string, namespace string) error
}
```

**Reference Implementation:** [`go/components/deployment/route/httproute.go`](../../../go/components/deployment/route/httproute.go)

### Default Behavior

The default implementation creates HTTPRoutes that:

1. Match requests on path `/{inferenceServerName}/{deploymentName}`
2. Rewrite the path to `/v2/models/{modelName}` (Triton V2 inference protocol)
3. Route to the inference server's Service

---

## Best Practices

1. **Idempotency**: All methods should handle repeated calls. Use `errors.IsAlreadyExists` and `client.IgnoreNotFound` appropriately.

2. **Structured Logging**: Use the provided `*zap.Logger` with contextual fields.

3. **Error Handling**: Wrap errors with context using `fmt.Errorf("message: %w", err)`.

4. **Health Checks**: Prefer checking Kubernetes resource status (Deployment conditions) over making HTTP calls when possible.
