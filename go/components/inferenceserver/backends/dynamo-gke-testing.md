# Testing Dynamo Backend on GKE

## Quick Start

### 1. Create Sandbox

```bash
ma sandbox create --gke \
  --exclude minio,cadence,mysql,michelangelo-ui
```

### 2. Deploy Dynamo Demo

```bash
ma sandbox demo --gke inference-dynamo
```

### 3. Port-Forward to Frontend

```bash
kubectl --context gke_michelanglo-oss-196506_us-east1_kubernetes-gke-dev01 \
  port-forward deployment/dynamo-dynamo-inference-server-frontend 8000:8000 -n default
```

### 4. Run Inference

```bash
curl localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen/Qwen3-0.6B",
    "messages": [{"role": "user", "content": "Hello, how are you?"}],
    "max_tokens": 50
  }'
```

### 5. Cleanup

```bash
ma sandbox delete --gke
```

---

## Sandbox vs Production Differences

| Setting | Sandbox (GKE) | Production |
|---------|---------------|------------|
| GPU tolerations | Added to all pods (operator, etcd, nats, frontend, worker) to allow scheduling on GPU-tainted nodes | Only GPU workloads should have GPU tolerations; control plane runs on CPU nodes |
| NIXL connector | Disabled (`--connector=none`) | Enabled with UCX/RDMA for high-performance KV-cache transfer |
| Node pools | Single GPU pool | Separate CPU and GPU node pools |
| Resource requests | Reduced (2 CPU, 4Gi memory) to fit on limited nodes | Appropriately sized per workload requirements |
| Namespace | `default` | Dedicated namespace with proper RBAC |

### Key Sandbox Workarounds

1. **GPU Node Tolerations**: All Dynamo platform components (operator, etcd, nats) include `nvidia.com/gpu` tolerations because the sandbox cluster may only have GPU nodes available.

2. **Disabled NIXL/UCX**: The NIXL connector requires UCX with InfiniBand/RoCE networking. Standard GKE clusters lack this, so we use `--connector=none`.

3. **LD_LIBRARY_PATH**: Explicitly set to `/usr/local/nvidia/lib64:/usr/local/cuda/lib64` for GKE GPU driver injection.

4. We should add a new getsvcdetails or something similar. This would return the details for the frontend svc. This 

---

## Model Loading

The demo uses `Qwen/Qwen3-0.6B`, a **public HuggingFace model** that requires no authentication.

For **gated models** (Llama, Mistral, etc.) that require HuggingFace tokens:

```bash
# Create secret with HF token
kubectl create secret generic hf-token-secret \
  --from-literal=HF_TOKEN=<your-token> -n default
```

Then add `envFromSecret: hf-token-secret` to the DynamoGraphDeployment service spec.
