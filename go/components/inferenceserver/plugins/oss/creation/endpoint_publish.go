package creation

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/endpoints"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &EndpointPublishActor{}

// EndpointPublishActor reconciles the per-cluster endpoints published for an
// InferenceServer, so other components in the control plane can address the
// server across all the clusters it is reachable in.
type EndpointPublishActor struct {
	publisher endpoints.EndpointPublisher
	source    endpoints.EndpointSource
	logger    *zap.Logger
}

// NewEndpointPublishActor creates the condition actor that maintains the
// control-plane Service + per-cluster EndpointSlices for an InferenceServer.
func NewEndpointPublishActor(publisher endpoints.EndpointPublisher, source endpoints.EndpointSource, logger *zap.Logger) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &EndpointPublishActor{
		publisher: publisher,
		source:    source,
		logger:    logger,
	}
}

// GetType returns the condition type identifier for endpoint publishing.
func (a *EndpointPublishActor) GetType() string {
	return common.EndpointPublishConditionType
}

// Retrieve checks that the published EndpointSlices match the spec's cluster
// set: every ClusterTarget has a slice, and there are no orphan slices for
// clusters removed from the spec.
func (a *EndpointPublishActor) Retrieve(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Retrieving endpoint publish condition")

	desired := desiredClusterIDs(resource)
	observed, err := a.publisher.Get(ctx, resource)
	if err != nil {
		a.logger.Error("Failed to read published endpoints",
			zap.Error(err),
			zap.String("operation", "get_published_endpoints"),
			zap.String("namespace", resource.Namespace),
			zap.String("inferenceServer", resource.Name))
		return conditionsutil.GenerateFalseCondition(condition, "GetFailed", err.Error()), nil
	}
	observedIDs := observedClusterIDs(observed)

	if missing := setDiff(desired, observedIDs); len(missing) > 0 {
		return conditionsutil.GenerateFalseCondition(condition, "EndpointSliceMissing", strings.Join(missing, ",")), nil
	}
	if extra := setDiff(observedIDs, desired); len(extra) > 0 {
		return conditionsutil.GenerateFalseCondition(condition, "OrphanEndpointSlice", strings.Join(extra, ",")), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run resolves each ClusterTarget's Gateway endpoint in a parallel fan-out,
// then a single Sync call reconciles the published Service + EndpointSlices
// (creating missing, deleting orphans). Partial resolve failures surface as
// UNKNOWN so a transient error on one cluster does not flip the
// IS to STATE_FAILED.
func (a *EndpointPublishActor) Run(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running endpoint publish")

	endpointMap := map[string]endpoints.Endpoint{}
	var failures []string
	for _, target := range resource.Spec.ClusterTargets {
		ep, err := a.source.Resolve(ctx, target)
		if err != nil {
			a.logger.Error("Failed to resolve cluster endpoint",
				zap.Error(err),
				zap.String("operation", "resolve_endpoint"),
				zap.String("namespace", resource.Namespace),
				zap.String("inferenceServer", resource.Name),
				zap.String("cluster_id", target.GetClusterId()))
			failures = append(failures, fmt.Sprintf("%s: resolve: %v", target.GetClusterId(), err))
			continue
		}
		endpointMap[target.GetClusterId()] = ep
	}

	if err := a.publisher.Sync(ctx, resource, endpointMap); err != nil {
		a.logger.Error("Failed to sync published endpoints",
			zap.Error(err),
			zap.String("operation", "sync_endpoints"),
			zap.String("namespace", resource.Namespace),
			zap.String("inferenceServer", resource.Name))
		return conditionsutil.GenerateFalseCondition(condition, "SyncFailed", err.Error()), nil
	}

	if len(failures) > 0 {
		// Partial resolve failures are transient, so report UNKNOWN.
		return conditionsutil.GenerateUnknownCondition(condition, "PartialEndpointPublish", strings.Join(failures, "; ")), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// desiredClusterIDs returns the set of cluster IDs currently in the IS spec.
func desiredClusterIDs(resource *v2pb.InferenceServer) map[string]struct{} {
	out := make(map[string]struct{}, len(resource.Spec.ClusterTargets))
	for _, target := range resource.Spec.ClusterTargets {
		out[target.GetClusterId()] = struct{}{}
	}
	return out
}

// observedClusterIDs lifts a published endpoint map to a key set so it can be
// compared against the desired set with a single setDiff helper.
func observedClusterIDs(m map[string]endpoints.Endpoint) map[string]struct{} {
	out := make(map[string]struct{}, len(m))
	for k := range m {
		out[k] = struct{}{}
	}
	return out
}

// setDiff returns the sorted slice of keys in `a` that are not in `b`.
// Sorted output keeps condition messages deterministic between reconciles.
func setDiff(a, b map[string]struct{}) []string {
	var out []string
	for k := range a {
		if _, found := b[k]; !found {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
