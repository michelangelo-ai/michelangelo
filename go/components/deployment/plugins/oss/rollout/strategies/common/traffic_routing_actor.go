package common

import (
	"context"
	"fmt"

	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/proxy"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

const metadataKeyControlPlaneServiceName = "control_plane_service_name"

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &TrafficRoutingActor{}

// TrafficRoutingActor manages HTTPRoute configuration to route deployment traffic to models.
type TrafficRoutingActor struct {
	ProxyProvider proxy.ProxyProvider
	Gateway       gateways.Gateway
	Logger        *zap.Logger
}

// GetType returns the condition type identifier for traffic routing.
func (a *TrafficRoutingActor) GetType() string {
	return common.ActorTypeTrafficRouting
}

// Retrieve checks if the Gateway API HTTPRoute is correctly configured for the deployment.
func (a *TrafficRoutingActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.Logger.Info("Retrieving traffic routing configuration for deployment", zap.String("deployment", deployment.Name))
	controlPlaneServiceName, err := a.Gateway.GetControlPlaneServiceName(ctx, a.Logger, deployment.Spec.GetInferenceServer().Name, deployment.Namespace)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "MissingControlPlaneService", fmt.Sprintf("control plane service not found for inference server %s", deployment.Spec.GetInferenceServer().Name)), nil
	}
	if controlPlaneServiceName == "" {
		return conditionsutil.GenerateFalseCondition(condition, "MissingControlPlaneService", fmt.Sprintf("control plane service not found for inference server %s", deployment.Spec.GetInferenceServer().Name)), nil
	}

	// Store the service name in metadata for use by Run
	if setterErr := setControlPlaneServiceNameInMetadata(condition, controlPlaneServiceName); setterErr != nil {
		a.Logger.Error("failed to set traffic routing metadata", zap.Error(setterErr))
	}

	ok, err := a.ProxyProvider.CheckDeploymentRouteStatus(ctx, a.Logger,
		deployment.Name, deployment.Namespace, deployment.Spec.GetInferenceServer().Name, deployment.Spec.DesiredRevision.Name, controlPlaneServiceName)
	if err != nil {
		a.Logger.Error("failed to check deployment route status",
			zap.Error(err),
			zap.String("operation", "check_deployment_route_status"),
			zap.String("namespace", deployment.Namespace),
			zap.String("deployment", deployment.Name))
		return conditionsutil.GenerateFalseCondition(condition, "CheckDeploymentRouteStatusFailed", fmt.Sprintf("Failed to check deployment route status: %v", err)), nil
	}
	if !ok {
		return conditionsutil.GenerateFalseCondition(condition, "DeploymentRouteNotConfigured", "Deployment route is not configured"), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run creates or updates the HTTPRoute to enable traffic routing to the deployed model.
func (a *TrafficRoutingActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.Logger.Info("Running traffic routing configuration for deployment", zap.String("deployment", deployment.Name))

	if deployment.Spec.GetInferenceServer() == nil {
		return conditionsutil.GenerateFalseCondition(condition, "MissingInferenceServer", fmt.Sprintf("inference server not specified for deployment %s", deployment.Name)), nil
	}

	// Read service name from metadata (stored by Retrieve)
	controlPlaneServiceName := getControlPlaneServiceNameFromMetadata(condition)
	if controlPlaneServiceName == "" {
		return conditionsutil.GenerateFalseCondition(condition, "MissingControlPlaneService", fmt.Sprintf("control plane service name not found in metadata for inference server %s", deployment.Spec.GetInferenceServer().Name)), nil
	}
	err := a.ProxyProvider.EnsureDeploymentRoute(ctx, a.Logger, deployment.Name, deployment.Namespace, deployment.Spec.GetInferenceServer().Name, deployment.Spec.DesiredRevision.Name, controlPlaneServiceName)
	if err != nil {
		a.Logger.Error("failed to add deployment route",
			zap.Error(err),
			zap.String("operation", "ensure_deployment_route"),
			zap.String("namespace", deployment.Namespace),
			zap.String("deployment", deployment.Name))
		return conditionsutil.GenerateFalseCondition(condition, "AddDeploymentRouteFailed", fmt.Sprintf("Failed to add deployment route: %v", err)), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// getControlPlaneServiceNameFromMetadata extracts the control plane service name from condition metadata.
func getControlPlaneServiceNameFromMetadata(condition *apipb.Condition) string {
	if condition.Metadata == nil {
		return ""
	}
	structVal := &types.Struct{}
	if err := types.UnmarshalAny(condition.Metadata, structVal); err != nil {
		return ""
	}
	fields := structVal.GetFields()
	if fields == nil {
		return ""
	}
	if val, ok := fields[metadataKeyControlPlaneServiceName]; ok {
		return val.GetStringValue()
	}
	return ""
}

// setControlPlaneServiceNameInMetadata stores the control plane service name in condition metadata.
func setControlPlaneServiceNameInMetadata(condition *apipb.Condition, serviceName string) error {
	structVal := &types.Struct{
		Fields: map[string]*types.Value{
			metadataKeyControlPlaneServiceName: {
				Kind: &types.Value_StringValue{StringValue: serviceName},
			},
		},
	}
	var err error
	condition.Metadata, err = types.MarshalAny(structVal)
	return err
}
