package kueue

import (
	"fmt"
	"math"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/kueue/state"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Tier is the class a candidate cluster ranks in for a job (RFC-20260731
// Phase 2, decision step 4). Lower tiers rank first.
type Tier int

const (
	// tierNone is no tier: the job waits, or goes to a cluster that is not
	// ranked because it is not Kueue-managed.
	tierNone Tier = 0
	// tierFitsNow (tier 1): the ClusterQueue is scorable and the job's ask
	// fits in its free quota now. Ranked by most bottleneck slack.
	tierFitsNow Tier = 1
	// tierQueueDepth (tier 2): the cluster's state is synced, but the ask
	// does not fit now or the free quota cannot be computed. Ranked by fewest
	// pending workloads, in-flight included, then smallest deficit, unknown
	// last.
	tierQueueDepth Tier = 2
	// tierUnsynced (tier 3): the cluster's state is not synced yet. Ranked in
	// cluster_only order.
	tierUnsynced Tier = 3
	// tierHeld (tier 4): the LocalQueue is inactive or the ClusterQueue is
	// stopped, so Kueue holds the job there until the queue resumes.
	tierHeld Tier = 4
)

// candidate is a Ready, Kueue-managed cluster that carries the job's
// LocalQueue (decision step 2), scored for the job.
type candidate struct {
	cluster *v2pb.Cluster
	// order is the cluster's position among the Ready clusters, which is
	// cluster_only's order.
	order int
	// view is the cluster's synced view of the LocalQueue; the zero View
	// when the cluster's state is not synced and the LocalQueue was read
	// directly.
	view state.View
	// ask is what the job asks of the cluster's quota; nil when it could not
	// be computed.
	ask corev1.ResourceList
	// scorable is whether free quota can be computed for the job: the view is
	// scorable and the ask is known.
	scorable bool
	// free is the ClusterQueue's free quota for each requested resource;
	// set when scorable.
	free corev1.ResourceList
	// score is the job's fit in the free quota; set when scorable.
	score quotaScore
	// pending is the ClusterQueue's pending workloads plus the jobs in flight
	// to it; set when pendingKnown.
	pending      int32
	pendingKnown bool
	// eligible is whether the ClusterQueue's nominal quota can ever admit the
	// job (decision step 3).
	eligible bool
	tier     Tier
}

// quotaScore is how a job's ask fits in a ClusterQueue's free quota.
type quotaScore struct {
	// fits is whether the ask fits in the free quota of every requested
	// resource.
	fits bool
	// slack is the bottleneck slack: the minimum, over the requested
	// resources, of (free - ask) / nominal. The deficit, the maximum of
	// (ask - free) / nominal, is its negation.
	slack float64
	// binding is the requested resource the slack and the deficit are taken
	// at.
	binding corev1.ResourceName
}

// deficit is the maximum, over the requested resources, of
// (ask - free) / nominal.
func (s quotaScore) deficit() float64 {
	return -s.slack
}

// requestedResources returns the resources the ask requests, sorted. A
// resource asked at zero is not requested, as a worker group with no replicas
// adds zero-valued keys to an ask.
func requestedResources(ask corev1.ResourceList) []corev1.ResourceName {
	var requested []corev1.ResourceName
	for name, quantity := range ask {
		if quantity.Sign() > 0 {
			requested = append(requested, name)
		}
	}
	sort.Slice(requested, func(i, j int) bool { return requested[i] < requested[j] })
	return requested
}

// nominalCovers reports whether a ClusterQueue's nominal quota covers the ask
// of every requested resource; a resource the ClusterQueue does not cover
// counts as zero, as in Kueue. It also returns the first resource it does not
// cover.
func nominalCovers(ask, nominal corev1.ResourceList) (bool, corev1.ResourceName) {
	for _, name := range requestedResources(ask) {
		if quantity, n := ask[name], nominal[name]; quantity.Cmp(n) > 0 {
			return false, name
		}
	}
	return true, ""
}

// freeQuota returns a ClusterQueue's free quota for each requested resource:
// nominal - reserved - in-flight.
func freeQuota(ask corev1.ResourceList, cq *state.ClusterQueueView, inFlight corev1.ResourceList) corev1.ResourceList {
	free := corev1.ResourceList{}
	for _, name := range requestedResources(ask) {
		f := cq.Nominal[name].DeepCopy()
		f.Sub(cq.Reserved[name])
		f.Sub(inFlight[name])
		free[name] = f
	}
	return free
}

// scoreQuota returns how the ask fits in the free quota of a ClusterQueue
// with the given nominal quota.
func scoreQuota(ask, nominal, free corev1.ResourceList) quotaScore {
	score := quotaScore{fits: true, slack: math.Inf(1)}
	for _, name := range requestedResources(ask) {
		left := free[name].DeepCopy()
		left.Sub(ask[name])
		if left.Sign() < 0 {
			score.fits = false
		}
		if slack := ratio(left, nominal[name]); slack < score.slack {
			score.slack, score.binding = slack, name
		}
	}
	return score
}

// ratio returns x / y; ±Inf when y is zero and x is not.
func ratio(x, y resource.Quantity) float64 {
	if y.IsZero() {
		switch x.Sign() {
		case 1:
			return math.Inf(1)
		case -1:
			return math.Inf(-1)
		default:
			return 0
		}
	}
	return x.AsApproximateFloat64() / y.AsApproximateFloat64()
}

// tierOf returns the tier the candidate ranks in.
func tierOf(c candidate) Tier {
	switch {
	case !c.view.Synced:
		return tierUnsynced
	case !c.view.Active || (c.view.ClusterQueue != nil && c.view.ClusterQueue.Stopped()):
		return tierHeld
	case c.scorable && c.score.fits:
		return tierFitsNow
	default:
		return tierQueueDepth
	}
}

// ranksBefore reports whether candidate a ranks before b: by tier, then by
// the tier's order, then by cluster name.
func ranksBefore(a, b candidate) bool {
	if a.tier != b.tier {
		return a.tier < b.tier
	}
	switch a.tier {
	case tierFitsNow:
		if a.score.slack != b.score.slack {
			return a.score.slack > b.score.slack
		}
	case tierQueueDepth:
		if a.pendingKnown != b.pendingKnown {
			return a.pendingKnown
		}
		if a.pending != b.pending {
			return a.pending < b.pending
		}
		// a deficit is known for a scorable candidate only; unknown last
		if a.scorable != b.scorable {
			return a.scorable
		}
		if a.scorable && a.score.deficit() != b.score.deficit() {
			return a.score.deficit() < b.score.deficit()
		}
	case tierUnsynced:
		if a.order != b.order {
			return a.order < b.order
		}
	}
	return a.cluster.GetName() < b.cluster.GetName()
}

// rank orders the candidates: the eligible ones by rank, then the others by
// cluster name.
func rank(candidates []candidate) []candidate {
	ranked := append([]candidate(nil), candidates...)
	sort.SliceStable(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		if a.eligible != b.eligible {
			return a.eligible
		}
		if !a.eligible {
			return a.cluster.GetName() < b.cluster.GetName()
		}
		return ranksBefore(a, b)
	})
	return ranked
}

// summary describes the candidate for the decision log.
func (c candidate) summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: tier %d", c.cluster.GetName(), c.tier)
	if !c.eligible {
		b.WriteString(", not eligible")
	}
	if !c.view.Synced {
		b.WriteString(", queues not synced")
		return b.String()
	}
	if cq := c.view.ClusterQueue; cq != nil {
		fmt.Fprintf(&b, ", ClusterQueue %s", cq.Name)
	}
	if c.scorable {
		fmt.Fprintf(&b, ", slack %.3f, deficit %.3f on %s", c.score.slack, c.score.deficit(), c.score.binding)
	} else {
		b.WriteString(", not scorable")
	}
	if c.pendingKnown {
		fmt.Fprintf(&b, ", pending %d", c.pending)
	}
	return b.String()
}

// formatResources formats a resource list for logs and messages, sorted by
// resource name.
func formatResources(resources corev1.ResourceList) string {
	parts := make([]string, 0, len(resources))
	for _, name := range requestedResources(resources) {
		quantity := resources[name]
		parts = append(parts, fmt.Sprintf("%s: %s", name, quantity.String()))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}
