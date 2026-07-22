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

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &ModelCleanupActor{}

// ModelCleanupActor removes the previous model revision from a cluster after all
// traffic has moved to the new revision. It is backend-aware: for Triton it removes
// the model from the ConfigMap; for KServe this is a no-op. One instance per cluster.
type ModelCleanupActor struct {
	clientFactory   clientfactory.ClientFactory
	backendRegistry *backends.Registry
	backendType     v2pb.BackendType
	logger          *zap.Logger
	target          *v2pb.ClusterTarget
}

// NewModelCleanupActor creates a ModelCleanupActor for the given cluster and backend type.
func NewModelCleanupActor(
	clientFactory clientfactory.ClientFactory,
	backendRegistry *backends.Registry,
	backendType v2pb.BackendType,
	logger *zap.Logger,
	target *v2pb.ClusterTarget,
) *ModelCleanupActor {
	return &ModelCleanupActor{
		clientFactory:   clientFactory,
		backendRegistry: backendRegistry,
		backendType:     backendType,
		logger:          logger,
		target:          target,
	}
}

// GetType returns a unique condition key per cluster.
func (a *ModelCleanupActor) GetType() string {
	return osscommon.ActorTypeModelCleanup + "-" + a.target.GetClusterId()
}

// noCleanupNeeded returns true when there is no prior revision to remove, or when
// the prior revision is the same as the desired (idempotent re-deploy).
func noCleanupNeeded(deployment *v2pb.Deployment) bool {
	currentRevision := deployment.Status.GetCurrentRevision()
	if currentRevision == nil {
		return true
	}
	return currentRevision.GetName() == deployment.Spec.GetDesiredRevision().GetName()
}

// Retrieve checks whether the previous model has been unloaded.
// Returns TRUE immediately if there is no prior revision or backend reports no-op.
func (a *ModelCleanupActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	if noCleanupNeeded(deployment) {
		return conditionsutil.GenerateTrueCondition(condition), nil
	}

	// KServe: cleanup is always a no-op — signal done immediately.
	if a.backendType == v2pb.BACKEND_TYPE_KSERVE {
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

	inferenceServerName := deployment.Spec.GetInferenceServer().GetName()
	oldModel := deployment.Status.GetCurrentRevision().GetName()

	apiServerURL := osscommon.APIServerURLFromTarget(a.target)
	stillLoaded, err := backend.CheckModelStatus(ctx, a.logger, kubeClient, httpClient, apiServerURL, inferenceServerName, deployment.Namespace, oldModel)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "ModelStatusCheckFailed", err.Error()), nil
	}
	if stillLoaded {
		return conditionsutil.GenerateFalseCondition(condition, "OldModelStillLoaded",
			fmt.Sprintf("model %s still loaded in cluster %s", oldModel, a.target.GetClusterId())), nil
	}

	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run calls backend.UnloadModel to remove the previous model revision.
func (a *ModelCleanupActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	if noCleanupNeeded(deployment) {
		return conditionsutil.GenerateTrueCondition(condition), nil
	}

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
	oldModel := deployment.Status.GetCurrentRevision().GetName()

	if err := backend.UnloadModel(ctx, a.logger, kubeClient, dynClient, inferenceServerName, deployment.Namespace, oldModel); err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "UnloadModelFailed", err.Error()), nil
	}

	return conditionsutil.GenerateUnknownCondition(condition, "ModelUnloading",
		fmt.Sprintf("model %s unloading from cluster %s", oldModel, a.target.GetClusterId())), nil
}
