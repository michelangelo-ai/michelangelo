package creation

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &ValidationActor{}

// ValidationActor validates that inference server configuration meets Triton requirements.
type ValidationActor struct {
	backend               backends.Backend
	controlPlaneClusterId string
	logger                *zap.Logger
}

// NewValidationActor creates a condition actor for Triton configuration validation.
func NewValidationActor(backend backends.Backend, controlPlaneClusterId string, logger *zap.Logger) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &ValidationActor{
		backend:               backend,
		controlPlaneClusterId: controlPlaneClusterId,
		logger:                logger,
	}
}

// GetType returns the condition type identifier for validation.
func (a *ValidationActor) GetType() string {
	return common.TritonValidationConditionType
}

// Retrieve validates that the inference server configuration meets Triton backend requirements.
func (a *ValidationActor) Retrieve(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Retrieving Triton validation condition")

	// Validate Triton-specific requirements
	if resource.Spec.BackendType != v2pb.BACKEND_TYPE_TRITON {
		return conditionsutil.GenerateFalseCondition(condition, "InvalidBackendType", fmt.Sprintf("invalid backend type for Triton plugin: %v", resource.Spec.BackendType)), nil
	}

	// Validate cluster targets
	if err := a.validateClusterTargets(resource); err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "InvalidClusterTargets", err.Error()), nil
	}

	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run returns a failed condition since validation failures cannot be automatically fixed.
func (a *ValidationActor) Run(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	// This method is only run when Retrieve() fails.
	// If Retrieve() failed, then there's nothing we can do here, simply return the condition.
	return condition, nil
}

// validateClusterTargets validates that cluster targets are properly configured.
func (a *ValidationActor) validateClusterTargets(resource *v2pb.InferenceServer) error {
	clusterTargets := resource.Spec.ClusterTargets
	if len(clusterTargets) == 0 {
		return fmt.Errorf("at least one cluster target is required")
	}

	// Check if control plane cluster is present
	hasControlPlane := false
	if a.controlPlaneClusterId != "" {
		for _, target := range clusterTargets {
			if target.ClusterId == a.controlPlaneClusterId {
				hasControlPlane = true
				break
			}
		}
	}

	// If control plane cluster is present, it must be the only target (single-cluster setup)
	if hasControlPlane && len(clusterTargets) > 1 {
		return fmt.Errorf("control plane cluster %s cannot be mixed with remote clusters; use either single-cluster (control plane only) or multi-cluster (remote clusters only)", a.controlPlaneClusterId)
	}

	for _, target := range clusterTargets {
		if target.ClusterId == "" {
			return fmt.Errorf("cluster target must have a clusterId")
		}

		// Control plane cluster doesn't require kubernetes connection details
		if a.controlPlaneClusterId != "" && target.ClusterId == a.controlPlaneClusterId {
			a.logger.Debug("Cluster target is control plane, skipping connection validation",
				zap.String("clusterId", target.ClusterId))
			continue
		}

		// For remote clusters, validate kubernetes connection details
		k8sConfig := target.GetKubernetes()
		if k8sConfig == nil {
			return fmt.Errorf("cluster %s: kubernetes connection config is required for remote clusters", target.ClusterId)
		}

		if k8sConfig.Host == "" {
			return fmt.Errorf("cluster %s: host is required for remote clusters", target.ClusterId)
		}

		if k8sConfig.Port == "" {
			return fmt.Errorf("cluster %s: port is required for remote clusters", target.ClusterId)
		}

		if k8sConfig.TokenTag == "" {
			return fmt.Errorf("cluster %s: tokenTag is required for remote clusters", target.ClusterId)
		}

		if k8sConfig.CaDataTag == "" {
			return fmt.Errorf("cluster %s: caDataTag is required for remote clusters", target.ClusterId)
		}
	}

	return nil
}
