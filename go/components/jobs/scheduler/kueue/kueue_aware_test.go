package kueue

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/k8sengine"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/framework"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/kueue/state"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// testLocalQueue is the LocalQueue of testRayCluster's job: its namespace is
// its project.
const testLocalQueue = "ma-proj-ns"

var testNow = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

// resources returns the resource list of the name, quantity pairs.
func resources(pairs ...string) corev1.ResourceList {
	list := corev1.ResourceList{}
	for i := 0; i+1 < len(pairs); i += 2 {
		list[corev1.ResourceName(pairs[i])] = resource.MustParse(pairs[i+1])
	}
	return list
}

// assertResources asserts that the lists hold the same quantities.
func assertResources(t *testing.T, want, got corev1.ResourceList) {
	t.Helper()
	names := func(list corev1.ResourceList) []string {
		var names []string
		for name := range list {
			names = append(names, string(name))
		}
		sort.Strings(names)
		return names
	}
	if !assert.Equal(t, names(want), names(got), "resource names") {
		return
	}
	for name, quantity := range want {
		g := got[name]
		assert.Zero(t, quantity.Cmp(g), "%s: got %s, want %s", name, g.String(), quantity.String())
	}
}

// testAsk is what the test job asks of a cluster, unless the test says
// otherwise.
func testAsk() corev1.ResourceList {
	return resources("cpu", "16", "memory", "64Gi")
}

// clusterQueue returns a scorable ClusterQueue, with one flavor and no
// cohort, that covers the resources of its nominal quota.
func clusterQueue(name string, nominal, reserved corev1.ResourceList, pending int32) *state.ClusterQueueView {
	covered := make([]corev1.ResourceName, 0, len(nominal))
	for r := range nominal {
		covered = append(covered, r)
	}
	sort.Slice(covered, func(i, j int) bool { return covered[i] < covered[j] })
	return &state.ClusterQueueView{
		Name:             name,
		FlavorCount:      1,
		CoveredResources: covered,
		Nominal:          nominal,
		Reserved:         reserved,
		PendingWorkloads: pending,
	}
}

// roomy returns a ClusterQueue with all of its quota free for the test job.
func roomy(name string) *state.ClusterQueueView {
	return clusterQueue(name, resources("cpu", "64", "memory", "256Gi"), nil, 0)
}

// full returns a ClusterQueue whose nominal quota covers the test job's ask
// but whose free quota is too little for it now.
func full(name string, pending int32) *state.ClusterQueueView {
	return clusterQueue(name, resources("cpu", "64", "memory", "256Gi"), resources("cpu", "60", "memory", "250Gi"), pending)
}

// small returns a ClusterQueue whose nominal cpu is below the test job's ask.
func small(name string) *state.ClusterQueueView {
	return clusterQueue(name, resources("cpu", "8", "memory", "256Gi"), nil, 0)
}

// gpuPool and cpuPool are the ClusterQueues of clusters A and B in
// RFC-20260731's worked example.
func gpuPool() *state.ClusterQueueView {
	return clusterQueue("gpu-pool",
		resources("cpu", "64", "memory", "256Gi", string(gpu), "8"),
		resources("cpu", "44", "memory", "176Gi", string(gpu), "0"), 0)
}

func cpuPool() *state.ClusterQueueView {
	return clusterQueue("cpu-pool",
		resources("cpu", "128", "memory", "512Gi", string(gpu), "0"),
		resources("cpu", "88", "memory", "352Gi"), 0)
}

func multiFlavor(cq *state.ClusterQueueView) *state.ClusterQueueView {
	c := *cq
	c.FlavorCount = 2
	return &c
}

func inCohort(cq *state.ClusterQueueView, cohort string) *state.ClusterQueueView {
	c := *cq
	c.Cohort = cohort
	return &c
}

func stopped(cq *state.ClusterQueueView, policy string) *state.ClusterQueueView {
	c := *cq
	c.StopPolicy = policy
	return &c
}

// feeding returns the synced view of an active LocalQueue that feeds the
// ClusterQueue.
func feeding(cq *state.ClusterQueueView) state.View {
	return state.View{Synced: true, Exists: true, Active: true, ClusterQueue: cq}
}

func inactive(v state.View) state.View {
	v.Active = false
	return v
}

func inFlightJob(namespace, name, localQueue string, ask corev1.ResourceList) state.InFlightJob {
	return state.InFlightJob{
		RayCluster: &v2pb.RayCluster{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name}},
		LocalQueue: localQueue,
		Ask:        ask,
	}
}

// clusterFixture is a registered cluster in a placement test, with what its
// Kueue state, the job's demand and a direct LocalQueue read say of it.
type clusterFixture struct {
	name     string
	kueue    bool
	notReady bool
	// queues are the cluster's LocalQueues by name, in its synced state.
	queues map[string]state.View
	// unsynced means the cluster's state is not synced; get and getErr then
	// answer a direct read of the job's LocalQueue.
	unsynced bool
	get      bool
	getErr   error
	// listed means the cluster's current watch has listed its LocalQueues:
	// while its state is not synced, queues are then its LocalQueues as last
	// listed.
	listed bool
	// lookupErr fails every lookup in the cluster's synced state.
	lookupErr error
	// ask is the job's ask of the cluster; the fleet's when nil.
	ask    corev1.ResourceList
	askErr error
	// inFlight are the jobs in flight to the cluster.
	inFlight []state.InFlightJob
}

// withQueue returns a Ready, Kueue-managed cluster whose synced state shows
// the job's LocalQueue as the view.
func withQueue(name string, view state.View) clusterFixture {
	return clusterFixture{name: name, kueue: true, queues: map[string]state.View{testLocalQueue: view}}
}

// withoutQueue returns a Ready, Kueue-managed cluster whose synced state shows
// no LocalQueue.
func withoutQueue(name string) clusterFixture {
	return clusterFixture{name: name, kueue: true}
}

// unsyncedCluster returns a Ready, Kueue-managed cluster whose state is not
// synced, where a direct read finds the job's LocalQueue when exists.
func unsyncedCluster(name string, exists bool) clusterFixture {
	return clusterFixture{name: name, kueue: true, unsynced: true, get: exists}
}

// lastListed returns a Ready, Kueue-managed cluster whose state is not synced,
// as when its watch broke, but whose current watch has listed its LocalQueues:
// the job's among them when exists.
func lastListed(name string, exists bool) clusterFixture {
	fx := unsyncedCluster(name, exists)
	fx.listed = true
	if exists {
		fx.queues = map[string]state.View{testLocalQueue: feeding(roomy("cq-" + name))}
	}
	return fx
}

// plainCluster returns a Ready cluster that is not Kueue-managed.
func plainCluster(name string) clusterFixture {
	return clusterFixture{name: name}
}

// down returns the cluster, not Ready.
func (fx clusterFixture) down() clusterFixture {
	fx.notReady = true
	return fx
}

// testFleet is the registered clusters of a placement test: the cluster
// cache, Kueue state, demand and LocalQueue reader that the kueue_aware
// strategy and the Kueue backend read them through.
type testFleet struct {
	fixtures map[string]*clusterFixture
	clusters []*v2pb.Cluster
	// ask is the job's ask of a cluster whose fixture has none.
	ask corev1.ResourceList
	// inFlightErr fails every read of the jobs in flight.
	inFlightErr error

	// lookups counts the lookups in the Kueue state.
	lookups int
	// gets are the direct LocalQueue reads, as cluster/namespace/name.
	gets []string
	// recorded are the recorded assignments, which are in flight to their
	// clusters from then on, as the state's overlay counts them.
	recorded []recordedJob
}

type recordedJob struct {
	job     *v2pb.RayCluster
	cluster string
	at      time.Time
}

var (
	_ cluster.RegisteredClustersCache = (*testFleet)(nil)
	_ state.State                     = (*testFleet)(nil)
	_ state.Demand                    = (*testFleet)(nil)
	_ LocalQueues                     = (*testFleet)(nil)
)

// newTestFleet returns the clusters, the Ready ones in cluster_only order.
func newTestFleet(fixtures ...clusterFixture) *testFleet {
	f := &testFleet{fixtures: map[string]*clusterFixture{}, ask: testAsk()}
	for i := range fixtures {
		fx := fixtures[i]
		f.fixtures[fx.name] = &fx
		if fx.kueue {
			f.clusters = append(f.clusters, kueueCluster(fx.name))
		} else {
			f.clusters = append(f.clusters, defaultCluster(fx.name))
		}
	}
	return f
}

// strategy returns the kueue_aware strategy over the fleet.
func (f *testFleet) strategy() *KueueAwareStrategy {
	return NewKueueAwareStrategy(KueueAwareParams{
		ClusterCache: f,
		LocalQueues:  f,
		State:        f,
		Demand:       f,
		Config:       kueueConfig(),
		Logger:       logr.Discard(),
		Now:          func() time.Time { return testNow },
	})
}

func (f *testFleet) GetClusters(filter cluster.FilterType) []*v2pb.Cluster {
	var result []*v2pb.Cluster
	for _, c := range f.clusters {
		ready := !f.fixtures[c.GetName()].notReady
		if (filter == cluster.ReadyClusters && !ready) || (filter == cluster.UnreadyClusters && ready) {
			continue
		}
		result = append(result, c)
	}
	return result
}

func (f *testFleet) GetCluster(name string) *v2pb.Cluster {
	for _, c := range f.clusters {
		if c.GetName() == name {
			return c
		}
	}
	return nil
}

func (f *testFleet) Lookup(clusterName, localQueue string) (state.View, error) {
	f.lookups++
	fx := f.fixtures[clusterName]
	switch {
	case fx == nil || !fx.kueue || fx.unsynced:
		return state.View{}, nil
	case fx.lookupErr != nil:
		return state.View{}, fx.lookupErr
	}
	view, ok := fx.queues[localQueue]
	if !ok {
		return state.View{Synced: true}, nil
	}
	view.Synced = true
	return view, nil
}

// LastKnown answers from the cluster's LocalQueues: those of its synced state,
// or those last listed while it is not synced, once its watch has listed them.
func (f *testFleet) LastKnown(clusterName, localQueue string) (exists, known bool) {
	f.lookups++
	fx := f.fixtures[clusterName]
	if fx == nil || !fx.kueue || fx.lookupErr != nil || fx.unsynced && !fx.listed {
		return false, false
	}
	_, exists = fx.queues[localQueue]
	return exists, true
}

func (f *testFleet) Synced(clusterName string) bool {
	fx := f.fixtures[clusterName]
	return fx != nil && fx.kueue && !fx.unsynced
}

func (f *testFleet) Ask(_ *v2pb.RayCluster, c *v2pb.Cluster) (corev1.ResourceList, error) {
	fx := f.fixtures[c.GetName()]
	switch {
	case fx.askErr != nil:
		return nil, fx.askErr
	case fx.ask != nil:
		return fx.ask.DeepCopy(), nil
	default:
		return f.ask.DeepCopy(), nil
	}
}

func (f *testFleet) InFlight(_ context.Context, c *v2pb.Cluster, _ time.Time) (state.InFlight, error) {
	if f.inFlightErr != nil {
		return state.InFlight{}, f.inFlightErr
	}
	inFlight := state.InFlight{Jobs: append([]state.InFlightJob(nil), f.fixtures[c.GetName()].inFlight...)}
	for _, r := range f.recorded {
		if r.cluster == c.GetName() {
			inFlight.Jobs = append(inFlight.Jobs, inFlightJob(r.job.GetNamespace(), r.job.GetName(), testLocalQueue, f.ask.DeepCopy()))
		}
	}
	return inFlight, nil
}

func (f *testFleet) RecordAssignment(job *v2pb.RayCluster, clusterName string, at time.Time) {
	f.recorded = append(f.recorded, recordedJob{job: job.DeepCopy(), cluster: clusterName, at: at})
}

// Exists reads a LocalQueue directly: as the cluster's synced state shows it,
// or as the fixture says when that is not synced.
func (f *testFleet) Exists(_ context.Context, c *v2pb.Cluster, namespace, name string) (bool, error) {
	f.gets = append(f.gets, c.GetName()+"/"+namespace+"/"+name)
	fx := f.fixtures[c.GetName()]
	if fx.unsynced {
		return fx.get, fx.getErr
	}
	_, exists := fx.queues[name]
	return exists, nil
}

// readsOf returns the direct reads of the job's LocalQueue on the clusters.
func readsOf(clusters ...string) []string {
	var reads []string
	for _, c := range clusters {
		reads = append(reads, c+"/"+k8sengine.RayLocalNamespace+"/"+testLocalQueue)
	}
	return reads
}

// pinnedTo returns the labels of a job whose affinity pins the cluster.
func pinnedTo(name string) map[string]string {
	return map[string]string{constants.ClusterAffinityLabelKey: name}
}

func nameOf(c *v2pb.Cluster) string {
	if c == nil {
		return ""
	}
	return c.GetName()
}

func candidateNames(d Decision) []string {
	names := []string{}
	for _, c := range d.candidates {
		names = append(names, c.cluster.GetName())
	}
	return names
}

func TestKueueAwareSelect(t *testing.T) {
	notFoundAnywhere := `LocalQueue "ma-proj-ns" not found in namespace "default" on any Kueue-managed cluster, ` +
		`and no Ready cluster without Kueue to fall back to`
	noProject := fmt.Sprintf("cannot resolve a LocalQueue: the job has no project identity (a namespace, or a trusted %s label)",
		constants.ProjectNameLabelKey)

	type want struct {
		cluster string
		tier    Tier
		outcome string
		reason  string
		message string
		// reads are the clusters whose LocalQueue is read directly.
		reads []string
		// candidates are the candidates in rank order; not checked when nil.
		candidates []string
	}
	tests := []struct {
		name      string
		clusters  []clusterFixture
		labels    map[string]string
		noProject bool
		ask       corev1.ResourceList
		want      want
	}{
		// 1. Pin
		{
			name:     "a pin to a cluster without Kueue is assigned as today",
			clusters: []clusterFixture{withQueue("kueue-a", feeding(roomy("cq-a"))), plainCluster("plain")},
			labels:   pinnedTo("plain"),
			want:     want{cluster: "plain", outcome: outcomePinned, candidates: []string{}},
		},
		{
			name:     "a pin to a cluster without Kueue that is not Ready is assigned as today",
			clusters: []clusterFixture{plainCluster("plain").down(), withQueue("kueue-a", feeding(roomy("cq-a")))},
			labels:   pinnedTo("plain"),
			want:     want{cluster: "plain", outcome: outcomePinned},
		},
		{
			name:     "a Kueue-managed pin skips ranking",
			clusters: []clusterFixture{withQueue("kueue-a", feeding(roomy("cq-a"))), withQueue("kueue-b", feeding(full("cq-b", 3)))},
			labels:   pinnedTo("kueue-b"),
			want:     want{cluster: "kueue-b", tier: tierQueueDepth, outcome: outcomeAssigned, candidates: []string{"kueue-b"}},
		},
		{
			name: "a Kueue-managed pin that is not Ready waits, whatever else is Ready",
			clusters: []clusterFixture{
				withQueue("kueue-a", feeding(roomy("cq-a"))).down(), withQueue("kueue-b", feeding(roomy("cq-b"))), plainCluster("plain"),
			},
			labels: pinnedTo("kueue-a"),
			want: want{
				outcome: outcomeClusterNotReady,
				reason:  constants.KueueClusterNotReady,
				message: `no Ready Kueue-managed cluster carries LocalQueue "ma-proj-ns"; not Ready: kueue-a`,
			},
		},
		{
			name:     "a Kueue-managed pin without the LocalQueue waits, though a cluster without Kueue is Ready",
			clusters: []clusterFixture{withoutQueue("kueue-a"), plainCluster("plain")},
			labels:   pinnedTo("kueue-a"),
			want: want{
				outcome: outcomeQueueNotFound,
				reason:  constants.KueueQueueNotFound,
				message: `LocalQueue "ma-proj-ns" not found in namespace "default" on cluster "kueue-a", which the job's affinity pins`,
			},
		},
		{
			name:     "a Kueue-managed pin whose nominal quota is below the ask waits",
			clusters: []clusterFixture{withQueue("kueue-a", feeding(small("cq-a"))), withQueue("kueue-b", feeding(roomy("cq-b")))},
			labels:   pinnedTo("kueue-a"),
			want: want{
				outcome: outcomeQuotaExceeded,
				reason:  constants.KueueQuotaExceeded,
				message: `the job's ask exceeds the nominal quota for LocalQueue "ma-proj-ns" on every candidate cluster: ` +
					`kueue-a (ClusterQueue cq-a: cpu 16 > nominal 8)`,
			},
		},
		{
			name:     "a Kueue-managed pin whose state is not synced is read directly",
			clusters: []clusterFixture{unsyncedCluster("kueue-a", true), withQueue("kueue-b", feeding(roomy("cq-b")))},
			labels:   pinnedTo("kueue-a"),
			want:     want{cluster: "kueue-a", tier: tierUnsynced, outcome: outcomeAssigned, reads: []string{"kueue-a"}},
		},
		{
			name:     "a pin to a cluster that is not registered is ignored",
			clusters: []clusterFixture{withQueue("kueue-a", feeding(full("cq-a", 0))), withQueue("kueue-b", feeding(roomy("cq-b")))},
			labels:   pinnedTo("gone"),
			want:     want{cluster: "kueue-b", tier: tierFitsNow, outcome: outcomeAssigned},
		},

		// 2. Candidates
		{
			name: "candidates are the Ready, Kueue-managed clusters that carry the LocalQueue",
			clusters: []clusterFixture{
				withQueue("kueue-a", feeding(roomy("cq-a"))).down(),
				withoutQueue("kueue-b"),
				plainCluster("plain"),
				withQueue("kueue-c", feeding(full("cq-c", 1))),
			},
			want: want{cluster: "kueue-c", tier: tierQueueDepth, outcome: outcomeAssigned, candidates: []string{"kueue-c"}},
		},

		// 3. Eligible
		{
			name: "a resource the ClusterQueue does not cover counts as zero",
			clusters: []clusterFixture{
				withQueue("kueue-a", feeding(roomy("cq-a"))),
				withQueue("kueue-b", feeding(clusterQueue("cq-b",
					resources("cpu", "64", string(gpu), "2"), resources(string(gpu), "2"), 4))),
			},
			ask:  resources("cpu", "1", string(gpu), "1"),
			want: want{cluster: "kueue-b", tier: tierQueueDepth, outcome: outcomeAssigned, candidates: []string{"kueue-b", "kueue-a"}},
		},
		{
			name:     "a resource asked at zero is not requested",
			clusters: []clusterFixture{withQueue("kueue-a", feeding(roomy("cq-a")))},
			ask:      resources("cpu", "1", "memory", "1Gi", string(gpu), "0"),
			want:     want{cluster: "kueue-a", tier: tierFitsNow, outcome: outcomeAssigned},
		},
		{
			name:     "a candidate with several flavors stays eligible, whatever its quota",
			clusters: []clusterFixture{withQueue("kueue-a", feeding(multiFlavor(small("cq-a"))))},
			want:     want{cluster: "kueue-a", tier: tierQueueDepth, outcome: outcomeAssigned},
		},
		{
			name:     "a candidate in a cohort stays eligible, whatever its quota",
			clusters: []clusterFixture{withQueue("kueue-a", feeding(inCohort(small("cq-a"), "pool")))},
			want:     want{cluster: "kueue-a", tier: tierQueueDepth, outcome: outcomeAssigned},
		},
		{
			name: "a candidate whose ask cannot be computed stays eligible",
			clusters: []clusterFixture{{
				name: "kueue-a", kueue: true, queues: map[string]state.View{testLocalQueue: feeding(small("cq-a"))},
				askErr: errors.New("no mapper for the job"),
			}},
			want: want{cluster: "kueue-a", tier: tierQueueDepth, outcome: outcomeAssigned},
		},

		// 4. Rank
		{
			name: "tier 1 before tiers 2, 3 and 4",
			clusters: []clusterFixture{
				withQueue("kueue-a", inactive(feeding(roomy("cq-a")))),
				unsyncedCluster("kueue-b", true),
				withQueue("kueue-c", feeding(full("cq-c", 0))),
				withQueue("kueue-d", feeding(roomy("cq-d"))),
			},
			want: want{
				cluster: "kueue-d", tier: tierFitsNow, outcome: outcomeAssigned, reads: []string{"kueue-b"},
				candidates: []string{"kueue-d", "kueue-c", "kueue-b", "kueue-a"},
			},
		},
		{
			name: "tier 1: the most bottleneck slack",
			clusters: []clusterFixture{
				withQueue("kueue-a", feeding(clusterQueue("cq-a",
					resources("cpu", "64", "memory", "256Gi"), resources("cpu", "32", "memory", "64Gi"), 0))),
				withQueue("kueue-b", feeding(roomy("cq-b"))),
			},
			want: want{cluster: "kueue-b", tier: tierFitsNow, outcome: outcomeAssigned},
		},
		{
			name:     "tier 1: equal slack, by name",
			clusters: []clusterFixture{withQueue("kueue-b", feeding(roomy("cq-b"))), withQueue("kueue-a", feeding(roomy("cq-a")))},
			want:     want{cluster: "kueue-a", tier: tierFitsNow, outcome: outcomeAssigned},
		},
		{
			name:     "tier 2 when nothing fits now: the fewest pending workloads",
			clusters: []clusterFixture{withQueue("kueue-a", feeding(full("cq-a", 5))), withQueue("kueue-b", feeding(full("cq-b", 1)))},
			want:     want{cluster: "kueue-b", tier: tierQueueDepth, outcome: outcomeAssigned},
		},
		{
			name: "tier 2: jobs in flight count as pending",
			clusters: []clusterFixture{
				{
					name: "kueue-a", kueue: true, queues: map[string]state.View{testLocalQueue: feeding(full("cq-a", 1))},
					inFlight: []state.InFlightJob{
						inFlightJob("proj-ns", "job-2", testLocalQueue, resources("cpu", "1")),
						inFlightJob("proj-ns", "job-3", testLocalQueue, resources("cpu", "1")),
					},
				},
				withQueue("kueue-b", feeding(full("cq-b", 2))),
			},
			want: want{cluster: "kueue-b", tier: tierQueueDepth, outcome: outcomeAssigned},
		},
		{
			name: "tier 2: equal pending, the smallest deficit",
			clusters: []clusterFixture{
				withQueue("kueue-a", feeding(full("cq-a", 1))),
				withQueue("kueue-b", feeding(clusterQueue("cq-b",
					resources("cpu", "64", "memory", "256Gi"), resources("cpu", "56", "memory", "200Gi"), 1))),
			},
			want: want{cluster: "kueue-b", tier: tierQueueDepth, outcome: outcomeAssigned},
		},
		{
			name: "tier 2: an idle cluster that cannot be scored before a scorable one with pending workloads",
			clusters: []clusterFixture{
				withQueue("kueue-a", feeding(full("cq-a", 3))), withQueue("kueue-b", feeding(multiFlavor(full("cq-b", 0)))),
			},
			want: want{cluster: "kueue-b", tier: tierQueueDepth, outcome: outcomeAssigned},
		},
		{
			name: "tier 2: equal pending, a known deficit before an unknown one",
			clusters: []clusterFixture{
				withQueue("kueue-a", feeding(multiFlavor(full("cq-a", 1)))), withQueue("kueue-b", feeding(full("cq-b", 1))),
			},
			want: want{cluster: "kueue-b", tier: tierQueueDepth, outcome: outcomeAssigned},
		},
		{
			name:     "tier 2: equal, by name",
			clusters: []clusterFixture{withQueue("kueue-b", feeding(full("cq-b", 1))), withQueue("kueue-a", feeding(full("cq-a", 1)))},
			want:     want{cluster: "kueue-a", tier: tierQueueDepth, outcome: outcomeAssigned},
		},
		{
			name:     "tier 3 when the state is not synced, in cluster_only order",
			clusters: []clusterFixture{unsyncedCluster("kueue-z", true), unsyncedCluster("kueue-a", true)},
			want: want{
				cluster: "kueue-z", tier: tierUnsynced, outcome: outcomeAssigned,
				reads: []string{"kueue-z", "kueue-a"}, candidates: []string{"kueue-z", "kueue-a"},
			},
		},
		{
			name:     "tier 3 before tier 4",
			clusters: []clusterFixture{withQueue("kueue-a", feeding(stopped(roomy("cq-a"), "Hold"))), unsyncedCluster("kueue-b", true)},
			want:     want{cluster: "kueue-b", tier: tierUnsynced, outcome: outcomeAssigned, reads: []string{"kueue-b"}},
		},
		{
			name: "tier 4 when the LocalQueue is inactive or the ClusterQueue stopped, by name",
			clusters: []clusterFixture{
				withQueue("kueue-b", feeding(stopped(roomy("cq-b"), "HoldAndDrain"))), withQueue("kueue-a", inactive(feeding(roomy("cq-a")))),
			},
			want: want{cluster: "kueue-a", tier: tierHeld, outcome: outcomeAssigned},
		},

		// No eligible candidate
		{
			name: "not onboarded: cluster_only over the Ready clusters without Kueue",
			clusters: []clusterFixture{
				withoutQueue("kueue-a"), plainCluster("plain-b").down(), plainCluster("plain-c"), plainCluster("plain-d"),
			},
			want: want{cluster: "plain-c", outcome: outcomeFallback, candidates: []string{}},
		},
		{
			name:     "not onboarded, and no Ready cluster without Kueue",
			clusters: []clusterFixture{withoutQueue("kueue-a"), plainCluster("plain").down()},
			want:     want{outcome: outcomeQueueNotFound, reason: constants.KueueQueueNotFound, message: notFoundAnywhere},
		},
		{
			name: "no registered clusters",
			want: want{outcome: outcomeQueueNotFound, reason: constants.KueueQueueNotFound, message: notFoundAnywhere},
		},
		{
			name: "the LocalQueue only on clusters that are not Ready",
			clusters: []clusterFixture{
				withQueue("kueue-b", feeding(roomy("cq-b"))).down(), withQueue("kueue-a", feeding(roomy("cq-a"))).down(), plainCluster("plain"),
			},
			want: want{
				outcome: outcomeClusterNotReady,
				reason:  constants.KueueClusterNotReady,
				message: `no Ready Kueue-managed cluster carries LocalQueue "ma-proj-ns"; not Ready: kueue-a, kueue-b`,
			},
		},
		{
			name:     "a cluster that is not Ready whose watch has not listed its LocalQueues may carry the LocalQueue, and is not read",
			clusters: []clusterFixture{unsyncedCluster("kueue-a", true).down(), plainCluster("plain")},
			want: want{
				outcome: outcomeClusterNotReady,
				reason:  constants.KueueClusterNotReady,
				message: `no Ready Kueue-managed cluster carries LocalQueue "ma-proj-ns"; not Ready: kueue-a (queues not listed yet)`,
			},
		},
		{
			name: "a cluster that is not Ready and whose state cannot be read may carry the LocalQueue",
			clusters: []clusterFixture{
				{name: "kueue-a", kueue: true, notReady: true, lookupErr: errors.New("informer stopped")}, plainCluster("plain"),
			},
			want: want{
				outcome: outcomeClusterNotReady,
				reason:  constants.KueueClusterNotReady,
				message: `no Ready Kueue-managed cluster carries LocalQueue "ma-proj-ns"; not Ready: kueue-a (queues cannot be read)`,
			},
		},
		{
			name:     "a cluster that is not Ready whose synced state shows no LocalQueue is not waited for",
			clusters: []clusterFixture{withoutQueue("kueue-a").down(), plainCluster("plain")},
			want:     want{cluster: "plain", outcome: outcomeFallback},
		},
		{
			name:     "a cluster that is not Ready and not synced whose LocalQueues as last listed lack the LocalQueue is not waited for",
			clusters: []clusterFixture{lastListed("kueue-a", false).down(), plainCluster("plain")},
			want:     want{cluster: "plain", outcome: outcomeFallback, candidates: []string{}},
		},
		{
			name:     "a pin to a cluster that is not Ready and not synced whose LocalQueues as last listed lack the LocalQueue",
			clusters: []clusterFixture{lastListed("kueue-a", false).down(), plainCluster("plain")},
			labels:   pinnedTo("kueue-a"),
			want: want{
				outcome: outcomeQueueNotFound,
				reason:  constants.KueueQueueNotFound,
				message: `LocalQueue "ma-proj-ns" not found in namespace "default" on cluster "kueue-a", which the job's affinity pins`,
			},
		},
		{
			name:     "a cluster that is not Ready and not synced whose LocalQueues as last listed show the LocalQueue",
			clusters: []clusterFixture{lastListed("kueue-a", true).down(), plainCluster("plain")},
			want: want{
				outcome: outcomeClusterNotReady,
				reason:  constants.KueueClusterNotReady,
				message: `no Ready Kueue-managed cluster carries LocalQueue "ma-proj-ns"; not Ready: kueue-a`,
			},
		},
		{
			name:     "candidates over their nominal quota, beside a cluster that is not Ready",
			clusters: []clusterFixture{withQueue("kueue-a", feeding(small("cq-a"))), withQueue("kueue-b", feeding(roomy("cq-b"))).down()},
			want: want{
				outcome: outcomeQuotaExceeded,
				reason:  constants.KueueQuotaExceeded,
				message: `the job's ask exceeds the nominal quota for LocalQueue "ma-proj-ns" on every candidate cluster: ` +
					`kueue-a (ClusterQueue cq-a: cpu 16 > nominal 8)`,
			},
		},
		{
			name: "every candidate over its nominal quota",
			clusters: []clusterFixture{
				withQueue("kueue-b", feeding(small("cq-b"))),
				withQueue("kueue-a", feeding(clusterQueue("cq-a", resources("cpu", "64", "memory", "32Gi"), nil, 0))),
				plainCluster("plain"),
			},
			want: want{
				outcome: outcomeQuotaExceeded,
				reason:  constants.KueueQuotaExceeded,
				message: `the job's ask exceeds the nominal quota for LocalQueue "ma-proj-ns" on every candidate cluster: ` +
					`kueue-a (ClusterQueue cq-a: memory 64Gi > nominal 32Gi), kueue-b (ClusterQueue cq-b: cpu 16 > nominal 8)`,
				candidates: []string{"kueue-a", "kueue-b"},
			},
		},
		{
			name: "a Ready cluster whose LocalQueue cannot be read",
			clusters: []clusterFixture{
				{name: "kueue-a", kueue: true, unsynced: true, getErr: errors.New("connection refused")}, plainCluster("plain"),
			},
			want: want{
				outcome: outcomeCheckFailed,
				reason:  constants.KueueQueueCheckFailed,
				message: `checking LocalQueue "ma-proj-ns" on cluster "kueue-a": connection refused`,
				reads:   []string{"kueue-a"},
			},
		},
		{
			name: "Ready clusters whose state or LocalQueue cannot be read",
			clusters: []clusterFixture{
				{name: "kueue-a", kueue: true, lookupErr: errors.New("informer stopped")},
				{name: "kueue-b", kueue: true, unsynced: true, getErr: errors.New("timeout")},
				withQueue("kueue-c", feeding(small("cq-c"))),
			},
			want: want{
				outcome: outcomeCheckFailed,
				reason:  constants.KueueQueueCheckFailed,
				message: `checking LocalQueue "ma-proj-ns" on cluster "kueue-a": informer stopped (and 1 more clusters: kueue-b)`,
				reads:   []string{"kueue-b"},
			},
		},
		{
			name: "a cluster that cannot be read does not hold back an eligible candidate",
			clusters: []clusterFixture{
				{name: "kueue-a", kueue: true, unsynced: true, getErr: errors.New("timeout")}, withQueue("kueue-b", feeding(full("cq-b", 2))),
			},
			want: want{cluster: "kueue-b", tier: tierQueueDepth, outcome: outcomeAssigned, reads: []string{"kueue-a"}},
		},
		{
			name:      "no project identity: the job falls back",
			clusters:  []clusterFixture{withQueue("kueue-a", feeding(roomy("cq-a"))), plainCluster("plain")},
			noProject: true,
			want:      want{cluster: "plain", outcome: outcomeFallback},
		},
		{
			name:      "no project identity, and no Ready cluster without Kueue",
			clusters:  []clusterFixture{withQueue("kueue-a", feeding(roomy("cq-a")))},
			noProject: true,
			want:      want{outcome: outcomeQueueNotFound, reason: constants.KueueQueueNotFound, message: noProject},
		},
		{
			name:      "no project identity, pinned to a Kueue-managed cluster",
			clusters:  []clusterFixture{withQueue("kueue-a", feeding(roomy("cq-a"))), plainCluster("plain")},
			labels:    pinnedTo("kueue-a"),
			noProject: true,
			want:      want{outcome: outcomeQueueNotFound, reason: constants.KueueQueueNotFound, message: noProject},
		},

		// Direct reads while the state is not synced
		{
			name:     "a direct read that does not find the LocalQueue",
			clusters: []clusterFixture{unsyncedCluster("kueue-a", false), withQueue("kueue-b", feeding(full("cq-b", 0)))},
			want: want{
				cluster: "kueue-b", tier: tierQueueDepth, outcome: outcomeAssigned,
				reads: []string{"kueue-a"}, candidates: []string{"kueue-b"},
			},
		},
		{
			name:     "a direct read that does not find the LocalQueue, nor anywhere else",
			clusters: []clusterFixture{unsyncedCluster("kueue-a", false), plainCluster("plain")},
			want:     want{cluster: "plain", outcome: outcomeFallback, reads: []string{"kueue-a"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fleet := newTestFleet(tt.clusters...)
			if tt.ask != nil {
				fleet.ask = tt.ask
			}
			job := testRayCluster(tt.labels, "")
			if tt.noProject {
				job.Namespace = ""
			}

			d, err := fleet.strategy().Select(context.Background(), framework.BatchRayCluster{RayCluster: job})
			require.NoError(t, err)

			assert.Equal(t, tt.want.cluster, nameOf(d.Cluster), "cluster")
			assert.Equal(t, tt.want.tier, d.Tier, "tier")
			assert.Equal(t, tt.want.outcome, d.Outcome, "outcome")
			assert.Equal(t, tt.want.reason, d.Reason, "reason")
			assert.Equal(t, tt.want.message, d.Message, "message")
			assert.Equal(t, readsOf(tt.want.reads...), fleet.gets, "direct LocalQueue reads")
			if tt.want.candidates != nil {
				assert.Equal(t, tt.want.candidates, candidateNames(d), "candidates")
			}
			if tt.noProject {
				assert.Empty(t, d.LocalQueue)
			} else {
				assert.Equal(t, testLocalQueue, d.LocalQueue)
			}
			assert.Empty(t, fleet.recorded, "a decision records nothing in flight")
		})
	}
}

// TestKueueAwareSelectWorkedExample is RFC-20260731's worked example: A has
// more free capacity counting its idle GPUs, but the job asks for none, and B
// has the most slack on the job's bottleneck resource.
func TestKueueAwareSelectWorkedExample(t *testing.T) {
	fleet := newTestFleet(withQueue("kueue-a", feeding(gpuPool())), withQueue("kueue-b", feeding(cpuPool())))

	d, err := fleet.strategy().Select(context.Background(), framework.BatchRayCluster{RayCluster: testRayCluster(nil, "")})
	require.NoError(t, err)

	assert.Equal(t, "kueue-b", nameOf(d.Cluster))
	assert.Equal(t, tierFitsNow, d.Tier)
	require.Equal(t, []string{"kueue-b", "kueue-a"}, candidateNames(d))
	b, a := d.candidates[0], d.candidates[1]
	assertResources(t, resources("cpu", "40", "memory", "160Gi"), b.free)
	assert.InDelta(t, 0.1875, b.score.slack, 1e-9)
	assert.Equal(t, corev1.ResourceName("cpu"), b.score.binding)
	assertResources(t, resources("cpu", "20", "memory", "80Gi"), a.free)
	assert.InDelta(t, 0.0625, a.score.slack, 1e-9)
}

// TestKueueAwareSelectCountsInFlightPerClusterQueue places a job on a cluster
// with two LocalQueues that feed different ClusterQueues: the job's
// ClusterQueue's free quota and pending workloads count only the jobs in
// flight through LocalQueues that feed it, and never the job itself.
func TestKueueAwareSelectCountsInFlightPerClusterQueue(t *testing.T) {
	shared := clusterFixture{
		name:  "kueue-a",
		kueue: true,
		queues: map[string]state.View{
			testLocalQueue: feeding(clusterQueue("cq-proj", resources("cpu", "40"), resources("cpu", "10"), 2)),
			"ma-other":     feeding(clusterQueue("cq-other", resources("cpu", "100"), nil, 0)),
		},
		inFlight: []state.InFlightJob{
			inFlightJob("proj-ns", "job-2", testLocalQueue, resources("cpu", "10")),
			inFlightJob("other-ns", "job-3", "ma-other", resources("cpu", "30")),
			inFlightJob("gone-ns", "job-4", "ma-gone", resources("cpu", "50")),
			// the job being placed, recorded by an earlier decision
			inFlightJob("proj-ns", "job-1", testLocalQueue, resources("cpu", "16")),
		},
	}
	fleet := newTestFleet(shared,
		withQueue("kueue-b", feeding(clusterQueue("cq-b", resources("cpu", "100"), resources("cpu", "76"), 0))))
	fleet.ask = resources("cpu", "16")

	d, err := fleet.strategy().Select(context.Background(), framework.BatchRayCluster{RayCluster: testRayCluster(nil, "")})
	require.NoError(t, err)

	// cq-proj: 40 nominal - 10 reserved - 10 in flight through ma-proj-ns
	// leaves 20 free; counting job-3, job-4 or the job itself leaves too
	// little, and kueue-b (24 free of 100) would win
	assert.Equal(t, "kueue-a", nameOf(d.Cluster))
	assert.Equal(t, tierFitsNow, d.Tier)
	require.Equal(t, []string{"kueue-a", "kueue-b"}, candidateNames(d))
	a := d.candidates[0]
	assertResources(t, resources("cpu", "20"), a.free)
	assert.InDelta(t, 0.1, a.score.slack, 1e-9)
	assert.Equal(t, int32(3), a.pending, "2 pending workloads and 1 job in flight through ma-proj-ns")
	assert.InDelta(t, 0.08, d.candidates[1].score.slack, 1e-9)
}

func TestKueueAwareSelectErrors(t *testing.T) {
	t.Run("a job that is not a RayCluster", func(t *testing.T) {
		fleet := newTestFleet(withQueue("kueue-a", feeding(roomy("cq-a"))))
		sj := &v2pb.SparkJob{ObjectMeta: metav1.ObjectMeta{Name: "spark-1", Namespace: "proj-ns"}}

		_, err := fleet.strategy().Select(context.Background(), framework.BatchSparkJob{SparkJob: sj})
		require.ErrorContains(t, err, "kueue_aware places RayClusters only")
		assert.Zero(t, fleet.lookups)
	})
	t.Run("the jobs in flight cannot be read", func(t *testing.T) {
		fleet := newTestFleet(withQueue("kueue-a", feeding(roomy("cq-a"))))
		fleet.inFlightErr = errors.New("manager cache not started")

		_, err := fleet.strategy().Select(context.Background(), framework.BatchRayCluster{RayCluster: testRayCluster(nil, "")})
		require.ErrorIs(t, err, fleet.inFlightErr)
		assert.ErrorContains(t, err, "jobs in flight to cluster kueue-a")
	})
}

func TestKueueAwareSelectExcluding(t *testing.T) {
	type want struct {
		cluster string
		tier    Tier
		outcome string
		reason  string
		message string
		// waived is whether the decision was made without the exclusion.
		waived bool
		// candidates are the candidates in rank order; not checked when nil.
		candidates []string
	}
	tests := []struct {
		name     string
		clusters []clusterFixture
		labels   map[string]string
		excluded []string
		want     want
	}{
		{
			name:     "an excluded top candidate is skipped for the next, in its own tier",
			clusters: []clusterFixture{withQueue("kueue-a", feeding(roomy("cq-a"))), withQueue("kueue-b", feeding(full("cq-b", 1)))},
			excluded: []string{"kueue-a"},
			want:     want{cluster: "kueue-b", tier: tierQueueDepth, outcome: outcomeAssigned, candidates: []string{"kueue-b"}},
		},
		{
			name:     "every candidate excluded: the excluded top candidate, as without the exclusion",
			clusters: []clusterFixture{withQueue("kueue-a", feeding(roomy("cq-a"))), withQueue("kueue-b", feeding(full("cq-b", 1)))},
			excluded: []string{"kueue-a", "kueue-b"},
			want: want{
				cluster: "kueue-a", tier: tierFitsNow, outcome: outcomeAssigned, waived: true, candidates: []string{"kueue-a", "kueue-b"},
			},
		},
		{
			name:     "every candidate excluded: the job waits as it would without the exclusion",
			clusters: []clusterFixture{withQueue("kueue-a", feeding(small("cq-a")))},
			excluded: []string{"kueue-a"},
			want: want{
				outcome: outcomeQuotaExceeded,
				reason:  constants.KueueQuotaExceeded,
				message: `the job's ask exceeds the nominal quota for LocalQueue "ma-proj-ns" on every candidate cluster: ` +
					`kueue-a (ClusterQueue cq-a: cpu 16 > nominal 8)`,
				waived: true,
			},
		},
		{
			name:     "every candidate excluded beside a cluster without Kueue: the decision without the exclusion, not the fallback",
			clusters: []clusterFixture{withQueue("kueue-a", feeding(roomy("cq-a"))), plainCluster("plain")},
			excluded: []string{"kueue-a"},
			want:     want{cluster: "kueue-a", tier: tierFitsNow, outcome: outcomeAssigned, waived: true, candidates: []string{"kueue-a"}},
		},
		{
			name:     "a pinned and excluded Kueue-managed cluster keeps the pin",
			clusters: []clusterFixture{withQueue("kueue-a", feeding(roomy("cq-a"))), withQueue("kueue-b", feeding(full("cq-b", 3)))},
			labels:   pinnedTo("kueue-b"),
			excluded: []string{"kueue-b"},
			want:     want{cluster: "kueue-b", tier: tierQueueDepth, outcome: outcomeAssigned, waived: true, candidates: []string{"kueue-b"}},
		},
		{
			name:     "a pinned and excluded cluster without Kueue keeps the pin",
			clusters: []clusterFixture{withQueue("kueue-a", feeding(roomy("cq-a"))), plainCluster("plain")},
			labels:   pinnedTo("plain"),
			excluded: []string{"plain"},
			want:     want{cluster: "plain", outcome: outcomePinned, waived: true, candidates: []string{}},
		},
		{
			name:     "a pin that is not excluded is decided as without the exclusion",
			clusters: []clusterFixture{withQueue("kueue-a", feeding(roomy("cq-a"))), withQueue("kueue-b", feeding(full("cq-b", 3)))},
			labels:   pinnedTo("kueue-b"),
			excluded: []string{"kueue-a"},
			want:     want{cluster: "kueue-b", tier: tierQueueDepth, outcome: outcomeAssigned, candidates: []string{"kueue-b"}},
		},
		{
			name:     "an excluded fallback cluster: the next Ready cluster without Kueue",
			clusters: []clusterFixture{withoutQueue("kueue-a"), plainCluster("plain-a"), plainCluster("plain-b")},
			excluded: []string{"plain-a"},
			want:     want{cluster: "plain-b", outcome: outcomeFallback, candidates: []string{}},
		},
		{
			name:     "every fallback cluster excluded: the fallback without the exclusion",
			clusters: []clusterFixture{withoutQueue("kueue-a"), plainCluster("plain-a")},
			excluded: []string{"plain-a"},
			want:     want{cluster: "plain-a", outcome: outcomeFallback, waived: true, candidates: []string{}},
		},
		{
			name:     "an excluded cluster that is not considered changes nothing",
			clusters: []clusterFixture{withQueue("kueue-a", feeding(roomy("cq-a"))), withQueue("kueue-b", feeding(full("cq-b", 1)))},
			excluded: []string{"gone"},
			want:     want{cluster: "kueue-a", tier: tierFitsNow, outcome: outcomeAssigned, candidates: []string{"kueue-a", "kueue-b"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fleet := newTestFleet(tt.clusters...)
			job := framework.BatchRayCluster{RayCluster: testRayCluster(tt.labels, "")}

			d, err := fleet.strategy().SelectExcluding(context.Background(), job, tt.excluded)
			require.NoError(t, err)

			assert.Equal(t, tt.want.cluster, nameOf(d.Cluster), "cluster")
			assert.Equal(t, tt.want.tier, d.Tier, "tier")
			assert.Equal(t, tt.want.outcome, d.Outcome, "outcome")
			assert.Equal(t, tt.want.reason, d.Reason, "reason")
			assert.Equal(t, tt.want.message, d.Message, "message")
			assert.Equal(t, tt.want.waived, d.ExclusionWaived, "exclusion waived")
			assert.Equal(t, tt.excluded, d.Excluded, "excluded clusters")
			if tt.want.candidates != nil {
				assert.Equal(t, tt.want.candidates, candidateNames(d), "candidates")
			}
			assert.Equal(t, testLocalQueue, d.LocalQueue)
			assert.Empty(t, fleet.recorded, "a decision records nothing in flight")
		})
	}

	t.Run("no exclusions decide as Select", func(t *testing.T) {
		fleet := newTestFleet(withQueue("kueue-a", feeding(small("cq-a"))), withQueue("kueue-b", feeding(full("cq-b", 1))))
		job := framework.BatchRayCluster{RayCluster: testRayCluster(nil, "")}

		for _, excluded := range [][]string{nil, {}} {
			d, err := fleet.strategy().SelectExcluding(context.Background(), job, excluded)
			require.NoError(t, err)
			want, err := fleet.strategy().Select(context.Background(), job)
			require.NoError(t, err)

			assert.Equal(t, nameOf(want.Cluster), nameOf(d.Cluster))
			assert.Equal(t, want.Tier, d.Tier)
			assert.Equal(t, want.Outcome, d.Outcome)
			assert.Equal(t, candidateNames(want), candidateNames(d))
			assert.Empty(t, d.Excluded)
			assert.False(t, d.ExclusionWaived)
		}
	})
	t.Run("a job that is not a RayCluster", func(t *testing.T) {
		fleet := newTestFleet(withQueue("kueue-a", feeding(roomy("cq-a"))))
		sj := &v2pb.SparkJob{ObjectMeta: metav1.ObjectMeta{Name: "spark-1", Namespace: "proj-ns"}}

		_, err := fleet.strategy().SelectExcluding(context.Background(), framework.BatchSparkJob{SparkJob: sj}, []string{"kueue-a"})
		require.ErrorContains(t, err, "kueue_aware places RayClusters only")
	})
	t.Run("a decision that fails", func(t *testing.T) {
		fleet := newTestFleet(withQueue("kueue-a", feeding(roomy("cq-a"))))
		fleet.inFlightErr = errors.New("manager cache not started")

		_, err := fleet.strategy().SelectExcluding(context.Background(),
			framework.BatchRayCluster{RayCluster: testRayCluster(nil, "")}, []string{"gone"})
		require.ErrorIs(t, err, fleet.inFlightErr)
	})
	t.Run("a decision without the exclusion that fails", func(t *testing.T) {
		fleet := newTestFleet(withQueue("kueue-a", feeding(roomy("cq-a"))))
		fleet.inFlightErr = errors.New("manager cache not started")

		_, err := fleet.strategy().SelectExcluding(context.Background(),
			framework.BatchRayCluster{RayCluster: testRayCluster(nil, "")}, []string{"kueue-a"})
		require.ErrorIs(t, err, fleet.inFlightErr)
	})
}

func TestDecisionCandidateTier(t *testing.T) {
	fleet := newTestFleet(
		withQueue("kueue-a", feeding(small("cq-a"))),
		unsyncedCluster("kueue-b", true),
		withQueue("kueue-c", feeding(roomy("cq-c"))),
		plainCluster("plain"),
	)

	d, err := fleet.strategy().Select(context.Background(), framework.BatchRayCluster{RayCluster: testRayCluster(nil, "")})
	require.NoError(t, err)

	assert.Equal(t, "1", d.candidateTier("kueue-c"))
	assert.Equal(t, "2", d.candidateTier("kueue-a"), "a candidate that is not eligible ranks in a tier too")
	assert.Equal(t, "3", d.candidateTier("kueue-b"))
	assert.Equal(t, "none", d.candidateTier("plain"), "not a candidate")
	assert.Equal(t, "none", d.candidateTier(""), "no cluster")
}

func TestDecisionLogValues(t *testing.T) {
	fleet := newTestFleet(withQueue("kueue-a", feeding(gpuPool())), withQueue("kueue-b", feeding(cpuPool())), unsyncedCluster("kueue-c", true))
	d, err := fleet.strategy().Select(context.Background(), framework.BatchRayCluster{RayCluster: testRayCluster(nil, "")})
	require.NoError(t, err)

	values := d.logValues()
	require.Zero(t, len(values)%2)
	kv := map[string]any{}
	for i := 0; i < len(values); i += 2 {
		kv[values[i].(string)] = values[i+1]
	}
	assert.Equal(t, testLocalQueue, kv["localQueue"])
	assert.Equal(t, outcomeAssigned, kv["outcome"])
	assert.Equal(t, "1", kv["tier"])
	assert.Equal(t, "kueue-b", kv["chosenCluster"])
	assert.Equal(t, "{cpu: 16, memory: 64Gi}", kv["ask"])
	assert.Equal(t, "cpu", kv["bindingResource"])
	assert.InDelta(t, 0.1875, kv["slack"], 1e-9)
	assert.InDelta(t, -0.1875, kv["deficit"], 1e-9)
	assert.Equal(t, []string{
		"kueue-b: tier 1, ClusterQueue cpu-pool, slack 0.188, deficit -0.188 on cpu, pending 0",
		"kueue-a: tier 1, ClusterQueue gpu-pool, slack 0.062, deficit -0.062 on cpu, pending 0",
		"kueue-c: tier 3, queues not synced",
	}, kv["candidates"])
	assert.NotContains(t, kv, "reason")
	assert.NotContains(t, kv, "excludedClusters")

	replaced := Decision{
		Cluster: kueueCluster("kueue-a"), LocalQueue: testLocalQueue, Outcome: outcomeAssigned, Tier: tierFitsNow,
		Excluded: []string{"kueue-b"}, ExclusionWaived: true,
	}
	assert.Equal(t, []any{
		"localQueue", testLocalQueue, "outcome", outcomeAssigned, "tier", "1", "chosenCluster", "kueue-a",
		"excludedClusters", []string{"kueue-b"}, "exclusionWaived", true, "candidates", []string{},
	}, replaced.logValues())

	waiting := Decision{
		LocalQueue: testLocalQueue, Outcome: outcomeQuotaExceeded, Reason: constants.KueueQuotaExceeded, Message: "over quota",
	}
	values = waiting.logValues()
	assert.Equal(t, []any{
		"localQueue", testLocalQueue, "outcome", outcomeQuotaExceeded, "tier", "none",
		"reason", constants.KueueQuotaExceeded, "message", "over quota", "candidates", []string{},
	}, values)
}
