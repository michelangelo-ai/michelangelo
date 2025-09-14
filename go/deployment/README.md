# Deployment Controller

The Deployment Controller is responsible for managing ML model deployments in the Michelangelo AI platform. It orchestrates the entire deployment lifecycle from validation to rollout completion, with support for both OSS and enterprise environments.

## Overview

The deployment system follows a plugin-based architecture where different deployment strategies can be implemented as plugins. Each plugin contains actors that handle specific stages of the deployment process.

## Architecture

### Core Components

1. **Deployment Controller** (`controller.go`) - Main reconciliation loop
2. **Plugin System** (`plugins/`) - Modular deployment strategies
3. **Config Provider** (`provider/config/`) - Manages model configurations and routing
4. **Actors** - Individual components that handle specific deployment tasks

### Plugin Architecture

The system uses a condition-based actor model where:
- **Plugins** define the overall deployment strategy
- **Actors** handle specific conditions/stages
- **Conditions** track the state of each deployment stage

## OSS Plugin Implementation

The OSS plugin (`plugins/oss/`) implements a complete deployment workflow for open-source environments.

### Deployment Stages

#### 1. Pre-Placement Phase
- **ValidationActor**: Validates deployment configuration and model existence
- **AssetPreparationActor**: Ensures model assets are available in storage
- **ResourceAcquisitionActor**: Verifies inference server resources

#### 2. Placement Phase
- **ModelSyncActor**: Syncs model configuration and updates routing
- **RollingRolloutActor**: Implements rolling deployment strategy

#### 3. Post-Placement Phase
- **RolloutCompletionActor**: Handles cleanup and finalization
- **SteadyStateActor**: Monitors ongoing deployment health

### Model Mesh Support

The deployment system supports **model mesh** architecture where a single inference server can handle multiple deployment endpoints simultaneously.

#### Production Endpoint Pattern

```
http://{gateway}/{inference-server}-endpoint/{deployment-name}
```

**Example:**
```
http://localhost:8080/inference-server-bert-cola-endpoint/bert-cola-deployment
```

This endpoint is **model-agnostic** - it always serves the current model version without requiring changes to the URL when models are updated.

#### HTTPRoute Configuration

The system automatically creates deployment-specific routes in the HTTPRoute configuration:

```yaml
spec:
  rules:
  # Deployment-specific route (highest priority)
  - matches:
    - path:
        type: PathPrefix
        value: /inference-server-bert-cola-endpoint/bert-cola-deployment
    backendRefs:
    - name: inference-server-bert-cola-inference-service
      port: 80
    filters:
    - type: URLRewrite
      urlRewrite:
        path:
          type: ReplacePrefixMatch
          replacePrefixMatch: /v2/models/bert-cola-32

  # Inference server routes (fallback)
  - matches:
    - path:
        type: PathPrefix
        value: /inference-server-bert-cola-endpoint/inference-server-bert-cola/v2/models
    # ... other inference server routes
```

## Key Features

### 1. Model-Agnostic Routing

The system creates stable, deployment-based URLs that don't change when model versions are updated:

- **Static URL**: `/inference-server-bert-cola-endpoint/bert-cola-deployment`
- **Dynamic Backend**: Routes to current model (e.g., `bert-cola-32`)
- **Automatic Updates**: HTTPRoute is updated during deployment to point to new model

### 2. Deep Copy Fix for HTTPRoute Updates

The system includes a fix for Kubernetes deep copy issues when updating HTTPRoute configurations:

```go
// Update the HTTPRoute by setting the rules directly without deep copy
// This avoids the deep copy issue with complex nested structures
if httpRoute.Object["spec"] == nil {
    httpRoute.Object["spec"] = make(map[string]interface{})
}
spec := httpRoute.Object["spec"].(map[string]interface{})
spec["rules"] = rules
```

### 3. DynamicClient Integration

The deployment system uses Kubernetes DynamicClient for runtime resource manipulation:

```go
// Create dynamic client from manager's REST config
restConfig := client.GetConfig()
dynamicClient, err := dynamic.NewForConfig(restConfig)
if err != nil {
    log.Error(err, "Failed to create dynamic client")
    dynamicClient = nil
}
```

### 4. Gateway Integration

The system integrates with the gateway layer for:
- Model configuration updates (ConfigMaps)
- HTTPRoute/VirtualService routing updates
- Health monitoring and status reporting

## Deployment Workflow

### 1. Deployment Creation/Update

When a deployment resource is created or updated:

```yaml
apiVersion: michelangelo.api/v2
kind: Deployment
metadata:
  name: bert-cola-deployment
spec:
  desiredRevision:
    name: bert-cola-32
  inferenceServer:
    name: inference-server-bert-cola
```

### 2. Validation Phase

- Validates deployment spec (desired revision, inference server)
- Checks model existence in storage (MinIO/S3)
- Verifies inference server availability

### 3. Model Sync Phase

- Updates inference server ConfigMap with new model configuration
- Creates/updates HTTPRoute with deployment-specific routing
- Handles URL rewriting to route to specific model version

### 4. Rollout Phase

- Implements rolling deployment strategy
- Monitors model loading on inference servers
- Updates deployment status throughout process

### 5. Completion Phase

- Cleans up old model configurations
- Removes temporary deployment annotations
- Marks deployment as healthy and complete

## Configuration

### Model Storage

Models are stored in MinIO/S3 with the following pattern:
```
s3://deploy-models/{model-name}/
```

### Inference Server Integration

The system integrates with Triton Inference Server by default:
- ConfigMaps contain model configurations
- Model loading is triggered by config changes
- Health checks verify model availability

### Gateway Configuration

The system supports both:
- **Gateway API HTTPRoute** (preferred)
- **Istio VirtualService** (fallback)

## Monitoring and Observability

### Deployment Status

Deployments report status through:
- **Stage**: Current deployment phase (validation, placement, rollout, etc.)
- **State**: Overall health (healthy, unhealthy, initializing)
- **Conditions**: Detailed status of each actor
- **Current Revision**: Actually deployed model version

### Logging

Comprehensive logging throughout the deployment process:
- Actor execution logs
- HTTPRoute update logs
- Model sync progress
- Error details with context

## Development

### Adding New Actors

1. Implement the `ConditionActor` interface:
```go
type MyActor struct {
    client client.Client
    logger logr.Logger
}

func (a *MyActor) GetType() string {
    return "MyCondition"
}

func (a *MyActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
    // Check if condition is met
}

func (a *MyActor) Run(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) error {
    // Execute the action
}
```

2. Add to plugin's actor list:
```go
func (p *MyPlugin) GetActors() []plugins.ConditionActor {
    return []plugins.ConditionActor{
        &MyActor{client: p.client, logger: p.logger},
        // ... other actors
    }
}
```

### Testing

Test deployments by creating deployment resources:

```bash
kubectl apply -f - <<EOF
apiVersion: michelangelo.api/v2
kind: Deployment
metadata:
  name: test-deployment
spec:
  desiredRevision:
    name: my-model-v1
  inferenceServer:
    name: my-inference-server
EOF
```

Monitor deployment progress:
```bash
kubectl get deployments.michelangelo.api -o wide
kubectl describe deployment.michelangelo.api test-deployment
```

### Building

Build the controller:
```bash
bazel build //go/cmd/controllermgr
```

Run the controller:
```bash
bazel run //go/cmd/controllermgr
```

## Troubleshooting

### Common Issues

1. **Deep Copy Errors**: Fixed in current implementation with direct spec assignment
2. **DynamicClient Nil**: Ensure proper injection through module configuration
3. **HTTPRoute Not Found**: Verify Gateway API CRDs are installed
4. **Model Not Found**: Check MinIO/S3 storage accessibility

### Debug Commands

```bash
# Check deployment status
kubectl get deployments.michelangelo.api

# View HTTPRoute configuration
kubectl get httproute -o yaml

# Check inference server configuration
kubectl get configmaps

# View controller logs
kubectl logs -l app=controller-manager

# Test endpoint directly
curl http://localhost:8080/inference-server-{name}-endpoint/{deployment-name}
```

## Best Practices

1. **Model Naming**: Use semantic versioning for model names
2. **Resource Management**: Ensure inference servers have adequate resources
3. **Monitoring**: Set up alerts on deployment state changes
4. **Rollback**: Keep previous model versions available for quick rollback
5. **Testing**: Test deployments in staging before production

## Security Considerations

1. **Storage Access**: Secure MinIO/S3 credentials
2. **Network Policies**: Restrict inference server network access
3. **RBAC**: Limit deployment controller permissions
4. **Secrets Management**: Use Kubernetes secrets for sensitive configuration

## Performance Optimization

1. **Model Caching**: Cache frequently used models
2. **Resource Limits**: Set appropriate CPU/memory limits
3. **Scaling**: Use horizontal pod autoscaling for inference servers
4. **Monitoring**: Track model loading times and inference latency