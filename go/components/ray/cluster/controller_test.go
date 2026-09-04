package cluster

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/clientmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	matypes "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
)

const (
	rayClusterName  = "test-cluster"
	testNamespace   = "default"
	assignedCluster = "cluster-1"
)

// mockClusterCache is a test double for RegisteredClustersCache
type mockClusterCache struct {
	clusters map[string]*v2pb.Cluster
}

func newMockClusterCache() *mockClusterCache {
	return &mockClusterCache{
		clusters: make(map[string]*v2pb.Cluster),
	}
}

func (m *mockClusterCache) GetCluster(name string) *v2pb.Cluster {
	return m.clusters[name]
}

func (m *mockClusterCache) GetClusters(filter cluster.FilterType) []*v2pb.Cluster {
	clusters := make([]*v2pb.Cluster, 0, len(m.clusters))
	for _, c := range m.clusters {
		clusters = append(clusters, c)
	}
	return clusters
}

func (m *mockClusterCache) addCluster(name string, cluster *v2pb.Cluster) {
	m.clusters[name] = cluster
}

// mockSchedulerQueue is a test double for JobQueue
type mockSchedulerQueue struct {
	enqueueFunc func(ctx context.Context, job matypes.SchedulableJob) error
}

func (m *mockSchedulerQueue) Enqueue(ctx context.Context, job matypes.SchedulableJob) error {
	if m.enqueueFunc != nil {
		return m.enqueueFunc(ctx, job)
	}
	return nil
}

// mockAPIHandler wraps a fake client to provide api.Handler interface
type mockAPIHandler struct {
	client.Client
}

func (m *mockAPIHandler) Get(ctx context.Context, namespace, name string, opts *metav1.GetOptions, obj client.Object) error {
	return m.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, obj)
}

func (m *mockAPIHandler) UpdateStatus(ctx context.Context, obj client.Object, opts *metav1.UpdateOptions) error {
	return m.Client.Status().Update(ctx, obj)
}

func (m *mockAPIHandler) Update(ctx context.Context, obj client.Object, opts *metav1.UpdateOptions) error {
	return m.Client.Update(ctx, obj)
}

func (m *mockAPIHandler) Create(ctx context.Context, obj client.Object, opts *metav1.CreateOptions) error {
	return m.Client.Create(ctx, obj)
}

func (m *mockAPIHandler) Delete(ctx context.Context, obj client.Object, opts *metav1.DeleteOptions) error {
	return m.Client.Delete(ctx, obj)
}

func (m *mockAPIHandler) List(ctx context.Context, namespace string, opts *metav1.ListOptions, listOptionsExt *apipb.ListOptionsExt, list client.ObjectList) error {
	return m.Client.List(ctx, list, &client.ListOptions{Namespace: namespace})
}

func (m *mockAPIHandler) DeleteCollection(ctx context.Context, objType client.Object, namespace string, deleteOpts *metav1.DeleteOptions, listOpts *metav1.ListOptions) error {
	return nil
}

func (m *mockAPIHandler) Watch(ctx context.Context, namespace string, opts *metav1.ListOptions, obj client.ObjectList) (interface{}, error) {
	return nil, nil
}

// Helper function to create a basic RayCluster spec
func createRayClusterSpec() v2pb.RayClusterSpec {
	return v2pb.RayClusterSpec{
		RayVersion: "2.3.1",
		Head: &v2pb.RayHeadSpec{
			ServiceType: "clusterIP",
			Pod: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "test",
							Image: "test",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
						},
					},
				},
			},
		},
		Workers: []*v2pb.RayWorkerSpec{
			{
				Pod: &corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "test",
								Image: "test",
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("1"),
										corev1.ResourceMemory: resource.MustParse("1Gi"),
									},
								},
							},
						},
					},
				},
				MinInstances: 1,
				MaxInstances: 1,
			},
		},
	}
}

func TestReconcilerReconcile(t *testing.T) {
	ctx := context.Background()

	// Mock environment
	scheme := runtime.NewScheme()
	kubescheme.AddToScheme(scheme)
	v2pb.AddToScheme(scheme)

	// Test cases
	tests := []struct {
		name             string
		setup            func() []client.Object
		setupMocks       func(*clientmocks.MockFederatedClient, *mockClusterCache, *mockSchedulerQueue)
		expectedState    v2pb.RayClusterState
		expectedMessage  string
		errorAssertion   require.ErrorAssertionFunc
		postCheck        func(res ctrl.Result)
		verifyConditions func(t *testing.T, cluster *v2pb.RayCluster)
	}{
		{
			name: "No ray cluster",
			setup: func() []client.Object {
				return make([]client.Object, 0)
			},
			setupMocks:      func(mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache, msq *mockSchedulerQueue) {},
			expectedState:   v2pb.RAY_CLUSTER_STATE_INVALID,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, time.Duration(0), res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, cluster *v2pb.RayCluster) {},
		},
		{
			name: "Cluster should be enqueued",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				cluster := &v2pb.RayCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:       rayClusterName,
						Namespace:  testNamespace,
						Generation: 1,
					},
					Spec: createRayClusterSpec(),
				}
				objects = append(objects, cluster)
				return objects
			},
			setupMocks: func(mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache, msq *mockSchedulerQueue) {
				msq.enqueueFunc = func(ctx context.Context, job matypes.SchedulableJob) error {
					return nil
				}
			},
			expectedState:   v2pb.RAY_CLUSTER_STATE_INVALID,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				// Should requeue to wait for scheduling
				assert.Equal(t, requeueAfter, res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, cluster *v2pb.RayCluster) {
				// Verify EnqueuedCondition is TRUE
				var enqueuedCond *apipb.Condition
				for _, cond := range cluster.Status.StatusConditions {
					if cond.Type == EnqueuedCondition {
						enqueuedCond = cond
						break
					}
				}
				assert.NotNil(t, enqueuedCond, "EnqueuedCondition should exist")
				assert.Equal(t, apipb.CONDITION_STATUS_TRUE, enqueuedCond.Status)
			},
		},
		{
			name: "Cluster not yet scheduled - waiting",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				cluster := &v2pb.RayCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:       rayClusterName,
						Namespace:  testNamespace,
						Generation: 1,
					},
					Spec: createRayClusterSpec(),
					Status: v2pb.RayClusterStatus{
						StatusConditions: []*apipb.Condition{
							{
								Type:   EnqueuedCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
						},
					},
				}
				objects = append(objects, cluster)
				return objects
			},
			setupMocks: func(mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache, msq *mockSchedulerQueue) {
				msq.enqueueFunc = func(ctx context.Context, job matypes.SchedulableJob) error {
					return matypes.ErrJobAlreadyExists
				}
			},
			expectedState:   v2pb.RAY_CLUSTER_STATE_INVALID,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, requeueAfter, res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, cluster *v2pb.RayCluster) {},
		},
		{
			name: "Cluster scheduled and assigned - cluster created successfully",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				cluster := &v2pb.RayCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:       rayClusterName,
						Namespace:  testNamespace,
						Generation: 1,
					},
					Spec: createRayClusterSpec(),
					Status: v2pb.RayClusterStatus{
						StatusConditions: []*apipb.Condition{
							{
								Type:   EnqueuedCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   ScheduledCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
						},
						Assignment: &v2pb.AssignmentInfo{
							Cluster: assignedCluster,
						},
					},
				}
				objects = append(objects, cluster)
				return objects
			},
			setupMocks: func(mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache, msq *mockSchedulerQueue) {
				mcc.addCluster(assignedCluster, &v2pb.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: assignedCluster,
					},
				})
				mfc.EXPECT().CreateJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			},
			expectedState:   v2pb.RAY_CLUSTER_STATE_PROVISIONING,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, requeueAfter, res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, cluster *v2pb.RayCluster) {
				// Verify LaunchedCondition is TRUE
				var launchedCond *apipb.Condition
				for _, cond := range cluster.Status.StatusConditions {
					if cond.Type == LaunchedCondition {
						launchedCond = cond
						break
					}
				}
				assert.NotNil(t, launchedCond, "LaunchedCondition should exist")
				assert.Equal(t, apipb.CONDITION_STATUS_TRUE, launchedCond.Status)
			},
		},
		{
			name: "Cluster already exists - should not fail",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				cluster := &v2pb.RayCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:       rayClusterName,
						Namespace:  testNamespace,
						Generation: 1,
					},
					Spec: createRayClusterSpec(),
					Status: v2pb.RayClusterStatus{
						StatusConditions: []*apipb.Condition{
							{
								Type:   EnqueuedCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   ScheduledCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
						},
						Assignment: &v2pb.AssignmentInfo{
							Cluster: assignedCluster,
						},
					},
				}
				objects = append(objects, cluster)
				return objects
			},
			setupMocks: func(mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache, msq *mockSchedulerQueue) {
				mcc.addCluster(assignedCluster, &v2pb.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: assignedCluster,
					},
				})
				mfc.EXPECT().CreateJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(
					apiErrors.NewAlreadyExists(schema.GroupResource{Group: "ray.io", Resource: "rayclusters"}, rayClusterName))
			},
			expectedState:   v2pb.RAY_CLUSTER_STATE_PROVISIONING,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, requeueAfter, res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, cluster *v2pb.RayCluster) {
				// Should still mark as launched even if already exists
				var launchedCond *apipb.Condition
				for _, cond := range cluster.Status.StatusConditions {
					if cond.Type == LaunchedCondition {
						launchedCond = cond
						break
					}
				}
				assert.NotNil(t, launchedCond, "LaunchedCondition should exist")
				assert.Equal(t, apipb.CONDITION_STATUS_TRUE, launchedCond.Status)
			},
		},
		{
			name: "Cluster creation fails",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				cluster := &v2pb.RayCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:       rayClusterName,
						Namespace:  testNamespace,
						Generation: 1,
					},
					Spec: createRayClusterSpec(),
					Status: v2pb.RayClusterStatus{
						StatusConditions: []*apipb.Condition{
							{
								Type:   EnqueuedCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   ScheduledCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
						},
						Assignment: &v2pb.AssignmentInfo{
							Cluster: assignedCluster,
						},
					},
				}
				objects = append(objects, cluster)
				return objects
			},
			setupMocks: func(mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache, msq *mockSchedulerQueue) {
				mcc.addCluster(assignedCluster, &v2pb.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: assignedCluster,
					},
				})
				mfc.EXPECT().CreateJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("failed to create cluster"))
			},
			expectedState:   v2pb.RAY_CLUSTER_STATE_FAILED,
			expectedMessage: "",
			errorAssertion:  require.NoError, // Status update succeeds, so no error is returned
			postCheck: func(res ctrl.Result) {
				// Should requeue to allow retry
				assert.Equal(t, requeueAfter, res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, cluster *v2pb.RayCluster) {
				// Verify SucceededCondition is FALSE
				var succeededCond *apipb.Condition
				for _, cond := range cluster.Status.StatusConditions {
					if cond.Type == SucceededCondition {
						succeededCond = cond
						break
					}
				}
				assert.NotNil(t, succeededCond, "SucceededCondition should exist")
				assert.Equal(t, apipb.CONDITION_STATUS_FALSE, succeededCond.Status)
				assert.False(t, utils.IsImmutable(cluster),
					"cluster should not be immutable yet - cleanup has not run")
			},
		},
		{
			name: "Cluster launched - monitoring ready state",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				cluster := &v2pb.RayCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:       rayClusterName,
						Namespace:  testNamespace,
						Generation: 1,
					},
					Spec: createRayClusterSpec(),
					Status: v2pb.RayClusterStatus{
						State: v2pb.RAY_CLUSTER_STATE_PROVISIONING,
						StatusConditions: []*apipb.Condition{
							{
								Type:   EnqueuedCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   ScheduledCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   LaunchedCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
						},
						Assignment: &v2pb.AssignmentInfo{
							Cluster: assignedCluster,
						},
					},
				}
				objects = append(objects, cluster)
				return objects
			},
			setupMocks: func(mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache, msq *mockSchedulerQueue) {
				mcc.addCluster(assignedCluster, &v2pb.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: assignedCluster,
					},
				})
				mfc.EXPECT().GetJobClusterStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(
					&matypes.JobClusterStatus{
						Ray: &v2pb.RayClusterStatus{State: v2pb.RAY_CLUSTER_STATE_READY},
					}, nil)
			},
			expectedState:   v2pb.RAY_CLUSTER_STATE_READY,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, time.Duration(0), res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, cluster *v2pb.RayCluster) {},
		},
		{
			name: "Cluster launched - failed state",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				cluster := &v2pb.RayCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:       rayClusterName,
						Namespace:  testNamespace,
						Generation: 1,
					},
					Spec: createRayClusterSpec(),
					Status: v2pb.RayClusterStatus{
						State: v2pb.RAY_CLUSTER_STATE_PROVISIONING,
						StatusConditions: []*apipb.Condition{
							{
								Type:   EnqueuedCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   ScheduledCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   LaunchedCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
						},
						Assignment: &v2pb.AssignmentInfo{
							Cluster: assignedCluster,
						},
					},
				}
				objects = append(objects, cluster)
				return objects
			},
			setupMocks: func(mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache, msq *mockSchedulerQueue) {
				mcc.addCluster(assignedCluster, &v2pb.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: assignedCluster,
					},
				})
				mfc.EXPECT().GetJobClusterStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(
					&matypes.JobClusterStatus{
						Ray: &v2pb.RayClusterStatus{State: v2pb.RAY_CLUSTER_STATE_FAILED},
					}, nil)
			},
			expectedState:   v2pb.RAY_CLUSTER_STATE_FAILED,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, requeueAfter, res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, cluster *v2pb.RayCluster) {
				// Verify SucceededCondition is FALSE
				var succeededCond *apipb.Condition
				for _, cond := range cluster.Status.StatusConditions {
					if cond.Type == SucceededCondition {
						succeededCond = cond
						break
					}
				}
				assert.NotNil(t, succeededCond, "SucceededCondition should exist")
				assert.Equal(t, apipb.CONDITION_STATUS_FALSE, succeededCond.Status)
				assert.False(t, utils.IsImmutable(cluster),
					"cluster should not be immutable yet - this is a transient state on the way to termination")
			},
		},
		{
			name: "Cluster launched - suspended state keeps monitoring",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				cluster := &v2pb.RayCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:       rayClusterName,
						Namespace:  testNamespace,
						Generation: 1,
					},
					Spec: createRayClusterSpec(),
					Status: v2pb.RayClusterStatus{
						State: v2pb.RAY_CLUSTER_STATE_PROVISIONING,
						StatusConditions: []*apipb.Condition{
							{
								Type:   EnqueuedCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   ScheduledCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   LaunchedCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
						},
						Assignment: &v2pb.AssignmentInfo{
							Cluster: assignedCluster,
						},
					},
				}
				objects = append(objects, cluster)
				return objects
			},
			setupMocks: func(mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache, msq *mockSchedulerQueue) {
				mcc.addCluster(assignedCluster, &v2pb.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: assignedCluster,
					},
				})
				// Suspension (e.g. Kueue admission gating) is non-terminal: even
				// with a terminal-looking pod error attached, a suspended cluster
				// must keep being monitored rather than be marked failed.
				mfc.EXPECT().GetJobClusterStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(
					&matypes.JobClusterStatus{
						Ray: &v2pb.RayClusterStatus{
							State: v2pb.RAY_CLUSTER_STATE_SUSPENDED,
							PodErrors: []*v2pb.PodErrors{
								{
									Name:   "HeadPodReady",
									Reason: "HeadPodNotFound",
								},
							},
						},
						Reason: "ClusterSuspended",
					}, nil)
			},
			expectedState:   v2pb.RAY_CLUSTER_STATE_SUSPENDED,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, requeueAfter, res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, cluster *v2pb.RayCluster) {
				for _, cond := range cluster.Status.StatusConditions {
					if cond.Type == SucceededCondition {
						assert.NotEqual(t, apipb.CONDITION_STATUS_FALSE, cond.Status,
							"suspended cluster must not be marked failed")
					}
				}
			},
		},
		{
			name: "Cluster termination - succeeded type",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				cluster := &v2pb.RayCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:       rayClusterName,
						Namespace:  testNamespace,
						Generation: 1,
					},
					Spec: v2pb.RayClusterSpec{
						RayVersion: "2.3.1",
						Head:       &v2pb.RayHeadSpec{},
						Termination: &v2pb.TerminationSpec{
							Type:   v2pb.TERMINATION_TYPE_SUCCEEDED,
							Reason: "job completed successfully",
						},
					},
					Status: v2pb.RayClusterStatus{
						State: v2pb.RAY_CLUSTER_STATE_READY,
					},
				}
				objects = append(objects, cluster)
				return objects
			},
			setupMocks:      func(mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache, msq *mockSchedulerQueue) {},
			expectedState:   v2pb.RAY_CLUSTER_STATE_READY,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, time.Duration(0), res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, cluster *v2pb.RayCluster) {
				// Verify SucceededCondition is set to TRUE
				var succeededCond *apipb.Condition
				for _, cond := range cluster.Status.StatusConditions {
					if cond.Type == SucceededCondition {
						succeededCond = cond
						break
					}
				}
				assert.NotNil(t, succeededCond, "SucceededCondition should exist")
				assert.Equal(t, apipb.CONDITION_STATUS_TRUE, succeededCond.Status)
				assert.False(t, utils.IsImmutable(cluster),
					"cluster should not be immutable yet - kill/cleanup has not converged")
			},
		},
		{
			name: "Cluster termination - failed type",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				cluster := &v2pb.RayCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:       rayClusterName,
						Namespace:  testNamespace,
						Generation: 1,
					},
					Spec: v2pb.RayClusterSpec{
						RayVersion: "2.3.1",
						Head:       &v2pb.RayHeadSpec{},
						Termination: &v2pb.TerminationSpec{
							Type:   v2pb.TERMINATION_TYPE_FAILED,
							Reason: "job failed",
						},
					},
					Status: v2pb.RayClusterStatus{
						State: v2pb.RAY_CLUSTER_STATE_READY,
					},
				}
				objects = append(objects, cluster)
				return objects
			},
			setupMocks:      func(mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache, msq *mockSchedulerQueue) {},
			expectedState:   v2pb.RAY_CLUSTER_STATE_READY,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, time.Duration(0), res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, cluster *v2pb.RayCluster) {
				// Verify SucceededCondition is set to FALSE
				var succeededCond *apipb.Condition
				for _, cond := range cluster.Status.StatusConditions {
					if cond.Type == SucceededCondition {
						succeededCond = cond
						break
					}
				}
				assert.NotNil(t, succeededCond, "SucceededCondition should exist")
				assert.Equal(t, apipb.CONDITION_STATUS_FALSE, succeededCond.Status)
				assert.False(t, utils.IsImmutable(cluster),
					"cluster should not be immutable yet - kill/cleanup has not converged")
			},
		},
		{
			name: "Cluster cleanup - not scheduled",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				cluster := &v2pb.RayCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:       rayClusterName,
						Namespace:  testNamespace,
						Generation: 1,
					},
					Spec: v2pb.RayClusterSpec{
						RayVersion: "2.3.1",
						Head:       &v2pb.RayHeadSpec{},
						Termination: &v2pb.TerminationSpec{
							Type:   v2pb.TERMINATION_TYPE_SUCCEEDED,
							Reason: "completed",
						},
					},
					Status: v2pb.RayClusterStatus{
						StatusConditions: []*apipb.Condition{
							{
								Type:   SucceededCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   KillingCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
						},
					},
				}
				objects = append(objects, cluster)
				return objects
			},
			setupMocks:      func(mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache, msq *mockSchedulerQueue) {},
			expectedState:   v2pb.RAY_CLUSTER_STATE_TERMINATED,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, time.Duration(0), res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, cluster *v2pb.RayCluster) {
				// Verify KilledCondition is TRUE
				var killedCond *apipb.Condition
				for _, cond := range cluster.Status.StatusConditions {
					if cond.Type == KilledCondition {
						killedCond = cond
						break
					}
				}
				assert.NotNil(t, killedCond, "KilledCondition should exist")
				assert.Equal(t, apipb.CONDITION_STATUS_TRUE, killedCond.Status)
				assert.True(t, utils.IsImmutable(cluster),
					"cluster should be marked immutable once fully terminated")
			},
		},
		{
			name: "Cluster cleanup - scheduled but not launched",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				cluster := &v2pb.RayCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:       rayClusterName,
						Namespace:  testNamespace,
						Generation: 1,
					},
					Spec: v2pb.RayClusterSpec{
						RayVersion: "2.3.1",
						Head:       &v2pb.RayHeadSpec{},
						Termination: &v2pb.TerminationSpec{
							Type:   v2pb.TERMINATION_TYPE_SUCCEEDED,
							Reason: "completed",
						},
					},
					Status: v2pb.RayClusterStatus{
						StatusConditions: []*apipb.Condition{
							{
								Type:   ScheduledCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   SucceededCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   KillingCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
						},
						Assignment: &v2pb.AssignmentInfo{
							Cluster: assignedCluster,
						},
					},
				}
				objects = append(objects, cluster)
				return objects
			},
			setupMocks: func(mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache, msq *mockSchedulerQueue) {
				mcc.addCluster(assignedCluster, &v2pb.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: assignedCluster,
					},
				})
			},
			expectedState:   v2pb.RAY_CLUSTER_STATE_TERMINATED,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, time.Duration(0), res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, cluster *v2pb.RayCluster) {
				// Verify KilledCondition is TRUE
				var killedCond *apipb.Condition
				for _, cond := range cluster.Status.StatusConditions {
					if cond.Type == KilledCondition {
						killedCond = cond
						break
					}
				}
				assert.NotNil(t, killedCond, "KilledCondition should exist")
				assert.Equal(t, apipb.CONDITION_STATUS_TRUE, killedCond.Status)
				assert.True(t, utils.IsImmutable(cluster),
					"cluster should be marked immutable once fully terminated")
			},
		},
		{
			name: "Cluster cleanup - launched",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				cluster := &v2pb.RayCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:       rayClusterName,
						Namespace:  testNamespace,
						Generation: 1,
					},
					Spec: v2pb.RayClusterSpec{
						RayVersion: "2.3.1",
						Head:       &v2pb.RayHeadSpec{},
						Termination: &v2pb.TerminationSpec{
							Type:   v2pb.TERMINATION_TYPE_SUCCEEDED,
							Reason: "completed",
						},
					},
					Status: v2pb.RayClusterStatus{
						StatusConditions: []*apipb.Condition{
							{
								Type:   ScheduledCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   LaunchedCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   SucceededCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   KillingCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
						},
						Assignment: &v2pb.AssignmentInfo{
							Cluster: assignedCluster,
						},
					},
				}
				objects = append(objects, cluster)
				return objects
			},
			setupMocks: func(mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache, msq *mockSchedulerQueue) {
				mcc.addCluster(assignedCluster, &v2pb.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: assignedCluster,
					},
				})
				mfc.EXPECT().DeleteJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			},
			expectedState:   v2pb.RAY_CLUSTER_STATE_TERMINATED,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, time.Duration(0), res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, cluster *v2pb.RayCluster) {
				// Verify KilledCondition is TRUE and KillingCondition is FALSE
				var killedCond, killingCond *apipb.Condition
				for _, cond := range cluster.Status.StatusConditions {
					if cond.Type == KilledCondition {
						killedCond = cond
					}
					if cond.Type == KillingCondition {
						killingCond = cond
					}
				}
				assert.NotNil(t, killedCond, "KilledCondition should exist")
				assert.Equal(t, apipb.CONDITION_STATUS_TRUE, killedCond.Status)
				assert.NotNil(t, killingCond, "KillingCondition should exist")
				assert.Equal(t, apipb.CONDITION_STATUS_FALSE, killingCond.Status)
				assert.True(t, utils.IsImmutable(cluster),
					"cluster should be marked immutable once fully terminated")
			},
		},
		{
			name: "Cluster cleanup - already deleted",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				cluster := &v2pb.RayCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:       rayClusterName,
						Namespace:  testNamespace,
						Generation: 1,
					},
					Spec: v2pb.RayClusterSpec{
						RayVersion: "2.3.1",
						Head:       &v2pb.RayHeadSpec{},
						Termination: &v2pb.TerminationSpec{
							Type:   v2pb.TERMINATION_TYPE_SUCCEEDED,
							Reason: "completed",
						},
					},
					Status: v2pb.RayClusterStatus{
						StatusConditions: []*apipb.Condition{
							{
								Type:   ScheduledCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   LaunchedCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   SucceededCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   KillingCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
						},
						Assignment: &v2pb.AssignmentInfo{
							Cluster: assignedCluster,
						},
					},
				}
				objects = append(objects, cluster)
				return objects
			},
			setupMocks: func(mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache, msq *mockSchedulerQueue) {
				mcc.addCluster(assignedCluster, &v2pb.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: assignedCluster,
					},
				})
				mfc.EXPECT().DeleteJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(
					apiErrors.NewNotFound(schema.GroupResource{Group: "ray.io", Resource: "rayclusters"}, rayClusterName))
			},
			expectedState:   v2pb.RAY_CLUSTER_STATE_TERMINATED,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, time.Duration(0), res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, cluster *v2pb.RayCluster) {
				// Verify KilledCondition is TRUE
				var killedCond *apipb.Condition
				for _, cond := range cluster.Status.StatusConditions {
					if cond.Type == KilledCondition {
						killedCond = cond
						break
					}
				}
				assert.NotNil(t, killedCond, "KilledCondition should exist")
				assert.Equal(t, apipb.CONDITION_STATUS_TRUE, killedCond.Status)
				assert.True(t, utils.IsImmutable(cluster),
					"cluster should be marked immutable once fully terminated")
			},
		},
		{
			name: "Cluster UNKNOWN with terminal pod errors escalated to FAILED",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				cluster := &v2pb.RayCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:       rayClusterName,
						Namespace:  testNamespace,
						Generation: 1,
					},
					Spec: createRayClusterSpec(),
					Status: v2pb.RayClusterStatus{
						State: v2pb.RAY_CLUSTER_STATE_PROVISIONING,
						StatusConditions: []*apipb.Condition{
							{
								Type:   EnqueuedCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   ScheduledCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   LaunchedCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
						},
						Assignment: &v2pb.AssignmentInfo{
							Cluster: assignedCluster,
						},
					},
				}
				objects = append(objects, cluster)
				return objects
			},
			setupMocks: func(mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache, msq *mockSchedulerQueue) {
				mcc.addCluster(assignedCluster, &v2pb.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: assignedCluster,
					},
				})
				mfc.EXPECT().GetJobClusterStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(
					&matypes.JobClusterStatus{
						Ray: &v2pb.RayClusterStatus{
							State: v2pb.RAY_CLUSTER_STATE_UNKNOWN,
							PodErrors: []*v2pb.PodErrors{
								{
									Name:    "HeadPodReady",
									Reason:  "CrashLoopBackOff",
									Message: "container ray-head is crashing",
								},
							},
						},
						Reason: "CrashLoopBackOff",
					}, nil)
			},
			expectedState:   v2pb.RAY_CLUSTER_STATE_FAILED,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, requeueAfter, res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, cluster *v2pb.RayCluster) {
				var succeededCond *apipb.Condition
				for _, cond := range cluster.Status.StatusConditions {
					if cond.Type == SucceededCondition {
						succeededCond = cond
						break
					}
				}
				assert.NotNil(t, succeededCond, "SucceededCondition should exist")
				assert.Equal(t, apipb.CONDITION_STATUS_FALSE, succeededCond.Status)
				assert.Equal(t, "CrashLoopBackOff", succeededCond.Reason)
				assert.Len(t, cluster.Status.PodErrors, 1)
				assert.Equal(t, "CrashLoopBackOff", cluster.Status.PodErrors[0].Reason)
				assert.False(t, utils.IsImmutable(cluster),
					"cluster should not be immutable yet - this is a transient state on the way to termination")
			},
		},
		{
			name: "Cluster UNKNOWN with non-terminal pod errors populates PodErrors but stays UNKNOWN",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				cluster := &v2pb.RayCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:       rayClusterName,
						Namespace:  testNamespace,
						Generation: 1,
					},
					Spec: createRayClusterSpec(),
					Status: v2pb.RayClusterStatus{
						State: v2pb.RAY_CLUSTER_STATE_PROVISIONING,
						StatusConditions: []*apipb.Condition{
							{
								Type:   EnqueuedCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   ScheduledCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   LaunchedCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
						},
						Assignment: &v2pb.AssignmentInfo{
							Cluster: assignedCluster,
						},
					},
				}
				objects = append(objects, cluster)
				return objects
			},
			setupMocks: func(mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache, msq *mockSchedulerQueue) {
				mcc.addCluster(assignedCluster, &v2pb.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: assignedCluster,
					},
				})
				mfc.EXPECT().GetJobClusterStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(
					&matypes.JobClusterStatus{
						Ray: &v2pb.RayClusterStatus{
							State: v2pb.RAY_CLUSTER_STATE_UNKNOWN,
							PodErrors: []*v2pb.PodErrors{
								{
									Name:    "HeadPodReady",
									Reason:  "ContainersNotReady",
									Message: "containers with unready status: [head]",
								},
							},
						},
						Reason: "ContainersNotReady",
					}, nil)
			},
			expectedState:   v2pb.RAY_CLUSTER_STATE_UNKNOWN,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, requeueAfter, res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, cluster *v2pb.RayCluster) {
				assert.Len(t, cluster.Status.PodErrors, 1)
				assert.Equal(t, "ContainersNotReady", cluster.Status.PodErrors[0].Reason)
			},
		},
		{
			// A freshly-created RayCluster whose head pod has not been scheduled
			// yet reports HeadPodReady=False / HeadPodNotFound. This must be
			// treated as transient (keep monitoring), not terminal: otherwise the
			// controller races KubeRay and tears the cluster down before the head
			// pod ever exists.
			name: "Cluster UNKNOWN with HeadPodNotFound stays UNKNOWN (head pod not yet scheduled)",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				cluster := &v2pb.RayCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:       rayClusterName,
						Namespace:  testNamespace,
						Generation: 1,
					},
					Spec: createRayClusterSpec(),
					Status: v2pb.RayClusterStatus{
						State: v2pb.RAY_CLUSTER_STATE_PROVISIONING,
						StatusConditions: []*apipb.Condition{
							{
								Type:   EnqueuedCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   ScheduledCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   LaunchedCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
						},
						Assignment: &v2pb.AssignmentInfo{
							Cluster: assignedCluster,
						},
					},
				}
				objects = append(objects, cluster)
				return objects
			},
			setupMocks: func(mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache, msq *mockSchedulerQueue) {
				mcc.addCluster(assignedCluster, &v2pb.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: assignedCluster,
					},
				})
				mfc.EXPECT().GetJobClusterStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(
					&matypes.JobClusterStatus{
						Ray: &v2pb.RayClusterStatus{
							State: v2pb.RAY_CLUSTER_STATE_UNKNOWN,
							PodErrors: []*v2pb.PodErrors{
								{
									Name:    "HeadPodReady",
									Reason:  "HeadPodNotFound",
									Message: "Head Pod not found",
								},
							},
						},
						Reason: "HeadPodNotFound",
					}, nil)
			},
			expectedState:   v2pb.RAY_CLUSTER_STATE_UNKNOWN,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, requeueAfter, res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, cluster *v2pb.RayCluster) {
				assert.Len(t, cluster.Status.PodErrors, 1)
				assert.Equal(t, "HeadPodNotFound", cluster.Status.PodErrors[0].Reason)
				for _, cond := range cluster.Status.StatusConditions {
					if cond.Type == SucceededCondition {
						assert.NotEqual(t, apipb.CONDITION_STATUS_FALSE, cond.Status,
							"SucceededCondition must not be marked FALSE for a transient HeadPodNotFound")
					}
				}
			},
		},
		{
			name: "Cluster UNKNOWN without terminal pod errors continues monitoring",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				cluster := &v2pb.RayCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:       rayClusterName,
						Namespace:  testNamespace,
						Generation: 1,
					},
					Spec: createRayClusterSpec(),
					Status: v2pb.RayClusterStatus{
						State: v2pb.RAY_CLUSTER_STATE_PROVISIONING,
						StatusConditions: []*apipb.Condition{
							{
								Type:   EnqueuedCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   ScheduledCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   LaunchedCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
						},
						Assignment: &v2pb.AssignmentInfo{
							Cluster: assignedCluster,
						},
					},
				}
				objects = append(objects, cluster)
				return objects
			},
			setupMocks: func(mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache, msq *mockSchedulerQueue) {
				mcc.addCluster(assignedCluster, &v2pb.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: assignedCluster,
					},
				})
				mfc.EXPECT().GetJobClusterStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(
					&matypes.JobClusterStatus{
						Ray: &v2pb.RayClusterStatus{
							State: v2pb.RAY_CLUSTER_STATE_UNKNOWN,
						},
					}, nil)
			},
			expectedState:   v2pb.RAY_CLUSTER_STATE_UNKNOWN,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, requeueAfter, res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, cluster *v2pb.RayCluster) {
				for _, cond := range cluster.Status.StatusConditions {
					if cond.Type == SucceededCondition {
						assert.Equal(t, apipb.CONDITION_STATUS_UNKNOWN, cond.Status,
							"SucceededCondition should stay UNKNOWN when no terminal errors")
					}
				}
			},
		},
		{
			name: "Cluster missing on remote compute cluster is marked failed and immutable",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				cluster := &v2pb.RayCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:       rayClusterName,
						Namespace:  testNamespace,
						Generation: 1,
					},
					Spec: createRayClusterSpec(),
					Status: v2pb.RayClusterStatus{
						State: v2pb.RAY_CLUSTER_STATE_PROVISIONING,
						StatusConditions: []*apipb.Condition{
							{
								Type:   EnqueuedCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   ScheduledCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   LaunchedCondition,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
						},
						Assignment: &v2pb.AssignmentInfo{
							Cluster: assignedCluster,
						},
					},
				}
				objects = append(objects, cluster)
				return objects
			},
			setupMocks: func(mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache, msq *mockSchedulerQueue) {
				mcc.addCluster(assignedCluster, &v2pb.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: assignedCluster,
					},
				})
				// The RayCluster has vanished from the remote compute cluster.
				mfc.EXPECT().GetJobClusterStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(
					nil, apiErrors.NewNotFound(
						schema.GroupResource{Group: "ray.io", Resource: "rayclusters"}, rayClusterName))
			},
			expectedState:   v2pb.RAY_CLUSTER_STATE_FAILED,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				// Terminal condition: the controller must not requeue.
				assert.Equal(t, time.Duration(0), res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, cluster *v2pb.RayCluster) {
				var succeededCond, killedCond *apipb.Condition
				for _, cond := range cluster.Status.StatusConditions {
					switch cond.Type {
					case SucceededCondition:
						succeededCond = cond
					case KilledCondition:
						killedCond = cond
					}
				}
				require.NotNil(t, succeededCond, "SucceededCondition should exist")
				assert.Equal(t, apipb.CONDITION_STATUS_FALSE, succeededCond.Status,
					"SucceededCondition should be FALSE for a failed cluster")
				require.NotNil(t, killedCond, "KilledCondition should exist")
				assert.Equal(t, apipb.CONDITION_STATUS_TRUE, killedCond.Status,
					"KilledCondition should be TRUE for a terminal cluster")
				assert.True(t, utils.IsImmutable(cluster),
					"cluster should be marked immutable so reconciliation stops")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			objects := tc.setup()
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).WithStatusSubresource(objects...).Build()

			mockFedClient := clientmocks.NewMockFederatedClient(mockCtrl)
			mockCache := newMockClusterCache()
			mockQueue := &mockSchedulerQueue{}
			tc.setupMocks(mockFedClient, mockCache, mockQueue)

			apiHandler := &mockAPIHandler{Client: fakeClient}

			r := &Reconciler{
				Handler:         apiHandler,
				federatedClient: mockFedClient,
				clusterCache:    mockCache,
				schedulerQueue:  mockQueue,
			}

			requestRayCluster := types.NamespacedName{
				Name:      rayClusterName,
				Namespace: testNamespace,
			}

			// Act
			res, err := r.Reconcile(ctx, ctrl.Request{
				NamespacedName: requestRayCluster,
			})

			// Assert
			tc.errorAssertion(t, err)
			tc.postCheck(res)

			var updatedRayCluster v2pb.RayCluster
			getErr := fakeClient.Get(ctx, requestRayCluster, &updatedRayCluster)
			if getErr == nil {
				assert.Equal(t, tc.expectedState, updatedRayCluster.Status.State)
				if tc.expectedMessage != "" {
					assert.Contains(t, updatedRayCluster.Status.String(), tc.expectedMessage)
				}
				tc.verifyConditions(t, &updatedRayCluster)
			}
		})
	}
}

// TestReconcilerReconcile_KillConvergesToImmutable exercises a normal (non-NotFound)
// termination across multiple reconciles: the first reconcile sets Succeeded and Killing
// but cleanup has not run yet, so the object must stay mutable; only once cleanup
// converges on Killing=FALSE/Killed=TRUE on the following reconcile should the object be
// frozen. This is the "mark immutable for all terminal states" path, as opposed to the
// single-shot NotFound-on-remote path covered by "Cluster missing on remote compute
// cluster is marked failed and immutable" above.
func TestReconcilerReconcile_KillConvergesToImmutable(t *testing.T) {
	ctx := context.Background()

	scheme := runtime.NewScheme()
	kubescheme.AddToScheme(scheme)
	v2pb.AddToScheme(scheme)

	cluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:       rayClusterName,
			Namespace:  testNamespace,
			Generation: 1,
		},
		Spec: v2pb.RayClusterSpec{
			RayVersion: "2.3.1",
			Head:       &v2pb.RayHeadSpec{},
			Termination: &v2pb.TerminationSpec{
				Type:   v2pb.TERMINATION_TYPE_SUCCEEDED,
				Reason: "completed",
			},
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).WithStatusSubresource(cluster).Build()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockFedClient := clientmocks.NewMockFederatedClient(mockCtrl)

	r := &Reconciler{
		Handler:         &mockAPIHandler{Client: fakeClient},
		federatedClient: mockFedClient,
		clusterCache:    newMockClusterCache(),
		schedulerQueue:  &mockSchedulerQueue{},
	}

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: rayClusterName, Namespace: testNamespace}}

	// First reconcile: Succeeded and Killing get set, but the cluster was never
	// scheduled/launched so cleanup only sees the change on the next pass.
	_, err := r.Reconcile(ctx, req)
	require.NoError(t, err)

	var afterFirst v2pb.RayCluster
	require.NoError(t, fakeClient.Get(ctx, req.NamespacedName, &afterFirst))
	assert.NotEqual(t, v2pb.RAY_CLUSTER_STATE_TERMINATED, afterFirst.Status.State)
	assert.False(t, utils.IsImmutable(&afterFirst),
		"cluster should not be immutable yet - cleanup has not converged")

	// Second reconcile: cleanup now observes Killing=TRUE, completes, and the object
	// should be frozen immediately after.
	_, err = r.Reconcile(ctx, req)
	require.NoError(t, err)

	var afterSecond v2pb.RayCluster
	require.NoError(t, fakeClient.Get(ctx, req.NamespacedName, &afterSecond))
	assert.Equal(t, v2pb.RAY_CLUSTER_STATE_TERMINATED, afterSecond.Status.State)
	assert.True(t, utils.IsImmutable(&afterSecond),
		"cluster should be marked immutable once fully terminated")
}
