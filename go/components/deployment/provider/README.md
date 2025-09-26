# Deployment Provider

This package defines the provider interface and implementations for ML model deployment in Michelangelo. The provider system enables support for multiple inference serving platforms through a common abstraction layer.

## Overview

The `Provider` interface in `interface.go` defines the contract for deployment providers that handle the lifecycle of ML model deployments. Each provider implementation is responsible for:

1. **Create Deployments**: Deploy ML models to specific inference serving platforms
2. **Rollout Updates**: Handle model version updates and traffic routing changes
3. **Status Monitoring**: Retrieve and update deployment status from the underlying platform
4. **Retirement**: Clean up and retire model deployments

## Available Providers

### Triton Inference Server (`tritoninferenceserver/`)
- **Purpose**: Deploys models to NVIDIA Triton Inference Server
- **Features**: 
  - Dynamic model configuration via ConfigMaps
  - Istio VirtualService integration for traffic routing
  - Support for PyTorch traced models
- **Use Case**: High-performance inference for deep learning models

### KServe (`kserve/`)
- **Purpose**: Deploys models using the KServe serving platform
- **Features**: 
  - Kubernetes-native model serving
  - Built-in autoscaling and monitoring
  - Multi-framework support
- **Use Case**: Cloud-native ML model serving with enterprise features

## Provider Interface

```go
type Provider interface {
    // CreateDeployment creates a new model deployment
    CreateDeployment(ctx context.Context, log logr.Logger, deployment *v2pb.Deployment, model *v2pb.Model) error
    
    // Rollout handles model version updates and traffic routing changes
    Rollout(ctx context.Context, log logr.Logger, deployment *v2pb.Deployment, model *v2pb.Model) error
    
    // GetStatus retrieves current deployment status from the platform
    GetStatus(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error
    
    // Retire cleans up and removes the deployment
    Retire(ctx context.Context, log logr.Logger, deployment *v2pb.Deployment) error
}
```

## Key Components

- **`interface.go`**: Defines the `Provider` interface for deployment lifecycle management
- **Provider implementations**: Each subdirectory contains a specific provider implementation
- **`BUILD.bazel`**: Build configuration for the provider package

## Usage

Providers are registered with the deployment controller through dependency injection. The controller selects the appropriate provider based on the deployment specification and delegates lifecycle operations to the chosen provider.

## Adding New Providers

To add support for a new inference serving platform:

1. Create a new subdirectory for your provider (e.g., `myplatform/`)
2. Implement the `Provider` interface in `client.go`
3. Create a `module.go` file for dependency injection registration
4. Add appropriate `BUILD.bazel` configuration
5. Update the deployment controller to recognize your provider

## Architecture

The provider system follows a plugin architecture where:
- The deployment controller orchestrates the overall deployment lifecycle
- Providers handle platform-specific implementation details
- Common deployment logic is abstracted in the controller layer
- Each provider can have its own configuration and dependencies

This design allows Michelangelo to support multiple inference serving platforms while maintaining a consistent deployment API and user experience.