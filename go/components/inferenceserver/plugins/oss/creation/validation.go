package creation

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &ValidationActor{}

// ValidationActor validates that inference server configuration meets requirements.
type ValidationActor struct {
	registry *backends.Registry
	logger   *zap.Logger
}

// NewValidationActor creates a condition actor for inference server configuration validation.
func NewValidationActor(registry *backends.Registry, logger *zap.Logger) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &ValidationActor{
		registry: registry,
		logger:   logger,
	}
}

// GetType returns the condition type identifier for validation.
func (a *ValidationActor) GetType() string {
	return common.ValidationConditionType
}

// Retrieve validates that the inference server configuration meets backend requirements.
func (a *ValidationActor) Retrieve(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Retrieving inference server validation condition")

	// Validate that the backend type is registered in the registry
	_, err := a.registry.GetBackend(resource.Spec.BackendType)
	if err != nil {
		return conditionUtils.GenerateFalseCondition(condition, "InvalidBackendType", fmt.Sprintf("unsupported backend type: %v", resource.Spec.BackendType)), nil
	}

	if len(resource.Spec.ClusterTargets) == 0 {
		return conditionUtils.GenerateFalseCondition(condition, "NoClusterTargets", "spec.cluster_targets must declare at least one cluster"), nil
	}

	// Validate cluster rollout strategy annotation before operational actors attempt multi-cluster iteration.
	if strategy := common.GetRolloutStrategy(resource); !common.IsKnownRolloutStrategy(strategy) {
		return conditionUtils.GenerateFalseCondition(condition, "InvalidRolloutStrategy",
			fmt.Sprintf("unknown cluster rollout strategy %q; supported: rolling", strategy)), nil
	}

	if err := validateServingImage(resource.Spec.InitSpec.GetServingSpec().GetImage()); err != nil {
		return conditionUtils.GenerateFalseCondition(condition, "InvalidServingImage", err.Error()), nil
	}

	return conditionUtils.GenerateTrueCondition(condition), nil
}

// Run returns a failed condition since validation failures cannot be automatically fixed.
func (a *ValidationActor) Run(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	// This method is only run when Retrieve() fails.
	// If Retrieve() failed, then there's nothing we can do here, simply return the condition.
	return condition, nil
}

// validateServingImage checks the image override's URI, pull policy, and pull
// secret names.
func validateServingImage(image *v2pb.ServingImage) error {
	if image == nil {
		return nil
	}

	if uri := image.GetUri(); uri != "" {
		if err := validateImageURI(uri); err != nil {
			return fmt.Errorf("spec.initSpec.servingSpec.image.uri %q: %w", uri, err)
		}
	}

	switch policy := image.GetImagePullPolicy(); policy {
	case "", string(corev1.PullAlways), string(corev1.PullIfNotPresent), string(corev1.PullNever):
	default:
		return fmt.Errorf("spec.initSpec.servingSpec.image.imagePullPolicy %q: must be one of %q, %q, or %q",
			policy, corev1.PullAlways, corev1.PullIfNotPresent, corev1.PullNever)
	}

	for _, name := range image.GetImagePullSecrets() {
		if strings.TrimSpace(name) == "" {
			return errors.New("spec.initSpec.servingSpec.image.imagePullSecrets must not contain empty names")
		}
	}

	return nil
}

// validateImageURI requires a whitespace-free reference carrying an explicit tag
// or digest, so a workload's image is never an implicit ":latest". The full
// reference grammar is left to the container runtime.
func validateImageURI(uri string) error {
	if strings.ContainsAny(uri, " \t\n\r") {
		return errors.New("must not contain whitespace")
	}

	// A digest pins the image, so no tag is required alongside it.
	if at := strings.LastIndex(uri, "@"); at != -1 {
		if at == len(uri)-1 {
			return errors.New(`digest is empty after "@"`)
		}
		return nil
	}

	// A colon before the final slash is a registry port (registry:5000/repo),
	// not a tag.
	tagSep := strings.LastIndex(uri, ":")
	if tagSep == -1 || tagSep < strings.LastIndex(uri, "/") {
		return errors.New("must include an explicit tag or digest")
	}
	if tagSep == len(uri)-1 {
		return errors.New(`tag is empty after ":"`)
	}

	return nil
}
