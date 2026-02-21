package steadystate

import (
	"context"
	"fmt"
	"net/http"

	"go.uber.org/zap"

	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// SteadyStateActor monitors deployment health and maintains stable operation after rollout completion.
type SteadyStateActor struct {
	logger        *zap.Logger
	registry      *backends.Registry
	clientFactory clientfactory.ClientFactory
	client        client.Client
	httpClient    *http.Client
}

// GetType returns the condition type identifier for steady state.
func (a *SteadyStateActor) GetType() string {
	return common.ActorTypeSteadyState
}

// Retrieve checks if deployment is in steady state (complete and healthy).
func (a *SteadyStateActor) Retrieve(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if deployment is in steady state (complete and healthy)
	a.logger.Info("Monitoring steady state for deployment", zap.String("deployment", resource.Name))
	targetClusters := common.GetInferenceServerTargetClusters(ctx, a.client, resource)
	// todo: ghosharitra: this is a placeholder for now.
	backend, err := a.registry.GetBackend(v2pb.BACKEND_TYPE_TRITON)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "BackendNotFound", fmt.Sprintf("Failed to get backend: %v", err)), err
	}

	// todo: ghosoharitra: to handle cases such as these, we can create a helper function which takes in (isControlPlaneCluster, client, httpClient, clientfactory), then return a map[clusterID]Client
	if len(targetClusters) == 0 {
		healthy, err := backend.IsHealthy(ctx, a.logger, a.client, resource.Name, resource.Namespace)
		if err != nil {
			return conditionsutil.GenerateFalseCondition(condition, "HealthCheckFailed", fmt.Sprintf("Failed to check health of inference server: %v", err)), nil
		}
		if !healthy {
			return conditionsutil.GenerateFalseCondition(condition, "HealthCheckFailed", "Inference server is not healthy"), nil
		}
		modelReady, err := backend.CheckModelStatus(ctx, a.logger, a.client, a.httpClient, resource.Name, resource.Namespace, resource.Spec.DesiredRevision.Name)
		if err != nil {
			return conditionsutil.GenerateFalseCondition(condition, "ModelHealthCheckFailed", fmt.Sprintf("Failed to check model status: %v", err)), nil
		}
		if !modelReady {
			return conditionsutil.GenerateFalseCondition(condition, "ModelHealthCheckFailed", "Model is not ready"), nil
		}
		return conditionsutil.GenerateTrueCondition(condition), nil
	}

	// Check if the inference server is healthy for all remote target clusters
	for _, targetCluster := range targetClusters {
		targetClusterClient, err := a.clientFactory.GetClient(ctx, targetCluster)
		if err != nil {
			// todo: ghosharitra: during errors, we should just log error and continue
			return conditionsutil.GenerateFalseCondition(condition, "HealthCheckFailed", fmt.Sprintf("Failed to get client for cluster %s: %v", targetCluster.ClusterId, err)), nil
		}
		targetHTTPClient, err := a.clientFactory.GetHTTPClient(ctx, targetCluster)
		if err != nil {
			// todo: ghosharitra: during errors, we should just log error and continue
			return conditionsutil.GenerateFalseCondition(condition, "HealthCheckFailed", fmt.Sprintf("Failed to get HTTP client for cluster %s: %v", targetCluster.ClusterId, err)), nil
		}
		healthy, err := backend.IsHealthy(ctx, a.logger, targetClusterClient, resource.Name, resource.Namespace)
		if err != nil {
			return conditionsutil.GenerateFalseCondition(condition, "HealthCheckFailed", fmt.Sprintf("Failed to check health of inference server: %v", err)), nil
		}
		if !healthy {
			return conditionsutil.GenerateFalseCondition(condition, "HealthCheckFailed", "Inference server is not healthy"), nil
		}
		// Check if the desired model is ready in the target cluster
		modelReady, err := backend.CheckModelStatus(ctx, a.logger, targetClusterClient, targetHTTPClient, resource.Name, resource.Namespace, resource.Spec.DesiredRevision.Name)
		if err != nil {
			a.logger.Error("failed to check model status",
				zap.Error(err),
				zap.String("operation", "steady_state_model_check"),
				zap.String("namespace", resource.Namespace),
				zap.String("deployment", resource.Name),
				zap.String("model", resource.Spec.DesiredRevision.Name),
				zap.String("cluster_id", targetCluster.ClusterId))
			return conditionsutil.GenerateFalseCondition(condition, "ModelHealthCheckFailed", fmt.Sprintf("Failed to check model status in cluster %s: %v", targetCluster.ClusterId, err)), nil
		}
		if !modelReady {
			return conditionsutil.GenerateFalseCondition(condition, "ModelHealthCheckFailed", fmt.Sprintf("Model is not ready in cluster %s", targetCluster.ClusterId)), nil
		}
	}

	a.logger.Info("Deployment is in steady state", zap.String("deployment", resource.Name))
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run continuously monitors inference server and model health to maintain steady state.
func (a *SteadyStateActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// nothing actionable for steady state, simply return the condition
	return condition, nil
}
