package common

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	goapi "github.com/michelangelo-ai/michelangelo/go/api"
	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	osscommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	modelconfig "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &RollingRolloutActor{}

// RollingRolloutActor loads a model into a single target cluster's inference server. One
// instance is created per cluster at actor-chain construction time.
type RollingRolloutActor struct {
	clientFactory clientfactory.ClientFactory
	// apiHandler reads the Model, which lives in the control plane rather than in the
	// target cluster this actor rolls out to, and may be served from metadata storage.
	apiHandler          goapi.Handler
	backendRegistry     *backends.Registry
	modelConfigProvider modelconfig.ModelConfigProvider
	logger              *zap.Logger
	target              *v2pb.ClusterTarget
}

// NewRollingRolloutActor creates a RollingRolloutActor for the given cluster.
func NewRollingRolloutActor(
	clientFactory clientfactory.ClientFactory,
	apiHandler goapi.Handler,
	backendRegistry *backends.Registry,
	modelConfigProvider modelconfig.ModelConfigProvider,
	logger *zap.Logger,
	target *v2pb.ClusterTarget,
) *RollingRolloutActor {
	return &RollingRolloutActor{
		clientFactory:       clientFactory,
		apiHandler:          apiHandler,
		backendRegistry:     backendRegistry,
		modelConfigProvider: modelConfigProvider,
		logger:              logger,
		target:              target,
	}
}

// GetType returns the condition type identifier, including the cluster ID so each
// cluster gets its own condition entry in status.conditions.
func (a *RollingRolloutActor) GetType() string {
	return osscommon.ActorTypeRollingRollout + "-" + a.target.GetClusterId()
}

// Retrieve checks whether the model is loaded and ready in Triton. Once ready, it records
// that result on the condition so subsequent calls short-circuit without another Triton poll.
func (a *RollingRolloutActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	if loaded, _ := osscommon.ReadModelLoadedFlag(condition); loaded {
		return conditionsutil.GenerateTrueCondition(condition), nil
	}

	kubeClient, err := a.clientFactory.GetClient(ctx, a.target)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "ClientUnavailable", err.Error()), nil
	}

	httpClient, err := a.clientFactory.GetHTTPClient(ctx, a.target)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "HTTPClientUnavailable", err.Error()), nil
	}

	backend, err := a.backendRegistry.GetBackend(v2pb.BACKEND_TYPE_TRITON)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "BackendUnavailable", err.Error()), nil
	}

	modelName := deployment.Spec.GetDesiredRevision().GetName()
	inferenceServerName := deployment.Spec.GetInferenceServer().GetName()

	apiServerURL := osscommon.APIServerURLFromTarget(a.target)
	ready, err := backend.CheckModelStatus(ctx, a.logger, kubeClient, httpClient, apiServerURL, inferenceServerName, deployment.Namespace, modelName)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "ModelStatusCheckFailed", err.Error()), nil
	}
	if !ready {
		return conditionsutil.GenerateFalseCondition(condition, "ModelNotReady", fmt.Sprintf("model %s not yet loaded in cluster %s", modelName, a.target.GetClusterId())), nil
	}

	if err := osscommon.WriteModelLoadedFlag(condition); err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "MetadataWriteFailed", err.Error()), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run registers the desired model in the cluster's inference server ConfigMap, triggering the
// server to begin loading it. Returns UNKNOWN so the engine continues polling via Retrieve.
func (a *RollingRolloutActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	kubeClient, err := a.clientFactory.GetClient(ctx, a.target)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "ClientUnavailable", err.Error()), nil
	}

	inferenceServerName := deployment.Spec.GetInferenceServer().GetName()
	modelName := deployment.Spec.GetDesiredRevision().GetName()

	storagePath, err := osscommon.ResolveDeploymentModelStoragePath(ctx, a.apiHandler, deployment)
	if err != nil {
		var resolutionErr *osscommon.ModelResolutionError
		if errors.As(err, &resolutionErr) {
			return conditionsutil.GenerateFalseCondition(condition, resolutionErr.Reason, resolutionErr.Message), nil
		}
		return conditionsutil.GenerateFalseCondition(condition, "ModelResolutionFailed", err.Error()), nil
	}

	if err := a.modelConfigProvider.AddModelToConfig(ctx, a.logger, kubeClient, inferenceServerName, deployment.Namespace, modelconfig.ModelConfigEntry{
		Name:        modelName,
		StoragePath: storagePath,
	}); err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "AddModelToConfigFailed", err.Error()), nil
	}

	return conditionsutil.GenerateUnknownCondition(condition, "ModelLoading", fmt.Sprintf("model %s loading in cluster %s", modelName, a.target.GetClusterId())), nil
}
