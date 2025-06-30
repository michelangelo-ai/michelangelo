# Provider Comparison: Triton vs Dynamo

This document compares the two inference serving providers available in Michelangelo and provides guidance on when to use each.

## Quick Comparison

| Aspect | Triton Provider | Dynamo Provider |
|--------|----------------|-----------------|
| **Primary Use Case** | Traditional ML Models | Large Language Models |
| **Model Formats** | ONNX, TensorFlow, PyTorch, TensorRT | HuggingFace, Custom LLMs |
| **Serving Architecture** | Single-server model loading | Distributed multi-service graph |
| **Memory Management** | Basic GPU memory allocation | Advanced KV cache + multi-tier storage |
| **Scaling Strategy** | Horizontal pod replication | Dynamic GPU scheduling + disaggregated serving |
| **Request Routing** | Load balancer | Smart KV cache-aware routing |
| **Performance Focus** | Model throughput | Token generation efficiency |

## When to Use Triton Provider

### ✅ **Best For:**
- **Computer Vision Models**: Image classification, object detection, segmentation
- **Traditional ML**: Sklearn models, XGBoost, classical algorithms  
- **Structured Data**: Tabular data predictions, recommendation systems
- **Real-time Inference**: Low-latency requirements with consistent response times
- **Batch Processing**: High-throughput batch inference workloads
- **Model Ensemble**: Serving multiple related models together

### 🏗️ **Architecture Benefits:**
- **Simple Deployment**: Direct model file loading
- **Resource Predictability**: Fixed resource allocation per model
- **Wide Format Support**: Extensive model format compatibility
- **Mature Ecosystem**: Battle-tested in production environments

### 💡 **Example Use Cases:**
```go
// Image classification service
provider.DeployModel(ctx, log, "vision-server", "default", &ModelRequest{
    ModelName: "resnet50-classifier",
    ModelPath: "s3://models/resnet50.onnx",
})

// Recommendation system
provider.DeployModel(ctx, log, "recsys-server", "default", &ModelRequest{
    ModelName: "collaborative-filtering",
    ModelPath: "s3://models/cf-model.savedmodel",
})
```

## When to Use Dynamo Provider

### ✅ **Best For:**
- **Large Language Models**: GPT, LLaMA, Mistral, CodeLLaMA
- **Conversational AI**: Chatbots, virtual assistants, Q&A systems
- **Text Generation**: Content creation, code generation, creative writing
- **Complex Reasoning**: Multi-step reasoning, chain-of-thought tasks
- **High-Concurrency**: Many simultaneous conversations
- **Resource Optimization**: Maximizing GPU utilization for expensive models

### 🏗️ **Architecture Benefits:**
- **Intelligent Caching**: KV cache reuse across requests
- **Disaggregated Serving**: Separate prefill/decode optimization
- **Dynamic Scheduling**: Automatic resource allocation based on load
- **Multi-Backend Support**: Choose optimal inference engine per model

### 💡 **Example Use Cases:**
```go
// Conversational AI service
provider.DeployModel(ctx, log, "chat-server", "default", &dynamo.ModelRequest{
    ModelName: "llama2-70b-chat",
    ModelPath: "meta-llama/Llama-2-70b-chat-hf",
    Backend:   "vllm",
    Environment: map[string]string{
        "TENSOR_PARALLEL_SIZE": "8",
    },
})

// Code generation service  
provider.DeployModel(ctx, log, "code-server", "default", &dynamo.ModelRequest{
    ModelName: "codellama-34b",
    ModelPath: "codellama/CodeLlama-34b-Instruct-hf",
    Backend:   "tensorrtllm",
})
```

## Multi-Provider Architecture

You can use both providers simultaneously in the same cluster:

```go
import (
    "github.com/michelangelo-ai/michelangelo/go/inferenceserver/provider/triton"
    "github.com/michelangelo-ai/michelangelo/go/inferenceserver/provider/dynamo"
)

// Provider registry
providers := map[string]serving.Provider{
    "triton": triton.NewTritonInferenceServerProvider(dynamicClient),
    "dynamo": dynamo.NewDynamoInferenceServerProvider(dynamicClient),
}

// Route based on model type
func getProvider(modelType string) serving.Provider {
    switch modelType {
    case "llm", "chat", "text-generation":
        return providers["dynamo"]
    case "vision", "tabular", "classical":
        return providers["triton"]
    default:
        return providers["triton"] // Default fallback
    }
}
```

## Performance Characteristics

### Triton Provider Performance
- **Latency**: 1-50ms (depending on model complexity)
- **Throughput**: 100-10,000 requests/second
- **Memory**: Predictable GPU memory usage
- **Scaling**: Linear with replica count

### Dynamo Provider Performance  
- **First Token Latency**: 50-500ms (depending on model size)
- **Token Generation**: 10-100 tokens/second per request
- **Memory**: Dynamic with intelligent cache management
- **Scaling**: Non-linear with smart resource sharing

## Cost Considerations

### Triton Provider Costs
- **Predictable**: Fixed resource allocation
- **Efficient**: Good utilization for steady workloads  
- **Scaling**: Linear cost increase with load

### Dynamo Provider Costs
- **Variable**: Dynamic resource allocation
- **Optimized**: Better utilization for LLM workloads
- **Sharing**: Cost benefits from KV cache reuse

## Migration Strategies

### From Traditional ML to LLMs
```yaml
# Phase 1: Deploy both providers
apiVersion: v1
kind: ConfigMap  
metadata:
  name: model-routing-config
data:
  providers.yaml: |
    routes:
      - pattern: ".*-chat$"
        provider: dynamo
        backend: vllm
      - pattern: ".*-classifier$" 
        provider: triton
      - pattern: ".*"
        provider: triton  # Default

# Phase 2: Gradually migrate models
# Phase 3: Optimize resource allocation
```

### Hybrid Deployments
```go
// Serve different model types optimally
type ModelRouter struct {
    tritonProvider serving.Provider
    dynamoProvider serving.Provider
}

func (r *ModelRouter) RouteModel(modelConfig ModelConfig) serving.Provider {
    if modelConfig.IsLLM() {
        return r.dynamoProvider
    }
    return r.tritonProvider
}
```

## Best Practices

### Resource Planning
- **Triton**: Allocate based on peak model memory requirements
- **Dynamo**: Plan for dynamic allocation with burst capacity

### Monitoring
- **Triton**: Focus on model latency and throughput metrics
- **Dynamo**: Monitor token generation rates and cache hit ratios

### Deployment Strategy
- **Triton**: Blue/green deployments for model updates
- **Dynamo**: Component-based rolling updates

## Conclusion

Both providers serve complementary roles in a modern ML infrastructure:

- **Use Triton** for traditional ML models requiring predictable performance
- **Use Dynamo** for LLMs requiring advanced serving optimizations  
- **Use Both** for comprehensive ML serving covering all model types

The choice depends on your specific use case, performance requirements, and cost constraints. Many organizations benefit from deploying both providers to handle their diverse ML workloads optimally.