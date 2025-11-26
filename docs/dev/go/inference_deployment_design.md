# Inference Server and Deployment Controllers

This document provides an overview of how the inference server and deployment controllers work together to serve ML models in Michelangelo.

---

## How the Inference Server Works

The inference server is designed to feel native to Kubernetes. Rather than introducing custom scheduling or orchestration, it follows the standard Kubernetes operator pattern that platform engineers are already familiar with.

Users define their desired serving configuration by creating an `InferenceServer` resource. This resource specifies the backend type (e.g., Triton), resource requirements, and replica count. The inference server controller watches for these resources and reconciles the cluster state to match what the user requested.

When the controller processes an `InferenceServer` resource, it creates standard Kubernetes objects. A Deployment runs the inference server pods, a Service provides networking within the cluster, and an HTTPRoute exposes the service externally through the Gateway API. This means model serving automatically benefits from Kubernetes' built-in capabilities like pod scheduling, rolling updates, and health monitoring.

The controller continuously monitors these resources and updates the `InferenceServer` status to reflect the current state. If pods become unhealthy or resources drift from the desired configuration, the controller takes corrective action.

---

## Deployment Controller and ConfigMap Coordination

The deployment controller manages model rollouts to inference servers. The key design decision here is a ConfigMap-based coordination pattern that keeps the deployment and inference server controllers loosely coupled.

Each inference server has an associated ConfigMap that holds the list of models it should serve. The ConfigMap is named using the pattern `<inference-server>-model-config` and contains a JSON list of model entries, each specifying a model name and its storage path.

When a user creates or updates a Deployment resource with a new model version, the deployment controller writes the new model entry to the ConfigMap. The inference server controller detects this change and issues a load command to the underlying server process. Importantly, this happens without restarting pods—models are loaded dynamically into the running server.

The deployment controller then monitors the model status, waiting for the inference server to report that the model is ready to serve traffic. Once the new model is healthy, the controller removes old model versions from the ConfigMap, which triggers the inference server to unload them.

This design separates concerns cleanly. The deployment controller focuses on rollout logic, health checking, and traffic management. The inference server controller handles the mechanics of actually loading and serving models. The ConfigMap serves as the contract between them.

---

## Current Backend Implementation

The system currently implements NVIDIA Triton Inference Server as the serving backend. Triton is a production-grade inference server that supports multiple ML frameworks including TensorFlow, PyTorch, and ONNX. It provides features like dynamic batching, model versioning, and GPU acceleration.

The architecture is designed to support multiple inference server implementations. This is achieved through two abstraction layers.

The first is the Gateway interface, which defines the operations that any backend must support. These include creating and deleting infrastructure, loading and unloading models, and checking health status. The second is the Backend interface, which each serving technology implements with its specific logic. The Triton backend, for example, knows how to construct Triton-specific deployment configurations and how to call Triton's model repository API.

Backends register themselves in a plugin registry at startup. When the controller processes an `InferenceServer` resource, it looks up the appropriate backend based on the `backendType` field and delegates operations to it. Adding support for a new inference server (such as TensorFlow Serving or TorchServe) would involve implementing the Backend interface and registering it in the registry. The core controller logic remains unchanged.

---

## Controller Plugins

Both controllers use a plugin architecture to separate core reconciliation logic from strategy-specific behavior. This makes the code easier to test and extend.

### Deployment Controller

The deployment controller uses a rolling rollout strategy. The rollout proceeds through a sequence of actors that each handle one aspect of the deployment.

The ModelSyncActor adds the new model to the ConfigMap and waits for it to become ready in the inference server. It polls the inference server's health endpoint with a configurable timeout, ensuring the model is actually serving traffic before proceeding.

The ModelCleanupActor runs after the new model is ready. It identifies old model versions that are no longer needed and removes them from the ConfigMap. This triggers the inference server to unload the stale models and free resources.

The RollingRolloutActor coordinates the overall rollout, tracking progress through the deployment stages and updating status accordingly.

Additional plugins handle failure scenarios. The RollbackPlugin reverts to the previous model version if the rollout fails. The SteadyStatePlugin monitors deployed models for ongoing health after the rollout completes. The CleanupPlugin handles resource cleanup when a deployment is deleted.

### Inference Server Controller

The inference server controller uses backend-specific plugins. The Triton plugin handles the full lifecycle of a Triton-based inference server.

During creation, the ValidationActor checks that the InferenceServer spec is valid before any resources are created. The ResourceCreationActor then creates the Kubernetes Deployment and Service with Triton-specific configuration. The HealthCheckActor monitors pod readiness and Triton server health, waiting for the server to be ready to accept models. Finally, the ProxyConfigurationActor configures the Gateway API routing so external traffic can reach the server.

For deletion, separate actors handle resource cleanup in the correct order, ensuring graceful shutdown of models before removing infrastructure.

---

## Summary

The inference server controller creates standard Kubernetes resources to serve models, making the system feel native to operators familiar with Kubernetes patterns. Model updates flow through a shared ConfigMap, which provides loose coupling between the deployment and inference server controllers. Triton is the current serving backend, but the interface-based design allows adding new backends without modifying core controller logic. Both controllers use straightforward plugin architectures that focus on their core responsibilities and make the codebase easier to extend.

