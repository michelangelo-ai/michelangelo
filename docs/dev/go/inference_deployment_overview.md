# Inference Server and Deployment Controllers

This document provides a high-level overview of how the inference server and deployment controllers work together to serve ML models.

---

## 1. How the Inference Server Works

The inference server is a Kubernetes-native controller that manages model-serving infrastructure. It follows the standard Kubernetes operator pattern:

1. **Custom Resource Definition (CRD)**: Users create an `InferenceServer` resource describing the desired serving configuration (backend type, resources, replicas).

2. **Controller Reconciliation**: The inference server controller watches for changes to `InferenceServer` resources. When a change is detected, it reconciles the actual state to match the desired state by:
   - Creating the underlying Kubernetes resources (Deployment, Service)
   - Setting up Gateway API routing (HTTPRoute) for external access
   - Monitoring health and updating status

3. **Native Kubernetes Primitives**: Rather than custom infrastructure, the controller creates standard Kubernetes objects:
   - A **Deployment** runs the inference server pods
   - A **Service** provides cluster-internal networking
   - An **HTTPRoute** exposes the service through the Gateway API

This approach means model serving benefits from Kubernetes' built-in capabilities: pod scheduling, rolling updates, health checks, and resource management.

---

## 2. Deployment Controller and ConfigMap Coordination

The deployment controller handles model rollouts to inference servers using a **ConfigMap-based coordination pattern**:

### How it works:

1. **Shared ConfigMap**: Each inference server has an associated ConfigMap (named `<inference-server>-model-config`) that holds the list of models to serve.

2. **Deployment Controller Updates ConfigMap**: When a user creates or updates a `Deployment` resource with a new model version, the deployment controller:
   - Adds the new model entry to the ConfigMap
   - Waits for the model to become ready
   - Removes old model versions after successful rollout

3. **Inference Server Syncs Models**: The inference server reads the ConfigMap and issues load/unload commands to the underlying server process. This happens without restarting pods—models are loaded dynamically.

### Coordination Flow:

```
User creates Deployment
        │
        ▼
Deployment Controller writes model to ConfigMap
        │
        ▼
Inference Server detects ConfigMap change
        │
        ▼
Inference Server loads model into serving runtime
        │
        ▼
Deployment Controller verifies model is ready
        │
        ▼
Old model version cleaned up from ConfigMap
```

This decoupled design allows the deployment controller to focus on rollout logic while the inference server handles the actual model loading.

---

## 3. Current Backend: Triton Inference Server

The current implementation uses **NVIDIA Triton Inference Server** as the serving backend. Triton supports multiple ML frameworks (TensorFlow, PyTorch, ONNX, etc.) and provides features like dynamic batching and model versioning.

### Supporting Multiple Backends

The system is designed to support different inference server implementations through two abstraction layers:

1. **Gateway Interface**: Defines operations that any backend must support:
   - `CreateInfrastructure` / `DeleteInfrastructure` - manage serving pods
   - `LoadModel` / `UnloadModel` - dynamic model management
   - `CheckModelStatus` / `IsHealthy` - health monitoring

2. **Backend Interface**: Each backend (Triton, TensorFlow Serving, etc.) implements the Gateway interface with its specific logic.

3. **Plugin Registry**: Backends register themselves at startup. The controller selects the appropriate backend based on the `backendType` specified in the `InferenceServer` resource.

To add a new backend:
- Implement the `Backend` interface with the vendor-specific logic
- Register it in the plugin registry
- Users can then specify the new backend type in their `InferenceServer` resources

---

## 4. Controller Plugins

Both controllers use a plugin architecture to separate core reconciliation logic from strategy-specific behavior.

### Deployment Controller Plugins

The deployment controller uses a **rolling rollout strategy** with the following actors:

| Actor | Purpose |
|-------|---------|
| **ModelSyncActor** | Adds the new model to the ConfigMap and waits for it to be ready in the inference server |
| **ModelCleanupActor** | Removes old model versions from the ConfigMap after successful rollout |
| **RollingRolloutActor** | Coordinates the rollout stages and tracks progress |

Additional plugins handle edge cases:
- **RollbackPlugin** - reverts to previous version if rollout fails
- **SteadyStatePlugin** - monitors deployed models for ongoing health
- **CleanupPlugin** - handles resource cleanup on deployment deletion

### Inference Server Controller Plugins

The inference server controller uses backend-specific plugins. The **Triton plugin** includes:

| Actor | Purpose |
|-------|---------|
| **ValidationActor** | Validates the InferenceServer spec before creation |
| **ResourceCreationActor** | Creates the Kubernetes Deployment and Service for Triton |
| **HealthCheckActor** | Monitors pod readiness and Triton server health |
| **ProxyConfigurationActor** | Configures Gateway API routing (HTTPRoute) |

For deletion, separate actors handle resource cleanup in the correct order.

---

## Summary

- The inference server controller creates Kubernetes-native resources (Deployments, Services, HTTPRoutes) to serve models
- Model updates flow through a shared ConfigMap, allowing loose coupling between deployment and serving
- Triton is the current backend, but the interface-based design supports adding new backends
- Both controllers use simple plugin architectures focused on their core responsibilities

