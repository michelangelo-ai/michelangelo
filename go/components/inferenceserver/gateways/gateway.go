package gateways

import (
	"context"
	"fmt"
	"net/http"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// gateway implements the Gateway interface
type gateway struct {
	registry *backends.Registry
}

// Params contains dependencies for creating a Gateway.
type Params struct {
	Registry *backends.Registry
}

// NewGatewayWithBackends creates a new inference server gateway with a backend registry.
func NewGatewayWithBackends(p Params) Gateway {
	return &gateway{
		registry: p.Registry,
	}
}

// CheckModelStatus dispatches model status checking based on backend type
func (g *gateway) CheckModelStatus(ctx context.Context, logger *zap.Logger, kubeClient client.Client, httpClient *http.Client, modelName string, inferenceServerName string, namespace string, backendType v2pb.BackendType) (bool, error) {
	if backendType == v2pb.BACKEND_TYPE_INVALID {
		return false, fmt.Errorf("invalid backend type: %v", backendType)
	}
	backend, err := g.registry.GetBackend(backendType)
	if err != nil {
		return false, fmt.Errorf("failed to get backend for model %s on %s/%s: %w", modelName, namespace, inferenceServerName, err)
	}
	return backend.CheckModelStatus(ctx, logger, kubeClient, httpClient, inferenceServerName, namespace, modelName)
}

// IsHealthy dispatches health checking based on backend type
func (g *gateway) InferenceServerIsHealthy(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string, backendType v2pb.BackendType) (bool, error) {
	if backendType == v2pb.BACKEND_TYPE_INVALID {
		return false, fmt.Errorf("invalid backend type: %v", backendType)
	}
	backend, err := g.registry.GetBackend(backendType)
	if err != nil {
		return false, fmt.Errorf("unable to get backend for inference server %s in namespace %s: %w", inferenceServerName, namespace, err)
	}
	return backend.IsHealthy(ctx, logger, kubeClient, inferenceServerName, namespace)
}
