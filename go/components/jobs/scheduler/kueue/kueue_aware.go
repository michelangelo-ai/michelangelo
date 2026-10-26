package kueue

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"

	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/k8sengine"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/framework"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/kueue/state"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Outcomes of a kueue_aware decision, as the decision metric labels them.
const (
	// outcomeAssigned: assigned to the top-ranked eligible candidate.
	outcomeAssigned = "assigned"
	// outcomePinned: assigned, as today, to the cluster the job's affinity
	// pins, which is not Kueue-managed.
	outcomePinned = "pinned"
	// outcomeFallback: the job's LocalQueue is on no considered
	// Kueue-managed cluster, so it is assigned by cluster_only over the Ready
	// clusters that are not Kueue-managed.
	outcomeFallback = "fallback"
	// outcomeQueueNotFound: waits with KueueQueueNotFound.
	outcomeQueueNotFound = "queue_not_found"
	// outcomeClusterNotReady: waits with KueueClusterNotReady.
	outcomeClusterNotReady = "cluster_not_ready"
	// outcomeQuotaExceeded: waits with KueueQuotaExceeded.
	outcomeQuotaExceeded = "quota_exceeded"
	// outcomeCheckFailed: waits with KueueQueueCheckFailed, because a Ready
	// cluster's LocalQueue could not be read.
	outcomeCheckFailed = "check_failed"
)

// KueueAwareParams carries the dependencies of KueueAwareStrategy.
type KueueAwareParams struct {
	// ClusterCache lists the registered clusters and their readiness.
	ClusterCache cluster.RegisteredClustersCache
	// LocalQueues reads a LocalQueue directly, for clusters whose state is
	// not synced.
	LocalQueues LocalQueues
	// State is the Kueue state of the Kueue-managed clusters.
	State state.State
	// Demand is what jobs ask of the clusters' quotas, and what is in flight.
	Demand state.Demand
	// Config is the jobs.scheduler configuration, which resolves a job's
	// LocalQueue.
	Config maconfig.SchedulerConfig
	// Logger logs.
	Logger logr.Logger
	// Now is the clock; time.Now when nil.
	Now func() time.Time
}

// KueueAwareStrategy is the kueue_aware assignment strategy (RFC-20260731
// Phase 2): it places a RayCluster on the Kueue-managed cluster where it is
// likely admitted soonest, by the clusters' Kueue state, and says why a job
// that cannot be placed yet waits. Kueue remains the only queue: placement
// only picks which cluster's queue.
type KueueAwareStrategy struct {
	clusters cluster.RegisteredClustersCache
	queues   LocalQueues
	state    state.State
	demand   state.Demand
	cfg      maconfig.SchedulerConfig
	log      logr.Logger
	now      func() time.Time
}

// NewKueueAwareStrategy constructs the kueue_aware strategy.
func NewKueueAwareStrategy(p KueueAwareParams) *KueueAwareStrategy {
	now := p.Now
	if now == nil {
		now = time.Now
	}
	return &KueueAwareStrategy{
		clusters: p.ClusterCache,
		queues:   p.LocalQueues,
		state:    p.State,
		demand:   p.Demand,
		cfg:      p.Config,
		log:      p.Logger.WithValues(constants.Component, _controllerName),
		now:      now,
	}
}

// Decision is a kueue_aware placement decision.
type Decision struct {
	// Cluster is the cluster the job is assigned to; nil when it waits.
	Cluster *v2pb.Cluster
	// Tier is the tier Cluster ranked in; zero when the job waits, or goes to
	// a cluster that is not Kueue-managed.
	Tier Tier
	// Outcome is what the decision did, as the decision metric labels it.
	Outcome string
	// Reason is the Enqueued=False reason the job waits with:
	// KueueQueueNotFound, KueueClusterNotReady or KueueQuotaExceeded, or
	// KueueQueueCheckFailed when a Ready cluster's LocalQueue could not be
	// read. Empty when Cluster is set.
	Reason string
	// Message explains Reason, naming the LocalQueue.
	Message string
	// LocalQueue is the job's LocalQueue; empty when the job has no project.
	LocalQueue string
	// Excluded are the clusters a re-placed job was assigned to before, which
	// the decision left out of the considered clusters; empty for a job that
	// was not re-placed.
	Excluded []string
	// ExclusionWaived is whether leaving Excluded out left no candidate, so
	// that the decision was made without leaving them out.
	ExclusionWaived bool

	// candidates are the candidates, eligible ones first in rank order.
	candidates []candidate
}

// candidateTier returns the tier the named cluster ranks in among the
// decision's candidates, as the decision metric labels it: "none" when it is
// not a candidate.
func (d Decision) candidateTier(name string) string {
	for _, c := range d.candidates {
		if c.cluster.GetName() == name {
			return tierLabel(c.tier)
		}
	}
	return tierLabel(tierNone)
}

// logValues returns the decision as key-value pairs for the decision log:
// the outcome, the chosen candidate's binding resource, slack and deficit,
// and every candidate.
func (d Decision) logValues() []any {
	kv := []any{"localQueue", d.LocalQueue, "outcome", d.Outcome, "tier", tierLabel(d.Tier)}
	if d.Cluster != nil {
		kv = append(kv, "chosenCluster", d.Cluster.GetName())
	}
	if d.Reason != "" {
		kv = append(kv, "reason", d.Reason, "message", d.Message)
	}
	if len(d.Excluded) > 0 {
		kv = append(kv, "excludedClusters", d.Excluded, "exclusionWaived", d.ExclusionWaived)
	}
	if d.Cluster != nil && len(d.candidates) > 0 && d.candidates[0].cluster.GetName() == d.Cluster.GetName() {
		if chosen := d.candidates[0]; chosen.scorable {
			kv = append(kv,
				"ask", formatResources(chosen.ask),
				"bindingResource", string(chosen.score.binding),
				"slack", chosen.score.slack,
				"deficit", chosen.score.deficit())
		}
	}
	summaries := make([]string, 0, len(d.candidates))
	for _, c := range d.candidates {
		summaries = append(summaries, c.summary())
	}
	return append(kv, "candidates", summaries)
}

// tierLabel returns the tier as the decision metric labels it.
func tierLabel(t Tier) string {
	if t == tierNone {
		return "none"
	}
	return strconv.Itoa(int(t))
}

// RecordAssignment adds the job's recorded assignment to the cluster to the
// Kueue state's in-flight demand, until the manager cache shows it.
func (s *KueueAwareStrategy) RecordAssignment(job *v2pb.RayCluster, clusterName string) {
	s.demand.RecordAssignment(job, clusterName, s.now())
}

// Select decides the job's placement by RFC-20260731 Phase 2's decision
// table:
//  1. Pin: a job whose affinity names a registered cluster considers only
//     that cluster. A pin skips ranking, not the checks: a Kueue-managed pin
//     must pass steps 2 and 3, and any other pin is assigned as today.
//  2. Candidates: the Ready, Kueue-managed clusters that carry the job's
//     LocalQueue, read from the watch cache, or directly while the cluster's
//     cache has not synced.
//  3. Eligible: a scorable candidate whose nominal quota covers the ask of
//     every requested resource, an uncovered resource counting as zero.
//     Unscorable candidates stay.
//  4. Rank: by tier, then within the tier, then by cluster name.
//  5. Assign the top-ranked candidate.
//
// With no eligible candidate the job waits with a reason, except an unpinned
// job whose LocalQueue is on no considered Kueue-managed cluster: it falls
// back to cluster_only over the Ready clusters that are not Kueue-managed.
// Select places RayClusters only: other jobs have no ask until RFC Phase 3.
func (s *KueueAwareStrategy) Select(ctx context.Context, job framework.BatchJob) (Decision, error) {
	return s.selectWithout(ctx, job, nil)
}

// SelectExcluding decides the placement of a job re-placed off the excluded
// clusters (RFC 0f): as Select does, with the excluded clusters left out of
// the considered clusters, the fallback's included. When that leaves no
// candidate, the decision without leaving them out is used, so the job waits
// as it would, or goes back to a cluster it left. No candidate is left when
// the decision assigns no cluster, or when it falls back but the decision
// without leaving them out does not: leaving clusters out does not make a
// project with a LocalQueue fall back. A pinned job considers only its pin,
// so leaving the pin out leaves nothing and the pin is kept.
func (s *KueueAwareStrategy) SelectExcluding(ctx context.Context, job framework.BatchJob, excluded []string) (Decision, error) {
	if len(excluded) == 0 {
		return s.Select(ctx, job)
	}
	d, err := s.selectWithout(ctx, job, excluded)
	if err != nil {
		return Decision{}, err
	}
	if d.Cluster == nil || d.Outcome == outcomeFallback {
		unexcluded, err := s.selectWithout(ctx, job, nil)
		if err != nil {
			return Decision{}, err
		}
		if d.Cluster == nil || unexcluded.Outcome != outcomeFallback {
			d = unexcluded
			d.ExclusionWaived = true
		}
	}
	d.Excluded = excluded
	return d, nil
}

// selectWithout decides the job's placement as Select does, with the excluded
// clusters left out of the considered clusters.
func (s *KueueAwareStrategy) selectWithout(ctx context.Context, job framework.BatchJob, excluded []string) (Decision, error) {
	rc, ok := job.GetObject().(*v2pb.RayCluster)
	if !ok {
		return Decision{}, fmt.Errorf("kueue_aware places RayClusters only, not %T", job.GetObject())
	}
	var localQueue string
	if project := utils.ProjectNameForJob(job.GetLabels(), job.GetNamespace(), s.cfg.Kueue.TrustProjectLabel); project != "" {
		localQueue = utils.ResolveLocalQueueName(s.cfg.Kueue, project)
	}

	pinned := s.pinnedCluster(job)
	// a pin left out leaves nothing to consider
	if pinned != nil && !utils.IsKueueManaged(pinned) && !slices.Contains(excluded, pinned.GetName()) {
		return Decision{Cluster: pinned, Outcome: outcomePinned, LocalQueue: localQueue}, nil
	}
	clusters := excludeClusters(
		consideredClusters(pinned, s.clusters.GetClusters(cluster.ReadyClusters), s.clusters.GetClusters(cluster.AllClusters)),
		excluded)
	sv, err := s.survey(ctx, rc, localQueue, clusters)
	if err != nil {
		return Decision{}, err
	}
	return decide(sv, localQueue, pinned), nil
}

// pinnedCluster returns the registered cluster the job's affinity names, read
// as cluster_only reads it; nil when it names none, or one that is not
// registered.
func (s *KueueAwareStrategy) pinnedCluster(job framework.BatchJob) *v2pb.Cluster {
	selector := job.GetAffinity().GetResourceAffinity().GetSelector()
	if selector == nil {
		return nil
	}
	name := selector.MatchLabels[constants.ClusterAffinityLabelKey]
	if name == "" {
		return nil
	}
	return s.clusters.GetCluster(name)
}

// consideredCluster is a registered cluster a decision considers.
type consideredCluster struct {
	cluster *v2pb.Cluster
	ready   bool
	// order is the cluster's position among the Ready clusters, which is
	// cluster_only's order.
	order int
}

// consideredClusters returns the clusters a decision considers: only the
// pinned cluster, if there is one, else every registered cluster, the Ready
// ones first in their listed order. A cluster that is not listed Ready is
// considered not Ready.
func consideredClusters(pinned *v2pb.Cluster, ready, all []*v2pb.Cluster) []consideredCluster {
	order := make(map[string]int, len(ready))
	for i, c := range ready {
		order[c.GetName()] = i
	}
	if pinned != nil {
		i, isReady := order[pinned.GetName()]
		return []consideredCluster{{cluster: pinned, ready: isReady, order: i}}
	}
	considered := make([]consideredCluster, 0, len(all))
	for i, c := range ready {
		considered = append(considered, consideredCluster{cluster: c, ready: true, order: i})
	}
	for _, c := range all {
		if _, isReady := order[c.GetName()]; !isReady {
			considered = append(considered, consideredCluster{cluster: c, order: len(ready)})
		}
	}
	return considered
}

// excludeClusters returns the considered clusters but the excluded ones, in
// order.
func excludeClusters(clusters []consideredCluster, excluded []string) []consideredCluster {
	if len(excluded) == 0 {
		return clusters
	}
	kept := make([]consideredCluster, 0, len(clusters))
	for _, c := range clusters {
		if !slices.Contains(excluded, c.cluster.GetName()) {
			kept = append(kept, c)
		}
	}
	return kept
}

// survey is what the considered clusters say about the job's LocalQueue.
type survey struct {
	// candidates are the Ready, Kueue-managed clusters that carry it.
	candidates []candidate
	// notReady are the Kueue-managed clusters that are not Ready and may
	// carry it: their synced state or their LocalQueues as last listed show
	// it, or neither can say, as unknown says why.
	notReady []string
	unknown  map[string]string
	// failed are the Ready, Kueue-managed clusters whose LocalQueue could not
	// be read, with the first error.
	failed    []string
	failedErr error
	// fallback is the first Ready cluster that is not Kueue-managed, in
	// cluster_only order.
	fallback *v2pb.Cluster
}

// survey reads the job's LocalQueue on each considered Kueue-managed cluster
// and scores the candidates. A job without a LocalQueue is on none.
func (s *KueueAwareStrategy) survey(ctx context.Context, job *v2pb.RayCluster, localQueue string, clusters []consideredCluster) (survey, error) {
	sv := survey{unknown: map[string]string{}}
	for _, c := range clusters {
		name := c.cluster.GetName()
		switch {
		case !utils.IsKueueManaged(c.cluster):
			if c.ready && sv.fallback == nil {
				sv.fallback = c.cluster
			}
			continue
		case localQueue == "":
			continue
		}
		view, err := s.state.Lookup(name, localQueue)
		if !c.ready {
			if carries, unknown := s.notReadyCarries(name, localQueue, view, err); carries {
				sv.notReady = append(sv.notReady, name)
				if unknown != "" {
					sv.unknown[name] = unknown
				}
			}
			continue
		}
		if err == nil && !view.Synced {
			var exists bool
			if exists, err = s.queues.Exists(ctx, c.cluster, k8sengine.RayLocalNamespace, localQueue); err == nil {
				view = state.View{Exists: exists}
			}
		}
		if err != nil {
			sv.failed = append(sv.failed, name)
			if sv.failedErr == nil {
				sv.failedErr = err
			}
			continue
		}
		if !view.Exists {
			continue
		}
		cand, err := s.score(ctx, job, c, view)
		if err != nil {
			return survey{}, err
		}
		sv.candidates = append(sv.candidates, cand)
	}
	return sv, nil
}

// notReadyCarries reports whether a Kueue-managed cluster that is not Ready
// may carry the LocalQueue, given its Lookup. Such a cluster is not dialed: its
// synced view decides, else its LocalQueues as last listed. When neither can
// say, because its state cannot be read or its current watch has not listed
// its LocalQueues yet, it may carry the LocalQueue, and unknown says why.
func (s *KueueAwareStrategy) notReadyCarries(name, localQueue string, view state.View, err error) (carries bool, unknown string) {
	switch {
	case err != nil:
		return true, "queues cannot be read"
	case view.Synced:
		return view.Exists, ""
	}
	exists, known := s.state.LastKnown(name, localQueue)
	if !known {
		return true, "queues not listed yet"
	}
	return exists, ""
}

// score scores a candidate for the job (decision steps 3 and 4): its
// eligibility, and its tier with the quantities the tier ranks by. Free quota
// and pending workloads are the ClusterQueue's: they count only the jobs in
// flight through LocalQueues that feed it.
func (s *KueueAwareStrategy) score(ctx context.Context, job *v2pb.RayCluster, c consideredCluster, view state.View) (candidate, error) {
	cand := candidate{cluster: c.cluster, order: c.order, view: view, eligible: true}
	ask, err := s.demand.Ask(job, c.cluster)
	if err != nil {
		// a job that cannot be mapped to the cluster fails at dispatch; it is
		// scored here as if its ask were unknown
		s.log.Info("Cannot compute the job's ask on the cluster; scoring the cluster without it",
			constants.Job, job.GetNamespace()+"/"+job.GetName(), "cluster", c.cluster.GetName(), "error", err.Error())
	} else {
		cand.ask = ask
	}
	if cq := view.ClusterQueue; view.Synced && cq != nil {
		inFlightAsk, inFlightJobs, err := s.inFlightTo(ctx, job, c.cluster, cq.Name)
		if err != nil {
			return candidate{}, err
		}
		cand.pending, cand.pendingKnown = cq.PendingWorkloads+inFlightJobs, true
		if cand.scorable = view.Scorable() && cand.ask != nil; cand.scorable {
			cand.eligible, _ = nominalCovers(cand.ask, cq.Nominal)
			cand.free = freeQuota(cand.ask, cq, inFlightAsk)
			cand.score = scoreQuota(cand.ask, cq.Nominal, cand.free)
		}
	}
	cand.tier = tierOf(cand)
	return cand, nil
}

// inFlightTo returns the summed ask and the number of the jobs in flight to
// the named ClusterQueue on the cluster: those whose LocalQueue feeds it. The
// job being placed is not in flight.
func (s *KueueAwareStrategy) inFlightTo(ctx context.Context, job *v2pb.RayCluster, c *v2pb.Cluster, clusterQueue string) (corev1.ResourceList, int32, error) {
	inFlight, err := s.demand.InFlight(ctx, c, s.now())
	if err != nil {
		return nil, 0, fmt.Errorf("kueue_aware: jobs in flight to cluster %s: %w", c.GetName(), err)
	}
	feeds := map[string]bool{}
	ask := corev1.ResourceList{}
	var count int32
	self := k8stypes.NamespacedName{Namespace: job.GetNamespace(), Name: job.GetName()}
	for _, entry := range inFlight.Jobs {
		key := k8stypes.NamespacedName{Namespace: entry.RayCluster.GetNamespace(), Name: entry.RayCluster.GetName()}
		if key == self {
			continue
		}
		feeding, seen := feeds[entry.LocalQueue]
		if !seen {
			feeding = s.feeds(c.GetName(), entry.LocalQueue, clusterQueue)
			feeds[entry.LocalQueue] = feeding
		}
		if !feeding {
			continue
		}
		count++
		for name, quantity := range entry.Ask {
			sum := ask[name]
			sum.Add(quantity)
			ask[name] = sum
		}
	}
	return ask, count, nil
}

// feeds reports whether the LocalQueue on the cluster feeds the named
// ClusterQueue.
func (s *KueueAwareStrategy) feeds(clusterName, localQueue, clusterQueue string) bool {
	if localQueue == "" {
		return false
	}
	view, err := s.state.Lookup(clusterName, localQueue)
	if err != nil {
		s.log.V(1).Info("Cannot read an in-flight job's LocalQueue; not counting the job",
			"cluster", clusterName, "localQueue", localQueue, "error", err.Error())
		return false
	}
	return view.ClusterQueue != nil && view.ClusterQueue.Name == clusterQueue
}

// decide applies the decision table to the survey: the top-ranked eligible
// candidate, else the outcome of a job with no eligible candidate.
func decide(sv survey, localQueue string, pinned *v2pb.Cluster) Decision {
	d := Decision{LocalQueue: localQueue, candidates: rank(sv.candidates)}
	if len(d.candidates) > 0 && d.candidates[0].eligible {
		d.Cluster, d.Tier, d.Outcome = d.candidates[0].cluster, d.candidates[0].tier, outcomeAssigned
		return d
	}
	switch {
	case len(sv.failed) > 0:
		// a cluster that could not be read may be an eligible candidate
		d.Outcome, d.Reason = outcomeCheckFailed, constants.KueueQueueCheckFailed
		d.Message = fmt.Sprintf("checking LocalQueue %q on cluster %q: %v", localQueue, sv.failed[0], sv.failedErr)
		if more := len(sv.failed) - 1; more > 0 {
			d.Message += fmt.Sprintf(" (and %d more clusters: %s)", more, strings.Join(sv.failed[1:], ", "))
		}
	case len(sv.candidates) > 0:
		d.Outcome, d.Reason = outcomeQuotaExceeded, constants.KueueQuotaExceeded
		d.Message = quotaExceededMessage(localQueue, d.candidates)
	case len(sv.notReady) > 0:
		d.Outcome, d.Reason = outcomeClusterNotReady, constants.KueueClusterNotReady
		d.Message = clusterNotReadyMessage(localQueue, sv.notReady, sv.unknown)
	case pinned == nil && sv.fallback != nil:
		// the job's LocalQueue is on no considered Kueue-managed cluster: a
		// project that is not onboarded runs as today
		d.Cluster, d.Outcome = sv.fallback, outcomeFallback
	default:
		d.Outcome, d.Reason = outcomeQueueNotFound, constants.KueueQueueNotFound
		d.Message = queueNotFoundMessage(localQueue, pinned)
	}
	return d
}

// queueNotFoundMessage explains KueueQueueNotFound.
func queueNotFoundMessage(localQueue string, pinned *v2pb.Cluster) string {
	switch {
	case localQueue == "":
		return fmt.Sprintf("cannot resolve a LocalQueue: the job has no project identity (a namespace, or a trusted %s label)",
			constants.ProjectNameLabelKey)
	case pinned != nil:
		return fmt.Sprintf("LocalQueue %q not found in namespace %q on cluster %q, which the job's affinity pins",
			localQueue, k8sengine.RayLocalNamespace, pinned.GetName())
	default:
		return fmt.Sprintf("LocalQueue %q not found in namespace %q on any Kueue-managed cluster, and no Ready cluster without Kueue to fall back to",
			localQueue, k8sengine.RayLocalNamespace)
	}
}

// clusterNotReadyMessage explains KueueClusterNotReady, saying why a cluster's
// queues are unknown when they are.
func clusterNotReadyMessage(localQueue string, notReady []string, unknown map[string]string) string {
	names := append([]string(nil), notReady...)
	sort.Strings(names)
	for i, name := range names {
		if why := unknown[name]; why != "" {
			names[i] = name + " (" + why + ")"
		}
	}
	return fmt.Sprintf("no Ready Kueue-managed cluster carries LocalQueue %q; not Ready: %s", localQueue, strings.Join(names, ", "))
}

// quotaExceededMessage explains KueueQuotaExceeded.
func quotaExceededMessage(localQueue string, candidates []candidate) string {
	parts := make([]string, 0, len(candidates))
	for _, c := range candidates {
		cq := c.view.ClusterQueue
		_, resource := nominalCovers(c.ask, cq.Nominal)
		ask, nominal := c.ask[resource], cq.Nominal[resource]
		parts = append(parts, fmt.Sprintf("%s (ClusterQueue %s: %s %s > nominal %s)",
			c.cluster.GetName(), cq.Name, resource, ask.String(), nominal.String()))
	}
	return fmt.Sprintf("the job's ask exceeds the nominal quota for LocalQueue %q on every candidate cluster: %s",
		localQueue, strings.Join(parts, ", "))
}
