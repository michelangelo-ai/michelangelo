package state

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// inFlightWindow is how long a job stays in flight to its cluster after it was
// launched there, or seen held or admitted there: long enough for the
// cluster's Kueue status to show it.
const inFlightWindow = 30 * time.Second

// AssignedClusterIndex is the manager cache's index of RayClusters by the
// cluster they are assigned to.
const AssignedClusterIndex = "status.assignment.cluster"

// assignedCluster is the AssignedClusterIndex index function.
func assignedCluster(obj client.Object) []string {
	rc, ok := obj.(*v2pb.RayCluster)
	if !ok {
		return nil
	}
	if c := rc.Status.GetAssignment().GetCluster(); c != "" {
		return []string{c}
	}
	return nil
}

// inFlight reports whether a job assigned to a cluster is in flight there at
// now: it is not ending, and it is not launched yet; or it was launched less
// than 30 s ago and has no Queued condition; or its Queued condition, True or
// False, last changed less than 30 s ago.
func inFlight(rc *v2pb.RayCluster, now time.Time) bool {
	if ending(rc) {
		return false
	}
	launched := findCondition(rc.Status.StatusConditions, constants.LaunchedCondition)
	if launched.GetStatus() != apipb.CONDITION_STATUS_TRUE {
		return true
	}
	queued := findCondition(rc.Status.StatusConditions, constants.QueuedCondition)
	switch queued.GetStatus() {
	case apipb.CONDITION_STATUS_TRUE, apipb.CONDITION_STATUS_FALSE:
		return now.Sub(conditionTime(queued)) < inFlightWindow
	default:
		return now.Sub(conditionTime(launched)) < inFlightWindow
	}
}

// ending reports whether a job is done or being torn down, so it holds no
// quota it is about to ask for.
func ending(rc *v2pb.RayCluster) bool {
	switch rc.Status.State {
	case v2pb.RAY_CLUSTER_STATE_TERMINATING, v2pb.RAY_CLUSTER_STATE_TERMINATED, v2pb.RAY_CLUSTER_STATE_FAILED:
		return true
	}
	// a termination request or a decided outcome starts the teardown, as the
	// RayCluster controller's shouldTerminateCluster reads them
	if !rc.DeletionTimestamp.IsZero() || rc.Spec.GetTermination().GetType() != v2pb.TERMINATION_TYPE_INVALID {
		return true
	}
	succeeded := findCondition(rc.Status.StatusConditions, constants.SucceededCondition)
	return succeeded.GetStatus() == apipb.CONDITION_STATUS_TRUE || succeeded.GetStatus() == apipb.CONDITION_STATUS_FALSE
}

// findCondition returns the condition of the given type; nil if there is none.
func findCondition(conditions []*apipb.Condition, conditionType string) *apipb.Condition {
	for _, c := range conditions {
		if c.GetType() == conditionType {
			return c
		}
	}
	return nil
}

// conditionTime returns when a condition last changed. LastUpdatedTimestamp
// holds Unix seconds, as jobsutils.UpdateCondition writes it; the proto
// documents milliseconds, so a value too large to be seconds is read as
// milliseconds.
func conditionTime(c *apipb.Condition) time.Time {
	const millisecondsFrom = 1e11 // in seconds, the year 5138
	ts := c.GetLastUpdatedTimestamp()
	if ts >= millisecondsFrom {
		return time.UnixMilli(ts)
	}
	return time.Unix(ts, 0)
}

// tracker computes asks, and the jobs in flight to the clusters.
type tracker struct {
	// reader reads RayClusters from the manager cache, indexed by
	// AssignedClusterIndex.
	reader client.Reader
	mapper types.Mapper
	log    logr.Logger

	mu sync.Mutex
	// recorded are the assignments recorded in the last 30 s, by job.
	recorded map[k8stypes.NamespacedName]*recordedAssignment
}

type recordedAssignment struct {
	job     *v2pb.RayCluster
	cluster string
	at      time.Time
}

func newTracker(reader client.Reader, mapper types.Mapper, log logr.Logger) *tracker {
	return &tracker{
		reader:   reader,
		mapper:   mapper,
		log:      log,
		recorded: map[k8stypes.NamespacedName]*recordedAssignment{},
	}
}

func (t *tracker) ask(job *v2pb.RayCluster, cluster *v2pb.Cluster) (corev1.ResourceList, error) {
	local, err := t.mapRayCluster(job, cluster)
	if err != nil {
		return nil, err
	}
	return RayClusterAsk(local), nil
}

func (t *tracker) recordAssignment(job *v2pb.RayCluster, cluster string, at time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.recorded[client.ObjectKeyFromObject(job)] = &recordedAssignment{job: job.DeepCopy(), cluster: cluster, at: at}
}

func (t *tracker) inFlight(ctx context.Context, cluster *v2pb.Cluster, now time.Time) (InFlight, error) {
	var assigned v2pb.RayClusterList
	if err := t.reader.List(ctx, &assigned, client.MatchingFields{AssignedClusterIndex: cluster.Name}); err != nil {
		return InFlight{}, fmt.Errorf("list the RayClusters assigned to cluster %s: %w", cluster.Name, err)
	}
	var jobs []*v2pb.RayCluster
	listed := map[k8stypes.NamespacedName]bool{}
	for i := range assigned.Items {
		job := &assigned.Items[i]
		listed[client.ObjectKeyFromObject(job)] = true
		if inFlight(job, now) {
			jobs = append(jobs, job)
		}
	}
	recorded, err := t.recordedAssignments(ctx, cluster.Name, now, listed)
	if err != nil {
		return InFlight{}, err
	}
	return t.entries(append(jobs, recorded...), cluster), nil
}

// recordedAssignments returns the jobs recorded as assigned to the cluster in
// the 30 s before now whose assignment the manager cache does not show yet;
// listed are the jobs the cache shows assigned to the cluster. Entries that
// expired or that the cache shows are dropped.
func (t *tracker) recordedAssignments(
	ctx context.Context, cluster string, now time.Time, listed map[k8stypes.NamespacedName]bool,
) ([]*v2pb.RayCluster, error) {
	t.mu.Lock()
	var pending []*recordedAssignment
	for key, r := range t.recorded {
		switch {
		case now.Sub(r.at) >= inFlightWindow:
			delete(t.recorded, key)
		case r.cluster == cluster:
			pending = append(pending, r)
		}
	}
	t.mu.Unlock()

	var jobs []*v2pb.RayCluster
	for _, r := range pending {
		key := client.ObjectKeyFromObject(r.job)
		shown := listed[key]
		if !shown {
			var cached v2pb.RayCluster
			switch err := t.reader.Get(ctx, key, &cached); {
			case err == nil:
				// assigned to any cluster: the persisted assignment wins
				shown = cached.Status.GetAssignment().GetCluster() != ""
			case !apierrors.IsNotFound(err):
				return nil, fmt.Errorf("get RayCluster %s: %w", key, err)
			}
		}
		if shown {
			t.forget(key, r)
			continue
		}
		jobs = append(jobs, r.job.DeepCopy())
	}
	return jobs, nil
}

// forget drops a recorded assignment, unless it was recorded again since.
func (t *tracker) forget(key k8stypes.NamespacedName, r *recordedAssignment) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.recorded[key] == r {
		delete(t.recorded, key)
	}
}

// entries returns the in-flight entries of the jobs on the cluster. A job that
// cannot be mapped to the cluster is left out: it cannot be created there.
func (t *tracker) entries(jobs []*v2pb.RayCluster, cluster *v2pb.Cluster) InFlight {
	result := InFlight{Ask: corev1.ResourceList{}}
	for _, job := range jobs {
		local, err := t.mapRayCluster(job, cluster)
		if err != nil {
			t.log.V(1).Info("Leaving a job that cannot be mapped out of the in-flight demand",
				"job", client.ObjectKeyFromObject(job), "cluster", cluster.Name, "error", err.Error())
			continue
		}
		entry := InFlightJob{
			RayCluster: job,
			LocalQueue: local.Labels[constants.KueueQueueNameLabelKey],
			Ask:        RayClusterAsk(local),
		}
		addResources(result.Ask, entry.Ask)
		result.Jobs = append(result.Jobs, entry)
	}
	return result
}

// mapRayCluster returns the KubeRay RayCluster the job runs as on the cluster.
func (t *tracker) mapRayCluster(job *v2pb.RayCluster, cluster *v2pb.Cluster) (*rayv1.RayCluster, error) {
	// the mapper writes into the pod templates it shares with its input
	obj, err := t.mapper.MapGlobalJobClusterToLocal(job.DeepCopy(), cluster)
	if err != nil {
		return nil, err
	}
	local, ok := obj.(*rayv1.RayCluster)
	if !ok {
		return nil, fmt.Errorf("RayCluster %s/%s mapped to %T, not a KubeRay RayCluster", job.Namespace, job.Name, obj)
	}
	return local, nil
}
