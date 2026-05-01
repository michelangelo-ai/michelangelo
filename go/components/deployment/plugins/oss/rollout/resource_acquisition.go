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
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &ResourceAcquisitionActor{}

// ResourceAcquisitionActor verifies inference server capacity is available for model deployment.
type ResourceAcquisitionActor struct {
	logger        *zap.Logger
	registry      backends.Registry
	clientFactory clientfactory.ClientFactory
	defaultClient client.Client
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

	// todo: ghosharitra: this is where we should check all target inference server health and capacity. Then we should add an annotation in the CR for healthy targets.
	// Check if the inference server is healthy for all target clusters
	// todo: ghosharitra: this needs cleanup.
	backend, err := a.registry.GetBackend(v2pb.BACKEND_TYPE_TRITON)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "BackendNotFound", fmt.Sprintf("Failed to get backend: %v", err)), err
	}
	targetClusters := common.GetInferenceServerTargetClusters(ctx, a.defaultClient, resource)
	if len(targetClusters) == 0 {
		healthy, err := backend.IsHealthy(ctx, a.logger, a.defaultClient, resource.Name, resource.Namespace)
		if err != nil {
			return conditionsutil.GenerateFalseCondition(condition, "HealthCheckFailed", fmt.Sprintf("Failed to check health of inference server: %v", err)), nil
		}
		if !healthy {
			return conditionsutil.GenerateFalseCondition(condition, "HealthCheckFailed", "Inference server is not healthy"), nil
		}
		return conditionsutil.GenerateTrueCondition(condition), nil
	}

	for _, targetCluster := range targetClusters {
		targetClusterClient, err := a.clientFactory.GetClient(ctx, targetCluster)
		if err != nil {
			// todo: ghosharitra: I think we should just log error and continue here.
			return conditionsutil.GenerateFalseCondition(condition, "HealthCheckFailed", fmt.Sprintf("Failed to get client for cluster %s: %v", targetCluster.ClusterId, err)), nil
		}
		healthy, err := backend.IsHealthy(ctx, a.logger, targetClusterClient, resource.Name, resource.Namespace)
		if err != nil {
			return conditionsutil.GenerateFalseCondition(condition, "HealthCheckFailed", fmt.Sprintf("Failed to check health of inference server: %v", err)), nil
		}
		if !healthy {
			return conditionsutil.GenerateFalseCondition(condition, "HealthCheckFailed", fmt.Sprintf("Inference server is not healthy in cluster %s", targetCluster.ClusterId)), nil
		}
	}
	// todo: ghosharitra: I think the run() function can be responsible for creating the annotation. In retrieve we can check if the currently set annotation matches the inference server CR.
	// TODO(#620): ghosharitra: check inference-server capacity to see if model can be loaded.

	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run returns failure since resource acquisition cannot be automatically remediated.
func (a *ResourceAcquisitionActor) Run(ctx context.Context, _ *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// If resources were available, Retrieve() would have marked the condition as true.
	// Since resources are not available, there are no further actions to acquire them at this stage.
	return condition, nil
}
