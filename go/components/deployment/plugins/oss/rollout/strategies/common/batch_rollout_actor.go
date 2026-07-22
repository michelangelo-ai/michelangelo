package common

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	osscommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &BatchRolloutActor{}

// BatchRolloutActor loads a model onto multiple clusters concurrently, then waits until
// every cluster in the batch reports the model ready. Used by the rolling strategy to
// advance incrementPercentage% of clusters at a time before routing traffic.
type BatchRolloutActor struct {
	clientFactory   clientfactory.ClientFactory
	backendRegistry *backends.Registry
	backendType     v2pb.BackendType
	logger          *zap.Logger
	targets         []*v2pb.ClusterTarget
	batchIndex      int
}

// NewBatchRolloutActor creates a BatchRolloutActor for a group of clusters.
func NewBatchRolloutActor(
	clientFactory clientfactory.ClientFactory,
	backendRegistry *backends.Registry,
	backendType v2pb.BackendType,
	logger *zap.Logger,
	targets []*v2pb.ClusterTarget,
	batchIndex int,
) *BatchRolloutActor {
	return &BatchRolloutActor{
		clientFactory:   clientFactory,
		backendRegistry: backendRegistry,
		backendType:     backendType,
		logger:          logger,
		targets:         targets,
		batchIndex:      batchIndex,
	}
}

// GetType returns a unique condition key for this batch.
func (a *BatchRolloutActor) GetType() string {
	return fmt.Sprintf("%s-batch%d", osscommon.ActorTypeRollingRollout, a.batchIndex)
}

// Retrieve checks whether all clusters in the batch have loaded the model.
// Short-circuits per-cluster once loaded (stored in Struct metadata keyed by cluster ID).
func (a *BatchRolloutActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	loaded, err := osscommon.ReadBatchLoadedClusters(condition)
	if err != nil {
		// Metadata malformed — reset and re-poll.
		loaded = nil
	}
	if loaded == nil {
		loaded = make(map[string]bool, len(a.targets))
	}

	backend, err := a.backendRegistry.GetBackend(a.backendType)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "BackendUnavailable", err.Error()), nil
	}

	modelName := deployment.Spec.GetDesiredRevision().GetName()
	inferenceServerName := deployment.Spec.GetInferenceServer().GetName()

	allReady := true
	for _, target := range a.targets {
		clusterID := target.GetClusterId()
		if loaded[clusterID] {
			continue
		}

		kubeClient, err := a.clientFactory.GetClient(ctx, target)
		if err != nil {
			return conditionsutil.GenerateFalseCondition(condition, "ClientUnavailable",
				fmt.Sprintf("cluster %s: %s", clusterID, err.Error())), nil
		}
		httpClient, err := a.clientFactory.GetHTTPClient(ctx, target)
		if err != nil {
			return conditionsutil.GenerateFalseCondition(condition, "HTTPClientUnavailable",
				fmt.Sprintf("cluster %s: %s", clusterID, err.Error())), nil
		}

		apiServerURL := osscommon.APIServerURLFromTarget(target)
		ready, err := backend.CheckModelStatus(ctx, a.logger, kubeClient, httpClient, apiServerURL, inferenceServerName, deployment.Namespace, modelName)
		if err != nil {
			return conditionsutil.GenerateFalseCondition(condition, "ModelStatusCheckFailed",
				fmt.Sprintf("cluster %s: %s", clusterID, err.Error())), nil
		}
		if ready {
			loaded[clusterID] = true
		} else {
			allReady = false
		}
	}

	if err := osscommon.WriteBatchLoadedClusters(condition, loaded); err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "MetadataWriteFailed", err.Error()), nil
	}

	if !allReady {
		pendingClusters := make([]string, 0)
		for _, t := range a.targets {
			if !loaded[t.GetClusterId()] {
				pendingClusters = append(pendingClusters, t.GetClusterId())
			}
		}
		return conditionsutil.GenerateFalseCondition(condition, "BatchNotReady",
			fmt.Sprintf("model %s still loading in clusters: %v", modelName, pendingClusters)), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run triggers model loading on all clusters in the batch concurrently.
// Returns UNKNOWN so the engine continues polling via Retrieve.
func (a *BatchRolloutActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	backend, err := a.backendRegistry.GetBackend(a.backendType)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "BackendUnavailable", err.Error()), nil
	}

	modelName := deployment.Spec.GetDesiredRevision().GetName()
	inferenceServerName := deployment.Spec.GetInferenceServer().GetName()
	// TODO(#696): make storage path configurable.
	storageURI := fmt.Sprintf("s3://deploy-models/%s/", modelName)

	type result struct {
		clusterID string
		err       error
	}
	results := make(chan result, len(a.targets))

	var wg sync.WaitGroup
	for _, target := range a.targets {
		wg.Add(1)
		go func(t *v2pb.ClusterTarget) {
			defer wg.Done()
			clusterID := t.GetClusterId()

			kubeClient, err := a.clientFactory.GetClient(ctx, t)
			if err != nil {
				results <- result{clusterID, fmt.Errorf("get kube client: %w", err)}
				return
			}
			dynClient, err := a.clientFactory.GetDynamicClient(ctx, t)
			if err != nil {
				results <- result{clusterID, fmt.Errorf("get dynamic client: %w", err)}
				return
			}
			if err := backend.LoadModel(ctx, a.logger, kubeClient, dynClient, inferenceServerName, deployment.Namespace, modelName, storageURI); err != nil {
				results <- result{clusterID, fmt.Errorf("load model: %w", err)}
				return
			}
			results <- result{clusterID, nil}
		}(target)
	}

	wg.Wait()
	close(results)

	var failures []string
	for r := range results {
		if r.err != nil {
			failures = append(failures, fmt.Sprintf("%s: %s", r.clusterID, r.err.Error()))
			a.logger.Error("failed to load model in cluster",
				zap.String("cluster", r.clusterID),
				zap.String("model", modelName),
				zap.Error(r.err))
		}
	}
	if len(failures) > 0 {
		return conditionsutil.GenerateFalseCondition(condition, "LoadModelFailed",
			fmt.Sprintf("failed clusters: %v", failures)), nil
	}

	clusterIDs := make([]string, 0, len(a.targets))
	for _, t := range a.targets {
		clusterIDs = append(clusterIDs, t.GetClusterId())
	}
	return conditionsutil.GenerateUnknownCondition(condition, "BatchLoading",
		fmt.Sprintf("model %s loading in batch %d clusters: %v", modelName, a.batchIndex, clusterIDs)), nil
}
