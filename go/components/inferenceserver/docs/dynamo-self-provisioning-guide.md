# Dynamo Self-Provisioning Guide

This document outlines how to provision Dynamo inference components (Frontend, Prefill Worker, Decode Worker) directly through Michelangelo's inference server controller, bypassing the external Dynamo operator.

## Background

The Dynamo operator translates a `DynamoGraphDeployment` CR into standard Kubernetes resources:
- **Kubernetes Deployments** (or LeaderWorkerSets for multi-node)
- **Services** for frontend exposure and model endpoint discovery
- **Ingresses/VirtualServices** for external traffic routing
- **ConfigMaps** for configuration data

The Dynamo components are designed to be decoupled from the operator, making self-provisioning entirely feasible.

## Component Overview

### Container Commands

| Component | Command |
|-----------|---------|
| Frontend | `python3 -m dynamo.frontend` |
| Decode Worker (Aggregated) | `python3 -m dynamo.vllm --model <model> --connector none` |
| Decode Worker (Disaggregated) | `python3 -m dynamo.vllm --model <model> --is-decode-worker --connector <connector>` |
| Prefill Worker | `python3 -m dynamo.vllm --model <model> --is-prefill-worker --connector <connector>` |

### Container Images

Use the official Dynamo runtime images:
- `nvcr.io/nvidia/ai-dynamo/vllm-runtime:<tag>`

These images contain all Python/Rust bindings needed for the Dynamo distributed runtime.

## Service Discovery

Dynamo's `DistributedRuntime` requires a service discovery mechanism for components (Frontend, Workers) to find each other. This is configured via the `DYN_DISCOVERY_BACKEND` environment variable.

**Reference**: [Dynamo Distributed Runtime - Service Discovery Backends](https://docs.nvidia.com/dynamo/design-docs/distributed-runtime#service-discovery-backends)

### Discovery Backend Options

| Backend | `DYN_DISCOVERY_BACKEND` | Requires | Best For |
|---------|-------------------------|----------|----------|
| KV Store (etcd) | `kv_store` (default) | etcd cluster | Full operator deployment |
| Kubernetes | `kubernetes` | CRD + RBAC | Self-provisioned deployment |

> **Important**: The default is `kv_store`, which uses etcd for both discovery AND internal KV storage. For self-provisioning without etcd, you must explicitly set `DYN_DISCOVERY_BACKEND=kubernetes`.

### Option 1: Kubernetes Discovery (Recommended for Self-Provisioning)

```yaml
env:
  - name: DYN_DISCOVERY_BACKEND
    value: "kubernetes"
  - name: DYN_STORE_KV
    value: "mem"  # In-memory KV store (no etcd needed)
```

**How it works:**
```
┌─────────────────────────────────────────────────────────────────┐
│                    Kubernetes API Server                        │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │      DynamoWorkerMetadata CRs (worker registrations)     │   │
│  └─────────────────────────────────────────────────────────┘   │
│           ▲ CREATE                       │ WATCH               │
└───────────┼──────────────────────────────┼──────────────────────┘
            │                              ▼
     ┌──────┴──────┐                ┌─────────────┐
     │   Worker    │                │  Frontend   │
     └─────────────┘                └─────────────┘
```

- Workers create `DynamoWorkerMetadata` CRs to register themselves
- Frontend watches these CRs via the Kubernetes API
- Pods need RBAC permissions to create/watch these CRs
- **No operator required** - just the CRD and RBAC

**Requirements:**

1. **DynamoWorkerMetadata CRD** - Schema definition for worker registration
2. **ClusterRole** - Permissions to manage `dynamoworkermetadatas` and watch `endpointslices`
3. **ClusterRoleBinding** - Grants permissions to pod ServiceAccounts

See `python/michelangelo/cli/sandbox/resources/dynamo-discovery-rbac.yaml` for the complete setup.

**RBAC Permissions Required:**
```yaml
rules:
  - apiGroups: ["discovery.k8s.io"]
    resources: ["endpointslices"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["nvidia.com"]
    resources: ["dynamoworkermetadatas"]
    verbs: ["create", "get", "list", "watch", "update", "patch", "delete"]
```

### Option 2: KV Store Discovery (etcd)

```yaml
env:
  - name: DYN_DISCOVERY_BACKEND
    value: "kv_store"  # This is the default
  - name: DYN_STORE_KV
    value: "etcd"      # This is the default
  - name: ETCD_ENDPOINTS
    value: "etcd.dynamo-system:2379"
```

**How it works:**
- Workers register themselves in etcd at `/services/{namespace}/{component}/{endpoint}-{lease_id}`
- Frontend watches etcd for worker registrations
- Requires running etcd cluster (typically deployed by Dynamo operator)

**When to use:**
- Full Dynamo operator deployment
- Need centralized configuration store beyond discovery

### Why Not In-Memory with KV Store Discovery?

You might wonder: can we use `DYN_DISCOVERY_BACKEND=kv_store` with `DYN_STORE_KV=mem` to avoid both etcd AND the CRD?

**No** - this doesn't work for multi-pod deployments:

| Configuration | Result |
|---------------|--------|
| `kv_store` + `etcd` | ✅ Works - etcd is shared across pods |
| `kv_store` + `mem` | ❌ Fails - each pod has isolated in-memory store |
| `kubernetes` + `mem` | ✅ Works - K8s API is shared, KV store only for non-discovery data |

With `kv_store` + `mem`, the worker registers itself in its own local memory, but the frontend looks in its own empty memory store and never finds the worker.

### KV Store vs Discovery Backend (Important Distinction)

These are **two different configurations**:

| Env Variable | Purpose | Options |
|--------------|---------|---------|
| `DYN_DISCOVERY_BACKEND` | How components discover each other | `kubernetes`, `kv_store` |
| `DYN_STORE_KV` | Internal KV storage for runtime data | `etcd`, `mem`, `file` |

When using `kubernetes` discovery:
- Discovery uses K8s API (CRDs, EndpointSlices)
- KV store can be `mem` since discovery doesn't need it
- KV store is still used for other runtime data (load metrics, etc.)

## Required Environment Variables

### All Components

| Variable | Description | Example |
|----------|-------------|---------|
| `DYN_NAMESPACE` | Dynamo logical namespace | `dynamo` |
| `DYN_COMPONENT` | Component name | `VllmDecodeWorker` |
| `DYN_DISCOVERY_BACKEND` | Discovery backend type | `kubernetes` or `etcd` |
| `DYN_PARENT_DGD_K8S_NAME` | Parent deployment name | `my-deployment` |
| `DYN_PARENT_DGD_K8S_NAMESPACE` | Parent deployment namespace | `default` |
| `POD_NAME` | Pod name (via Downward API) | - |
| `POD_NAMESPACE` | Pod namespace (via Downward API) | - |
| `POD_UID` | Pod UID (via Downward API) | - |

### Worker-Specific

| Variable | Description | Example |
|----------|-------------|---------|
| `DYN_SYSTEM_ENABLED` | Enable system status server | `true` |
| `DYN_SYSTEM_PORT` | System status server port | `9090` |
| `DYN_HEALTH_CHECK_ENABLED` | Enable health checks | `true` or `false` |
| `DYN_SYSTEM_USE_ENDPOINT_HEALTH_STATUS` | Endpoints for health status | `["generate"]` |

### Frontend-Specific

| Variable | Description | Example |
|----------|-------------|---------|
| `DYNAMO_PORT` | HTTP service port | `8000` |
| `DYN_HTTP_PORT` | HTTP port (alias) | `8000` |

### LoRA-Specific

| Variable | Description | Example |
|----------|-------------|---------|
| `DYN_LORA_ENABLED` | Enable LoRA support | `true` |
| `DYN_LORA_PATH` | Local path for LoRA downloads | `/tmp/dynamo_loras` |

### S3/MinIO (for LoRA downloads)

| Variable | Description | Example |
|----------|-------------|---------|
| `AWS_ENDPOINT` | S3-compatible endpoint | `http://minio:9000` |
| `AWS_ACCESS_KEY_ID` | Access key | - |
| `AWS_SECRET_ACCESS_KEY` | Secret key | - |
| `AWS_REGION` | Region | `us-east-1` |
| `AWS_ALLOW_HTTP` | Allow HTTP connections | `true` |

## Required Labels and Annotations

### Pod Labels

| Label | Description | Example |
|-------|-------------|---------|
| `nvidia.com/dynamo-component-type` | Component type | `frontend`, `worker`, `prefill`, `decode` |
| `nvidia.com/dynamo-sub-component-type` | Sub-component type | `prefill`, `decode` |
| `nvidia.com/dynamo-base-model` | Base model name | `Qwen/Qwen3-0.6B` |
| `nvidia.com/dynamo-base-model-hash` | Hash of model name | - |
| `nvidia.com/dynamo-discovery-enabled` | Enable discovery | `true` |
| `nvidia.com/dynamo-namespace` | Dynamo namespace | `dynamo` |
| `nvidia.com/dynamo-component` | Component name | `VllmDecodeWorker` |

## Probes Configuration

### Frontend

```yaml
livenessProbe:
  httpGet:
    path: /live
    port: 8000
  initialDelaySeconds: 15
  periodSeconds: 10
  timeoutSeconds: 1
  failureThreshold: 3

readinessProbe:
  httpGet:
    path: /health
    port: 8000
  initialDelaySeconds: 10
  periodSeconds: 10
  timeoutSeconds: 3
  failureThreshold: 3
```

### Workers

```yaml
livenessProbe:
  httpGet:
    path: /live
    port: 9090
  periodSeconds: 5
  timeoutSeconds: 4
  failureThreshold: 1

readinessProbe:
  httpGet:
    path: /health
    port: 9090
  periodSeconds: 10
  timeoutSeconds: 4
  failureThreshold: 3

startupProbe:
  httpGet:
    path: /live
    port: 9090
  periodSeconds: 10
  timeoutSeconds: 5
  failureThreshold: 720  # 2 hours for model loading
```

## Ports

| Component | Port | Name | Purpose |
|-----------|------|------|---------|
| Frontend | 8000 | http | HTTP API endpoint |
| Worker | 9090 | system | System status server, health checks, LoRA management |

## Architecture Options

### Option A: Aggregated Deployment (Simpler)

```
┌─────────────────┐     ┌─────────────────┐
│    Frontend     │────▶│  Decode Worker  │
│ (HTTP ingress)  │     │  (vLLM engine)  │
└─────────────────┘     └─────────────────┘
```

- Frontend handles HTTP requests
- Decode Worker does both prefill and decode phases
- Workers discovered via K8s EndpointSlices
- Use `--connector none` for workers

**Worker Command:**
```bash
python3 -m dynamo.vllm --model Qwen/Qwen3-0.6B --connector none --enforce-eager
```

### Option B: Disaggregated Deployment (P/D Separation)

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│    Frontend     │────▶│ Prefill Worker  │────▶│ Decode Worker   │
│ (HTTP ingress)  │     │ (KV producer)   │     │ (KV consumer)   │
└─────────────────┘     └─────────────────┘     └─────────────────┘
                               │                       ▲
                               └───── KV Transfer ─────┘
                                   (nixl/lmcache)
```

- Prefill workers produce KV cache
- Decode workers consume KV cache
- Requires KV connector for cache transfer

**Prefill Worker Command:**
```bash
python3 -m dynamo.vllm --model Qwen/Qwen3-0.6B --is-prefill-worker --connector kvbm nixl
```

**Decode Worker Command:**
```bash
python3 -m dynamo.vllm --model Qwen/Qwen3-0.6B --is-decode-worker --connector nixl
```

## Connector Options

| Connector | Description | Use Case |
|-----------|-------------|----------|
| `none` | No KV transfer | Aggregated deployment |
| `nixl` | GPU-to-GPU RDMA transfer | Disaggregated with fast interconnect |
| `lmcache` | CPU-based KV cache | Disaggregated with CPU caching |
| `kvbm` | KV Block Manager | Disaggregated with block-level management |

### NIXL Requirements

NIXL (NVIDIA Inference Xfer Library) provides the fastest KV cache transfer using GPU Direct RDMA. However, it has strict hardware requirements.

#### Hardware Requirements

| Requirement | Details |
|-------------|---------|
| **InfiniBand or RoCE NICs** | Mellanox ConnectX-6/7 or similar RDMA-capable NICs |
| **RDMA-capable GPUs** | NVIDIA A100, H100, or newer with GPU Direct RDMA support |
| **Network Fabric** | InfiniBand switch fabric OR RoCE-enabled Ethernet |
| **Architecture** | x86_64/AMD64 only (ARM64 not supported) |

#### GKE Node Pool Requirements

For GKE, you need specific machine types with InfiniBand pre-configured:

| Machine Type | GPUs | InfiniBand | Notes |
|--------------|------|------------|-------|
| `a3-highgpu-8g` | 8x H100 80GB | Yes (NVSwitch + InfiniBand) | Best for NIXL |
| `a3-megagpu-8g` | 8x H100 80GB | Yes (NVLink + InfiniBand) | Best for NIXL |
| `a2-ultragpu-8g` | 8x A100 80GB | Yes (NVLink) | Good for NIXL |
| `a2-highgpu-*` | 1-8x A100 40GB | No | NIXL not supported |
| `g2-standard-*` | L4 GPUs | No | NIXL not supported |

#### Software/Kubernetes Requirements

1. **NVIDIA Network Operator** - Deploys RDMA device plugin and drivers
   ```bash
   helm install network-operator nvidia/network-operator \
     --namespace nvidia-network-operator --create-namespace
   ```

2. **SR-IOV Device Plugin** - Exposes RDMA devices to pods

3. **Pod Security Context** - Required capabilities:
   ```yaml
   securityContext:
     capabilities:
       add:
         - IPC_LOCK      # Required for RDMA memory registration
         - SYS_RESOURCE  # Required for memory limits
   ```

4. **Resource Requests** - RDMA device allocation:
   ```yaml
   resources:
     limits:
       rdma/ib_verbs: 1       # InfiniBand verbs device
       # OR
       rdma/shared_device: 1  # Shared RDMA device
   ```

5. **Environment Variables**:
   ```yaml
   env:
     - name: UCX_TLS
       value: "rc,cuda_copy,cuda_ipc"  # UCX transport layers
     - name: UCX_NET_DEVICES
       value: "mlx5_0:1"  # InfiniBand device
   ```

#### NIXL Connector Configuration

**Prefill Worker:**
```bash
python3 -m dynamo.vllm --model <model> --is-prefill-worker --connector kvbm nixl
```

**Decode Worker:**
```bash
python3 -m dynamo.vllm --model <model> --is-decode-worker --connector nixl
```

### LMCache Requirements (Alternative)

LMCache uses CPU memory as an intermediate KV cache, making it work on **standard GKE clusters** without InfiniBand.

#### Trade-offs

| Aspect | NIXL | LMCache |
|--------|------|---------|
| **Speed** | Fastest (GPU-to-GPU) | Slower (GPU→CPU→GPU) |
| **Hardware** | InfiniBand required | Standard Ethernet |
| **Memory** | GPU VRAM only | Uses CPU RAM for cache |
| **Complexity** | High (special networking) | Low (standard K8s) |

#### LMCache Configuration

**Prefill Worker:**
```bash
python3 -m dynamo.vllm --model <model> --is-prefill-worker --connector lmcache
```

**Decode Worker:**
```bash
python3 -m dynamo.vllm --model <model> --is-decode-worker --connector lmcache
```

**Environment Variables:**
```yaml
env:
  - name: LMCACHE_DISTRIBUTED_URL
    value: "lmcache://lmcache-service:7000"  # LMCache server address
```

> **Note**: LMCache may require a separate LMCache server deployment for distributed caching. Check the [Dynamo LMCache Integration docs](https://docs.nvidia.com/dynamo/latest/backends/vllm/LMCache_Integration.html) for current requirements.

## vLLM Arguments

### Common Arguments

| Argument | Description | Example |
|----------|-------------|---------|
| `--model` | HuggingFace model ID | `Qwen/Qwen3-0.6B` |
| `--enforce-eager` | Disable CUDA graphs (for dev) | - |
| `--gpu-memory-utilization` | GPU memory fraction | `0.9` |
| `--max-model-len` | Maximum sequence length | `4096` |

### LoRA Arguments

| Argument | Description | Example |
|----------|-------------|---------|
| `--enable-lora` | Enable LoRA support | - |
| `--max-lora-rank` | Maximum LoRA rank | `64` |
| `--max-loras` | Maximum concurrent LoRAs | `4` |

### Disaggregated Arguments

| Argument | Description |
|----------|-------------|
| `--is-prefill-worker` | Run as prefill-only worker |
| `--is-decode-worker` | Run as decode-only worker |
| `--kv-events-config` | KV event publishing config |

## Services Configuration

### Frontend Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-deployment-frontend
  labels:
    nvidia.com/dynamo-component-type: frontend
spec:
  type: ClusterIP
  ports:
    - port: 8000
      targetPort: 8000
      name: http
  selector:
    nvidia.com/dynamo-component-type: frontend
```

### Worker Headless Service (for discovery)

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-deployment-workers
  labels:
    nvidia.com/dynamo-discovery-enabled: "true"
spec:
  clusterIP: None  # Headless
  ports:
    - port: 9090
      targetPort: 9090
      name: system
  selector:
    nvidia.com/dynamo-component-type: worker
```

## What Michelangelo Controller Needs to Do

1. **Create Deployments**: Standard K8s Deployments with:
   - Container commands/args as specified above
   - Environment variables for discovery, namespace, component
   - Resource limits (GPU, memory, shared memory)
   - Probes (liveness/readiness)
   - Downward API volume for pod identity

2. **Create Services**:
   - Headless service for worker discovery
   - ClusterIP service for frontend HTTP access

3. **Handle Labels/Annotations**: Set the required labels for discovery

4. **Shared Memory**: Workers need shared memory volume:
   ```yaml
   volumes:
     - name: shared-memory
       emptyDir:
         medium: Memory
         sizeLimit: 8Gi
   volumeMounts:
     - name: shared-memory
       mountPath: /dev/shm
   ```

5. **Downward API**: For pod identity after potential CRIU restore:
   ```yaml
   volumes:
     - name: podinfo
       downwardAPI:
         items:
           - path: "pod_name"
             fieldRef:
               fieldPath: metadata.name
           - path: "pod_namespace"
             fieldRef:
               fieldPath: metadata.namespace
           - path: "pod_uid"
             fieldRef:
               fieldPath: metadata.uid
   ```

## Disaggregated Serving Requirements

Disaggregated serving separates the prefill and decode phases into specialized workers, enabling better hardware utilization and improved performance for certain workloads.

### Architecture Overview

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│    Frontend     │     │ Prefill Worker  │     │ Decode Worker   │
│    (1 pod)      │     │    (N pods)     │     │    (M pods)     │
└────────┬────────┘     └────────┬────────┘     └────────┬────────┘
         │                       │                       │
         │   Request routing     │   Direct NIXL/RDMA    │
         └───────────────────────┼───────────────────────┘
                                 │
                          GPU-to-GPU transfer
                          (peer-to-peer, no broker)
```

**Key insight**: NIXL KV transfer happens **directly between worker pods** - no additional services or brokers are required.

### Infrastructure Components Explained

Dynamo has several optional infrastructure components. Here's what each does and whether you need it:

| Component | Purpose | Required? |
|-----------|---------|-----------|
| **etcd** (Dynamo's own) | Service discovery - workers register themselves | **No** - use Kubernetes discovery instead |
| **NATS** | Request plane (routing) and event plane (KV events) | **No** - use direct HTTP + ZMQ instead |
| **ZMQ** | Event plane for KV cache events | **Optional** - only for cache-aware routing |
| **NIXL** | Direct GPU-to-GPU KV transfer | **Yes** - peer-to-peer, no extra pods needed |

**Note**: Kubernetes also uses etcd internally, but that's separate from Dynamo's etcd. When using `DYN_DISCOVERY_BACKEND=kubernetes`, Dynamo uses the Kubernetes API for discovery - no separate etcd cluster needed.

### Minimal Disaggregated Setup

For basic disaggregated serving, you only need:
- 1 Frontend Deployment + Service
- N Prefill Worker Deployments + Headless Service
- M Decode Worker Deployments + Headless Service

Configure pods to use Kubernetes-native discovery:

```yaml
env:
  - name: DYN_DISCOVERY_BACKEND
    value: "kubernetes"
  - name: DYN_EVENT_PLANE
    value: "zmq"
```

### RDMA Requirements (Critical for Performance)

Without RDMA, KV transfer falls back to TCP and performance degrades by **~40x**. RDMA requires:

#### 1. Pod Security Context

```yaml
securityContext:
  capabilities:
    add: ["IPC_LOCK"]  # Required for RDMA memory registration
```

#### 2. RDMA Resource Requests

```yaml
resources:
  limits:
    nvidia.com/gpu: "1"
    rdma/ib: "1"  # Request RDMA resources (match TP size)
  requests:
    nvidia.com/gpu: "1"
    rdma/ib: "1"
```

#### 3. UCX Environment Variables

```yaml
env:
  - name: UCX_TLS
    value: "rc_x,rc,dc_x,dc,cuda_copy,cuda_ipc"
  - name: UCX_RNDV_SCHEME
    value: "get_zcopy"
  - name: UCX_RNDV_THRESH
    value: "0"
```

### NIXL Configuration

Each worker needs a unique NIXL side channel port for peer-to-peer coordination:

```yaml
env:
  - name: VLLM_NIXL_SIDE_CHANNEL_PORT
    value: "20097"  # Must be unique per worker pod
```

**Port assignment strategy**: Use a deterministic formula based on pod ordinal, e.g., `20096 + pod_index`.

### KVBM (KV Block Manager) Configuration

For prefill workers with CPU caching (recommended for large KV caches):

```yaml
env:
  # CPU cache size
  - name: DYN_KVBM_CPU_CACHE_GB
    value: "20"
  
  # KV event port (unique per prefill worker)
  - name: DYN_VLLM_KV_EVENT_PORT
    value: "20081"
  
  # ZMQ coordination ports (unique per prefill worker)
  - name: DYN_KVBM_LEADER_ZMQ_PUB_PORT
    value: "56001"
  - name: DYN_KVBM_LEADER_ZMQ_ACK_PORT
    value: "56002"
```

### Complete Prefill Worker Pod Spec

```yaml
spec:
  containers:
  - name: main
    image: nvcr.io/nvidia/ai-dynamo/vllm-runtime:0.8.0
    command: ["python3", "-m", "dynamo.vllm"]
    args:
      - --model
      - Qwen/Qwen3-0.6B
      - --is-prefill-worker
      - --connector
      - kvbm
      - nixl
      - --max-model-len
      - "32000"
      - --kv-events-config
      - '{"publisher":"zmq","topic":"kv-events","endpoint":"tcp://*:20081"}'
    env:
      # Discovery
      - name: DYN_DISCOVERY_BACKEND
        value: "kubernetes"
      - name: DYN_NAMESPACE
        value: "dynamo"
      - name: DYN_COMPONENT
        value: "VllmPrefillWorker"
      # System status
      - name: DYN_SYSTEM_ENABLED
        value: "true"
      - name: DYN_SYSTEM_PORT
        value: "9090"
      # KVBM
      - name: DYN_KVBM_CPU_CACHE_GB
        value: "20"
      - name: DYN_VLLM_KV_EVENT_PORT
        value: "20081"
      # NIXL
      - name: VLLM_NIXL_SIDE_CHANNEL_PORT
        value: "20097"
      # UCX/RDMA
      - name: UCX_TLS
        value: "rc_x,rc,dc_x,dc,cuda_copy,cuda_ipc"
      - name: UCX_RNDV_SCHEME
        value: "get_zcopy"
      - name: UCX_RNDV_THRESH
        value: "0"
    securityContext:
      capabilities:
        add: ["IPC_LOCK"]
    resources:
      limits:
        nvidia.com/gpu: "1"
        rdma/ib: "1"
      requests:
        nvidia.com/gpu: "1"
        rdma/ib: "1"
        memory: "100Gi"
    volumeMounts:
      - name: shared-memory
        mountPath: /dev/shm
  volumes:
    - name: shared-memory
      emptyDir:
        medium: Memory
        sizeLimit: 16Gi
```

### Complete Decode Worker Pod Spec

```yaml
spec:
  containers:
  - name: main
    image: nvcr.io/nvidia/ai-dynamo/vllm-runtime:0.8.0
    command: ["python3", "-m", "dynamo.vllm"]
    args:
      - --model
      - Qwen/Qwen3-0.6B
      - --is-decode-worker
      - --connector
      - nixl
      - --max-model-len
      - "32000"
    env:
      # Discovery
      - name: DYN_DISCOVERY_BACKEND
        value: "kubernetes"
      - name: DYN_NAMESPACE
        value: "dynamo"
      - name: DYN_COMPONENT
        value: "VllmDecodeWorker"
      # System status
      - name: DYN_SYSTEM_ENABLED
        value: "true"
      - name: DYN_SYSTEM_PORT
        value: "9090"
      # NIXL
      - name: VLLM_NIXL_SIDE_CHANNEL_PORT
        value: "20098"
      # UCX/RDMA
      - name: UCX_TLS
        value: "rc_x,rc,dc_x,dc,cuda_copy,cuda_ipc"
      - name: UCX_RNDV_SCHEME
        value: "get_zcopy"
      - name: UCX_RNDV_THRESH
        value: "0"
    securityContext:
      capabilities:
        add: ["IPC_LOCK"]
    resources:
      limits:
        nvidia.com/gpu: "1"
        rdma/ib: "1"
      requests:
        nvidia.com/gpu: "1"
        rdma/ib: "1"
    volumeMounts:
      - name: shared-memory
        mountPath: /dev/shm
  volumes:
    - name: shared-memory
      emptyDir:
        medium: Memory
        sizeLimit: 16Gi
```

### Controller Responsibilities for Disaggregated Serving

| Concern | How Controller Should Handle |
|---------|------------------------------|
| Worker discovery | Set `DYN_DISCOVERY_BACKEND=kubernetes`, create headless Services with labels |
| NIXL side channel | Assign unique `VLLM_NIXL_SIDE_CHANNEL_PORT` per worker (e.g., 20096 + index) |
| RDMA | Request `rdma/ib` resources, add `IPC_LOCK` capability, set UCX env vars |
| KVBM coordination | Unique ZMQ ports per prefill worker |
| KV events | Unique `DYN_VLLM_KV_EVENT_PORT` per prefill worker |
| Shared memory | 16Gi emptyDir for vLLM workers |

### Verifying RDMA is Active

After deployment, check worker logs for UCX initialization:

```bash
kubectl logs <worker-pod> | grep -i "UCX\|NIXL"
```

Expected output:
```
NIXL INFO Backend UCX was instantiated
```

If you only see TCP transports, RDMA is not active - check your RDMA device plugin and resource requests.

### When to Use Disaggregated vs Aggregated

| Workload | Recommendation |
|----------|----------------|
| Short inputs, short outputs | Aggregated (simpler) |
| Long inputs (>8000 tokens), short outputs | Disaggregated |
| Need independent P/D scaling | Disaggregated |
| No RDMA available | Aggregated (disaggregated without RDMA is 40x slower) |

## Testing Inference

Once the frontend and worker pods are running, you can test inference using the OpenAI-compatible API.

### Accessing the Frontend

**Port-forward to the frontend service:**
```bash
kubectl port-forward svc/dynamo-sp-<inference-server-name>-frontend 8000:8000
```

Or directly to the pod:
```bash
kubectl port-forward $(kubectl get pods -l nvidia.com/dynamo-component-type=frontend -o name) 8000:8000
```

### API Endpoints

**List available models:**
```bash
curl http://localhost:8000/v1/models
```

**Chat completion:**
```bash
curl http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen/Qwen3-0.6B",
    "messages": [{"role": "user", "content": "Hello, how are you?"}],
    "max_tokens": 50
  }'
```

**Text completion:**
```bash
curl http://localhost:8000/v1/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen/Qwen3-0.6B",
    "prompt": "The capital of France is",
    "max_tokens": 20
  }'
```

**Streaming response:**
```bash
curl http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen/Qwen3-0.6B",
    "messages": [{"role": "user", "content": "Tell me a short joke"}],
    "max_tokens": 100,
    "stream": true
  }'
```

## LoRA Loading

**Important**: Dynamo's LoRA downloader only supports `file://` and `s3://` URIs natively. HuggingFace `hf://` URIs are NOT supported.

For LoRA adapters:
1. Download from HuggingFace externally
2. Upload to S3/MinIO
3. Reference via `s3://bucket/path/to/lora`

Or use the `--lora-modules` vLLM flag to pre-load LoRAs at startup from local paths.

## References

- Dynamo Operator Source: `/Users/ghosharitra/dynamo/deploy/operator/`
- Dynamo Examples: `/Users/ghosharitra/dynamo/examples/backends/vllm/deploy/`
- Discovery Module: `/Users/ghosharitra/dynamo/lib/runtime/src/discovery/`
- Component Defaults: `/Users/ghosharitra/dynamo/deploy/operator/internal/dynamo/component_*.go`
