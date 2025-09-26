package common

import (
	"strconv"
	"strings"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// Actor type constants following Uber patterns
const (
	ActorTypeValidation          = "Validated"
	ActorTypeAssetPreparation    = "AssetsPrepared"
	ActorTypeResourceAcquisition = "ResourcesAcquired"
	ActorTypeModelSync          = "ModelSynced"
	ActorTypeRollingRollout     = "RollingRolloutComplete"
	ActorTypeBlastRollout       = "BlastRolloutComplete"
	ActorTypeZonalRollout       = "ZonalRolloutComplete"
	ActorTypeShadowDeployment   = "ShadowDeploymentComplete"
	ActorTypeShadowAnalysis     = "ShadowAnalysisComplete"
	ActorTypeShadowPromotion    = "ShadowPromotionComplete"
	ActorTypeDisaggregatedRollout = "DisaggregatedRolloutComplete"
	ActorTypeRolloutCompletion  = "RolloutCompleted"
	ActorTypeCleanup            = "CleanupComplete"
	ActorTypeRollback           = "RollbackComplete"
	ActorTypeSteadyState        = "StateSteady"
)

// Rollout configuration constants
const (
	DefaultRolloutIncrement = 30
	AnnotationRolloutIncrement = "rollout.michelangelo.ai/increment-percentage"
	AnnotationRolloutStrategy = "rollout.michelangelo.ai/strategy"
)

// Available models in OSS environment
var availableModels = []string{
	"bert-cola-6",
	"bert-cola-7", 
	"bert-cola-8",
	"bert-cola-23",
}

// IsModelAvailable checks if a model is available in the OSS environment
func IsModelAvailable(modelName string) bool {
	for _, model := range availableModels {
		if model == modelName {
			return true
		}
	}
	return false
}

// GetAvailableModels returns a comma-separated list of available models
func GetAvailableModels() string {
	return strings.Join(availableModels, ", ")
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