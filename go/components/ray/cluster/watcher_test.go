package cluster

import (
	"context"
	"testing"

	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// _unhealthyState is the KubeRay cluster state string for unhealthy clusters.
// KubeRay does not export a typed constant for it.
const _unhealthyState = rayv1.ClusterState("unhealthy")

func newTestLogger(t *testing.T) *Reconciler {
	zapLog := zaptest.NewLogger(t)
	logger := zapr.NewLogger(zapLog)

	scheme := runtime.NewScheme()
	kubescheme.AddToScheme(scheme)
	v2pb.AddToScheme(scheme)

	return &Reconciler{
		logger: logger,
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

	launchedCond := findCondition(updated.Status.StatusConditions, LaunchedCondition)
	require.NotNil(t, launchedCond)
	assert.Equal(t, apipb.CONDITION_STATUS_TRUE, launchedCond.Status)
	assert.Equal(t, "ClusterReady", launchedCond.Reason)
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

func TestPodEventHandler_SkipsNonRunningPod(t *testing.T) {
	globalCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1},
		Status:     v2pb.RayClusterStatus{State: v2pb.RAY_CLUSTER_STATE_PROVISIONING},
	}
	r := setupWatcherTest(t, globalCluster)

	r.podEventHandler(newHeadPod("head-pod", rayClusterName, corev1.PodPending))

	updated := getUpdatedCluster(t, r)
	assert.Nil(t, updated.Status.HeadNode, "non-running head pod should not set HeadNode")
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
			Name: constants.HeadContainerName,
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
	assert.Equal(t, constants.HeadContainerName, updated.Status.PodErrors[0].ContainerName)
}

func TestPodDeleteEventHandler_Tombstone(t *testing.T) {
	r := newTestLogger(t)
	// Unusable tombstone payload should be handled gracefully.
	r.podDeleteEventHandler(cache.DeletedFinalStateUnknown{Key: "default/head-pod", Obj: nil})
}
