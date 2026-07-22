package common

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	osscommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &RollingRolloutActor{}

// RollingRolloutActor loads a model into a single target cluster's inference server.
// It is backend-aware: for Triton it writes to the model ConfigMap; for KServe it
// patches the InferenceService storageUri. One instance is created per cluster.
type RollingRolloutActor struct {
	clientFactory   clientfactory.ClientFactory
	backendRegistry *backends.Registry
	backendType     v2pb.BackendType
	logger          *zap.Logger
	target          *v2pb.ClusterTarget
}

// NewRollingRolloutActor creates a RollingRolloutActor for the given cluster and backend type.
func NewRollingRolloutActor(
	clientFactory clientfactory.ClientFactory,
	backendRegistry *backends.Registry,
	backendType v2pb.BackendType,
	logger *zap.Logger,
	target *v2pb.ClusterTarget,
) *RollingRolloutActor {
	return &RollingRolloutActor{
		clientFactory:   clientFactory,
		backendRegistry: backendRegistry,
		backendType:     backendType,
		logger:          logger,
		target:          target,
	}
}

// GetType returns the condition type identifier, including the cluster ID.
func (a *RollingRolloutActor) GetType() string {
	return osscommon.ActorTypeRollingRollout + "-" + a.target.GetClusterId()
}

// Retrieve checks whether the model is loaded and ready. Once ready, records that
// result on the condition so subsequent calls short-circuit without re-polling.
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

	backend, err := a.backendRegistry.GetBackend(a.backendType)
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
		return conditionsutil.GenerateFalseCondition(condition, "ModelNotReady",
			fmt.Sprintf("model %s not yet loaded in cluster %s", modelName, a.target.GetClusterId())), nil
	}

	if err := osscommon.WriteModelLoadedFlag(condition); err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "MetadataWriteFailed", err.Error()), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run calls backend.LoadModel to register the desired model version for serving.
// Returns UNKNOWN so the engine continues polling via Retrieve.
func (a *RollingRolloutActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	kubeClient, err := a.clientFactory.GetClient(ctx, a.target)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "ClientUnavailable", err.Error()), nil
	}

	dynClient, err := a.clientFactory.GetDynamicClient(ctx, a.target)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "DynamicClientUnavailable", err.Error()), nil
	}

	backend, err := a.backendRegistry.GetBackend(a.backendType)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "BackendUnavailable", err.Error()), nil
	}

	inferenceServerName := deployment.Spec.GetInferenceServer().GetName()
	modelName := deployment.Spec.GetDesiredRevision().GetName()
	// TODO(#696): make the storage path configurable w.r.t. storage client and location.
	storageURI := fmt.Sprintf("s3://deploy-models/%s/", modelName)

	if err := backend.LoadModel(ctx, a.logger, kubeClient, dynClient, inferenceServerName, deployment.Namespace, modelName, storageURI); err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "LoadModelFailed", err.Error()), nil
	}

	return conditionsutil.GenerateUnknownCondition(condition, "ModelLoading",
		fmt.Sprintf("model %s loading in cluster %s", modelName, a.target.GetClusterId())), nil
}
