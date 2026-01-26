package endpointregistry

import (
	"context"

	"github.com/go-logr/logr"
	"go.uber.org/zap"

	backendCommon "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends/common"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var _ EndpointRegistry = &singleClusterRegistry{}

// singleClusterRegistry is an implementation of EndpointRegistry that is used when we want to deploy the inference servers within the control plane cluster.
// In this scenario, there's no need for inter cluster discovery and we can simply rely on the inference server k8s service.
type singleClusterRegistry struct {
	logger *logr.Logger
}

// newSingleClusterRegistry creates a new single cluster registry.
func newSingleClusterRegistry() EndpointRegistry {
	return &singleClusterRegistry{}
}

func (r *singleClusterRegistry) EnsureRegisteredEndpoint(_ context.Context, _ *zap.Logger, _ ClusterEndpoint, _ *v2pb.ClusterTarget) error {
	return nil
}

func (r *singleClusterRegistry) DeleteRegisteredEndpoint(_ context.Context, _ *zap.Logger, _ string, _ string, _ string) error {
	return nil
}

func (r *singleClusterRegistry) ListRegisteredEndpoints(_ context.Context, _ *zap.Logger, _ string, _ string) ([]ClusterEndpoint, error) {
	return nil, nil
}

func (r *singleClusterRegistry) GetControlPlaneServiceName(inferenceServerName string) string {
	return backendCommon.GenerateInferenceServiceName(inferenceServerName)
}
