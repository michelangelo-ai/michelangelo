//go:generate mamockgen ClusterReadiness
package cluster

// Readiness is a registered cluster's readiness as last probed by this
// process.
type Readiness int

const (
	// ReadinessUnknown means the cluster is registered but this process has
	// not probed it yet, e.g. just after a restart or a leader failover.
	ReadinessUnknown Readiness = iota
	// ReadinessReady means the last probe found the cluster Ready.
	ReadinessReady
	// ReadinessNotReady means the last probe found the cluster not Ready.
	ReadinessNotReady
	// ReadinessNotRegistered means no cluster of that name is registered.
	ReadinessNotRegistered
)

// String returns the readiness for logs.
func (r Readiness) String() string {
	switch r {
	case ReadinessUnknown:
		return "unknown"
	case ReadinessReady:
		return "ready"
	case ReadinessNotReady:
		return "not ready"
	case ReadinessNotRegistered:
		return "not registered"
	default:
		return "invalid"
	}
}

// ClusterReadiness reports the readiness of registered clusters without
// treating a cluster that was never probed as not Ready, which
// RegisteredClustersCache's filters cannot tell apart.
type ClusterReadiness interface {
	// GetClusterReadiness returns the readiness of the named cluster.
	GetClusterReadiness(name string) Readiness
}

var _ ClusterReadiness = (*Reconciler)(nil)

// NewClusterReadiness provides the reconciler's readiness view.
func NewClusterReadiness(r *Reconciler) ClusterReadiness {
	return r
}

// GetClusterReadiness returns the readiness of the named cluster.
func (r *Reconciler) GetClusterReadiness(name string) Readiness {
	data := r.clusterDataMap.get(name)
	if data == nil {
		return ReadinessNotRegistered
	}
	status := data.GetClusterStatus()
	switch {
	case status == nil:
		return ReadinessUnknown
	case isClusterReady(status):
		return ReadinessReady
	default:
		return ReadinessNotReady
	}
}
