package rollback

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	goapi "github.com/michelangelo-ai/michelangelo/go/api"
	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/common/routing"
	plugincommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/common"
	osscommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/common/routenames"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Condition reasons reported by the rollback actors.
const (
	ReasonCandidateStillInModelConfig = "CandidateStillInModelConfig"
	ReasonPreviousModelNotServing     = "PreviousModelNotServing"
	ReasonTrafficRouteNotRestored     = "TrafficRouteNotRestored"
	ReasonTrafficRouteStillPresent    = "TrafficRouteStillPresent"
	ReasonRestoringPreviousModel      = "RestoringPreviousModel"
	ReasonPreviousModelLoadFailed     = "PreviousModelLoadFailed"
	ReasonRollbackTimeout             = "RollbackTimeout"
	ReasonDiscoveryRouteStillPresent  = "DiscoveryRouteStillPresent"
	ReasonModelConfigReadFailed       = "ModelConfigReadFailed"
)

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &ClusterRollbackActor{}

// ClusterRollbackActor undoes a rollout in one cluster. It makes sure the previous revision
// is in the cluster's model config in the serving phase, waits until every replica reports
// it loaded, points the deployment's traffic rule back at it, and only then removes the
// candidate from the model config so the replicas unload it. When there is no previous
// revision the traffic rule is removed instead. The order matters: traffic is never routed
// to a model that is not loaded everywhere, and the candidate is not unloaded while a rule
// still points at it. One instance is created per cluster.
type ClusterRollbackActor struct {
	clientFactory       clientfactory.ClientFactory
	apiHandler          goapi.Handler
	backendRegistry     *backends.Registry
	modelConfigProvider modelconfig.ModelConfigProvider
	routeManager        routing.Manager
	logger              *zap.Logger
	settings            osscommon.RolloutSettings
	target              *v2pb.ClusterTarget
	now                 func() time.Time
}

// NewClusterRollbackActor creates a ClusterRollbackActor for the given cluster.
func NewClusterRollbackActor(p Params, target *v2pb.ClusterTarget) *ClusterRollbackActor {
	now := p.Now
	if now == nil {
		now = time.Now
	}
	logger := p.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ClusterRollbackActor{
		clientFactory:       p.ClientFactory,
		apiHandler:          p.APIHandler,
		backendRegistry:     p.BackendRegistry,
		modelConfigProvider: p.ModelConfigProvider,
		routeManager:        p.RouteManager,
		logger:              logger,
		settings:            p.Settings,
		target:              target,
		now:                 now,
	}
}

// GetType returns the condition type identifier, including the cluster ID so each cluster
// gets its own condition entry in status.conditions.
func (a *ClusterRollbackActor) GetType() string {
	return osscommon.ActorTypeRollback + "-" + a.target.GetClusterId()
}

// Retrieve reports TRUE once the candidate is gone from the cluster's model config and the
// deployment's traffic rule points at the previous revision (or is gone when there is none).
func (a *ClusterRollbackActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	candidate, current := revisionNames(deployment)
	if candidate == "" || candidate == current {
		return conditionsutil.GenerateTrueCondition(condition), nil
	}

	isName := deployment.Spec.GetInferenceServer().GetName()
	clusterID := a.target.GetClusterId()

	kubeClient, err := a.clientFactory.GetClient(ctx, a.target)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, osscommon.ReasonClientUnavailable, err.Error()), nil
	}
	entries, err := a.modelConfigProvider.GetModelsFromConfig(ctx, a.logger, kubeClient, isName, deployment.Namespace)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, ReasonModelConfigReadFailed, err.Error()), nil
	}
	if _, ok := modelconfig.FindEntry(entries, deployment.Name, candidate); ok {
		return conditionsutil.GenerateFalseCondition(condition, ReasonCandidateStillInModelConfig,
			fmt.Sprintf("candidate model %s is still in the model config of cluster %s", candidate, clusterID)), nil
	}

	dynamicClient, err := a.clientFactory.GetDynamicClient(ctx, a.target)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "DynamicClientUnavailable", err.Error()), nil
	}
	trafficRoute := routenames.TrafficRouteName(isName)
	matchPath := routenames.TrafficMatchPath(isName, deployment.Name)

	if current == "" {
		exists, err := a.routeManager.RuleExists(ctx, dynamicClient, trafficRoute, deployment.Namespace, routing.Rule{MatchPath: matchPath})
		if err != nil {
			return conditionsutil.GenerateFalseCondition(condition, "TrafficRouteStatusCheckFailed", err.Error()), nil
		}
		if exists {
			return conditionsutil.GenerateFalseCondition(condition, ReasonTrafficRouteStillPresent,
				fmt.Sprintf("traffic route for deployment %s still exists in cluster %s although there is no previous model to serve", deployment.Name, clusterID)), nil
		}
		return conditionsutil.GenerateTrueCondition(condition), nil
	}

	entry, ok := modelconfig.FindEntry(entries, deployment.Name, current)
	if !ok || entry.CurrentPhase() != modelconfig.ModelPhaseServing {
		return conditionsutil.GenerateFalseCondition(condition, ReasonPreviousModelNotServing,
			fmt.Sprintf("previous model %s is not in the serving phase in cluster %s", current, clusterID)), nil
	}
	exists, err := a.routeManager.RuleExists(ctx, dynamicClient, trafficRoute, deployment.Namespace, routing.Rule{
		MatchPath:   matchPath,
		RewritePath: routenames.TrafficRewritePath(current),
	})
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "TrafficRouteStatusCheckFailed", err.Error()), nil
	}
	if !exists {
		return conditionsutil.GenerateFalseCondition(condition, ReasonTrafficRouteNotRestored,
			fmt.Sprintf("traffic route for deployment %s does not point at previous model %s in cluster %s", deployment.Name, current, clusterID)), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run restores the previous revision in the cluster and removes the candidate. It returns
// UNKNOWN while the previous revision is being loaded again, and a terminal FALSE if that
// load fails or exceeds the rollback budget.
func (a *ClusterRollbackActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	if osscommon.IsTerminalFailure(condition, ReasonPreviousModelLoadFailed, ReasonRollbackTimeout) {
		return condition, nil
	}

	candidate, current := revisionNames(deployment)
	if candidate == "" || candidate == current {
		return conditionsutil.GenerateTrueCondition(condition), nil
	}

	progress, err := osscommon.ReadRolloutProgress(condition)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "MetadataReadFailed", err.Error()), nil
	}
	if !progress.Started() {
		progress.StartedAt = a.now().Unix()
		if err := osscommon.WriteRolloutProgress(condition, progress); err != nil {
			return conditionsutil.GenerateFalseCondition(condition, "MetadataWriteFailed", err.Error()), nil
		}
	}

	isName := deployment.Spec.GetInferenceServer().GetName()
	clusterID := a.target.GetClusterId()

	kubeClient, err := a.clientFactory.GetClient(ctx, a.target)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, osscommon.ReasonClientUnavailable, err.Error()), nil
	}
	dynamicClient, err := a.clientFactory.GetDynamicClient(ctx, a.target)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "DynamicClientUnavailable", err.Error()), nil
	}

	if current != "" {
		if blocked := a.restorePrevious(ctx, deployment, condition, progress, kubeClient, dynamicClient, current); blocked != nil {
			return blocked, nil
		}
	} else {
		a.logger.Info("Removing traffic route; there is no previous model to restore",
			zap.String("deployment", deployment.Name), zap.String("cluster", clusterID))
		if err := a.routeManager.RemoveRules(ctx, dynamicClient, routenames.TrafficRouteName(isName), deployment.Namespace, routenames.TrafficMatchPath(isName, deployment.Name)); err != nil {
			return conditionsutil.GenerateFalseCondition(condition, "TrafficRouteRemovalFailed", err.Error()), nil
		}
	}

	a.logger.Info("Removing candidate model from model config",
		zap.String("deployment", deployment.Name), zap.String("model", candidate), zap.String("cluster", clusterID))
	if err := a.modelConfigProvider.RemoveModelFromConfig(ctx, a.logger, kubeClient, isName, deployment.Namespace, deployment.GetName(), candidate); err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "RemoveCandidateModelFailed", err.Error()), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// restorePrevious puts the previous revision back in the serving phase, waits for every
// replica to have it loaded, and points the traffic rule at it. It returns nil when traffic
// is restored, or the condition to report while it is not.
func (a *ClusterRollbackActor) restorePrevious(
	ctx context.Context,
	deployment *v2pb.Deployment,
	condition *apipb.Condition,
	progress osscommon.RolloutProgress,
	kubeClient client.Client,
	dynamicClient dynamic.Interface,
	current string,
) *apipb.Condition {
	isName := deployment.Spec.GetInferenceServer().GetName()
	clusterID := a.target.GetClusterId()

	entries, err := a.modelConfigProvider.GetModelsFromConfig(ctx, a.logger, kubeClient, isName, deployment.Namespace)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, ReasonModelConfigReadFailed, err.Error())
	}
	entry, found := modelconfig.FindEntry(entries, deployment.Name, current)
	if !found {
		storagePath, err := a.resolveStoragePath(ctx, deployment)
		if err != nil {
			var resolutionErr *plugincommon.ModelResolutionError
			if errors.As(err, &resolutionErr) {
				return conditionsutil.GenerateFalseCondition(condition, resolutionErr.Reason, resolutionErr.Message)
			}
			return conditionsutil.GenerateFalseCondition(condition, "ModelResolutionFailed", err.Error())
		}
		entry = modelconfig.ModelConfigEntry{Name: current, StoragePath: storagePath, DeploymentName: deployment.GetName()}
	}
	if !found || entry.CurrentPhase() != modelconfig.ModelPhaseServing || entry.CanaryPod != "" {
		entry.Phase = modelconfig.ModelPhaseServing
		entry.CanaryPod = ""
		a.logger.Info("Restoring previous model in model config",
			zap.String("deployment", deployment.Name), zap.String("model", current), zap.String("cluster", clusterID))
		if err := a.modelConfigProvider.AddModelToConfig(ctx, a.logger, kubeClient, isName, deployment.Namespace, entry); err != nil {
			return conditionsutil.GenerateFalseCondition(condition, "RestorePreviousModelFailed", err.Error())
		}
	}

	status, failure := osscommon.ProbeModelStatus(ctx, a.logger, a.clientFactory, a.backendRegistry, osscommon.BackendTypeOf(deployment), a.target, deployment.Namespace, isName, current)
	if failure != nil {
		return conditionsutil.GenerateFalseCondition(condition, failure.Reason, failure.Message)
	}
	if failed := status.Failed(); len(failed) > 0 {
		return conditionsutil.GenerateFalseCondition(condition, ReasonPreviousModelLoadFailed,
			fmt.Sprintf("previous model %s failed to load in cluster %s on %s", current, clusterID, describeFailedReplicas(failed)))
	}
	if !status.Ready() {
		if progress.TimedOut(a.now(), a.settings.RollbackTimeout) {
			return conditionsutil.GenerateFalseCondition(condition, ReasonRollbackTimeout,
				fmt.Sprintf("previous model %s not loaded on every replica in cluster %s within %s: %s", current, clusterID, a.settings.RollbackTimeout, status.Summary()))
		}
		return conditionsutil.GenerateUnknownCondition(condition, ReasonRestoringPreviousModel,
			fmt.Sprintf("waiting for previous model %s on every replica in cluster %s before restoring traffic: %s", current, clusterID, status.Summary()))
	}

	rule := routing.Rule{
		MatchPath:   routenames.TrafficMatchPath(isName, deployment.Name),
		MatchType:   routing.PathMatchPrefix,
		RewritePath: routenames.TrafficRewritePath(current),
		RewriteType: routing.RewritePrefix,
		BackendName: isName + "-inference-service",
	}
	a.logger.Info("Restoring traffic route to previous model",
		zap.String("deployment", deployment.Name), zap.String("model", current), zap.String("cluster", clusterID))
	if err := a.routeManager.AddRules(ctx, dynamicClient, routenames.TrafficRouteName(isName), deployment.Namespace, rule); err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "TrafficRouteRestoreFailed", err.Error())
	}
	return nil
}

// resolveStoragePath finds the artifact location of the previous revision. The resolver
// works on a deployment's desired revision, so it is given a copy whose desired revision is
// the current one.
func (a *ClusterRollbackActor) resolveStoragePath(ctx context.Context, deployment *v2pb.Deployment) (string, error) {
	previous := deployment.DeepCopy()
	previous.Spec.DesiredRevision = deployment.Status.GetCurrentRevision()
	return plugincommon.ResolveDeploymentModelStoragePath(ctx, a.apiHandler, previous)
}

// revisionNames returns the candidate and current revision names, empty when unset.
func revisionNames(deployment *v2pb.Deployment) (candidate string, current string) {
	return deployment.Status.GetCandidateRevision().GetName(), deployment.Status.GetCurrentRevision().GetName()
}

func describeFailedReplicas(failed []backends.ReplicaModelStatus) string {
	parts := make([]string, 0, len(failed))
	for _, replica := range failed {
		parts = append(parts, fmt.Sprintf("%s: %s", replica.Replica, replica.Reason))
	}
	return strings.Join(parts, "; ")
}
