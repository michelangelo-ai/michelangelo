//go:generate mamockgen ModelConfigProvider

package modelconfig

import (
	"context"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ModelPhase is the rollout phase of a model config entry. It tells the model-sync
// daemon where to load the model and tells a replica's readiness probe whether the model
// must be loaded before the replica may receive traffic.
type ModelPhase string

const (
	// ModelPhaseCanary loads the model on the single replica named by CanaryPod only, so a
	// model that cannot load is contained to one replica.
	ModelPhaseCanary ModelPhase = "canary"
	// ModelPhaseStaged loads the model on every replica without gating readiness on it.
	// Traffic is not routed to a staged model.
	ModelPhaseStaged ModelPhase = "staged"
	// ModelPhaseServing loads the model on every replica and makes it a readiness
	// requirement: a replica missing a serving model is withheld from the inference Service.
	ModelPhaseServing ModelPhase = "serving"
)

// ModelConfigEntry is one deployment's use of a model, with the model's storage location.
// Entries are identified by the (DeploymentName, Name) pair, so several deployments can
// serve the same model, each with its own entry.
type ModelConfigEntry struct {
	Name           string `json:"name"`
	StoragePath    string `json:"storage_path"`
	DeploymentName string `json:"deployment_name"`
	// Phase is the entry's rollout phase. Empty is read as serving, so entries written
	// before phases existed keep their meaning.
	Phase ModelPhase `json:"phase,omitempty"`
	// CanaryPod names the replica that loads a canary entry. Ignored in other phases.
	CanaryPod string `json:"canary_pod,omitempty"`
}

// CurrentPhase returns the entry's phase, defaulting to ModelPhaseServing.
func (e ModelConfigEntry) CurrentPhase() ModelPhase {
	if e.Phase == "" {
		return ModelPhaseServing
	}
	return e.Phase
}

// FindEntry returns the entry the named deployment holds for a model, if any.
func FindEntry(entries []ModelConfigEntry, deploymentName string, modelName string) (ModelConfigEntry, bool) {
	for _, entry := range entries {
		if entry.Name == modelName && entry.DeploymentName == deploymentName {
			return entry, true
		}
	}
	return ModelConfigEntry{}, false
}

// ModelConfigProvider manages model configurations for inference servers.
// This facilitates model management through a sidecar pattern, where a sidecar container
// watches the config and loads/unloads models accordingly.
// Configurations are stored in a backing store (e.g., Kubernetes ConfigMap, or other storage).
// The InferenceServer controller creates/deletes the config, while the Deployment controller adds/removes model entries.
type ModelConfigProvider interface {
	// CreateModelConfig creates a new model config with model configurations.
	CreateModelConfig(ctx context.Context, logger *zap.Logger, kubeclient client.Client, inferenceServerName string, namespace string, labels map[string]string, annotations map[string]string) error

	// CheckModelConfigExists checks if a model config exists for an inference server.
	CheckModelConfigExists(ctx context.Context, logger *zap.Logger, kubeclient client.Client, inferenceServerName string, namespace string) (bool, error)

	// DeleteModelConfig removes the entire model config for an inference server.
	DeleteModelConfig(ctx context.Context, logger *zap.Logger, kubeclient client.Client, inferenceServerName string, namespace string) error

	// GetModelsFromConfig retrieves all models from a config.
	GetModelsFromConfig(ctx context.Context, logger *zap.Logger, kubeclient client.Client, inferenceServerName string, namespace string) ([]ModelConfigEntry, error)

	// AddModelToConfig adds a deployment's entry for a model to an existing config, or
	// refreshes the storage path, phase and canary pod of the entry it already holds.
	AddModelToConfig(ctx context.Context, logger *zap.Logger, kubeclient client.Client, inferenceServerName string, namespace string, entry ModelConfigEntry) error

	// RemoveModelFromConfig removes one deployment's entry for a model, leaving other
	// deployments' entries for that model in place.
	RemoveModelFromConfig(ctx context.Context, logger *zap.Logger, kubeclient client.Client, inferenceServerName string, namespace string, deploymentName string, modelName string) error
}
