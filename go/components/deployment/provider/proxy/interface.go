package proxy

import (
	"context"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"

	"github.com/go-logr/logr"
)

// ProxyProvider defines the interface for proxy providers (e.g., Istio)
// that handle traffic routing to serving infrastructure
type ProxyProvider interface {
	// UpdateProxy updates proxy routing resources
	UpdateProxy(ctx context.Context, log logr.Logger, deployment *v2pb.Deployment) error

	// GetProxyStatus checks the status of proxy routing
	// Returns: (modelName string, error) - modelName is the current production route model name, empty if not found
	GetProxyStatus(ctx context.Context, log logr.Logger, deployment *v2pb.Deployment) (string, error)
}
