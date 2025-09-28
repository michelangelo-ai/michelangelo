package strategies

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetZonalActors returns actors for zonal rollout strategy (zone-by-zone deployment)
func GetZonalActors(params Params, deployment *v2pb.Deployment) []plugins.ConditionActor {
	return []plugins.ConditionActor{
		&ZonalRolloutActor{
			client:  params.Client,
			gateway: params.Gateway,
			logger:  params.Logger,
		},
	}
}

// ZonalRolloutActor implements zone-by-zone deployment strategy
type ZonalRolloutActor struct {
	client  client.Client
	gateway gateways.Gateway
	logger  logr.Logger
}

func (a *ZonalRolloutActor) GetType() string {
	return common.ActorTypeZonalRollout
}

func (a *ZonalRolloutActor) GetLogger() logr.Logger {
	return a.logger
}

func (a *ZonalRolloutActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if zonal rollout is complete
	if resource.Status.CurrentRevision != nil &&
		resource.Spec.DesiredRevision != nil &&
		resource.Status.CurrentRevision.Name == resource.Spec.DesiredRevision.Name &&
		resource.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE {

		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "ZonalRolloutCompleted",
			Message: "Zonal rollout completed successfully",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "ZonalRolloutPending",
		Message: "Zonal rollout has not started yet",
	}, nil
}

func (a *ZonalRolloutActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running zonal rollout for deployment", "deployment", resource.Name)

	if resource.Spec.DesiredRevision != nil {
		modelName := resource.Spec.DesiredRevision.Name
		inferenceServerName := resource.Spec.GetInferenceServer().Name

		a.logger.Info("Starting zonal rollout",
			"model", modelName,
			"inference_server", inferenceServerName)

		// Get zones from deployment replicas/infrastructure
		zones, err := a.getTargetZones(ctx, resource)
		if err != nil {
			a.logger.Error(err, "Failed to get target zones")
			return &apipb.Condition{
				Type:    a.GetType(),
				Status:  apipb.CONDITION_STATUS_FALSE,
				Reason:  "ZonalRolloutFailed",
				Message: fmt.Sprintf("Failed to get target zones: %v", err),
			}, nil
		}

		// Deploy zone by zone
		for i, zone := range zones {
			a.logger.Info("Deploying to zone", "zone", zone, "step", i+1, "totalZones", len(zones))

			if err := a.deployToZone(ctx, resource, zone, modelName); err != nil {
				a.logger.Error(err, "Failed to deploy to zone", "zone", zone)
				return &apipb.Condition{
					Type:    a.GetType(),
					Status:  apipb.CONDITION_STATUS_FALSE,
					Reason:  "ZonalRolloutFailed",
					Message: fmt.Sprintf("Failed to deploy to zone %s: %v", zone, err),
				}, nil
			}

			// For OSS, skip the wait between zones to simplify
			a.logger.Info("Zone deployment completed", "zone", zone)
		}

		// Simulate zonal rollout completion
		resource.Status.CurrentRevision = resource.Spec.DesiredRevision
		resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
		resource.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
		a.logger.Info("Zonal rollout completed successfully", "zones", len(zones), "model", modelName)
	}

	return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_TRUE, Reason: "Success", Message: "Operation completed successfully"}, nil
}

func (a *ZonalRolloutActor) getTargetZones(ctx context.Context, deployment *v2pb.Deployment) ([]string, error) {
	// In OSS implementation, we use Kubernetes nodes' zone labels
	// In Uber's implementation, this would query UNS for zone mapping

	nodes := &corev1.NodeList{}
	if err := a.client.List(ctx, nodes); err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	zoneSet := make(map[string]bool)
	for _, node := range nodes.Items {
		if zone, ok := node.Labels["topology.kubernetes.io/zone"]; ok {
			zoneSet[zone] = true
		} else if zone, ok := node.Labels["failure-domain.beta.kubernetes.io/zone"]; ok {
			// Fallback for older Kubernetes versions
			zoneSet[zone] = true
		}
	}

	// Convert set to slice
	zones := make([]string, 0, len(zoneSet))
	for zone := range zoneSet {
		zones = append(zones, zone)
	}

	if len(zones) == 0 {
		// Default to single zone if no zone labels found
		zones = []string{"default-zone"}
	}

	return zones, nil
}

func (a *ZonalRolloutActor) deployToZone(ctx context.Context, deployment *v2pb.Deployment, zone string, modelName string) error {
	// Update model configuration for this zone
	// In OSS implementation, we update ConfigMap with zone-specific annotations
	updateRequest := gateways.ModelConfigUpdateRequest{
		InferenceServer: deployment.Spec.GetInferenceServer().Name,
		Namespace:       deployment.Namespace,
		BackendType:     v2pb.BACKEND_TYPE_TRITON, // Default to Triton
		ModelConfigs: []gateways.ModelConfigEntry{
			{
				Name:   modelName,
				S3Path: fmt.Sprintf("s3://deploy-models/%s/", modelName),
			},
		},
	}

	if err := a.gateway.UpdateModelConfig(ctx, a.logger, updateRequest); err != nil {
		return fmt.Errorf("failed to update model config for zone %s: %w", zone, err)
	}

	// Wait for zone deployment to stabilize
	return a.waitForZoneStability(ctx, deployment, zone)
}

func (a *ZonalRolloutActor) waitForZoneStability(ctx context.Context, deployment *v2pb.Deployment, zone string) error {
	// Wait for pods in the zone to be ready
	// In OSS implementation, we check deployment readiness
	// In Uber's implementation, this would check UNS health for the zone

	timeout := time.After(5 * time.Minute)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for zone %s to stabilize", zone)
		case <-ticker.C:
			// Check if inference server is healthy
			healthy, err := a.gateway.IsHealthy(ctx, a.logger, deployment.Spec.GetInferenceServer().Name, v2pb.BACKEND_TYPE_TRITON)
			if err != nil {
				a.logger.Info("Health check failed, retrying", "zone", zone, "error", err)
				continue
			}
			if healthy {
				a.logger.Info("Zone deployment stabilized", "zone", zone)
				return nil
			}
		}
	}
}
