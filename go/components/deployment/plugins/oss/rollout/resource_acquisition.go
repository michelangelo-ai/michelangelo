package rollout

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &ResourceAcquisitionActor{}

// ResourceAcquisitionActor verifies inference server capacity is available for model deployment.
type ResourceAcquisitionActor struct {
	logger          *zap.Logger
	client          client.Client
	backendRegistry *backends.Registry
}

// GetType returns the condition type identifier for resource acquisition.
func (a *ResourceAcquisitionActor) GetType() string {
	return common.ActorTypeResourceAcquisition
}

// Retrieve checks if the inference server is healthy and has capacity for the model.
func (a *ResourceAcquisitionActor) Retrieve(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	if resource.Spec.GetInferenceServer() == nil {
		return conditionsutil.GenerateFalseCondition(condition, "NoInferenceServer", "No inference server specified for deployment"), nil
	}

	// Check if the inference server is healthy
	serverBackend, err := a.backendRegistry.GetBackend(v2pb.BACKEND_TYPE_TRITON)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "HealthCheckFailed", fmt.Sprintf("Failed to get backend for inference server %s: %v", resource.Spec.GetInferenceServer().Name, err)), err
	}
	if healthy, err := serverBackend.IsHealthy(ctx, a.logger, a.client, resource.Spec.GetInferenceServer().Name, resource.Namespace); err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "HealthCheckFailed", fmt.Sprintf("Failed to check health of inference server: %v", err)), err
	} else if !healthy {
		return conditionsutil.GenerateFalseCondition(condition, "HealthCheckFailed", "Inference server is not healthy"), nil
	}

	// TODO(#620): ghosharitra: check inference-server capacity to see if model can be loaded.
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run returns failure since resource acquisition cannot be automatically remediated.
func (a *ResourceAcquisitionActor) Run(ctx context.Context, _ *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// If resources were available, Retrieve() would have marked the condition as true.
	// Since resources are not available, there are no further actions to acquire them at this stage.
	return condition, nil
}
