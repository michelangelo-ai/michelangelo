//go:generate mamockgen Gateway

package gateways

import (
	"context"
	"net/http"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Gateway provides a unified interface for interacting with inference servers across different backend types.
type Gateway interface {
	// CheckModelStatus verifies if a model is ready to serve requests.
	CheckModelStatus(ctx context.Context, logger *zap.Logger, kubeClient client.Client, httpClient *http.Client, modelName string, inferenceServerName string, namespace string, backendType v2pb.BackendType) (bool, error)
	// InferenceServerIsHealthy checks if the inference server is healthy.
	InferenceServerIsHealthy(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string, backendType v2pb.BackendType) (bool, error)
}
