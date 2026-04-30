package creation

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &HealthCheckActor{}

// HealthCheckActor verifies inference server health by polling backend health endpoints.
type HealthCheckActor struct {
	registry      *backends.Registry
	logger        *zap.Logger
	clientFactory clientfactory.ClientFactory
}

// NewHealthCheckActor creates a condition actor for inference server health verification.
func NewHealthCheckActor(clientFactory clientfactory.ClientFactory, registry *backends.Registry, logger *zap.Logger) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &HealthCheckActor{
		clientFactory: clientFactory,
		registry:      registry,
		logger:        logger,
	}
}

// GetType returns the condition type identifier for health checks.
func (a *HealthCheckActor) GetType() string {
	return common.HealthCheckConditionType
}

// Retrieve checks the current health status of the inference server.
func (a *HealthCheckActor) Retrieve(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Retrieving inference server health condition")

	backend, err := a.registry.GetBackend(resource.Spec.BackendType)
	if err != nil {
		return conditionUtils.GenerateFalseCondition(condition, "BackendNotFound", fmt.Sprintf("Failed to get backend: %v", err)), nil
	}

	clusterStatuses, failures := a.checkClusterHealth(ctx, backend, resource)
	resource.Status.ClusterStatuses = clusterStatuses

	if len(failures) > 0 {
		return conditionUtils.GenerateFalseCondition(condition, "HealthCheckFailed", strings.Join(failures, "; ")), nil
	}
	return conditionUtils.GenerateTrueCondition(condition), nil
}

// Run returns the condition unchanged. Health check failures are not auto-remediable.
func (a *HealthCheckActor) Run(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	return condition, nil
}

// checkClusterHealth polls IsHealthy on each cluster target and returns per-cluster statuses
// along with a list of failure messages for clusters that are not yet healthy.
func (a *HealthCheckActor) checkClusterHealth(ctx context.Context, backend backends.Backend, resource *v2pb.InferenceServer) ([]*v2pb.ClusterTargetStatus, []string) {
	clusterStatuses := make([]*v2pb.ClusterTargetStatus, 0, len(resource.Spec.ClusterTargets))
	var failures []string

	for _, target := range resource.Spec.ClusterTargets {
		kubeClient, err := a.clientFactory.GetClient(ctx, target)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: client error: %v", target.GetClusterId(), err))
			clusterStatuses = append(clusterStatuses, &v2pb.ClusterTargetStatus{
				ClusterId: target.GetClusterId(),
				State:     v2pb.INFERENCE_SERVER_STATE_CREATING,
				Message:   err.Error(),
			})
			continue
		}

		healthy, err := backend.IsHealthy(ctx, a.logger, kubeClient, resource.Name, resource.Namespace)
		if err != nil {
			a.logger.Error("Health check failed",
				zap.Error(err),
				zap.String("operation", "health_check"),
				zap.String("namespace", resource.Namespace),
				zap.String("inferenceServer", resource.Name),
				zap.String("cluster_id", target.GetClusterId()))
			failures = append(failures, fmt.Sprintf("%s: %v", target.GetClusterId(), err))
			clusterStatuses = append(clusterStatuses, &v2pb.ClusterTargetStatus{
				ClusterId: target.GetClusterId(),
				State:     v2pb.INFERENCE_SERVER_STATE_CREATING,
				Message:   err.Error(),
			})
			continue
		}

		state := v2pb.INFERENCE_SERVER_STATE_CREATING
		if healthy {
			state = v2pb.INFERENCE_SERVER_STATE_SERVING
		} else {
			failures = append(failures, fmt.Sprintf("%s: not healthy", target.GetClusterId()))
		}
		clusterStatuses = append(clusterStatuses, &v2pb.ClusterTargetStatus{
			ClusterId: target.GetClusterId(),
			State:     state,
		})
	}

	return clusterStatuses, failures
}
