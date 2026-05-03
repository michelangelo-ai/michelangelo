package common

import (
	"context"
	"fmt"
	"net/http"

	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	osscommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/discovery"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/route"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	modelconfig "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Params holds the shared dependencies for all per-cluster placement actors.
type Params struct {
	ClientFactory          clientfactory.ClientFactory
	RouteProvider          route.RouteProvider
	ModelDiscoveryProvider discovery.ModelDiscoveryProvider
	BackendRegistry        *backends.Registry
	ModelConfigProvider    modelconfig.ModelConfigProvider
	Logger                 *zap.Logger

	// ControlPlaneDynamicClient is the dynamic client for the control-plane cluster. Kept here
	// so future actors that operate on control-plane-only resources can access it without an
	// extra ClientFactory call.
	ControlPlaneDynamicClient dynamic.Interface

	// ControlPlaneKubeClient is the controller-runtime client for the control-plane cluster.
	ControlPlaneKubeClient client.Client

	// ControlPlaneHTTPClient is the HTTP client for the control-plane cluster.
	ControlPlaneHTTPClient *http.Client
}

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &RollingRolloutActor{}

// RollingRolloutActor loads a model into a single target cluster's inference server. One
// instance is created per cluster at actor-chain construction time.
type RollingRolloutActor struct {
	params Params
	target *v2pb.ClusterTarget
}

// NewRollingRolloutActor creates a RollingRolloutActor for the given cluster.
func NewRollingRolloutActor(params Params, target *v2pb.ClusterTarget) *RollingRolloutActor {
	return &RollingRolloutActor{params: params, target: target}
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

	kubeClient, err := a.params.ClientFactory.GetClient(ctx, a.target)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "ClientUnavailable", err.Error()), nil
	}

	httpClient, err := a.params.ClientFactory.GetHTTPClient(ctx, a.target)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "HTTPClientUnavailable", err.Error()), nil
	}

	backend, err := a.params.BackendRegistry.GetBackend(v2pb.BACKEND_TYPE_TRITON)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "BackendUnavailable", err.Error()), nil
	}

	modelName := deployment.Spec.GetDesiredRevision().GetName()
	inferenceServerName := deployment.Spec.GetInferenceServer().GetName()

	apiServerURL := osscommon.APIServerURLFromTarget(a.target)
	ready, err := backend.CheckModelStatus(ctx, a.params.Logger, kubeClient, httpClient, apiServerURL, inferenceServerName, deployment.Namespace, modelName)
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
	kubeClient, err := a.params.ClientFactory.GetClient(ctx, a.target)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "ClientUnavailable", err.Error()), nil
	}

	inferenceServerName := deployment.Spec.GetInferenceServer().GetName()
	modelName := deployment.Spec.GetDesiredRevision().GetName()

	// TODO(#696): make the storage path configurable w.r.t. storage client and location.
	storagePath := fmt.Sprintf("s3://deploy-models/%s/", modelName)
	if err := a.params.ModelConfigProvider.AddModelToConfig(ctx, a.params.Logger, kubeClient, inferenceServerName, deployment.Namespace, modelconfig.ModelConfigEntry{
		Name:        modelName,
		StoragePath: storagePath,
	}); err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "AddModelToConfigFailed", err.Error()), nil
	}

	return conditionsutil.GenerateUnknownCondition(condition, "ModelLoading", fmt.Sprintf("model %s loading in cluster %s", modelName, a.target.GetClusterId())), nil
}
