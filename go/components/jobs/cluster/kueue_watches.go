package cluster

import (
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// KueueWatches keeps the Kueue state of the registered clusters in step with
// the clusters: the controller starts, restarts and stops a Kueue-managed
// cluster's watches as it adds, refreshes, deletes and probes the cluster.
type KueueWatches interface {
	// Watch starts watching the cluster's Kueue queues with a client built from
	// the cluster's current spec and credentials, replacing a running watch of
	// the cluster.
	Watch(cluster *v2pb.Cluster)
	// Rewatch restarts the named cluster's watch, if it has one, with a client
	// rebuilt from the cluster's current credentials.
	Rewatch(name string)
	// Unwatch stops watching the named cluster, if it is watched.
	Unwatch(name string)
}

// noKueueWatches is the KueueWatches of a controller with no Kueue state wired in.
type noKueueWatches struct{}

func (noKueueWatches) Watch(*v2pb.Cluster) {}
func (noKueueWatches) Rewatch(string)      {}
func (noKueueWatches) Unwatch(string)      {}

// watches returns the controller's Kueue watch control.
func (r *Reconciler) watches() KueueWatches {
	if r.kueueWatches == nil {
		return noKueueWatches{}
	}
	return r.kueueWatches
}

// watchChange is what a cluster spec change means for the cluster's Kueue watches.
type watchChange int

const (
	keepWatches watchChange = iota
	// startWatches starts the watches, or restarts them with a new client.
	startWatches
	stopWatches
)

// kueueWatchChange returns what a spec change from oldSpec to newSpec means for
// the cluster's Kueue watches; oldSpec is nil for a cluster being added. A
// Kueue-managed cluster is watched from when it is added or marked, rewatched
// when its client must be rebuilt, and no longer watched once it is unmarked.
func kueueWatchChange(oldSpec, newSpec *v2pb.ClusterSpec) watchChange {
	wasManaged := oldSpec != nil && isKueueManaged(oldSpec)
	switch {
	case isKueueManaged(newSpec) && (!wasManaged || clientNeedsRebuild(oldSpec, newSpec)):
		return startWatches
	case wasManaged && !isKueueManaged(newSpec):
		return stopWatches
	default:
		return keepWatches
	}
}

// isKueueManaged reports whether Kueue admits the cluster's jobs.
func isKueueManaged(spec *v2pb.ClusterSpec) bool {
	return spec.GetSchedulerType() == v2pb.SCHEDULER_TYPE_KUEUE
}

// updateKueueWatches starts, restarts or stops the cluster's Kueue watches as
// change says.
func (r *Reconciler) updateKueueWatches(obj *v2pb.Cluster, change watchChange) {
	switch change {
	case startWatches:
		r.watches().Watch(obj)
	case stopWatches:
		r.watches().Unwatch(obj.Name)
	}
}
