package cluster

import (
	"context"
	"testing"

	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
	"go.uber.org/zap/zaptest/observer"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/k8sengine"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	matypes "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	jobsutils "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// _unhealthyState is the KubeRay cluster state string for unhealthy clusters.
// KubeRay does not export a typed constant for it.
const _unhealthyState = rayv1.ClusterState("unhealthy")

// headContainerName is what the job author called the ray container in the
// e2e template. Container names come straight from the submitted pod spec, so
// the watcher cannot match on any fixed name — see isRayContainer.
const headContainerName = "head"

// Log-persistence values the test mapper renders its log URL from.
const (
	testLogBucket     = "ray-history"
	testLogPathPrefix = "log"
	testLogURLFormat  = "http://logs.test/{{.Bucket}}/{{.PathPrefix}}/{{.ClusterName}}_{{.RayLocalNamespace}}/"
)

// testLogURL is what testLogURLFormat renders to for the cluster these tests use.
const testLogURL = "http://logs.test/" + testLogBucket + "/" + testLogPathPrefix + "/" +
	rayClusterName + "_" + k8sengine.RayLocalNamespace + "/"

// newTestMapper returns the real k8sengine mapper rather than a stub. The watcher
// delegates every globally-meaningful part of the status — log URL, reason, and the
// pod errors KubeRay reports through conditions — to it, so exercising the real
// implementation is what makes these tests evidence that the CR ends up carrying
// what the compute cluster actually said.
func newTestMapper() matypes.Mapper {
	return k8sengine.NewMapper(
		k8sengine.LogPersistenceConfig{
			Enabled:      true,
			Bucket:       testLogBucket,
			PathPrefix:   testLogPathPrefix,
			LogURLFormat: testLogURLFormat,
		},
		maconfig.SchedulerConfig{},
		env.Context{},
	).Mapper
}

func newTestLogger(t *testing.T) *Reconciler {
	zapLog := zaptest.NewLogger(t)
	logger := zapr.NewLogger(zapLog)

	scheme := runtime.NewScheme()
	kubescheme.AddToScheme(scheme)
	v2pb.AddToScheme(scheme)

	return &Reconciler{
		logger: logger,
		mapper: newTestMapper(),
	}
}

func newKubeRayCluster(name string, state rayv1.ClusterState) *rayv1.RayCluster {
	return &rayv1.RayCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels: map[string]string{
				constants.ProjectNameLabelKey:   testNamespace,
				constants.OwnerServiceLabelKey:  constants.MAOwnerServiceLabelValue,
				constants.JobControlPlaneEnvKey: "test",
			},
		},
		Status: rayv1.RayClusterStatus{
			State: state,
		},
	}
}

// newHeadPod builds a running Ray head pod with the labels/annotations the
// watcher relies on (head-node type, cluster name, project name, ports).
func newHeadPod(name, clusterName string, phase corev1.PodPhase) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels: map[string]string{
				constants.RayNodeTypeLabelKey:    constants.RayHeadNodeType,
				constants.RayClusterNameLabelKey: clusterName,
				constants.ProjectNameLabelKey:    testNamespace,
			},
			Annotations: map[string]string{
				constants.DynamicPortAnnotationKeyPrefix + constants.RayClientPort:       "10001",
				constants.DynamicPortAnnotationKeyPrefix + constants.JupyterNotebookPort: "8888",
			},
		},
		Status: corev1.PodStatus{
			Phase: phase,
			PodIP: "10.0.0.1",
		},
	}
}

func setupWatcherTest(t *testing.T, globalCluster *v2pb.RayCluster) *Reconciler {
	t.Helper()

	scheme := runtime.NewScheme()
	kubescheme.AddToScheme(scheme)
	v2pb.AddToScheme(scheme)

	objects := []runtime.Object{}
	if globalCluster != nil {
		objects = append(objects, globalCluster)
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objects...).
		WithStatusSubresource(&v2pb.RayCluster{}).
		Build()

	zapLog := zaptest.NewLogger(t)
	logger := zapr.NewLogger(zapLog)

	apiHandler := &mockAPIHandler{Client: fakeClient}

	return &Reconciler{
		Handler: apiHandler,
		logger:  logger,
		mapper:  newTestMapper(),
	}
}

func getUpdatedCluster(t *testing.T, r *Reconciler) v2pb.RayCluster {
	t.Helper()
	var updated v2pb.RayCluster
	err := r.Handler.(*mockAPIHandler).Client.Get(context.Background(),
		types.NamespacedName{Name: rayClusterName, Namespace: testNamespace}, &updated)
	require.NoError(t, err)
	return updated
}

func findCondition(conds []*apipb.Condition, condType string) *apipb.Condition {
	for _, cond := range conds {
		if cond.Type == condType {
			return cond
		}
	}
	return nil
}

func TestMapKubeRayClusterState(t *testing.T) {
	tests := []struct {
		name     string
		input    rayv1.ClusterState
		expected v2pb.RayClusterState
	}{
		{name: "ready state", input: rayv1.Ready, expected: v2pb.RAY_CLUSTER_STATE_READY},
		{name: "failed state", input: rayv1.Failed, expected: v2pb.RAY_CLUSTER_STATE_FAILED},
		{name: "unhealthy state", input: _unhealthyState, expected: v2pb.RAY_CLUSTER_STATE_UNHEALTHY},
		{name: "suspended state", input: rayv1.Suspended, expected: v2pb.RAY_CLUSTER_STATE_SUSPENDED},
		{name: "empty state", input: "", expected: v2pb.RAY_CLUSTER_STATE_UNKNOWN},
		{name: "unknown state string", input: "SomeNewState", expected: v2pb.RAY_CLUSTER_STATE_UNKNOWN},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := mapKubeRayClusterState(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestRayClusterEventHandler_Ready(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status: v2pb.RayClusterStatus{
			State: v2pb.RAY_CLUSTER_STATE_PROVISIONING,
		},
	}
	r := setupWatcherTest(t, globalCluster)

	r.rayClusterEventHandler(newKubeRayCluster(rayClusterName, rayv1.Ready))

	updated := getUpdatedCluster(t, r)
	assert.Equal(t, v2pb.RAY_CLUSTER_STATE_READY, updated.Status.State)

	// The cluster handler owns State/conditions only; HeadNode is owned by the
	// pod handler and must not be touched here.
	assert.Nil(t, updated.Status.HeadNode, "cluster handler should not set HeadNode")

	// The log URL is derived by the mapper, not by the controller: a ready
	// cluster with log persistence configured must carry a browsable URL.
	assert.Equal(t, testLogURL, updated.Status.LogUrl)

	launchedCond := findCondition(updated.Status.StatusConditions, LaunchedCondition)
	require.NotNil(t, launchedCond)
	assert.Equal(t, apipb.CONDITION_STATUS_TRUE, launchedCond.Status)
	assert.Equal(t, "ClusterReady", launchedCond.Reason)
}

func TestRayClusterEventHandler_ReadyDedup(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status: v2pb.RayClusterStatus{
			State:  v2pb.RAY_CLUSTER_STATE_READY,
			LogUrl: testLogURL,
			StatusConditions: []*apipb.Condition{
				{Type: LaunchedCondition, Status: apipb.CONDITION_STATUS_TRUE},
			},
		},
	}
	r := setupWatcherTest(t, globalCluster)

	// Sending a Ready event when global is already READY+Launched=TRUE and has
	// its log URL should be a no-op (de-dup): the launched condition's timestamp
	// must not change. The informer re-syncs every _reSyncPeriod, so a converged
	// cluster that kept rewriting itself would churn the API server all day.
	r.rayClusterEventHandler(newKubeRayCluster(rayClusterName, rayv1.Ready))

	updated := getUpdatedCluster(t, r)
	assert.Equal(t, v2pb.RAY_CLUSTER_STATE_READY, updated.Status.State)
	launchedCond := findCondition(updated.Status.StatusConditions, LaunchedCondition)
	require.NotNil(t, launchedCond)
	// Timestamp should be zero (unchanged from the test fixture) because the
	// handler exited before calling UpdateStatusWithRetries.
	assert.Equal(t, int64(0), launchedCond.LastUpdatedTimestamp)
}

// TestRayClusterEventHandler_ReadyBackfillsLogURL covers the one case where an
// already-READY cluster must still be written: it is missing its log URL, either
// because it went ready under a build that did not set one or because log
// persistence was configured afterwards. The de-dup short-circuit has to let that
// through once and then go quiet again, otherwise the cluster either never gets a
// log URL or rewrites itself on every re-sync.
func TestRayClusterEventHandler_ReadyBackfillsLogURL(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status: v2pb.RayClusterStatus{
			State: v2pb.RAY_CLUSTER_STATE_READY,
			StatusConditions: []*apipb.Condition{
				{Type: LaunchedCondition, Status: apipb.CONDITION_STATUS_TRUE},
			},
		},
	}
	r := setupWatcherTest(t, globalCluster)

	r.rayClusterEventHandler(newKubeRayCluster(rayClusterName, rayv1.Ready))

	backfilled := getUpdatedCluster(t, r)
	assert.Equal(t, testLogURL, backfilled.Status.LogUrl, "log URL should be backfilled")

	// Now that it is converged, a repeat event must not write again.
	r.rayClusterEventHandler(newKubeRayCluster(rayClusterName, rayv1.Ready))

	settled := getUpdatedCluster(t, r)
	assert.Equal(t, backfilled.ResourceVersion, settled.ResourceVersion,
		"a converged cluster should not be rewritten on re-sync")
}

// replicaFailure builds the KubeRay condition that reports a pod the cluster
// could not bring up. This is the channel through which a creation failure
// (bad image, unschedulable pod, quota denial) becomes visible to the watcher.
func replicaFailure(reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:    string(rayv1.RayClusterReplicaFailure),
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: message,
	}
}

// withConditions attaches KubeRay status conditions to a local cluster.
func withConditions(rc *rayv1.RayCluster, conds ...metav1.Condition) *rayv1.RayCluster {
	rc.Status.Conditions = append(rc.Status.Conditions, conds...)
	return rc
}

// suspended marks a local cluster as held by an admission controller, the way
// Kueue does before it admits the workload.
func suspended(rc *rayv1.RayCluster) *rayv1.RayCluster {
	suspend := true
	rc.Spec.Suspend = &suspend
	return rc
}

// TestRayClusterEventHandler_FailedSurfacesKubeRayReason asserts that a failure
// is reported with the reason and pod errors KubeRay gave, not a generic
// "ClusterFailed". Without this, an operator reading the CR learns that the
// cluster failed but not that, say, the image could not be pulled.
func TestRayClusterEventHandler_FailedSurfacesKubeRayReason(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status:     v2pb.RayClusterStatus{State: v2pb.RAY_CLUSTER_STATE_PROVISIONING},
	}
	r := setupWatcherTest(t, globalCluster)

	r.rayClusterEventHandler(withConditions(
		newKubeRayCluster(rayClusterName, rayv1.Failed),
		replicaFailure("FailedCreateWorkerPod", "pods \"worker-0\" is forbidden: exceeded quota"),
	))

	updated := getUpdatedCluster(t, r)
	assert.Equal(t, v2pb.RAY_CLUSTER_STATE_FAILED, updated.Status.State)

	succeededCond := findCondition(updated.Status.StatusConditions, SucceededCondition)
	require.NotNil(t, succeededCond)
	assert.Equal(t, apipb.CONDITION_STATUS_FALSE, succeededCond.Status)
	assert.Equal(t, "FailedCreateWorkerPod", succeededCond.Reason,
		"the KubeRay reason should win over the generic fallback")

	require.Len(t, updated.Status.PodErrors, 1)
	assert.Equal(t, string(rayv1.RayClusterReplicaFailure), updated.Status.PodErrors[0].Name)
	assert.Equal(t, "FailedCreateWorkerPod", updated.Status.PodErrors[0].Reason)
	assert.Contains(t, updated.Status.PodErrors[0].Message, "exceeded quota")
}

// TestRayClusterEventHandler_UnknownWithTerminalPodErrorsFails covers the case
// that matters most for a cluster that never comes up: KubeRay reports a
// terminal pod failure while status.state is still empty. The CR has to
// converge on FAILED rather than sitting in UNKNOWN forever.
func TestRayClusterEventHandler_UnknownWithTerminalPodErrorsFails(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status:     v2pb.RayClusterStatus{State: v2pb.RAY_CLUSTER_STATE_PROVISIONING},
	}
	r := setupWatcherTest(t, globalCluster)

	// Empty KubeRay state maps to UNKNOWN.
	r.rayClusterEventHandler(withConditions(
		newKubeRayCluster(rayClusterName, ""),
		replicaFailure("FailedCreateHeadPod", "Error creating: pods is forbidden"),
	))

	updated := getUpdatedCluster(t, r)
	assert.Equal(t, v2pb.RAY_CLUSTER_STATE_FAILED, updated.Status.State,
		"terminal pod errors should promote UNKNOWN to FAILED")
	succeededCond := findCondition(updated.Status.StatusConditions, SucceededCondition)
	require.NotNil(t, succeededCond)
	assert.Equal(t, apipb.CONDITION_STATUS_FALSE, succeededCond.Status)
	assert.Equal(t, "FailedCreateHeadPod", succeededCond.Reason)
}

// TestRayClusterEventHandler_UnknownWithNonTerminalPodErrorsStaysUnknown is the
// other half of that behaviour: HeadPodNotFound is what KubeRay reports in the
// ordinary window between creating a RayCluster and scheduling its head pod, so
// it must be recorded without tearing the cluster down.
func TestRayClusterEventHandler_UnknownWithNonTerminalPodErrorsStaysUnknown(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status:     v2pb.RayClusterStatus{State: v2pb.RAY_CLUSTER_STATE_PROVISIONING},
	}
	r := setupWatcherTest(t, globalCluster)

	r.rayClusterEventHandler(withConditions(
		newKubeRayCluster(rayClusterName, ""),
		metav1.Condition{
			Type:    string(rayv1.HeadPodReady),
			Status:  metav1.ConditionFalse,
			Reason:  "HeadPodNotFound",
			Message: "Head Pod not found",
		},
	))

	updated := getUpdatedCluster(t, r)
	assert.Equal(t, v2pb.RAY_CLUSTER_STATE_UNKNOWN, updated.Status.State)
	assert.Nil(t, findCondition(updated.Status.StatusConditions, SucceededCondition),
		"a head pod that has not been scheduled yet is not a failure")
	require.Len(t, updated.Status.PodErrors, 1)
	assert.Equal(t, "HeadPodNotFound", updated.Status.PodErrors[0].Reason)
}

// TestRayClusterEventHandler_SuspendedSetsQueued covers admission gating: the
// cluster is held, not failing, so the CR must say Queued and must not grow a
// Succeeded=FALSE. Pods are intentionally absent while suspended, which is why
// the pod-level failure condition below must not be treated as an error (#1700).
func TestRayClusterEventHandler_SuspendedSetsQueued(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status:     v2pb.RayClusterStatus{State: v2pb.RAY_CLUSTER_STATE_PROVISIONING},
	}
	r := setupWatcherTest(t, globalCluster)

	r.rayClusterEventHandler(suspended(withConditions(
		newKubeRayCluster(rayClusterName, ""),
		replicaFailure("FailedCreateWorkerPod", "no pods while suspended"),
	)))

	updated := getUpdatedCluster(t, r)
	assert.Equal(t, v2pb.RAY_CLUSTER_STATE_SUSPENDED, updated.Status.State)

	queuedCond := findCondition(updated.Status.StatusConditions, QueuedCondition)
	require.NotNil(t, queuedCond, "a suspended cluster should report Queued")
	assert.Equal(t, apipb.CONDITION_STATUS_TRUE, queuedCond.Status)
	assert.Equal(t, "AwaitingAdmission", queuedCond.Reason)

	assert.Nil(t, findCondition(updated.Status.StatusConditions, SucceededCondition),
		"suspension is not a failure")
	assert.Empty(t, updated.Status.PodErrors,
		"pod-level conditions carry no meaning while suspended")
}

// TestRayClusterEventHandler_ReadyClearsQueued is the other end of the admission
// lifecycle: once the cluster is admitted and ready, the Queued condition has to
// be flipped, or every admitted cluster reads as still waiting.
func TestRayClusterEventHandler_ReadyClearsQueued(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status: v2pb.RayClusterStatus{
			State: v2pb.RAY_CLUSTER_STATE_SUSPENDED,
			StatusConditions: []*apipb.Condition{
				{Type: QueuedCondition, Status: apipb.CONDITION_STATUS_TRUE, Reason: "AwaitingAdmission"},
			},
		},
	}
	r := setupWatcherTest(t, globalCluster)

	r.rayClusterEventHandler(newKubeRayCluster(rayClusterName, rayv1.Ready))

	updated := getUpdatedCluster(t, r)
	assert.Equal(t, v2pb.RAY_CLUSTER_STATE_READY, updated.Status.State)

	queuedCond := findCondition(updated.Status.StatusConditions, QueuedCondition)
	require.NotNil(t, queuedCond)
	assert.Equal(t, apipb.CONDITION_STATUS_FALSE, queuedCond.Status)
	assert.Equal(t, "ClusterAdmitted", queuedCond.Reason)
}

// TestRayClusterEventHandler_ReadyDoesNotAddQueued guards the narrowness of that
// flip: a cluster that was never held for admission should not acquire a Queued
// condition just by becoming ready.
func TestRayClusterEventHandler_ReadyDoesNotAddQueued(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status:     v2pb.RayClusterStatus{State: v2pb.RAY_CLUSTER_STATE_PROVISIONING},
	}
	r := setupWatcherTest(t, globalCluster)

	r.rayClusterEventHandler(newKubeRayCluster(rayClusterName, rayv1.Ready))

	updated := getUpdatedCluster(t, r)
	assert.Nil(t, findCondition(updated.Status.StatusConditions, QueuedCondition))
}

// TestMergePodErrors pins the merge semantics every writer depends on. The
// informer re-syncs every _reSyncPeriod and a wedged pod is re-reported on each
// status change, so entries must update in place rather than accumulate; and
// the condition-derived entries (keyed by condition type) must not collide with
// the pod-derived ones (keyed by pod name).
func TestMergePodErrors(t *testing.T) {
	t.Run("repeated events update in place", func(t *testing.T) {
		cluster := &v2pb.RayCluster{Status: v2pb.RayClusterStatus{}}

		mergePodErrors(cluster, []*v2pb.PodErrors{
			{Name: "ReplicaFailure", Reason: "FailedCreateWorkerPod", Message: "first"},
		})
		mergePodErrors(cluster, []*v2pb.PodErrors{
			{Name: "ReplicaFailure", Reason: "FailedCreateWorkerPod", Message: "second"},
		})

		require.Len(t, cluster.Status.PodErrors, 1, "re-syncs must not grow the list")
		assert.Equal(t, "second", cluster.Status.PodErrors[0].Message)
	})

	t.Run("pod-keyed errors survive", func(t *testing.T) {
		cluster := &v2pb.RayCluster{
			Status: v2pb.RayClusterStatus{
				PodErrors: []*v2pb.PodErrors{
					{Name: "test-cluster-head-abc12", Reason: "OOMKilled", Message: "from pod watcher"},
				},
			},
		}

		mergePodErrors(cluster, []*v2pb.PodErrors{
			{Name: "ReplicaFailure", Reason: "FailedCreateWorkerPod"},
		})

		require.Len(t, cluster.Status.PodErrors, 2)
		assert.Equal(t, "OOMKilled", cluster.Status.PodErrors[0].Reason)
		assert.Equal(t, "FailedCreateWorkerPod", cluster.Status.PodErrors[1].Reason)
	})

	t.Run("respects the pod error cap", func(t *testing.T) {
		// No workers configured, so the cap is 2 * (1 + 0).
		cluster := &v2pb.RayCluster{
			Status: v2pb.RayClusterStatus{
				PodErrors: []*v2pb.PodErrors{
					{Name: "pod-a", Reason: "OOMKilled"},
					{Name: "pod-b", Reason: "OOMKilled"},
				},
			},
		}

		mergePodErrors(cluster, []*v2pb.PodErrors{
			{Name: "ReplicaFailure", Reason: "FailedCreateWorkerPod"},
		})

		assert.Len(t, cluster.Status.PodErrors, 2, "the cap should hold")
	})

	t.Run("known entries are still refreshed at the cap", func(t *testing.T) {
		// A wedged pod is re-reported on every status change. Once the cap is
		// reached its message must still be allowed to move forward, otherwise
		// the CR freezes on the first thing the pod said.
		cluster := &v2pb.RayCluster{
			Status: v2pb.RayClusterStatus{
				PodErrors: []*v2pb.PodErrors{
					{Name: "pod-a", Reason: "ErrImagePull", Message: "first"},
					{Name: "pod-b", Reason: "OOMKilled"},
				},
			},
		}

		mergePodErrors(cluster, []*v2pb.PodErrors{
			{Name: "pod-a", Reason: "ImagePullBackOff", Message: "second"},
		})

		require.Len(t, cluster.Status.PodErrors, 2)
		assert.Equal(t, "ImagePullBackOff", cluster.Status.PodErrors[0].Reason)
		assert.Equal(t, "second", cluster.Status.PodErrors[0].Message)
	})

	t.Run("teardown noise does not erase the root cause", func(t *testing.T) {
		// Killing a failed cluster deletes its pods, and the delete handler then
		// re-reports every container as ContainerStatusUnknown. Since the merge
		// keys on pod name alone, an unguarded overwrite would leave the CR
		// saying only that the container "could not be located", which tells a
		// user nothing about why their cluster died.
		cluster := &v2pb.RayCluster{
			Status: v2pb.RayClusterStatus{
				PodErrors: []*v2pb.PodErrors{
					{Name: "pod-a", ContainerName: "head", Reason: "ImagePullBackOff", Message: `Back-off pulling image "nope:latest"`},
				},
			},
		}

		mergePodErrors(cluster, []*v2pb.PodErrors{{
			Name: "pod-a", ContainerName: "head", ExitCode: 137,
			Reason:  "ContainerStatusUnknown",
			Message: "The container could not be located when the pod was terminated",
		}})

		require.Len(t, cluster.Status.PodErrors, 1)
		assert.Equal(t, "ImagePullBackOff", cluster.Status.PodErrors[0].Reason,
			"the diagnosis recorded while the pod was alive must survive its teardown")
	})

	t.Run("teardown noise is kept when nothing better is known", func(t *testing.T) {
		// Half a diagnosis beats none: if the pod died without ever reporting a
		// container error there is nothing to protect, so record what we have.
		cluster := &v2pb.RayCluster{Status: v2pb.RayClusterStatus{}}

		mergePodErrors(cluster, []*v2pb.PodErrors{
			{Name: "pod-a", Reason: "ContainerStatusUnknown"},
		})
		mergePodErrors(cluster, []*v2pb.PodErrors{
			{Name: "pod-a", Reason: "ContainerStatusUnknown", Message: "later"},
		})

		require.Len(t, cluster.Status.PodErrors, 1)
		assert.Equal(t, "ContainerStatusUnknown", cluster.Status.PodErrors[0].Reason)
		assert.Equal(t, "later", cluster.Status.PodErrors[0].Message,
			"post-mortem entries may still refresh each other")
	})

	t.Run("no errors is a no-op", func(t *testing.T) {
		cluster := &v2pb.RayCluster{Status: v2pb.RayClusterStatus{}}
		mergePodErrors(cluster, nil)
		assert.Empty(t, cluster.Status.PodErrors)
	})
}

// TestHandlePodEvent_RecordsUnschedulablePod covers the one failure mode with no
// container to blame. The pod never starts, so it has no container statuses and
// is never deleted either -- the delete path will not rescue it later. Its
// PodScheduled condition is the only record that the cluster asked for more than
// the compute cluster has.
func TestHandlePodEvent_RecordsUnschedulablePod(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status:     v2pb.RayClusterStatus{State: v2pb.RAY_CLUSTER_STATE_PROVISIONING},
	}
	r := setupWatcherTest(t, globalCluster)

	pod := newHeadPod("head-pod", rayClusterName, corev1.PodPending)
	pod.Status.Conditions = []corev1.PodCondition{{
		Type:    corev1.PodScheduled,
		Status:  corev1.ConditionFalse,
		Reason:  corev1.PodReasonUnschedulable,
		Message: "0/1 nodes are available: 1 Insufficient cpu.",
	}}

	r.handlePodEvent(pod)

	updated := getUpdatedCluster(t, r)
	require.Len(t, updated.Status.PodErrors, 1)
	assert.Equal(t, "head-pod", updated.Status.PodErrors[0].Name)
	assert.Equal(t, "Unschedulable", updated.Status.PodErrors[0].Reason)
	assert.Equal(t, "0/1 nodes are available: 1 Insufficient cpu.", updated.Status.PodErrors[0].Message)
	assert.False(t, jobsutils.HasTerminalPodErrors(updated.Status.PodErrors),
		"an unschedulable pod may still be admitted, so it must not fail the cluster")
}

// TestHandlePodEvent_ContainerErrorWinsOverScheduling pins the precedence. Once a
// container has something to say it is the better diagnosis, and the scheduling
// fallback must not shadow it.
func TestHandlePodEvent_ContainerErrorWinsOverScheduling(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status:     v2pb.RayClusterStatus{State: v2pb.RAY_CLUSTER_STATE_PROVISIONING},
	}
	r := setupWatcherTest(t, globalCluster)

	pod := newHeadPod("head-pod", rayClusterName, corev1.PodPending)
	pod.Status.Conditions = []corev1.PodCondition{{
		Type:   corev1.PodScheduled,
		Status: corev1.ConditionFalse,
		Reason: corev1.PodReasonUnschedulable,
	}}
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name:  headContainerName,
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
	}}

	r.handlePodEvent(pod)

	updated := getUpdatedCluster(t, r)
	require.Len(t, updated.Status.PodErrors, 1)
	assert.Equal(t, "CrashLoopBackOff", updated.Status.PodErrors[0].Reason)
}

func TestRayClusterEventHandler_Failed(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status:     v2pb.RayClusterStatus{State: v2pb.RAY_CLUSTER_STATE_PROVISIONING},
	}
	r := setupWatcherTest(t, globalCluster)

	r.rayClusterEventHandler(newKubeRayCluster(rayClusterName, rayv1.Failed))

	updated := getUpdatedCluster(t, r)
	assert.Equal(t, v2pb.RAY_CLUSTER_STATE_FAILED, updated.Status.State)

	succeededCond := findCondition(updated.Status.StatusConditions, SucceededCondition)
	require.NotNil(t, succeededCond)
	assert.Equal(t, apipb.CONDITION_STATUS_FALSE, succeededCond.Status)
}

func TestRayClusterEventHandler_Unhealthy(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status:     v2pb.RayClusterStatus{State: v2pb.RAY_CLUSTER_STATE_READY},
	}
	r := setupWatcherTest(t, globalCluster)

	r.rayClusterEventHandler(newKubeRayCluster(rayClusterName, _unhealthyState))

	updated := getUpdatedCluster(t, r)
	assert.Equal(t, v2pb.RAY_CLUSTER_STATE_UNHEALTHY, updated.Status.State)

	succeededCond := findCondition(updated.Status.StatusConditions, SucceededCondition)
	require.NotNil(t, succeededCond)
	assert.Equal(t, apipb.CONDITION_STATUS_FALSE, succeededCond.Status)
	assert.Equal(t, "ClusterUnhealthy", succeededCond.Reason)
}

func TestRayClusterEventHandler_UnknownWithTerminalPodErrors(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status: v2pb.RayClusterStatus{
			State:     v2pb.RAY_CLUSTER_STATE_PROVISIONING,
			PodErrors: []*v2pb.PodErrors{{Reason: "CrashLoopBackOff"}},
		},
	}
	r := setupWatcherTest(t, globalCluster)

	// Empty KubeRay state maps to UNKNOWN; terminal pod errors force FAILED.
	r.rayClusterEventHandler(newKubeRayCluster(rayClusterName, ""))

	updated := getUpdatedCluster(t, r)
	assert.Equal(t, v2pb.RAY_CLUSTER_STATE_FAILED, updated.Status.State)

	succeededCond := findCondition(updated.Status.StatusConditions, SucceededCondition)
	require.NotNil(t, succeededCond)
	assert.Equal(t, apipb.CONDITION_STATUS_FALSE, succeededCond.Status)
	assert.Equal(t, "ClusterFailedWithPodErrors", succeededCond.Reason)
}

func TestRayClusterEventHandler_Suspended(t *testing.T) {
	// Suspension (KubeRay status.state=suspended, e.g. Kueue admission gating)
	// is non-terminal: even with a genuinely terminal pod error already
	// recorded, a suspended cluster must keep being monitored (State=SUSPENDED)
	// and must never be marked failed. Guards against the UNKNOWN + terminal
	// pod-error path tearing down suspended clusters (#1700).
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status: v2pb.RayClusterStatus{
			State:     v2pb.RAY_CLUSTER_STATE_PROVISIONING,
			PodErrors: []*v2pb.PodErrors{{Reason: "CrashLoopBackOff"}},
		},
	}
	r := setupWatcherTest(t, globalCluster)

	r.rayClusterEventHandler(newKubeRayCluster(rayClusterName, rayv1.Suspended))

	updated := getUpdatedCluster(t, r)
	assert.Equal(t, v2pb.RAY_CLUSTER_STATE_SUSPENDED, updated.Status.State)

	succeededCond := findCondition(updated.Status.StatusConditions, SucceededCondition)
	if succeededCond != nil {
		assert.NotEqual(t, apipb.CONDITION_STATUS_FALSE, succeededCond.Status,
			"suspended cluster must not be marked failed")
	}
}

func TestRayClusterEventHandler_SuspendedViaSpec(t *testing.T) {
	// status.state lags spec.suspend when a cluster is suspended at creation
	// (its admission webhook sets spec.suspend before KubeRay writes state).
	// The watcher must report SUSPENDED from spec intent even though the empty
	// status.state would otherwise map to UNKNOWN and, with a terminal pod
	// error, force FAILED.
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status: v2pb.RayClusterStatus{
			State:     v2pb.RAY_CLUSTER_STATE_PROVISIONING,
			PodErrors: []*v2pb.PodErrors{{Reason: "CrashLoopBackOff"}},
		},
	}
	r := setupWatcherTest(t, globalCluster)

	local := newKubeRayCluster(rayClusterName, "") // empty state maps to UNKNOWN without suspend intent
	suspend := true
	local.Spec.Suspend = &suspend
	r.rayClusterEventHandler(local)

	updated := getUpdatedCluster(t, r)
	assert.Equal(t, v2pb.RAY_CLUSTER_STATE_SUSPENDED, updated.Status.State)

	succeededCond := findCondition(updated.Status.StatusConditions, SucceededCondition)
	if succeededCond != nil {
		assert.NotEqual(t, apipb.CONDITION_STATUS_FALSE, succeededCond.Status,
			"cluster suspended via spec.suspend must not be marked failed")
	}
}

func TestRayClusterEventHandler_InvalidObject(t *testing.T) {
	r := newTestLogger(t)
	// Should not panic on non-RayCluster object.
	r.rayClusterEventHandler("not a ray cluster")
}

func TestRayClusterDeleteEventHandler_ExternalDeletion(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status: v2pb.RayClusterStatus{
			State: v2pb.RAY_CLUSTER_STATE_READY,
			StatusConditions: []*apipb.Condition{
				{Type: LaunchedCondition, Status: apipb.CONDITION_STATUS_TRUE},
			},
		},
	}
	r := setupWatcherTest(t, globalCluster)

	r.rayClusterDeleteEventHandler(newKubeRayCluster(rayClusterName, rayv1.Ready))

	updated := getUpdatedCluster(t, r)
	// Should mark as killed since KillingCondition was not TRUE.
	killedCond := findCondition(updated.Status.StatusConditions, KilledCondition)
	require.NotNil(t, killedCond, "KilledCondition should exist")
	assert.Equal(t, apipb.CONDITION_STATUS_TRUE, killedCond.Status)
	succeededCond := findCondition(updated.Status.StatusConditions, SucceededCondition)
	require.NotNil(t, succeededCond, "SucceededCondition should exist")
	assert.Equal(t, apipb.CONDITION_STATUS_FALSE, succeededCond.Status)
	assert.Equal(t, constants.ClusterKilled, succeededCond.Reason)
	assert.Contains(t, succeededCond.Message, "deleted externally")
	// The backing resource is gone, so the cluster is terminal: State must flip to
	// FAILED and Killing to FALSE, otherwise isClusterFullyTerminal never holds and
	// cleanupCluster keeps issuing DeleteJobCluster against a resource that is gone.
	assert.Equal(t, v2pb.RAY_CLUSTER_STATE_FAILED, updated.Status.State)
	killingCond := findCondition(updated.Status.StatusConditions, KillingCondition)
	require.NotNil(t, killingCond, "KillingCondition should exist")
	assert.Equal(t, apipb.CONDITION_STATUS_FALSE, killingCond.Status)
	// Fully terminal clusters are frozen so the ingester archives them and
	// reconciliation stops for good.
	assert.True(t, utils.IsImmutable(&updated), "externally deleted cluster should be frozen")
}

func TestRayClusterDeleteEventHandler_SkipsImmutable(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status: v2pb.RayClusterStatus{
			State: v2pb.RAY_CLUSTER_STATE_FAILED,
			StatusConditions: []*apipb.Condition{
				{Type: SucceededCondition, Status: apipb.CONDITION_STATUS_FALSE},
			},
		},
	}
	utils.MarkImmutable(globalCluster)
	r := setupWatcherTest(t, globalCluster)

	r.rayClusterDeleteEventHandler(newKubeRayCluster(rayClusterName, rayv1.Ready))

	// A frozen cluster is already terminal and awaiting the ingester; the handler
	// must not write to it and restart reconciliation.
	updated := getUpdatedCluster(t, r)
	assert.Nil(t, findCondition(updated.Status.StatusConditions, KilledCondition),
		"immutable cluster should not be mutated by the delete handler")
}

func TestRayClusterDeleteEventHandler_ExpectedDeletion(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status: v2pb.RayClusterStatus{
			State: v2pb.RAY_CLUSTER_STATE_READY,
			StatusConditions: []*apipb.Condition{
				{Type: KillingCondition, Status: apipb.CONDITION_STATUS_TRUE},
			},
		},
	}
	r := setupWatcherTest(t, globalCluster)

	r.rayClusterDeleteEventHandler(newKubeRayCluster(rayClusterName, rayv1.Ready))

	updated := getUpdatedCluster(t, r)
	killedCond := findCondition(updated.Status.StatusConditions, KilledCondition)
	require.NotNil(t, killedCond, "KilledCondition should exist")
	assert.Equal(t, apipb.CONDITION_STATUS_TRUE, killedCond.Status)
	killingCond := findCondition(updated.Status.StatusConditions, KillingCondition)
	require.NotNil(t, killingCond, "KillingCondition should exist")
	assert.Equal(t, apipb.CONDITION_STATUS_FALSE, killingCond.Status)
	// Succeeded is still UNKNOWN here, so the cluster is not fully terminal yet and
	// must NOT be frozen — Reconcile's termination branch settles Succeeded and
	// freezes it on a later pass.
	assert.False(t, utils.IsImmutable(&updated),
		"cluster with unsettled Succeeded should not be frozen yet")
}

func TestRayClusterDeleteEventHandler_ExpectedDeletionFreezesWhenTerminal(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status: v2pb.RayClusterStatus{
			State: v2pb.RAY_CLUSTER_STATE_READY,
			StatusConditions: []*apipb.Condition{
				{Type: KillingCondition, Status: apipb.CONDITION_STATUS_TRUE},
				{Type: SucceededCondition, Status: apipb.CONDITION_STATUS_TRUE},
			},
		},
	}
	r := setupWatcherTest(t, globalCluster)

	r.rayClusterDeleteEventHandler(newKubeRayCluster(rayClusterName, rayv1.Ready))

	updated := getUpdatedCluster(t, r)
	// Settled Succeeded + Killing=FALSE + Killed=TRUE is fully terminal, so the delete
	// event — not just Reconcile's termination branch — has to freeze the object,
	// otherwise the ingester never archives it and it reconciles forever.
	assert.True(t, utils.IsImmutable(&updated), "fully terminal cluster should be frozen")
}

func TestRayClusterDeleteEventHandler_Tombstone(t *testing.T) {
	r := newTestLogger(t)
	// Should handle tombstone events with an unusable payload gracefully.
	tombstone := cache.DeletedFinalStateUnknown{Key: "default/test-cluster", Obj: nil}
	r.rayClusterDeleteEventHandler(tombstone)
}

func TestRayClusterAddEventHandler(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status:     v2pb.RayClusterStatus{State: v2pb.RAY_CLUSTER_STATE_PROVISIONING},
	}
	r := setupWatcherTest(t, globalCluster)

	r.rayClusterAddEventHandler(newKubeRayCluster(rayClusterName, rayv1.Ready))

	updated := getUpdatedCluster(t, r)
	assert.Equal(t, v2pb.RAY_CLUSTER_STATE_READY, updated.Status.State)
}

func TestRayClusterUpdateEventHandler(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status:     v2pb.RayClusterStatus{State: v2pb.RAY_CLUSTER_STATE_PROVISIONING},
	}
	r := setupWatcherTest(t, globalCluster)

	oldLocal := newKubeRayCluster(rayClusterName, "")
	newLocal := newKubeRayCluster(rayClusterName, rayv1.Ready)
	r.rayClusterUpdateEventHandler(oldLocal, newLocal)

	updated := getUpdatedCluster(t, r)
	assert.Equal(t, v2pb.RAY_CLUSTER_STATE_READY, updated.Status.State)
}

// Pod event handler tests.

func TestPodEventHandler_UpdatesHeadNode(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status:     v2pb.RayClusterStatus{State: v2pb.RAY_CLUSTER_STATE_PROVISIONING},
	}
	r := setupWatcherTest(t, globalCluster)

	r.podAddEventHandler(newHeadPod("head-pod", rayClusterName, corev1.PodRunning))

	updated := getUpdatedCluster(t, r)
	require.NotNil(t, updated.Status.HeadNode)
	assert.Equal(t, "10.0.0.1", updated.Status.HeadNode.Ip)
	assert.Equal(t, int32(10001), updated.Status.HeadNode.ClientPort)
	assert.Equal(t, int32(8888), updated.Status.HeadNode.JupyterNotebookPort)
}

// TestPodEventHandler_MissingPortAnnotationsAreNotErrors asserts that a head pod
// without the optional dynamic port annotations is still published, with -1 for
// the ports it does not expose, and that nothing is logged as an error.
//
// getPort reports an absent annotation as an error, but a cluster that publishes
// neither port is a valid configuration. Treating that as an error logged two
// error lines for every head pod event of every cluster, healthy ones included,
// which drowns out the errors that do mean something.
func TestPodEventHandler_MissingPortAnnotationsAreNotErrors(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status:     v2pb.RayClusterStatus{State: v2pb.RAY_CLUSTER_STATE_PROVISIONING},
	}
	r, _, logs := setupWatcherTestObserved(t, globalCluster)

	pod := newHeadPod("head-pod", rayClusterName, corev1.PodRunning)
	pod.Annotations = nil

	r.podAddEventHandler(pod)

	assert.Empty(t, errorLogs(logs),
		"an unpublished optional port is a valid configuration, not an error")

	updated := getUpdatedCluster(t, r)
	require.NotNil(t, updated.Status.HeadNode, "the head node must still be published")
	assert.Equal(t, "10.0.0.1", updated.Status.HeadNode.Ip)
	assert.Equal(t, int32(-1), updated.Status.HeadNode.ClientPort)
	assert.Equal(t, int32(-1), updated.Status.HeadNode.JupyterNotebookPort)
}

func TestPodAddEventHandler_IgnoresNonHeadPod(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status:     v2pb.RayClusterStatus{State: v2pb.RAY_CLUSTER_STATE_PROVISIONING},
	}
	r := setupWatcherTest(t, globalCluster)

	pod := newHeadPod("worker-pod", rayClusterName, corev1.PodRunning)
	pod.Labels[constants.RayNodeTypeLabelKey] = "worker"
	r.podAddEventHandler(pod)

	updated := getUpdatedCluster(t, r)
	assert.Nil(t, updated.Status.HeadNode, "non-head pod should not set HeadNode")
}

// TestHandlePodEvent_SkipsNonRunningPod goes through handlePodEvent rather than
// podEventHandler because that is where the phase gate lives: connection
// details need a running head pod, while errors are collected regardless.
func TestHandlePodEvent_SkipsNonRunningPod(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status:     v2pb.RayClusterStatus{State: v2pb.RAY_CLUSTER_STATE_PROVISIONING},
	}
	r := setupWatcherTest(t, globalCluster)

	r.handlePodEvent(newHeadPod("head-pod", rayClusterName, corev1.PodPending))

	updated := getUpdatedCluster(t, r)
	assert.Nil(t, updated.Status.HeadNode, "non-running head pod should not set HeadNode")
}

// TestHandlePodEvent_RecordsErrorOnLivePod covers the gap this watcher closes.
// A container stuck in ImagePullBackOff keeps its pod alive forever, so
// podDeleteEventHandler never fires; without the live path the failure would
// never reach the CR and the cluster would sit in UNKNOWN indefinitely.
func TestHandlePodEvent_RecordsErrorOnLivePod(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status:     v2pb.RayClusterStatus{State: v2pb.RAY_CLUSTER_STATE_PROVISIONING},
	}
	r := setupWatcherTest(t, globalCluster)

	pod := newHeadPod("head-pod", rayClusterName, corev1.PodPending)
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{
		{
			Name: headContainerName,
			State: corev1.ContainerState{
				Waiting: &corev1.ContainerStateWaiting{
					Reason:  "ImagePullBackOff",
					Message: `Back-off pulling image "does-not-exist:nope"`,
				},
			},
		},
	}
	r.handlePodEvent(pod)

	updated := getUpdatedCluster(t, r)
	require.Len(t, updated.Status.PodErrors, 1)
	assert.Equal(t, "head-pod", updated.Status.PodErrors[0].Name)
	assert.Equal(t, "ImagePullBackOff", updated.Status.PodErrors[0].Reason)
	assert.Equal(t, headContainerName, updated.Status.PodErrors[0].ContainerName)
	assert.True(t, jobsutils.HasTerminalPodErrors(updated.Status.PodErrors),
		"ImagePullBackOff must be terminal so the cluster can converge on FAILED")
}

// TestHandlePodEvent_RepeatedEventsDoNotDuplicate guards the idempotency the
// merge buys us. A wedged pod re-reports on every status change and the
// informer re-syncs every _reSyncPeriod, so an append-based write would grow
// the list without bound.
func TestHandlePodEvent_RepeatedEventsDoNotDuplicate(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status:     v2pb.RayClusterStatus{State: v2pb.RAY_CLUSTER_STATE_PROVISIONING},
	}
	r := setupWatcherTest(t, globalCluster)

	pod := newHeadPod("head-pod", rayClusterName, corev1.PodPending)
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{
		{
			Name:  headContainerName,
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ErrImagePull"}},
		},
	}
	r.handlePodEvent(pod)

	pod.Status.ContainerStatuses[0].State.Waiting.Reason = "ImagePullBackOff"
	r.handlePodEvent(pod)
	r.handlePodEvent(pod)

	updated := getUpdatedCluster(t, r)
	require.Len(t, updated.Status.PodErrors, 1, "re-reports must update in place")
	assert.Equal(t, "ImagePullBackOff", updated.Status.PodErrors[0].Reason)
}

// TestHandlePodEvent_HealthyStartupRecordsNothing is the counterweight: the
// live path must not mistake a pod on its way up for a failure. A starting pod
// reports ContainersReady=False with reason ContainersNotReady, which is why
// the live path uses the container-only extraction.
func TestHandlePodEvent_HealthyStartupRecordsNothing(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status:     v2pb.RayClusterStatus{State: v2pb.RAY_CLUSTER_STATE_PROVISIONING},
	}
	r := setupWatcherTest(t, globalCluster)

	pod := newHeadPod("head-pod", rayClusterName, corev1.PodPending)
	pod.Status.Conditions = []corev1.PodCondition{
		{
			Type:    corev1.ContainersReady,
			Status:  corev1.ConditionFalse,
			Reason:  "ContainersNotReady",
			Message: "containers with unready status: [ray-head]",
		},
	}
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{
		{
			Name:  headContainerName,
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ContainerCreating"}},
		},
	}
	r.handlePodEvent(pod)

	updated := getUpdatedCluster(t, r)
	assert.Empty(t, updated.Status.PodErrors, "a pod that is still starting is not a failure")
}

// TestHandlePodEvent_IgnoresCollectorSidecar pins the one name michelangelo
// does control. The collector is appended to every ray pod by the mapper, and
// its troubles are the platform's, not the cluster's.
func TestHandlePodEvent_IgnoresCollectorSidecar(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status:     v2pb.RayClusterStatus{State: v2pb.RAY_CLUSTER_STATE_PROVISIONING},
	}
	r := setupWatcherTest(t, globalCluster)

	pod := newHeadPod("head-pod", rayClusterName, corev1.PodPending)
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{
		{
			Name:  constants.CollectorContainerName,
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}},
		},
		{
			Name:  headContainerName,
			State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
		},
	}
	r.handlePodEvent(pod)

	updated := getUpdatedCluster(t, r)
	assert.Empty(t, updated.Status.PodErrors, "a sick sidecar is not a cluster failure")
}

// TestHandlePodEvent_RecordsInitContainerError covers the worker case. KubeRay
// builds wait-gcs-ready from the job's own image, so a bad image wedges the
// init container while every regular container still reads PodInitializing.
func TestHandlePodEvent_RecordsInitContainerError(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status:     v2pb.RayClusterStatus{State: v2pb.RAY_CLUSTER_STATE_PROVISIONING},
	}
	r := setupWatcherTest(t, globalCluster)

	pod := newHeadPod("worker-pod", rayClusterName, corev1.PodPending)
	pod.Labels[constants.RayNodeTypeLabelKey] = constants.RayWorkerNodeType
	pod.Status.InitContainerStatuses = []corev1.ContainerStatus{
		{
			Name:  "wait-gcs-ready",
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}},
		},
	}
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{
		{
			Name:  headContainerName,
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "PodInitializing"}},
		},
	}
	r.handlePodEvent(pod)

	updated := getUpdatedCluster(t, r)
	require.Len(t, updated.Status.PodErrors, 1)
	assert.Equal(t, "wait-gcs-ready", updated.Status.PodErrors[0].ContainerName)
	assert.Equal(t, "ImagePullBackOff", updated.Status.PodErrors[0].Reason)
}

func TestPodDeleteEventHandler_RecordsPodError(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status:     v2pb.RayClusterStatus{State: v2pb.RAY_CLUSTER_STATE_PROVISIONING},
	}
	r := setupWatcherTest(t, globalCluster)

	pod := newHeadPod("head-pod", rayClusterName, corev1.PodFailed)
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{
		{
			Name: headContainerName,
			State: corev1.ContainerState{
				Terminated: &corev1.ContainerStateTerminated{
					ExitCode: 1,
					Reason:   "CrashLoopBackOff",
					Message:  "container crashed",
				},
			},
		},
	}
	r.podDeleteEventHandler(pod)

	updated := getUpdatedCluster(t, r)
	require.Len(t, updated.Status.PodErrors, 1)
	assert.Equal(t, "CrashLoopBackOff", updated.Status.PodErrors[0].Reason)
	assert.Equal(t, headContainerName, updated.Status.PodErrors[0].ContainerName)
}

func TestPodDeleteEventHandler_Tombstone(t *testing.T) {
	r := newTestLogger(t)
	// Unusable tombstone payload should be handled gracefully.
	r.podDeleteEventHandler(cache.DeletedFinalStateUnknown{Key: "default/head-pod", Obj: nil})
}

// setupWatcherTestWithoutGlobal builds a Reconciler whose store holds no global
// RayCluster, and returns a counter of every write the handlers attempt.
//
// Counting the writes is what makes the assertion meaningful: asserting only
// that the handler did not error would also pass if it went on to resurrect the
// cluster, since the fake client happily creates an object on write.
func setupWatcherTestWithoutGlobal(t *testing.T) (*Reconciler, *int, *observer.ObservedLogs) {
	t.Helper()
	return setupWatcherTestObserved(t, nil)
}

// setupWatcherTestObserved is setupWatcherTest with the write count and the log
// output exposed, for assertions about what a handler did *not* do. Pass a nil
// globalCluster to simulate a CR that is already gone.
func setupWatcherTestObserved(t *testing.T, globalCluster *v2pb.RayCluster) (*Reconciler, *int, *observer.ObservedLogs) {
	t.Helper()

	scheme := runtime.NewScheme()
	kubescheme.AddToScheme(scheme)
	v2pb.AddToScheme(scheme)

	objects := []runtime.Object{}
	if globalCluster != nil {
		objects = append(objects, globalCluster)
	}

	writes := 0
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objects...).
		WithStatusSubresource(&v2pb.RayCluster{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				writes++
				return c.Update(ctx, obj, opts...)
			},
			SubResourceUpdate: func(ctx context.Context, c client.Client, subResource string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				writes++
				return c.SubResource(subResource).Update(ctx, obj, opts...)
			},
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				writes++
				return c.Patch(ctx, obj, patch, opts...)
			},
		}).
		Build()

	// Tee the test logger into an observer so the assertions can inspect the
	// severity the handlers logged at, not just whether they wrote.
	observed, logs := observer.New(zapcore.DebugLevel)
	zapLogger := zaptest.NewLogger(t).WithOptions(
		zap.WrapCore(func(core zapcore.Core) zapcore.Core {
			return zapcore.NewTee(core, observed)
		}))

	return &Reconciler{
		Handler: &mockAPIHandler{Client: fakeClient},
		logger:  zapr.NewLogger(zapLogger),
		mapper:  newTestMapper(),
	}, &writes, logs
}

// errorLogs returns the message of every error-level entry captured, so a
// failing assertion names the offending log line instead of only counting it.
func errorLogs(logs *observer.ObservedLogs) []string {
	var msgs []string
	for _, entry := range logs.FilterLevelExact(zapcore.ErrorLevel).All() {
		msgs = append(msgs, entry.Message)
	}
	return msgs
}

// TestWatcherHandlersIgnoreMissingGlobalCluster asserts that every handler
// treats a missing global RayCluster as an expected, silent no-op.
//
// The remote objects outlive the global CR during teardown: the ingester
// archives a terminal cluster out of etcd while KubeRay keeps its own
// RayCluster until TTL cleanup, and the pods outlive both. Events keep
// arriving through that window, and there is nothing left to record them on.
func TestWatcherHandlersIgnoreMissingGlobalCluster(t *testing.T) {
	newFailingPod := func(phase corev1.PodPhase) *corev1.Pod {
		pod := newHeadPod("head-pod", rayClusterName, phase)
		pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
			Name: headContainerName,
			State: corev1.ContainerState{
				Waiting: &corev1.ContainerStateWaiting{
					Reason:  "ImagePullBackOff",
					Message: `Back-off pulling image "nope:latest"`,
				},
			},
		}}
		return pod
	}

	tests := []struct {
		name  string
		event func(r *Reconciler)
	}{
		{
			// recordPodError, reached from the live-pod path.
			name:  "container error on a live pod",
			event: func(r *Reconciler) { r.handlePodEvent(newFailingPod(corev1.PodPending)) },
		},
		{
			// podEventHandler, which publishes the head pod's endpoints.
			name: "running head pod",
			event: func(r *Reconciler) {
				r.handlePodEvent(newHeadPod("head-pod", rayClusterName, corev1.PodRunning))
			},
		},
		{
			// recordPodError, reached from the delete path.
			name:  "pod delete",
			event: func(r *Reconciler) { r.podDeleteEventHandler(newFailingPod(corev1.PodFailed)) },
		},
		{
			name:  "ray cluster update",
			event: func(r *Reconciler) { r.rayClusterEventHandler(newKubeRayCluster(rayClusterName, rayv1.Ready)) },
		},
		{
			name: "ray cluster delete",
			event: func(r *Reconciler) {
				r.rayClusterDeleteEventHandler(newKubeRayCluster(rayClusterName, rayv1.Ready))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, writes, logs := setupWatcherTestWithoutGlobal(t)

			require.NotPanics(t, func() { tt.event(r) })

			assert.Zero(t, *writes, "a global cluster that no longer exists must not be written to")
			assert.Empty(t, errorLogs(logs),
				"losing the race against teardown is routine and must not be logged as an error")
		})
	}
}
