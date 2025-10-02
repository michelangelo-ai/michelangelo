# Michelangelo AI Deployment Demo

This demo showcases the complete Michelangelo AI deployment workflow with model mesh support, featuring:

- **Model-agnostic production endpoints** that remain stable across model updates
- **HTTPRoute-based routing** with automatic deployment-specific route creation
- **Triton Inference Server** integration with dynamic model loading
- **Gateway API** with Istio service mesh for traffic management

## Quick Start

### 1. Create and Start Sandbox

```bash
# Create the sandbox cluster (includes Istio setup)
ma sandbox create

# The sandbox already includes:
# - MinIO for model storage
# - Istio service mesh with Gateway API
# - Controller manager for deployment orchestration
```

### 2. Run the Demo

```bash
# Create all demo resources including deployment
ma sandbox demo
```

This will set up:
- ✅ Gateway API and Istio Gateway configuration
- ✅ Storage configuration (MinIO secrets and ConfigMaps)
- ✅ Triton Inference Server with model mesh support
- ✅ BERT model deployment with automatic HTTPRoute creation
- ✅ Training pipelines and project resources

### 3. Access the Production Endpoint

The **model-agnostic production endpoint** is available at:

```
http://localhost:8080/inference-server-bert-cola-endpoint/bert-cola-deployment
```

This endpoint:
- **Never changes** regardless of which model version is deployed
- **Automatically routes** to the current model (e.g., `bert-cola-32`)
- **Supports model mesh** for multiple deployments per inference server

## Demo Files

### Core Deployment Files

- **`deployment.yaml`** - Model deployment specification
- **`inferenceserver.yaml`** - Triton Inference Server configuration
- **`gateway-api-setup.yaml`** - Gateway API and HTTPRoute setup

### Supporting Files

- **`project.yaml`** - Michelangelo project definition
- **`training-pipeline.yaml`** - ML training pipeline example
- **`eval-pipeline.yaml`** - Model evaluation pipeline

### Legacy Files

- **`test-dynamo-deployment.yaml`** - Legacy deployment format
- **`test-istio-gateway.yaml`** - Legacy Istio configuration

## Architecture Overview

### Model Mesh Support

The system supports **model mesh** architecture where:

- One **Inference Server** can host multiple **Deployments**
- Each **Deployment** gets its own stable endpoint
- URLs follow the pattern: `gateway/inference-server/deployment-name`

### Routing Flow

```
Client Request
     ↓
Istio Gateway (ma-gateway)
     ↓
HTTPRoute (inference-server-bert-cola-http-route)
     ↓
Deployment-specific route: /inference-server-bert-cola-endpoint/bert-cola-deployment
     ↓
URL rewrite to: /v2/models/bert-cola-32
     ↓
Triton Inference Server (inference-server-bert-cola-inference-service)
     ↓
Model Response
```

### Automatic HTTPRoute Updates

When a new model is deployed:

1. **ValidationActor** validates the model exists in MinIO storage
2. **ModelSyncActor** updates the inference server ConfigMap
3. **ModelSyncActor** creates/updates HTTPRoute with deployment-specific routing
4. **RolloutActor** completes the deployment process

## Monitoring the Demo

### Check Deployment Status

```bash
# View all deployments
kubectl get deployments.michelangelo.api

# Get deployment details
kubectl describe deployment.michelangelo.api bert-cola-deployment
```

### Check HTTPRoute Configuration

```bash
# View HTTPRoute rules
kubectl get httproute inference-server-bert-cola-http-route -o yaml

# Check for deployment-specific routes
kubectl get httproute -o jsonpath='{.items[0].spec.rules[*].matches[*].path.value}'
```

### Check Inference Server

```bash
# View inference server pods
kubectl get pods -l app=inference-server

# Check model configuration
kubectl get configmap inference-server-bert-cola-model-config -o yaml

# View model loading logs
kubectl logs -l app=triton-inference-server -c model-sync
```

### Test Model Endpoints

```bash
# Test the production endpoint (model-agnostic)
curl http://localhost:8080/inference-server-bert-cola-endpoint/bert-cola-deployment

# Test model health
curl http://localhost:8080/inference-server-bert-cola-endpoint/bert-cola-deployment/v2/health

# Direct inference server access (for debugging)
kubectl port-forward svc/inference-server-bert-cola-inference-service 8000:80
curl http://localhost:8000/v2/models
```

## Updating the Model

To deploy a new model version:

1. **Upload model to MinIO** (simulated by updating the deployment spec)
2. **Update deployment.yaml** with new model name:
   ```yaml
   spec:
     desiredRevision:
       name: bert-cola-33  # New model version
   ```
3. **Apply the update**:
   ```bash
   kubectl apply -f python/michelangelo/cli/sandbox/demo/deployment.yaml
   ```

The system will automatically:
- Validate the new model exists
- Update the inference server ConfigMap
- Update HTTPRoute to point to the new model
- Maintain the same production endpoint URL

## Cleanup

```bash
# Delete all demo resources
kubectl delete -f python/michelangelo/cli/sandbox/demo/

# Or delete the entire sandbox
ma sandbox delete
```

## Troubleshooting

### Common Issues

1. **Gateway not accessible**
   ```bash
   # Check gateway status
   kubectl get gateway ma-gateway

   # Ensure port-forward is running
   kubectl port-forward svc/ma-gateway-istio 8080:80
   ```

2. **Model not loading**
   ```bash
   # Check model storage
   kubectl exec -it minio -- mc ls local/deploy-models/

   # Check inference server logs
   kubectl logs -l app=triton-inference-server -c triton
   ```

3. **HTTPRoute not working**
   ```bash
   # Check route configuration
   kubectl describe httproute inference-server-bert-cola-http-route

   # Check controller logs
   kubectl logs -l app=controller-manager
   ```

### Debug Commands

```bash
# Check all resources
kubectl get all,httproute,gateway,configmap,secret

# View controller manager logs
kubectl logs deployment/michelangelo-controllermgr

# Check Istio proxy status
kubectl get pods -n istio-system

# Test internal connectivity
kubectl run debug --image=alpine --rm -it -- sh
```

## Key Features Demonstrated

- ✅ **Model-agnostic endpoints** that never change
- ✅ **Automatic HTTPRoute management** with deployment-specific routing
- ✅ **Model mesh architecture** supporting multiple deployments
- ✅ **Gateway API integration** with Istio service mesh
- ✅ **Dynamic model loading** via ConfigMap updates
- ✅ **Deep copy fix** for HTTPRoute updates
- ✅ **DynamicClient integration** for runtime Kubernetes resource manipulation
- ✅ **Production-ready architecture** with proper error handling and monitoring