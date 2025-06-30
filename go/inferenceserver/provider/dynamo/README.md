# NVIDIA Dynamo Provider for Michelangelo

This provider implements NVIDIA Dynamo integration for serving Large Language Models (LLMs) with advanced distributed inference capabilities.

## Features

- **Multi-Backend Support**: vLLM, SGLang, TensorRT-LLM, MistralRS
- **Kubernetes Native**: Built for Kubernetes with CRD-based deployment
- **Smart KV Cache Routing**: Intelligent request routing to minimize recomputation
- **Disaggregated Serving**: Separate prefill/decode for optimal GPU utilization
- **Dynamic GPU Scheduling**: Automatic resource allocation based on demand
- **GitOps Ready**: Declarative configuration management

## Architecture

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│  Model Request  │───▶│  Dynamo Provider │───▶│ DynamoComponent │
└─────────────────┘    └──────────────────┘    └─────────────────┘
                                │                        │
                                ▼                        ▼
                    ┌──────────────────┐    ┌─────────────────┐
                    │ DeploymentState  │    │  Image Build    │
                    │   (ConfigMap)    │    │   (vLLM/etc)    │
                    └──────────────────┘    └─────────────────┘
                                │                        │
                                ▼                        ▼
                    ┌──────────────────────────────────────────┐
                    │       DynamoGraphDeployment              │
                    │  ┌──────────┐ ┌─────────┐ ┌─────────────┐ │
                    │  │Frontend  │ │Processor│ │VllmWorker(s)│ │
                    │  └──────────┘ └─────────┘ └─────────────┘ │
                    └──────────────────────────────────────────┘
```

## Configuration

### Backend Options

| Backend | Description | Use Case |
|---------|-------------|----------|
| `vllm` | vLLM inference engine | General LLM serving with PagedAttention |
| `sglang` | SGLang engine | Structured generation and complex prompting |
| `tensorrtllm` | NVIDIA TensorRT-LLM | Maximum performance on NVIDIA GPUs |
| `mistralrs` | MistralRS engine | Rust-based inference for specific models |

### DynamoConfig

```go
type DynamoConfig struct {
    Backend         string // "vllm", "sglang", "tensorrtllm", "mistralrs"
    ImageRegistry   string // Container registry for Dynamo images
    DefaultReplicas int    // Default number of worker replicas
    ComponentTag    string // Dynamo component tag
}
```

## Usage Examples

### Basic Setup

```go
import (
    "github.com/michelangelo-ai/michelangelo/go/inferenceserver/provider/dynamo"
    "k8s.io/client-go/dynamic"
)

// Create Dynamo provider with vLLM backend
config := &dynamo.DynamoConfig{
    Backend:         "vllm",
    ImageRegistry:   "your-registry.com",
    DefaultReplicas: 2,
    ComponentTag:    "latest",
}

provider := dynamo.NewDynamoInferenceServerProviderWithConfig(dynamicClient, config)
```

### Create Inference Server

```go
// Create empty inference server ready for model deployment
err := provider.CreateInferenceServer(ctx, log, "llm-server", "default", "model-config")
if err != nil {
    log.Error(err, "Failed to create inference server")
}
```

### Deploy Model

The Dynamo provider automatically extracts model information from the InferenceServer protobuf object, following the same pattern as other providers. Model deployment is triggered when an InferenceServer is created or updated:

```go
// Model deployment happens automatically when InferenceServer is created
// The provider extracts model path from InferenceServer.spec.model_name 
// or related Model resource deployable_artifact_uri

err := provider.CreateInferenceServer(ctx, log, "llm-server", "default", "model-config")
if err != nil {
    log.Error(err, "Failed to create inference server")
}

// Model updates are handled through UpdateInferenceServer
err = provider.UpdateInferenceServer(ctx, log, "llm-server", "default")
if err != nil {
    log.Error(err, "Failed to update inference server")
}
```

### Protobuf-Based Integration

The provider follows the established Michelangelo pattern by using protobuf structures instead of custom request objects:

1. **Model Path Extraction**: Automatically extracts model paths from InferenceServer protobuf spec or related Model resources
2. **Component Creation**: Creates DynamoComponent with extracted model information
3. **Deployment Management**: Creates and updates DynamoGraphDeployment for serving
4. **Status Synchronization**: Updates InferenceServer status based on Dynamo deployment state

```go
// The provider automatically handles model deployment through protobuf integration
// No custom request structures needed - uses existing Michelangelo APIs
```

## Kubernetes Resources

### DynamoGraphDeployment Example

```yaml
apiVersion: nvidia.com/v1alpha1
kind: DynamoGraphDeployment
metadata:
  name: llm-server-deployment
  namespace: default
spec:
  dynamoComponent: frontend-llama2-7b-chat-1234567890
  services:
    Frontend:
      replicas: 1
    Processor:
      replicas: 1
    VllmWorker:
      replicas: 4
      environment:
        MODEL_NAME: llama2-7b-chat
        TENSOR_PARALLEL_SIZE: "2"
        GPU_MEMORY_UTILIZATION: "0.9"
```

### DynamoComponent Example

```yaml
apiVersion: nvidia.com/v1alpha1
kind: DynamoComponent
metadata:
  name: frontend-llama2-7b-chat-1234567890
  namespace: default
spec:
  build:
    framework: vllm
    model: llama2-7b-chat
    modelPath: meta-llama/Llama-2-7b-chat-hf
  image:
    registry: your-registry.com
    tag: vllm-latest
```

## Comparison with Triton Provider

| Feature | Triton Provider | Dynamo Provider |
|---------|----------------|-----------------|
| **Model Types** | Traditional ML (ONNX, TF, PyTorch) | Large Language Models |
| **Serving Style** | Single model per server | Distributed multi-service |
| **Scaling** | Horizontal pod scaling | Advanced GPU scheduling |
| **Memory Management** | Basic GPU memory | Multi-tier KV cache |
| **Deployment** | Direct model loading | Component-based builds |
| **Best For** | CV, traditional ML | LLMs, generative AI |

## Performance Benefits

- **30x Throughput**: Proven improvements for large reasoning models
- **Intelligent Routing**: KV cache-aware request distribution
- **Memory Efficiency**: Multi-tier caching (GPU → CPU → SSD → Object Storage)
- **Disaggregated Serving**: Optimized prefill/decode separation

## Prerequisites

1. **NVIDIA Dynamo Operator**: Must be installed in the Kubernetes cluster
2. **GPU Resources**: NVIDIA GPUs with sufficient memory
3. **Container Registry**: For storing built Dynamo components
4. **Storage**: PVCs for model storage and KV cache

## Monitoring

```bash
# Check DynamoGraphDeployments
kubectl get dynamographdeployment

# Check DynamoComponents
kubectl get dynamocomponent

# Check deployment state
kubectl get configmap llm-server-dynamo-state -o jsonpath='{.data.deployment-state\.json}' | jq .
```

## Troubleshooting

### Common Issues

1. **Build Failures**: Check DynamoComponent status and logs
2. **Deployment Timeouts**: Verify GPU resources and quotas
3. **Model Loading Errors**: Check model path and registry access
4. **Performance Issues**: Tune environment variables (tensor parallelism, memory utilization)

### Debugging Commands

```bash
# Check provider logs
kubectl logs -l michelangelo.ai/provider=dynamo

# Check component build logs
kubectl logs -l nvidia.com/dynamo-component=<component-id>

# Check deployment status
kubectl describe dynamographdeployment <deployment-name>
```

This provider brings state-of-the-art LLM serving capabilities to Michelangelo, complementing the existing Triton provider for comprehensive ML model serving solutions.