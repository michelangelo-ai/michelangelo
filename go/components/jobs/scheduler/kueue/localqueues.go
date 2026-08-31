//go:generate mamockgen LocalQueues
package kueue

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/compute"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// DefaultKueueAPIVersion is the kueue.x-k8s.io API version used when
// jobs.scheduler.kueue.apiVersion is not configured. v1beta2 is served by
// Kueue v0.15+, the floor this backend targets (its RayCluster integration
// skips clusters that use ClusterSelector-based RayJobs, kueue#7218).
const DefaultKueueAPIVersion = "v1beta2"

// LocalQueues checks Kueue LocalQueue existence on compute clusters.
type LocalQueues interface {
	// Exists reports whether the named LocalQueue exists in the given
	// namespace on the given compute cluster. A missing kueue.x-k8s.io API
	// (Kueue not installed) reports false rather than an error: either way
	// the queue is not there to admit the job, and the caller surfaces the
	// same actionable failure condition.
	Exists(ctx context.Context, cluster *v2pb.Cluster, namespace, name string) (bool, error)
}

// localQueues implements LocalQueues with a raw GET against the Kueue API on
// the target cluster, following the AbsPath pattern the jobs client already
// uses for /healthz. Reading through the existing per-cluster REST client
// avoids introducing a Kueue client dependency for what is an existence
// check.
type localQueues struct {
	factory    compute.Factory
	apiVersion string
}

// NewLocalQueues constructs the compute-cluster-backed LocalQueues checker.
func NewLocalQueues(factory compute.Factory, cfg maconfig.SchedulerConfig) LocalQueues {
	apiVersion := cfg.Kueue.APIVersion
	if apiVersion == "" {
		apiVersion = DefaultKueueAPIVersion
	}
	return localQueues{factory: factory, apiVersion: apiVersion}
}

func (l localQueues) Exists(ctx context.Context, cluster *v2pb.Cluster, namespace, name string) (bool, error) {
	cs, err := l.factory.GetClientSetForCluster(cluster)
	if err != nil {
		return false, fmt.Errorf("get client for cluster: %w", err)
	}

	path := fmt.Sprintf("/apis/kueue.x-k8s.io/%s/namespaces/%s/localqueues/%s", l.apiVersion, namespace, name)
	err = cs.CoreV1.Get().AbsPath(path).Do(ctx).Error()
	if err == nil {
		return true, nil
	}
	if apierrors.IsNotFound(err) {
		// Covers both a missing LocalQueue and a missing kueue.x-k8s.io API
		// group (Kueue not installed on the cluster).
		return false, nil
	}
	return false, err
}
