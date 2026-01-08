package rollout

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// ResourceAcquisitionActor verifies inference server capacity is available for model deployment.
type ResourceAcquisitionActor struct {
	logger  *zap.Logger
	gateway gateways.Gateway
}

// GetType returns the condition type identifier for resource acquisition.
func (a *ResourceAcquisitionActor) GetType() string {
	return common.ActorTypeResourceAcquisition
}

// Retrieve checks if the inference server is healthy and has capacity for the model.
func (a *ResourceAcquisitionActor) Retrieve(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	if resource.Spec.GetInferenceServer() == nil {
		return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_FALSE, Reason: "NoInferenceServer", Message: "No inference server specified for deployment"}, nil
	}

	// Check if the inference server is healthy
	if healthy, err := a.gateway.InferenceServerIsHealthy(ctx, a.logger, resource.Spec.GetInferenceServer().Name, resource.Namespace, v2pb.BACKEND_TYPE_TRITON); err != nil {
		return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_FALSE, Reason: "HealthCheckFailed", Message: fmt.Sprintf("Failed to check health of inference server: %v", err)}, nil
	} else if !healthy {
		return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_FALSE, Reason: "HealthCheckFailed", Message: "Inference server is not healthy"}, nil
	}

	// TODO(#620): ghosharitra: check inference-server capacity to see if model can be loaded.

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "ResourcesAvailable",
		Message: "Resources are available and can be acquired",
	}, nil
}

// Run returns failure since resource acquisition cannot be automatically remediated.
func (a *ResourceAcquisitionActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// If resources were available, Retrieve() would have marked the condition as true.
	// Since resources are not available, there are no further actions to acquire them at this stage.
	// Mark the condition as false to indicate this is a terminal state.
	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "ResourcesNotAvailable",
		Message: "Resources are not available and cannot be acquired",
	}, nil
}
