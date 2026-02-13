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

Dynamo supports two discovery backends, configured via `DYN_DISCOVERY_BACKEND` environment variable:

### 1. Kubernetes Discovery (Recommended for Uber)

```yaml
env:
  - name: DYN_DISCOVERY_BACKEND
    value: "kubernetes"
```

- Workers register themselves via Kubernetes EndpointSlices
- Frontend discovers workers by watching EndpointSlices through the K8s API
- Requires headless Services for workers with appropriate labels
- No external dependencies (etcd, NATS) required for basic operation

### 2. etcd Discovery

```yaml
env:
  - name: DYN_DISCOVERY_BACKEND
    value: "etcd"
  - name: ETCD_ENDPOINTS
    value: "<etcd-address>:2379"
```

- Workers register themselves in etcd
- Frontend discovers workers from etcd
- Requires running etcd cluster

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
