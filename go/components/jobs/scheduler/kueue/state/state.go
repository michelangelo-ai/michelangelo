// Package state keeps the Kueue state of the Kueue-managed compute clusters for
// the Kueue scheduler backend to place jobs by: each cluster's ClusterQueues
// and LocalQueues, watched with the cluster's own credentials; what a job asks
// of a cluster's quota; and the jobs in flight to a cluster. It only reads the
// clusters.
package state

import (
	"context"
	"time"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	corev1 "k8s.io/api/core/v1"
)

//go:generate mamockgen State Demand

// State is the Kueue state of the Kueue-managed clusters, as last listed and
// watched.
type State interface {
	// Lookup returns the named LocalQueue on the named cluster, with the
	// ClusterQueue it feeds. For a cluster that is not watched, or not synced,
	// the view says only that, with a nil error: the error is for queues that
	// cannot be read.
	Lookup(cluster, localQueue string) (View, error)
	// LastKnown reports whether the named LocalQueue exists on the named
	// cluster as its LocalQueues were last listed and watched, also while its
	// watch is broken and Lookup says only that it is not synced. known is
	// false, and exists with it, until the cluster's current watch has listed
	// its LocalQueues, and when they cannot be read.
	LastKnown(cluster, localQueue string) (exists, known bool)
	// Synced reports whether the named cluster is watched and has listed its
	// queues since its watch started or last failed.
	Synced(cluster string) bool
}

// Demand is what jobs ask of the Kueue-managed clusters' quotas.
type Demand interface {
	// Ask returns what Kueue charges the cluster's quota for running the job
	// there: the requests of the pods of the KubeRay spec the job runs as.
	Ask(job *v2pb.RayCluster, cluster *v2pb.Cluster) (corev1.ResourceList, error)
	// InFlight returns the jobs in flight to the cluster at now: assigned to it
	// and not yet seen held or admitted there, or seen so less than 30 seconds
	// ago, so the cluster's Kueue status may not show them yet.
	InFlight(ctx context.Context, cluster *v2pb.Cluster, now time.Time) (InFlight, error)
	// RecordAssignment records that the job was assigned to the named cluster
	// at the given time. The job counts as in flight there from then until the
	// manager cache shows its assignment, for at most 30 seconds.
	RecordAssignment(job *v2pb.RayCluster, cluster string, at time.Time)
}

// View is a LocalQueue on a cluster, with the ClusterQueue it feeds.
type View struct {
	// Synced is whether the cluster's queues are synced; when they are not,
	// the view says nothing else.
	Synced bool
	// Exists is whether the LocalQueue exists.
	Exists bool
	// Active is whether the LocalQueue's Active condition is True.
	Active bool
	// ClusterQueue is the ClusterQueue the LocalQueue feeds; nil when either
	// does not exist.
	ClusterQueue *ClusterQueueView
}

// Scorable reports whether a placement can be scored on the view's
// ClusterQueue: the cluster is synced, and the ClusterQueue has exactly one
// flavor across its resource groups and no cohort, so its quota is all its own
// and in one flavor.
func (v View) Scorable() bool {
	return v.Synced && v.ClusterQueue != nil && v.ClusterQueue.FlavorCount == 1 && v.ClusterQueue.Cohort == ""
}

// ClusterQueueView is what placement reads of a ClusterQueue.
type ClusterQueueView struct {
	Name string
	// Cohort is the cohort the ClusterQueue shares quota in; empty for none.
	Cohort string
	// StopPolicy is spec.stopPolicy: None, Hold or HoldAndDrain; empty when
	// unset, which means None.
	StopPolicy string
	// FlavorCount is the number of flavors across the resource groups.
	FlavorCount int
	// CoveredResources are the resources the resource groups cover.
	CoveredResources []corev1.ResourceName
	// Nominal is each resource's nominal quota, summed over the flavors.
	Nominal corev1.ResourceList
	// Reserved is each resource's quota reserved by workloads
	// (status.flavorsReservation), summed over the flavors.
	Reserved corev1.ResourceList
	// PendingWorkloads is the number of workloads waiting for admission.
	PendingWorkloads int32
}

// Stopped reports whether the ClusterQueue's stop policy holds back new
// admissions.
func (cq *ClusterQueueView) Stopped() bool {
	return cq.StopPolicy != "" && cq.StopPolicy != "None"
}

// InFlight is the demand in flight to a cluster: the jobs assigned to it that
// its Kueue status may not show yet.
type InFlight struct {
	Jobs []InFlightJob
	// Ask is the jobs' asks, summed.
	Ask corev1.ResourceList
}

// InFlightJob is a job in flight to a cluster. Callers must not modify it.
type InFlightJob struct {
	RayCluster *v2pb.RayCluster
	// LocalQueue is the LocalQueue the job is dispatched to on the cluster;
	// empty if the cluster is not Kueue-managed.
	LocalQueue string
	// Ask is what Kueue charges the cluster's quota for the job.
	Ask corev1.ResourceList
}
