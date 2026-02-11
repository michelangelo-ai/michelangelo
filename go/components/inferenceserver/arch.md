# Inference Server Controller Architecture

This document describes the state machine and plugin/actor architecture for the Inference Server Controller.

## Overview

The Inference Server Controller follows a plugin-based architecture where:
1. The **Controller** receives reconciliation events and determines which plugin to invoke (Creation or Deletion)
2. **Plugins** (Creation, Deletion) define the workflow for each lifecycle phase
3. **Actors** are the individual units of work within each plugin, executed sequentially by the **Condition Engine**

## Controller Decision Logic

```mermaid
flowchart TD
    A[Reconcile Event] --> B{DeletionTimestamp set OR Decommissioned?}
    B -->|Yes| C[Deletion Plugin]
    B -->|No| D[Creation Plugin]
    
    
    C --> J{Cleanup Complete?}
    J -->|Yes| K[Remove Finalizer]
    K --> L[Resource Deleted]
    J -->|No| M[Active Requeue - 1 min]
    
    D --> N{All Conditions Satisfied?}
    N -->|Yes| O[Steady State Requeue - 10 min]
    N -->|No| M
```

## State Machine Diagram

```mermaid
stateDiagram-v2
    [*] --> CREATING: InferenceServer Created

    CREATING --> SERVING: All conditions satisfied
    CREATING --> FAILED: Condition failure
    CREATING --> CREATING: Conditions in progress

    SERVING --> DELETING: DeletionTimestamp set OR Decommissioned
    SERVING --> FAILED: Health check failure
    SERVING --> SERVING: Steady state monitoring

    FAILED --> CREATING: Spec updated
    FAILED --> DELETING: DeletionTimestamp set OR Decommissioned

    DELETING --> [*]: Cleanup complete
```

## Plugin and Actor Flow Diagram

```mermaid
flowchart TB
    subgraph Controller
        Reconcile[Reconcile]
        GetPlugin[GetPlugin]
        Reconcile --> GetPlugin
    end

    subgraph PluginRegistry
        direction LR
        OssPlugin[Oss Plugin]
    end

    GetPlugin --> PluginRegistry

    subgraph CreationPlugin[Creation Plugin]
        direction LR
        VA[ValidationActor]
        CWA[ClusterWorkloadsActor]
        CPDA[ControlPlaneDiscoveryActor]
        HCA[HealthCheckActor]
    end

    subgraph DeletionPlugin[Deletion Plugin]
        direction LR
        CA[CleanupActor]
    end

    Reconcile -->|Not Deleting| CreationPlugin
    Reconcile -->|Deleting or Decommissioned| DeletionPlugin

    VA --> CWA --> CPDA --> HCA

    subgraph BE["Backend «interface»"]
        CreateServer[CreateServer]
        CreateServer ~~~ GetServerStatus[GetServerStatus]
        GetServerStatus ~~~ DeleteServer[DeleteServer]
        DeleteServer ~~~ IsHealthy[IsHealthy]
        IsHealthy ~~~ CheckModelStatus[CheckModelStatus]
    end

    subgraph MC["ModelConfig «interface»"]
        CreateModelConfig[CreateModelConfig]
        CreateModelConfig ~~~ DeleteModelConfig[DeleteModelConfig]
    end

    subgraph ER["EndpointRegistry «interface»"]
        EnsureRegisteredEndpoint[EnsureRegisteredEndpoint]
        EnsureRegisteredEndpoint ~~~ DeleteRegisteredEndpoint[DeleteRegisteredEndpoint]
        DeleteRegisteredEndpoint ~~~ ListRegisteredEndpoints[ListRegisteredEndpoints]
        ListRegisteredEndpoints ~~~ GetControlPlaneServiceName[GetControlPlaneServiceName]
    end

    BE ~~~ MC ~~~ ER

    CWA --> CreateServer
    CWA --> GetServerStatus
    CWA --> CreateModelConfig
    HCA --> IsHealthy
    CA --> GetServerStatus
    CA --> DeleteServer
    CA --> DeleteModelConfig

    CPDA --> EnsureRegisteredEndpoint
    CPDA --> DeleteRegisteredEndpoint
    CPDA --> ListRegisteredEndpoints

    %% Node colors
    classDef greenNode fill:#d4edda,stroke:#27ae60,stroke-width:2px
    classDef redNode fill:#f8d7da,stroke:#e74c3c,stroke-width:2px

    class CreationPlugin,VA,CWA,CPDA,HCA greenNode
    class DeletionPlugin,CA redNode

    style BE fill:#e8daef,stroke:#8e44ad,stroke-width:2px
    style MC fill:#d4e6f1,stroke:#2980b9,stroke-width:2px
    style ER fill:#e8daef,stroke:#8e44ad,stroke-width:2px

    %% Creation Plugin arrows - Green
    linkStyle 2 stroke:#27ae60,stroke-width:2px
    linkStyle 4,5,6 stroke:#27ae60,stroke-width:2px
    linkStyle 17,18,19,20 stroke:#27ae60,stroke-width:2px
    linkStyle 24,25,26 stroke:#27ae60,stroke-width:2px

    %% Deletion Plugin arrows - Red
    linkStyle 3 stroke:#e74c3c,stroke-width:2px
    linkStyle 21,22,23 stroke:#e74c3c,stroke-width:2px
```

## Inference Server States

| State | Description | Triggered By |
|-------|-------------|--------------|
| `CREATING` | Initial state, provisioning infrastructure | New InferenceServer created |
| `SERVING` | Server is healthy and ready to serve requests | All creation conditions satisfied |
| `FAILED` | Server provisioning or health check failed | Condition failure |
| `DELETING` | Server resources being cleaned up | DeletionTimestamp set OR Decommissioned |

## Actor Types and Responsibilities

### Creation Plugin Actors (Sequential Execution)

1. **ValidationActor** (`TritonValidation`)
   - Validates backend type is TRITON
   - Validates deployment strategy configuration
   - Validates cluster targets for remote deployments

2. **ClusterWorkloadsActor** (`TritonClusterWorkloads`)
   - Creates Kubernetes Deployments and Services via Backend interface
   - Creates model ConfigMaps via ModelConfig interface
   - Provisions resources in all target clusters
   - Monitors cluster state until READY

3. **ControlPlaneDiscoveryActor** (`TritonControlPlaneDiscovery`)
   - Registers endpoints for remote clusters in control plane
   - Creates Istio ServiceEntry resources for cross-cluster routing
   - Prunes stale endpoints when clusters are removed

4. **HealthCheckActor** (`TritonHealthCheck`)
   - Polls backend health endpoints
   - Verifies server is healthy in all target clusters
   - Reports health status via conditions

### Deletion Plugin Actors

1. **CleanupActor** (`TritonCleanup`)
   - Deletes Kubernetes Deployments and Services via Backend interface
   - Deletes model ConfigMaps via ModelConfig interface
   - Removes resources from all target clusters
   - Verifies resources are fully deleted

## Interfaces

### Backend Interface

The Backend interface provides platform-specific logic for server and model management.

| Method | Description | Used By |
|--------|-------------|---------|
| `CreateServer()` | Provisions Kubernetes resources for inference server | ClusterWorkloadsActor |
| `GetServerStatus()` | Queries server state (CREATING, READY, FAILED, etc.) | ClusterWorkloadsActor, CleanupActor |
| `DeleteServer()` | Removes Kubernetes resources for inference server | CleanupActor |
| `IsHealthy()` | Checks backend health endpoints | HealthCheckActor |
| `CheckModelStatus()` | Checks if a model is loaded and ready | (Used by Deployment controller) |

### Gateway Interface

The Gateway interface provides a means for communicating directly with a deployed inference server from outside the InferenceServer controller. The Deployment Controller uses this interface to check model readiness and verify inference server health before and during model deployment.

Since each backend type (e.g., Triton, vLLM) has a unique way of verifying model status and server health, the Gateway implementation delegates to the appropriate Backend interface methods based on the `backendType` parameter.

```mermaid
flowchart LR
    subgraph DeploymentController["Deployment Controller"]
        RP[Rollout Plugin]
        SSP[SteadyState Plugin]
    end

    subgraph GW["Gateway «interface»"]
        direction TB
        CMS[CheckModelStatus]
        ISIH[InferenceServerIsHealthy]
    end

    subgraph Backends["Backend Implementations"]
        direction TB
        TB[Triton Backend]
        VB[vLLM Backend]
        OB[Other Backends...]
    end

    subgraph ISF["Inference Server"]
        direction TB
        HealthEndpoint[Health Endpoint]
        ModelEndpoint[Model Status Endpoint]
    end

    RP --> CMS
    SSP --> ISIH

    CMS --> TB
    CMS --> VB
    ISIH --> TB
    ISIH --> VB

    TB --> HealthEndpoint
    TB --> ModelEndpoint
    VB --> HealthEndpoint
    VB --> ModelEndpoint

    %% Styling
    classDef dcNode fill:#d4edda,stroke:#27ae60,stroke-width:2px
    classDef gwNode fill:#e8daef,stroke:#8e44ad,stroke-width:2px
    classDef beNode fill:#fdebd0,stroke:#e67e22,stroke-width:2px
    classDef isfNode fill:#ffeaa7,stroke:#f39c12,stroke-width:2px

    class RP,SSP dcNode
    class GW gwNode
    class TB,VB,OB beNode
    class HealthEndpoint,ModelEndpoint isfNode

    style GW fill:#e8daef,stroke:#8e44ad,stroke-width:2px
    style Backends fill:#fdebd0,stroke:#e67e22,stroke-width:2px
    style ISF fill:#ffeaa7,stroke:#f39c12,stroke-width:2px
```

| Method | Description | Used By |
|--------|-------------|---------|
| `CheckModelStatus()` | Verifies if a model is ready to serve requests | Rollout, Rollback, SteadyState Plugins |
| `InferenceServerIsHealthy()` | Checks if the inference server is healthy | SteadyState Plugin |

### ModelConfig Interface

The ModelConfig interface manages model configuration storage (e.g., Kubernetes ConfigMaps) for inference servers.

| Method | Description | Used By |
|--------|-------------|---------|
| `CreateModelConfig()` | Creates model configuration storage for an inference server | ClusterWorkloadsActor |
| `DeleteModelConfig()` | Deletes model configuration storage for an inference server | CleanupActor |

### EndpointRegistry Interface

The EndpointRegistry manages inference server endpoints across multiple clusters for service mesh routing.

| Method | Description | Used By |
|--------|-------------|---------|
| `EnsureRegisteredEndpoint()` | Registers cluster endpoint in control plane | ControlPlaneDiscoveryActor |
| `DeleteRegisteredEndpoint()` | Removes cluster endpoint registration | ControlPlaneDiscoveryActor |
| `ListRegisteredEndpoints()` | Lists all registered cluster endpoints | ControlPlaneDiscoveryActor |
| `GetControlPlaneServiceName()` | Returns control plane service name | (Used by Deployment controller) |

## Condition Engine Execution

The Condition Engine executes actors sequentially:

```
For each actor in plugin.GetActors():
    1. Retrieve() - Check current condition status (idempotent)
    2. If condition NOT satisfied:
        a. Run() - Execute action to progress condition
        b. Stop iteration (only one action per reconcile)
    3. PutCondition() - Store updated condition in server status
```

The engine returns:
- `AreSatisfied=true` - All conditions satisfied, requeue after 10 minutes (steady state)
- `AreSatisfied=false` - Conditions in progress, requeue after 1 minute (active)

## Deployment Strategies

The Inference Server supports two deployment strategies:

### Control Plane Cluster Deployment
- Server is deployed to the same cluster as the control plane
- No cross-cluster discovery needed
- Simplest deployment model

### Remote Cluster Deployment
- Server is deployed to one or more remote Kubernetes clusters
- Requires cluster targets with connection credentials
- Control plane discovery creates ServiceEntry for routing
- Supports multi-cluster inference serving

## Key Functions

| Function | Description |
|----------|-------------|
| `isDecommissioned()` | Returns true if `Spec.DecomSpec.Decommission` is set |
| `ParseState()` | Derives server state from conditions and deletion status |
| `UpdateDetails()` | Updates status with backend-specific information |
| `UpdateConditions()` | Filters conditions relevant to current plugin |
