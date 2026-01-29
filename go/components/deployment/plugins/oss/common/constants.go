package common

import (
	"strconv"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Actor type constants following Uber patterns
const (
	ActorTypeValidation           = "Validated"
	ActorTypeAssetPreparation     = "AssetsPrepared"
	ActorTypeResourceAcquisition  = "ResourcesAcquired"
	ActorTypeRollingRollout       = "RollingRolloutComplete"
	ActorTypeBlastRollout         = "BlastRolloutComplete"
	ActorTypeZonalRollout         = "ZonalRolloutComplete"
	ActorTypeShadowDeployment     = "ShadowDeploymentComplete"
	ActorTypeShadowAnalysis       = "ShadowAnalysisComplete"
	ActorTypeShadowPromotion      = "ShadowPromotionComplete"
	ActorTypeDisaggregatedRollout = "DisaggregatedRolloutComplete"
	ActorTypeTrafficRouting       = "TrafficRoutingConfigured"
	ActorTypeRolloutComplete      = "RolloutCompleted"
	ActorTypeCleanup              = "CleanupComplete"
	ActorTypeModelCleanup         = "ModelCleanupComplete"
	ActorTypeRollback             = "RollbackComplete"
	ActorTypeSteadyState          = "StateSteady"
)

// Rollout configuration constants
const (
	DefaultRolloutIncrement    = 30
	AnnotationRolloutIncrement = "rollout.michelangelo.ai/increment-percentage"
	AnnotationRolloutStrategy  = "rollout.michelangelo.ai/strategy"
)

// GetAvailableModels returns a message about available models
func GetAvailableModels() string {
	return "all models in configured storage"
}

// GetRolloutIncrement gets the rollout increment percentage from deployment annotations
func GetRolloutIncrement(deployment *v2pb.Deployment) int {
	if deployment.Annotations == nil {
		return DefaultRolloutIncrement
	}

	incrementStr, exists := deployment.Annotations[AnnotationRolloutIncrement]
	if !exists {
		return DefaultRolloutIncrement
	}

	increment, err := strconv.Atoi(incrementStr)
	if err != nil || increment <= 0 || increment > 100 {
		return DefaultRolloutIncrement
	}

	return increment
}

// GetRolloutStrategy gets the rollout strategy from deployment annotations
func GetRolloutStrategy(deployment *v2pb.Deployment) string {
	if deployment.Annotations == nil {
		return "rolling"
	}

	strategy, exists := deployment.Annotations[AnnotationRolloutStrategy]
	if !exists {
		return "rolling"
	}

	return strategy
}
