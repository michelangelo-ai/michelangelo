package kueue

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/kueue/state"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const gpu corev1.ResourceName = "nvidia.com/gpu"

func TestRequestedResources(t *testing.T) {
	ask := resources("memory", "64Gi", "cpu", "16", string(gpu), "0", "example.com/fpga", "1")
	assert.Equal(t, []corev1.ResourceName{"cpu", "example.com/fpga", "memory"}, requestedResources(ask),
		"sorted, and a resource asked at zero is not requested")
	assert.Empty(t, requestedResources(resources(string(gpu), "0")))
	assert.Empty(t, requestedResources(nil))
}

func TestNominalCovers(t *testing.T) {
	tests := []struct {
		name        string
		ask         corev1.ResourceList
		nominal     corev1.ResourceList
		want        bool
		wantMissing corev1.ResourceName
	}{
		{name: "covered with room", ask: testAsk(), nominal: resources("cpu", "64", "memory", "256Gi"), want: true},
		{name: "exactly the nominal quota", ask: testAsk(), nominal: resources("cpu", "16", "memory", "64Gi"), want: true},
		{name: "above the nominal quota", ask: testAsk(), nominal: resources("cpu", "8", "memory", "256Gi"), wantMissing: "cpu"},
		{
			name:        "a resource the ClusterQueue does not cover counts as zero",
			ask:         resources("cpu", "1", string(gpu), "1"),
			nominal:     resources("cpu", "64", "memory", "256Gi"),
			wantMissing: gpu,
		},
		{
			name:    "a resource asked at zero is not requested",
			ask:     resources("cpu", "1", string(gpu), "0"),
			nominal: resources("cpu", "64"),
			want:    true,
		},
		{
			name:        "the first resource not covered, by name",
			ask:         resources("memory", "1Ti", "cpu", "1000"),
			nominal:     resources("cpu", "64", "memory", "256Gi"),
			wantMissing: "cpu",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, missing := nominalCovers(tt.ask, tt.nominal)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantMissing, missing)
		})
	}
}

func TestFreeQuota(t *testing.T) {
	cq := clusterQueue("cq", resources("cpu", "64", "memory", "256Gi"), resources("cpu", "40", "memory", "100Gi"), 0)
	free := freeQuota(resources("cpu", "1", "memory", "1Gi", string(gpu), "1", "example.com/fpga", "0"), cq,
		resources("cpu", "10", string(gpu), "2"))
	// nominal - reserved - in flight for each requested resource, an
	// uncovered one counting as zero
	assertResources(t, resources("cpu", "14", "memory", "156Gi", string(gpu), "-2"), free)
}

func TestScoreQuota(t *testing.T) {
	tests := []struct {
		name        string
		ask         corev1.ResourceList
		nominal     corev1.ResourceList
		free        corev1.ResourceList
		wantFits    bool
		wantSlack   float64
		wantBinding corev1.ResourceName
	}{
		{
			name:        "worked example, A (GPU pool): its idle GPUs do not count",
			ask:         testAsk(),
			nominal:     resources("cpu", "64", "memory", "256Gi", string(gpu), "8"),
			free:        resources("cpu", "20", "memory", "80Gi", string(gpu), "8"),
			wantFits:    true,
			wantSlack:   0.0625,
			wantBinding: "cpu",
		},
		{
			name:        "worked example, B (CPU pool)",
			ask:         testAsk(),
			nominal:     resources("cpu", "128", "memory", "512Gi", string(gpu), "0"),
			free:        resources("cpu", "40", "memory", "160Gi", string(gpu), "0"),
			wantFits:    true,
			wantSlack:   0.1875,
			wantBinding: "cpu",
		},
		{
			name:        "the bottleneck is the tightest resource",
			ask:         testAsk(),
			nominal:     resources("cpu", "128", "memory", "512Gi"),
			free:        resources("cpu", "40", "memory", "70Gi"),
			wantFits:    true,
			wantSlack:   6.0 / 512,
			wantBinding: "memory",
		},
		{
			name:        "exactly fills the free quota",
			ask:         testAsk(),
			nominal:     resources("cpu", "64", "memory", "256Gi"),
			free:        resources("cpu", "16", "memory", "64Gi"),
			wantFits:    true,
			wantSlack:   0,
			wantBinding: "cpu",
		},
		{
			name:        "does not fit: the deficit is the negated slack",
			ask:         testAsk(),
			nominal:     resources("cpu", "64", "memory", "512Gi"),
			free:        resources("cpu", "8", "memory", "160Gi"),
			wantSlack:   -0.125,
			wantBinding: "cpu",
		},
		{
			name:        "free quota below zero after in-flight",
			ask:         resources("cpu", "16"),
			nominal:     resources("cpu", "40"),
			free:        resources("cpu", "-10"),
			wantSlack:   -0.65,
			wantBinding: "cpu",
		},
		{
			name:      "nothing requested fits",
			ask:       resources(string(gpu), "0"),
			nominal:   resources("cpu", "64"),
			free:      resources("cpu", "64"),
			wantFits:  true,
			wantSlack: math.Inf(1),
		},
		{
			name:        "an uncovered resource has no room",
			ask:         resources(string(gpu), "1"),
			nominal:     resources("cpu", "64"),
			free:        resources("cpu", "64"),
			wantSlack:   math.Inf(-1),
			wantBinding: gpu,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scoreQuota(tt.ask, tt.nominal, tt.free)
			assert.Equal(t, tt.wantFits, got.fits)
			if math.IsInf(tt.wantSlack, 0) {
				assert.Equal(t, tt.wantSlack, got.slack)
			} else {
				assert.InDelta(t, tt.wantSlack, got.slack, 1e-9)
				assert.InDelta(t, -tt.wantSlack, got.deficit(), 1e-9)
			}
			assert.Equal(t, tt.wantBinding, got.binding)
		})
	}
}

func TestTierOf(t *testing.T) {
	cq := clusterQueue("cq", resources("cpu", "64"), nil, 0)
	tests := []struct {
		name string
		cand candidate
		want Tier
	}{
		{name: "state not synced", cand: candidate{view: state.View{Exists: true}}, want: tierUnsynced},
		{
			name: "state not synced, even with what looks like a fit",
			cand: candidate{view: state.View{Exists: true}, scorable: true, score: quotaScore{fits: true}},
			want: tierUnsynced,
		},
		{name: "LocalQueue inactive", cand: candidate{view: inactive(feeding(cq)), scorable: true, score: quotaScore{fits: true}}, want: tierHeld},
		{name: "ClusterQueue on Hold", cand: candidate{view: feeding(stopped(cq, "Hold")), scorable: true, score: quotaScore{fits: true}}, want: tierHeld},
		{name: "ClusterQueue on HoldAndDrain", cand: candidate{view: feeding(stopped(cq, "HoldAndDrain"))}, want: tierHeld},
		{name: "stop policy None", cand: candidate{view: feeding(stopped(cq, "None")), scorable: true, score: quotaScore{fits: true}}, want: tierFitsNow},
		{name: "scorable and fits now", cand: candidate{view: feeding(cq), scorable: true, score: quotaScore{fits: true}}, want: tierFitsNow},
		{name: "scorable, does not fit now", cand: candidate{view: feeding(cq), scorable: true}, want: tierQueueDepth},
		{name: "synced, not scorable", cand: candidate{view: feeding(multiFlavor(cq))}, want: tierQueueDepth},
		{name: "synced, ClusterQueue not seen", cand: candidate{view: state.View{Synced: true, Exists: true, Active: true}}, want: tierQueueDepth},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tierOf(tt.cand))
		})
	}
}

func TestRank(t *testing.T) {
	cand := func(name string, tier Tier) candidate {
		return candidate{cluster: &v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: name}}, tier: tier, eligible: true}
	}
	withSlack := func(c candidate, slack float64) candidate {
		c.scorable, c.score = true, quotaScore{fits: c.tier == tierFitsNow, slack: slack}
		return c
	}
	withPending := func(c candidate, pending int32) candidate {
		c.pending, c.pendingKnown = pending, true
		return c
	}
	withOrder := func(c candidate, order int) candidate {
		c.order = order
		return c
	}
	ineligible := func(c candidate) candidate {
		c.eligible = false
		return c
	}
	tests := []struct {
		name       string
		candidates []candidate
		want       []string
	}{
		{
			name:       "by tier",
			candidates: []candidate{cand("a", tierHeld), cand("b", tierUnsynced), cand("c", tierQueueDepth), withSlack(cand("d", tierFitsNow), 0)},
			want:       []string{"d", "c", "b", "a"},
		},
		{
			name: "tier 1 by most bottleneck slack, then name",
			candidates: []candidate{
				withSlack(cand("a", tierFitsNow), 0.06), withSlack(cand("c", tierFitsNow), 0.19), withSlack(cand("b", tierFitsNow), 0.19),
			},
			want: []string{"b", "c", "a"},
		},
		{
			name: "tier 2 by fewest pending, then smallest deficit, then name",
			candidates: []candidate{
				withPending(withSlack(cand("a", tierQueueDepth), -0.1), 3),
				withPending(withSlack(cand("b", tierQueueDepth), -0.5), 1),
				withPending(withSlack(cand("c", tierQueueDepth), -0.2), 1),
				withPending(withSlack(cand("e", tierQueueDepth), -0.2), 1),
				withPending(withSlack(cand("d", tierQueueDepth), -0.2), 1),
			},
			want: []string{"c", "d", "e", "b", "a"},
		},
		{
			name: "tier 2: an unknown deficit after a known one, at equal pending",
			candidates: []candidate{
				withPending(cand("a", tierQueueDepth), 1),
				withPending(withSlack(cand("b", tierQueueDepth), -0.9), 1),
				withPending(cand("c", tierQueueDepth), 0),
			},
			want: []string{"c", "b", "a"},
		},
		{
			name: "tier 2: unknown pending last",
			candidates: []candidate{
				cand("a", tierQueueDepth),
				withPending(withSlack(cand("b", tierQueueDepth), -0.9), 7),
			},
			want: []string{"b", "a"},
		},
		{
			name:       "tier 3 in cluster_only order",
			candidates: []candidate{withOrder(cand("a", tierUnsynced), 2), withOrder(cand("z", tierUnsynced), 0), withOrder(cand("m", tierUnsynced), 1)},
			want:       []string{"z", "m", "a"},
		},
		{
			name:       "tier 4 by name",
			candidates: []candidate{withOrder(cand("b", tierHeld), 0), withOrder(cand("a", tierHeld), 1)},
			want:       []string{"a", "b"},
		},
		{
			name: "candidates that are not eligible last, by name",
			candidates: []candidate{
				ineligible(withSlack(cand("a", tierQueueDepth), -1)), cand("c", tierHeld), ineligible(withSlack(cand("b", tierQueueDepth), -2)),
			},
			want: []string{"c", "a", "b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, c := range rank(tt.candidates) {
				got = append(got, c.cluster.GetName())
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConsideredClusters(t *testing.T) {
	a, b, c := defaultCluster("a"), kueueCluster("b"), kueueCluster("c")
	tests := []struct {
		name   string
		pinned *v2pb.Cluster
		ready  []*v2pb.Cluster
		all    []*v2pb.Cluster
		want   []consideredCluster
	}{
		{
			name:  "every registered cluster, the Ready ones first in their order",
			ready: []*v2pb.Cluster{c, a},
			all:   []*v2pb.Cluster{a, b, c},
			want:  []consideredCluster{{cluster: c, ready: true, order: 0}, {cluster: a, ready: true, order: 1}, {cluster: b, order: 2}},
		},
		{
			name:  "a cluster listed Ready after the registered clusters were listed",
			ready: []*v2pb.Cluster{c},
			all:   []*v2pb.Cluster{a},
			want:  []consideredCluster{{cluster: c, ready: true, order: 0}, {cluster: a, order: 1}},
		},
		{
			name:   "only the pinned cluster, Ready",
			pinned: c,
			ready:  []*v2pb.Cluster{a, c},
			all:    []*v2pb.Cluster{a, b, c},
			want:   []consideredCluster{{cluster: c, ready: true, order: 1}},
		},
		{
			name:   "only the pinned cluster, not Ready",
			pinned: b,
			ready:  []*v2pb.Cluster{a, c},
			all:    []*v2pb.Cluster{a, b, c},
			want:   []consideredCluster{{cluster: b}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, consideredClusters(tt.pinned, tt.ready, tt.all))
		})
	}
}

func TestExcludeClusters(t *testing.T) {
	a, b, c := defaultCluster("a"), kueueCluster("b"), kueueCluster("c")
	considered := []consideredCluster{{cluster: c, ready: true, order: 0}, {cluster: a, ready: true, order: 1}, {cluster: b, order: 2}}
	tests := []struct {
		name     string
		excluded []string
		want     []consideredCluster
	}{
		{
			name: "no excluded clusters",
			want: considered,
		},
		{
			name:     "an empty exclusion",
			excluded: []string{},
			want:     considered,
		},
		{
			name:     "a Ready cluster, the others in their order",
			excluded: []string{"c"},
			want:     []consideredCluster{{cluster: a, ready: true, order: 1}, {cluster: b, order: 2}},
		},
		{
			name:     "a cluster that is not Ready",
			excluded: []string{"b"},
			want:     []consideredCluster{{cluster: c, ready: true, order: 0}, {cluster: a, ready: true, order: 1}},
		},
		{
			name:     "every cluster",
			excluded: []string{"a", "b", "c"},
			want:     []consideredCluster{},
		},
		{
			name:     "a cluster that is not considered",
			excluded: []string{"gone"},
			want:     considered,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, excludeClusters(considered, tt.excluded))
		})
	}
}
