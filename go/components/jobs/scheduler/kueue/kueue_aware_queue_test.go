package kueue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/framework"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/framework/frameworkmocks"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// newKueueAwareTestQueue returns a Kueue backend over the fleet that places
// RayClusters with the kueue_aware strategy or, in shadow mode, assigns the
// assignment strategy's choice and reports the kueue_aware decision beside it.
func newKueueAwareTestQueue(t *testing.T, fleet *testFleet, strategy framework.AssignmentStrategy, shadow bool,
	log logr.Logger, objs ...client.Object,
) (*KueueJobQueue, *countingHandler) {
	RegisterDecisionMetrics()
	handler := newCountingHandler(t, objs...)
	q := NewKueueJobQueue(Params{
		Handler:            handler,
		AssignmentStrategy: strategy,
		ClusterCache:       fleet,
		LocalQueues:        fleet,
		Config:             kueueConfig(),
		Scope:              tally.NewTestScope("test", nil),
		Logger:             log,
	}, WithKueueAwareStrategy(fleet.strategy(), shadow))
	return q, handler
}

// decisionCount reads the decision counter from the controller-runtime
// registry.
func decisionCount(t *testing.T, strategy, tier, outcome, actualTier string) float64 {
	t.Helper()
	want := map[string]string{"strategy": strategy, "tier": tier, "outcome": outcome, "actual_tier": actualTier}
	families, err := metrics.Registry.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() != "michelangelo_kueue_assignment_decisions_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			got := map[string]string{}
			for _, label := range metric.GetLabel() {
				got[label.GetName()] = label.GetValue()
			}
			matched := true
			for name, value := range want {
				matched = matched && got[name] == value
			}
			if matched {
				return metric.GetCounter().GetValue()
			}
		}
	}
	return 0
}

type logLines struct {
	mu    sync.Mutex
	lines []string
}

func (l *logLines) add(_, args string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, args)
}

// has reports whether a line contains all of substrings.
func (l *logLines) has(substrings ...string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, line := range l.lines {
		matched := true
		for _, s := range substrings {
			matched = matched && strings.Contains(line, s)
		}
		if matched {
			return true
		}
	}
	return false
}

// assertScheduledInTier asserts a recorded kueue_aware placement: the
// assignment, and Scheduled=True/ClusterMatched naming the cluster and the
// tier it ranked in.
func assertScheduledInTier(t *testing.T, status *v2pb.RayClusterStatus, clusterName string, tier Tier) {
	t.Helper()
	assert.Equal(t, clusterName, status.Assignment.GetCluster())
	cond := findCondition(status.StatusConditions, constants.ScheduledCondition)
	require.NotNil(t, cond)
	assert.Equal(t, apipb.CONDITION_STATUS_TRUE, cond.Status)
	assert.Equal(t, constants.ClusterMatched, cond.Reason)
	assert.Equal(t, fmt.Sprintf("cluster %s, tier %d", clusterName, tier), cond.Message)
}

func TestRegisterDecisionMetrics(t *testing.T) {
	RegisterDecisionMetrics()
	// Registering again is a no-op.
	require.NotPanics(t, RegisterDecisionMetrics)

	before := decisionCount(t, maconfig.AssignmentStrategyKueueAwareShadow, "3", outcomeAssigned, "none")
	countDecision(maconfig.AssignmentStrategyKueueAwareShadow, Decision{Tier: tierUnsynced, Outcome: outcomeAssigned}, "none")
	assert.Equal(t, before+1, decisionCount(t, maconfig.AssignmentStrategyKueueAwareShadow, "3", outcomeAssigned, "none"))
}

func TestEnqueueKueueAwareRecordsTheTopRankedCandidate(t *testing.T) {
	g := gomock.NewController(t)
	rc := testRayCluster(nil, "")
	fleet := newTestFleet(withQueue("kueue-a", feeding(gpuPool())), withQueue("kueue-b", feeding(cpuPool())))
	strategy := frameworkmocks.NewMockAssignmentEngine(g) // no Select expectation: must not be called

	q, handler := newKueueAwareTestQueue(t, fleet, strategy, false, logr.Discard(), rc)
	before := decisionCount(t, maconfig.AssignmentStrategyKueueAware, "1", outcomeAssigned, "")
	require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))

	// One status write carries the decision and its tier. The decision read
	// the LocalQueues in the watch cache: nothing is read again.
	writes := handler.statusWrites()
	require.Len(t, writes, 1)
	written := writes[0].(*v2pb.RayCluster)
	assertScheduledInTier(t, &written.Status, "kueue-b", tierFitsNow)
	enqueued := findCondition(written.Status.StatusConditions, constants.EnqueuedCondition)
	require.NotNil(t, enqueued)
	assert.Equal(t, apipb.CONDITION_STATUS_TRUE, enqueued.Status)
	assert.Empty(t, fleet.gets)

	// The recorded assignment is in flight to kueue-b from then on.
	require.Len(t, fleet.recorded, 1)
	recorded := fleet.recorded[0]
	assert.Equal(t, "kueue-b", recorded.cluster)
	assert.Equal(t, testNow, recorded.at)
	assert.Equal(t, "job-1", recorded.job.GetName())
	assert.Equal(t, "kueue-b", recorded.job.Status.Assignment.GetCluster())

	assert.Equal(t, before+1, decisionCount(t, maconfig.AssignmentStrategyKueueAware, "1", outcomeAssigned, ""))
}

// TestEnqueueKueueAwareBurstSpreadsByInFlight places two jobs back to back:
// the first one's assignment is in flight to kueue-b while kueue-b's Kueue
// status does not show it yet, so the second one goes to kueue-a.
func TestEnqueueKueueAwareBurstSpreadsByInFlight(t *testing.T) {
	g := gomock.NewController(t)
	first := testRayCluster(nil, "")
	second := testRayCluster(nil, "")
	second.Name = "job-2"
	fleet := newTestFleet(withQueue("kueue-a", feeding(gpuPool())), withQueue("kueue-b", feeding(cpuPool())))

	q, handler := newKueueAwareTestQueue(t, fleet, frameworkmocks.NewMockAssignmentEngine(g), false, logr.Discard(), first, second)
	require.NoError(t, q.Enqueue(context.Background(), schedulableJob(first)))
	require.NoError(t, q.Enqueue(context.Background(), schedulableJob(second)))

	assertScheduledInTier(t, &readRayCluster(t, handler, "proj-ns", "job-1").Status, "kueue-b", tierFitsNow)
	// kueue-b has 40 - 16 cpu and 160Gi - 64Gi memory free after the first
	// job, a slack of 0.0625 as kueue-a's, and the tie goes to kueue-a by name
	assertScheduledInTier(t, &readRayCluster(t, handler, "proj-ns", "job-2").Status, "kueue-a", tierFitsNow)
}

func TestEnqueueKueueAwareWaitsWithTheDecisionsReason(t *testing.T) {
	tests := []struct {
		name     string
		clusters []clusterFixture
		reason   string
		outcome  string
	}{
		{
			name:     "the LocalQueue is on no cluster",
			clusters: []clusterFixture{withoutQueue("kueue-a")},
			reason:   constants.KueueQueueNotFound,
			outcome:  outcomeQueueNotFound,
		},
		{
			name:     "every cluster that carries the LocalQueue is not Ready",
			clusters: []clusterFixture{withQueue("kueue-a", feeding(roomy("cq-a"))).down()},
			reason:   constants.KueueClusterNotReady,
			outcome:  outcomeClusterNotReady,
		},
		{
			name:     "the ask exceeds every candidate's nominal quota",
			clusters: []clusterFixture{withQueue("kueue-a", feeding(small("cq-a")))},
			reason:   constants.KueueQuotaExceeded,
			outcome:  outcomeQuotaExceeded,
		},
		{
			name:     "a Ready cluster's LocalQueue cannot be read",
			clusters: []clusterFixture{{name: "kueue-a", kueue: true, unsynced: true, getErr: errors.New("connection refused")}},
			reason:   constants.KueueQueueCheckFailed,
			outcome:  outcomeCheckFailed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomock.NewController(t)
			rc := testRayCluster(nil, "")
			fleet := newTestFleet(tt.clusters...)
			q, handler := newKueueAwareTestQueue(t, fleet, frameworkmocks.NewMockAssignmentEngine(g), false, logr.Discard(), rc)
			before := decisionCount(t, maconfig.AssignmentStrategyKueueAware, "none", tt.outcome, "")

			// The job waits: every attempt returns an error, and Enqueued=False
			// with the reason is written once.
			for range 2 {
				require.ErrorContains(t, q.Enqueue(context.Background(), schedulableJob(rc)), tt.reason)
			}
			require.Len(t, handler.statusWrites(), 1)
			got := readRayCluster(t, handler, rc.Namespace, rc.Name)
			assertNotScheduled(t, &got.Status)
			assert.Nil(t, got.Status.Assignment)
			enqueued := findCondition(got.Status.StatusConditions, constants.EnqueuedCondition)
			require.NotNil(t, enqueued)
			assert.Equal(t, apipb.CONDITION_STATUS_FALSE, enqueued.Status)
			assert.Equal(t, tt.reason, enqueued.Reason)
			assert.Contains(t, enqueued.Message, `"ma-proj-ns"`, "the message names the queue")

			assert.Empty(t, fleet.recorded)
			assert.Equal(t, before+2, decisionCount(t, maconfig.AssignmentStrategyKueueAware, "none", tt.outcome, ""))
		})
	}
}

func TestEnqueueKueueAwareWaitEndsWithAnAssignment(t *testing.T) {
	g := gomock.NewController(t)
	rc := testRayCluster(nil, "")
	fleet := newTestFleet(withQueue("kueue-a", feeding(small("cq-a"))))
	q, handler := newKueueAwareTestQueue(t, fleet, frameworkmocks.NewMockAssignmentEngine(g), false, logr.Discard(), rc)

	require.ErrorContains(t, q.Enqueue(context.Background(), schedulableJob(rc)), constants.KueueQuotaExceeded)
	require.Len(t, handler.statusWrites(), 1)

	// The ClusterQueue's nominal quota grows: the next attempt assigns the job.
	fleet.fixtures["kueue-a"].queues[testLocalQueue] = feeding(roomy("cq-a"))
	require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))
	require.Len(t, handler.statusWrites(), 2)
	got := readRayCluster(t, handler, rc.Namespace, rc.Name)
	assertScheduledInTier(t, &got.Status, "kueue-a", tierFitsNow)
	enqueued := findCondition(got.Status.StatusConditions, constants.EnqueuedCondition)
	require.NotNil(t, enqueued)
	assert.Equal(t, apipb.CONDITION_STATUS_TRUE, enqueued.Status)
	assert.Equal(t, constants.AddedToSchedulerQueue, enqueued.Reason)
}

func TestEnqueueKueueAwareAssignsClustersWithoutKueueAsToday(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		clusters []clusterFixture
		outcome  string
	}{
		{
			name:     "not onboarded: cluster_only over the clusters without Kueue",
			clusters: []clusterFixture{withoutQueue("kueue-a"), plainCluster("plain")},
			outcome:  outcomeFallback,
		},
		{
			name:     "pinned to a cluster without Kueue",
			labels:   pinnedTo("plain"),
			clusters: []clusterFixture{withQueue("kueue-a", feeding(roomy("cq-a"))), plainCluster("plain")},
			outcome:  outcomePinned,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomock.NewController(t)
			rc := testRayCluster(tt.labels, "")
			fleet := newTestFleet(tt.clusters...)
			q, handler := newKueueAwareTestQueue(t, fleet, frameworkmocks.NewMockAssignmentEngine(g), false, logr.Discard(), rc)
			before := decisionCount(t, maconfig.AssignmentStrategyKueueAware, "none", tt.outcome, "")

			require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))

			// no tier, and no LocalQueue to read on a cluster without Kueue
			assertScheduledTo(t, &readRayCluster(t, handler, rc.Namespace, rc.Name).Status, "plain")
			assert.Empty(t, fleet.gets)
			require.Len(t, fleet.recorded, 1)
			assert.Equal(t, "plain", fleet.recorded[0].cluster)
			assert.Equal(t, before+1, decisionCount(t, maconfig.AssignmentStrategyKueueAware, "none", tt.outcome, ""))
		})
	}
}

func TestEnqueueKueueAwareUsesARecordedAssignment(t *testing.T) {
	g := gomock.NewController(t)
	// An assignment recorded before decisions were persisted here.
	rc := testRayCluster(nil, "kueue-a")
	fleet := newTestFleet(withQueue("kueue-a", feeding(full("cq-a", 0))), withQueue("kueue-b", feeding(roomy("cq-b"))))
	q, handler := newKueueAwareTestQueue(t, fleet, frameworkmocks.NewMockAssignmentEngine(g), false, logr.Discard(), rc)

	require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))

	// validated as in Phase 0, and recorded without a tier
	assertScheduledTo(t, &readRayCluster(t, handler, rc.Namespace, rc.Name).Status, "kueue-a")
	assert.Equal(t, readsOf("kueue-a"), fleet.gets)
	assert.Zero(t, fleet.lookups, "no kueue_aware decision")
	require.Len(t, fleet.recorded, 1)
	assert.Equal(t, "kueue-a", fleet.recorded[0].cluster)
}

func TestEnqueueKueueAwarePlacesSparkJobsByTheAssignmentStrategy(t *testing.T) {
	for _, shadow := range []bool{false, true} {
		t.Run(fmt.Sprintf("shadow=%t", shadow), func(t *testing.T) {
			g := gomock.NewController(t)
			sj := &v2pb.SparkJob{ObjectMeta: metav1.ObjectMeta{Name: "spark-1", Namespace: "proj-ns"}}
			fleet := newTestFleet(withQueue("kueue-a", feeding(full("cq-a", 0))), withQueue("kueue-b", feeding(roomy("cq-b"))))
			strategy := frameworkmocks.NewMockAssignmentEngine(g)
			strategy.EXPECT().Select(gomock.Any(), gomock.Any()).
				Return(&v2pb.AssignmentInfo{Cluster: "kueue-a"}, true, "", nil)
			q, handler := newKueueAwareTestQueue(t, fleet, strategy, shadow, logr.Discard(), sj)

			require.NoError(t, q.Enqueue(context.Background(), framework.BatchSparkJob{SparkJob: sj}))

			got := &v2pb.SparkJob{}
			require.NoError(t, handler.Handler.Get(context.Background(), sj.Namespace, sj.Name, &metav1.GetOptions{}, got))
			assert.Equal(t, "kueue-a", got.Status.Assignment.GetCluster())
			cond := findCondition(got.Status.StatusConditions, constants.ScheduledCondition)
			require.NotNil(t, cond)
			assert.Equal(t, "cluster kueue-a", cond.Message)
			assert.Equal(t, readsOf("kueue-a"), fleet.gets, "validated as in Phase 0")
			assert.Zero(t, fleet.lookups, "no kueue_aware decision")
			assert.Empty(t, fleet.recorded, "only RayClusters count as in flight")
		})
	}
}

func TestEnqueueKueueAwareSelectErrorWritesNothing(t *testing.T) {
	g := gomock.NewController(t)
	rc := testRayCluster(nil, "")
	fleet := newTestFleet(withQueue("kueue-a", feeding(roomy("cq-a"))))
	fleet.inFlightErr = errors.New("manager cache not started")
	q, handler := newKueueAwareTestQueue(t, fleet, frameworkmocks.NewMockAssignmentEngine(g), false, logr.Discard(), rc)

	require.ErrorIs(t, q.Enqueue(context.Background(), schedulableJob(rc)), fleet.inFlightErr)
	assert.Empty(t, handler.statusWrites())
	assert.Empty(t, fleet.recorded)
}

func TestEnqueueKueueAwareStaleReadRecordsNothingInFlight(t *testing.T) {
	g := gomock.NewController(t)
	// Stored: another decision already placed the job on kueue-a.
	rc := testRayCluster(nil, "kueue-a")
	rc.Status.StatusConditions = []*apipb.Condition{{
		Type:    constants.ScheduledCondition,
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  constants.ClusterMatched,
		Message: "cluster kueue-a, tier 2",
	}}
	fleet := newTestFleet(withQueue("kueue-a", feeding(full("cq-a", 0))), withQueue("kueue-b", feeding(roomy("cq-b"))))
	q, handler := newKueueAwareTestQueue(t, fleet, frameworkmocks.NewMockAssignmentEngine(g), false, logr.Discard(), rc)
	// The decision's first read is stale: it predates the stored decision.
	handler.stale = testRayCluster(nil, "")

	require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))

	got := readRayCluster(t, handler, rc.Namespace, rc.Name)
	assert.Equal(t, "kueue-a", got.Status.Assignment.GetCluster())
	assert.Empty(t, fleet.recorded, "nothing was recorded, so nothing is in flight")
}

func TestEnqueueKueueAwareShadow(t *testing.T) {
	shadowFleet := func() *testFleet {
		return newTestFleet(
			withQueue("kueue-a", feeding(full("cq-a", 0))),
			withQueue("kueue-b", feeding(roomy("cq-b"))),
			plainCluster("plain"),
		)
	}
	tests := []struct {
		name  string
		fleet *testFleet
		// pick is the assignment strategy's choice; empty for none.
		pick string
		// chosen, tier and outcome are the kueue_aware decision's, and
		// actualTier is pick's tier among its candidates.
		chosen     string
		tier       string
		outcome    string
		actualTier string
		// reads are the validation's LocalQueue reads.
		reads []string
	}{
		{
			name:  "the assigned cluster ranks lower",
			fleet: shadowFleet(), pick: "kueue-a",
			chosen: "kueue-b", tier: "1", outcome: outcomeAssigned, actualTier: "2",
			reads: readsOf("kueue-a"),
		},
		{
			name:  "the assigned cluster is the top-ranked candidate",
			fleet: shadowFleet(), pick: "kueue-b",
			chosen: "kueue-b", tier: "1", outcome: outcomeAssigned, actualTier: "1",
			reads: readsOf("kueue-b"),
		},
		{
			name:  "the assigned cluster is not a candidate",
			fleet: shadowFleet(), pick: "plain",
			chosen: "kueue-b", tier: "1", outcome: outcomeAssigned, actualTier: "none",
		},
		{
			name:  "kueue_aware would have the job wait",
			fleet: newTestFleet(withQueue("kueue-a", feeding(small("cq-a")))), pick: "kueue-a",
			tier: "none", outcome: outcomeQuotaExceeded, actualTier: "2",
			reads: readsOf("kueue-a"),
		},
		{
			name:   "the assignment strategy finds no cluster",
			fleet:  shadowFleet(),
			chosen: "kueue-b", tier: "1", outcome: outcomeAssigned, actualTier: "none",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomock.NewController(t)
			rc := testRayCluster(nil, "")
			strategy := frameworkmocks.NewMockAssignmentEngine(g)
			if tt.pick != "" {
				strategy.EXPECT().Select(gomock.Any(), gomock.Any()).Return(&v2pb.AssignmentInfo{Cluster: tt.pick}, true, "", nil)
			} else {
				strategy.EXPECT().Select(gomock.Any(), gomock.Any()).Return(nil, false, "no clusters", nil)
			}
			logs := &logLines{}
			q, handler := newKueueAwareTestQueue(t, tt.fleet, strategy, true, funcr.New(logs.add, funcr.Options{}), rc)
			before := decisionCount(t, maconfig.AssignmentStrategyKueueAwareShadow, tt.tier, tt.outcome, tt.actualTier)

			require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))

			// The assignment strategy's choice is assigned, validated as in
			// Phase 0, with no tier.
			got := readRayCluster(t, handler, rc.Namespace, rc.Name)
			if tt.pick == "" {
				assertNotScheduled(t, &got.Status)
				assert.Empty(t, tt.fleet.recorded)
			} else {
				assertScheduledTo(t, &got.Status, tt.pick)
				require.Len(t, tt.fleet.recorded, 1)
				assert.Equal(t, tt.pick, tt.fleet.recorded[0].cluster)
			}
			assert.Equal(t, tt.reads, tt.fleet.gets)

			// The kueue_aware decision is counted and logged beside it.
			assert.Equal(t, before+1, decisionCount(t, maconfig.AssignmentStrategyKueueAwareShadow, tt.tier, tt.outcome, tt.actualTier))
			logged := []string{
				`"msg"="Kueue-aware placement decision (shadow)"`,
				`"outcome"="` + tt.outcome + `"`,
				`"clusterOnlyCluster"="` + tt.pick + `"`,
				`"actualTier"="` + tt.actualTier + `"`,
			}
			if tt.chosen != "" {
				logged = append(logged, `"chosenCluster"="`+tt.chosen+`"`)
			}
			assert.True(t, logs.has(logged...), "no shadow decision log line with %q in %q", logged, logs.lines)
		})
	}
}

func TestEnqueueKueueAwareShadowDecisionFailureAssignsAnyway(t *testing.T) {
	g := gomock.NewController(t)
	rc := testRayCluster(nil, "")
	fleet := newTestFleet(withQueue("kueue-a", feeding(roomy("cq-a"))))
	fleet.inFlightErr = errors.New("manager cache not started")
	strategy := frameworkmocks.NewMockAssignmentEngine(g)
	strategy.EXPECT().Select(gomock.Any(), gomock.Any()).Return(&v2pb.AssignmentInfo{Cluster: "kueue-a"}, true, "", nil)
	logs := &logLines{}
	q, handler := newKueueAwareTestQueue(t, fleet, strategy, true, funcr.New(logs.add, funcr.Options{}), rc)

	require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))

	assertScheduledTo(t, &readRayCluster(t, handler, rc.Namespace, rc.Name).Status, "kueue-a")
	assert.True(t, logs.has(`"msg"="Kueue-aware shadow decision failed"`, "manager cache not started"), "%q", logs.lines)
}

// TestEnqueueKueueAwareExcludesEarlierTargets places RayClusters re-placed off
// earlier targets, which their Launched condition records: the decision is
// made without them unless that leaves no candidate (RFC 0f).
func TestEnqueueKueueAwareExcludesEarlierTargets(t *testing.T) {
	tests := []struct {
		name     string
		excluded []string
		want     string
		tier     Tier
		// logged is what the decision's log line says of the exclusion; empty
		// when it says nothing of it.
		logged []string
	}{
		{
			name:     "the excluded top-ranked candidate is skipped for the next",
			excluded: []string{"kueue-b"},
			want:     "kueue-a", tier: tierQueueDepth,
			logged: []string{`"excludedClusters"=["kueue-b"]`, `"exclusionWaived"=false`},
		},
		{
			name:     "with every candidate excluded, the decision is made without the exclusion",
			excluded: []string{"kueue-a", "kueue-b"},
			want:     "kueue-b", tier: tierFitsNow,
			logged: []string{`"excludedClusters"=["kueue-a" "kueue-b"]`, `"exclusionWaived"=true`},
		},
		{
			name: "a job without exclusions is unchanged",
			want: "kueue-b", tier: tierFitsNow,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomock.NewController(t)
			rc := replacedRayCluster(t, nil, tt.excluded...)
			fleet := newTestFleet(withQueue("kueue-a", feeding(full("cq-a", 0))), withQueue("kueue-b", feeding(roomy("cq-b"))))
			strategy := frameworkmocks.NewMockAssignmentEngine(g) // no Select expectation: must not be called
			logs := &logLines{}

			q, handler := newKueueAwareTestQueue(t, fleet, strategy, false, funcr.New(logs.add, funcr.Options{}), rc)
			tier := tierLabel(tt.tier)
			before := decisionCount(t, maconfig.AssignmentStrategyKueueAware, tier, outcomeAssigned, "")
			require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))

			assertScheduledInTier(t, &readRayCluster(t, handler, rc.Namespace, rc.Name).Status, tt.want, tt.tier)
			assert.Equal(t, before+1, decisionCount(t, maconfig.AssignmentStrategyKueueAware, tier, outcomeAssigned, ""))
			logged := append([]string{`"msg"="Kueue-aware placement decision"`, `"chosenCluster"="` + tt.want + `"`}, tt.logged...)
			assert.True(t, logs.has(logged...), "no decision log line with %q in %q", logged, logs.lines)
			if len(tt.logged) == 0 {
				assert.False(t, logs.has("excludedClusters"), "%q", logs.lines)
			}
		})
	}
}

// TestEnqueueKueueAwareShadowExcludesEarlierTargets reports the kueue_aware
// decision beside cluster_only's pick for RayClusters re-placed off earlier
// targets: both sides decide without them unless that leaves no candidate.
func TestEnqueueKueueAwareShadowExcludesEarlierTargets(t *testing.T) {
	tests := []struct {
		name     string
		excluded []string
		// pick is cluster_only's choice, which is assigned.
		pick string
		// chosen, tier and actualTier are the kueue_aware decision's, which
		// assigns.
		chosen     string
		tier       string
		actualTier string
		waived     bool
		// reads are the validation's LocalQueue reads.
		reads []string
	}{
		{
			name:     "an excluded cluster is skipped on both sides",
			excluded: []string{"kueue-a"},
			pick:     "kueue-b",
			chosen:   "kueue-b", tier: "2", actualTier: "2",
			reads: readsOf("kueue-b"),
		},
		{
			name:     "with every cluster excluded, both sides decide without the exclusion",
			excluded: []string{"kueue-a", "kueue-b", "plain"},
			pick:     "kueue-a",
			chosen:   "kueue-a", tier: "1", actualTier: "1", waived: true,
			reads: readsOf("kueue-a"),
		},
		{
			name:     "with the Kueue-managed clusters excluded, kueue_aware does not fall back",
			excluded: []string{"kueue-a", "kueue-b"},
			pick:     "plain",
			chosen:   "kueue-a", tier: "1", actualTier: "none", waived: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := replacedRayCluster(t, nil, tt.excluded...)
			fleet := newTestFleet(
				withQueue("kueue-a", feeding(roomy("cq-a"))),
				withQueue("kueue-b", feeding(full("cq-b", 0))),
				plainCluster("plain"),
			)
			strategy := framework.NewClusterOnlyAssignmentStrategy(fleet, logr.Discard())
			logs := &logLines{}
			q, handler := newKueueAwareTestQueue(t, fleet, strategy, true, funcr.New(logs.add, funcr.Options{}), rc)
			before := decisionCount(t, maconfig.AssignmentStrategyKueueAwareShadow, tt.tier, outcomeAssigned, tt.actualTier)

			require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))

			assertScheduledTo(t, &readRayCluster(t, handler, rc.Namespace, rc.Name).Status, tt.pick)
			assert.Equal(t, tt.reads, fleet.gets)
			assert.Equal(t, before+1, decisionCount(t, maconfig.AssignmentStrategyKueueAwareShadow, tt.tier, outcomeAssigned, tt.actualTier))
			logged := []string{
				`"msg"="Kueue-aware placement decision (shadow)"`,
				`"chosenCluster"="` + tt.chosen + `"`,
				`"excludedClusters"=["` + strings.Join(tt.excluded, `" "`) + `"]`,
				fmt.Sprintf(`"exclusionWaived"=%t`, tt.waived),
				`"clusterOnlyCluster"="` + tt.pick + `"`,
				`"actualTier"="` + tt.actualTier + `"`,
			}
			assert.True(t, logs.has(logged...), "no shadow decision log line with %q in %q", logged, logs.lines)
		})
	}
}
