package framework

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-logr/logr"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// fakeClusterCache is a simple in-memory implementation of RegisteredClustersCache
type fakeClusterCache struct {
	list   []*v2pb.Cluster
	byName map[string]*v2pb.Cluster
}

func newFakeClusterCache(clusters ...*v2pb.Cluster) *fakeClusterCache {
	by := make(map[string]*v2pb.Cluster, len(clusters))
	for _, c := range clusters {
		by[c.GetName()] = c
	}
	return &fakeClusterCache{list: clusters, byName: by}
}

func (f *fakeClusterCache) GetClusters(_ cluster.FilterType) []*v2pb.Cluster { return f.list }
func (f *fakeClusterCache) GetCluster(name string) *v2pb.Cluster             { return f.byName[name] }

// newTestClusterOnlyStrategy creates a ClusterOnlyAssignmentStrategy for testing with a no-op logger
func newTestClusterOnlyStrategy(cache cluster.RegisteredClustersCache) ClusterOnlyAssignmentStrategy {
	return ClusterOnlyAssignmentStrategy{
		ClusterCache: cache,
		log:          logr.Discard(), // Use a no-op logger for tests
	}
}

func TestClusterOnlyEngine_Select(t *testing.T) {
	makeCluster := func(name string) *v2pb.Cluster {
		return &v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: name}}
	}

	makeJob := func(labels map[string]string) BatchJob {
		job := BatchSparkJob{SparkJob: &v2pb.SparkJob{ObjectMeta: metav1.ObjectMeta{Name: "job", Namespace: "ns"}}}
		if labels != nil {
			job.SparkJob.Spec = v2pb.SparkJobSpec{
				Affinity: &v2pb.Affinity{
					ResourceAffinity: &v2pb.ResourceAffinity{Selector: &metav1.LabelSelector{MatchLabels: labels}},
				},
			}
		}
		return job
	}

	tests := []struct {
		name        string
		cache       cluster.RegisteredClustersCache
		job         BatchJob
		wantFound   bool
		wantReason  string
		wantCluster string
	}{
		{
			name:        "affinity matches existing cluster",
			cache:       newFakeClusterCache(makeCluster("c1"), makeCluster("c2")),
			job:         makeJob(map[string]string{constants.ClusterAffinityLabelKey: "c2"}),
			wantFound:   true,
			wantReason:  "cluster_matched_by_affinity",
			wantCluster: "c2",
		},
		{
			name:        "affinity cluster not found, falls back to default",
			cache:       newFakeClusterCache(makeCluster("c1"), makeCluster("c2")),
			job:         makeJob(map[string]string{"resourcepool.michelangelo/cluster": "unknown"}),
			wantFound:   true,
			wantReason:  "cluster_default_selected",
			wantCluster: "c1",
		},
		{
			name:        "no affinity selects first available",
			cache:       newFakeClusterCache(makeCluster("first")),
			job:         makeJob(nil),
			wantFound:   true,
			wantReason:  "cluster_default_selected",
			wantCluster: "first",
		},
		{
			name:       "no clusters found",
			cache:      newFakeClusterCache(),
			job:        makeJob(nil),
			wantFound:  false,
			wantReason: "no_clusters_found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewClusterOnlyAssignmentStrategy(tt.cache)

			assign, found, reason, err := engine.Select(context.Background(), tt.job)
			if err != nil {
				t.Fatalf("Select returned error: %v", err)
			}
			if found != tt.wantFound {
				t.Fatalf("found = %v, want %v", found, tt.wantFound)
			}
			if reason != tt.wantReason {
				t.Fatalf("reason = %q, want %q", reason, tt.wantReason)
			}
			if tt.wantFound {
				if assign == nil {
					t.Fatalf("expected non-nil assignment")
				}
				if got := assign.GetCluster(); got != tt.wantCluster {
					t.Fatalf("assignment.Cluster = %q, want %q", got, tt.wantCluster)
				}
				if assign.GetResourcePool() != "" {
					t.Fatalf("resource_pool must be empty, got %q", assign.GetResourcePool())
				}
			} else {
				if assign != nil {
					t.Fatalf("expected nil assignment when not found, got %+v", assign)
				}
			}
		})
	}
}
