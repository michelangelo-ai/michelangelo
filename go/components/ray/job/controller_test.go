package job

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/clientmocks"
	jobscluster "github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	jobsutils "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	rayJobName      = "test-job"
	testNamespace   = "default"
	testClusterName = "test-cluster"
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

func (m *mockClusterCache) GetClusters(filter jobscluster.FilterType) []*v2pb.Cluster {
	clusters := make([]*v2pb.Cluster, 0, len(m.clusters))
	for _, cluster := range m.clusters {
		clusters = append(clusters, cluster)
	}
	return clusters
}

func (m *mockClusterCache) addCluster(name string, cluster *v2pb.Cluster) {
	m.clusters[name] = cluster
}

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	kubescheme.AddToScheme(scheme)
	v2pb.AddToScheme(scheme)
	return scheme
}

func TestReconciler_Reconcile(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme()

	tests := []struct {
		name             string
		setup            func() []client.Object
		setupMocks       func(*gomock.Controller, *clientmocks.MockFederatedClient, *mockClusterCache)
		expectedState    v2pb.RayJobState
		expectedMessage  string
		errorAssertion   require.ErrorAssertionFunc
		postCheck        func(res ctrl.Result)
		verifyConditions func(t *testing.T, job *v2pb.RayJob)
	}{
		{
			name: "No ray job",
			setup: func() []client.Object {
				return []client.Object{}
			},
			setupMocks:      func(ctrl *gomock.Controller, mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache) {},
			expectedState:   v2pb.RAY_JOB_STATE_INVALID,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, time.Duration(0), res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, job *v2pb.RayJob) {},
		},
		{
			name: "Cluster not set",
			setup: func() []client.Object {
				return []client.Object{
					&v2pb.RayJob{
						ObjectMeta: metav1.ObjectMeta{
							Name:       rayJobName,
							Namespace:  testNamespace,
							Generation: 1,
						},
						Spec: v2pb.RayJobSpec{Cluster: nil},
					},
				}
			},
			setupMocks:      func(ctrl *gomock.Controller, mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache) {},
			expectedState:   v2pb.RAY_JOB_STATE_FAILED,
			expectedMessage: "cluster is not set",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, time.Duration(0), res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, job *v2pb.RayJob) {},
		},
		{
			// An immutable RayJob must be skipped entirely. A non-immutable job with
			// no cluster set would be marked FAILED ("cluster is not set"); a job left
			// in its seeded INVALID state with no status conditions and no requeue
			// proves reconciliation was short-circuited by the immutable check.
			name: "immutable job is skipped",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				rayJob := &v2pb.RayJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:        rayJobName,
						Namespace:   testNamespace,
						Generation:  1,
						Annotations: map[string]string{api.ImmutableAnnotation: "true"},
					},
					Spec: v2pb.RayJobSpec{
						Cluster: nil,
					},
				}
				objects = append(objects, rayJob)
				return objects
			},
			setupMocks:      func(ctrl *gomock.Controller, mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache) {},
			expectedState:   v2pb.RAY_JOB_STATE_INVALID,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, time.Duration(0), res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, job *v2pb.RayJob) {
				assert.Empty(t, job.Status.StatusConditions, "immutable job must not be reconciled")
			},
		},
		{
			name: "Cluster not found",
			setup: func() []client.Object {
				return []client.Object{
					&v2pb.RayJob{
						ObjectMeta: metav1.ObjectMeta{
							Name:       rayJobName,
							Namespace:  testNamespace,
							Generation: 1,
						},
						Spec: v2pb.RayJobSpec{
							Cluster: &apipb.ResourceIdentifier{
								Name:      "missing-cluster",
								Namespace: testNamespace,
							},
						},
					},
				}
			},
			setupMocks:      func(ctrl *gomock.Controller, mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache) {},
			expectedState:   v2pb.RAY_JOB_STATE_FAILED,
			expectedMessage: "failed to find cluster",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, time.Duration(0), res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, job *v2pb.RayJob) {},
		},
		{
			name: "cluster is not ready",
			setup: func() []client.Object {
				return []client.Object{
					&v2pb.RayJob{
						ObjectMeta: metav1.ObjectMeta{
							Name:       rayJobName,
							Namespace:  testNamespace,
							Generation: 1,
						},
						Spec: v2pb.RayJobSpec{
							Cluster: &apipb.ResourceIdentifier{
								Name:      "existing-cluster",
								Namespace: testNamespace,
							},
							Entrypoint: "echo Hello World",
						},
					},
					&v2pb.RayCluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:       "existing-cluster",
							Namespace:  testNamespace,
							Generation: 1,
						},
						Status: v2pb.RayClusterStatus{
							State: v2pb.RAY_CLUSTER_STATE_PROVISIONING,
						},
					},
				}
			},
			setupMocks:      func(ctrl *gomock.Controller, mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache) {},
			expectedState:   v2pb.RAY_JOB_STATE_INITIALIZING,
			expectedMessage: "cluster default/existing-cluster is not ready",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, requeueAfter, res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, job *v2pb.RayJob) {},
		},
		{
			name: "cluster is ready but not assigned",
			setup: func() []client.Object {
				return []client.Object{
					&v2pb.RayJob{
						ObjectMeta: metav1.ObjectMeta{
							Name:       rayJobName,
							Namespace:  testNamespace,
							Generation: 1,
						},
						Spec: v2pb.RayJobSpec{
							Cluster: &apipb.ResourceIdentifier{
								Name:      "existing-cluster",
								Namespace: testNamespace,
							},
							Entrypoint: "echo Hello World",
						},
					},
					&v2pb.RayCluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:       "existing-cluster",
							Namespace:  testNamespace,
							Generation: 1,
						},
						Status: v2pb.RayClusterStatus{
							State:      v2pb.RAY_CLUSTER_STATE_READY,
							Assignment: nil,
						},
					},
				}
			},
			setupMocks:      func(ctrl *gomock.Controller, mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache) {},
			expectedState:   v2pb.RAY_JOB_STATE_INVALID,
			expectedMessage: "waiting for RayCluster assignment",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, requeueAfter, res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, job *v2pb.RayJob) {},
		},
		{
			name: "cluster assigned but not in cache",
			setup: func() []client.Object {
				return []client.Object{
					&v2pb.RayJob{
						ObjectMeta: metav1.ObjectMeta{
							Name:       rayJobName,
							Namespace:  testNamespace,
							Generation: 1,
						},
						Spec: v2pb.RayJobSpec{
							Cluster: &apipb.ResourceIdentifier{
								Name:      "existing-cluster",
								Namespace: testNamespace,
							},
							Entrypoint: "echo Hello World",
						},
					},
					&v2pb.RayCluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:       "existing-cluster",
							Namespace:  testNamespace,
							Generation: 1,
						},
						Status: v2pb.RayClusterStatus{
							State: v2pb.RAY_CLUSTER_STATE_READY,
							Assignment: &v2pb.AssignmentInfo{
								Cluster: "missing-cluster",
							},
						},
					},
				}
			},
			setupMocks:      func(ctrl *gomock.Controller, mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache) {},
			expectedState:   v2pb.RAY_JOB_STATE_INVALID,
			expectedMessage: "waiting for RayCluster assignment",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, requeueAfter, res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, job *v2pb.RayJob) {},
		},
		{
			name: "cluster is ready and assigned - job created successfully",
			setup: func() []client.Object {
				return []client.Object{
					&v2pb.RayJob{
						ObjectMeta: metav1.ObjectMeta{
							Name:       rayJobName,
							Namespace:  testNamespace,
							Generation: 1,
						},
						Spec: v2pb.RayJobSpec{
							Cluster: &apipb.ResourceIdentifier{
								Name:      "existing-cluster",
								Namespace: testNamespace,
							},
							Entrypoint: "echo Hello World",
						},
					},
					&v2pb.RayCluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:       "existing-cluster",
							Namespace:  testNamespace,
							Generation: 1,
						},
						Status: v2pb.RayClusterStatus{
							State: v2pb.RAY_CLUSTER_STATE_READY,
							Assignment: &v2pb.AssignmentInfo{
								Cluster: assignedCluster,
							},
						},
						Spec: v2pb.RayClusterSpec{
							Head: &v2pb.RayHeadSpec{
								Pod: &corev1.PodTemplateSpec{
									Spec: corev1.PodSpec{
										Containers: []corev1.Container{},
									},
								},
							},
						},
					},
				}
			},
			setupMocks: func(ctrl *gomock.Controller, mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache) {
				mcc.addCluster(assignedCluster, &v2pb.Cluster{
					ObjectMeta: metav1.ObjectMeta{Name: assignedCluster},
				})
				mfc.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			},
			expectedState:   v2pb.RAY_JOB_STATE_INITIALIZING,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, requeueAfter, res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, job *v2pb.RayJob) {
				var launchedCond *apipb.Condition
				for _, cond := range job.GetStatus().StatusConditions {
					if cond.Type == "Launched" {
						launchedCond = cond
						break
					}
				}
				assert.NotNil(t, launchedCond, "LaunchedCondition should exist")
				assert.Equal(t, apipb.CONDITION_STATUS_TRUE, launchedCond.Status)
			},
		},
		{
			name: "job creation fails",
			setup: func() []client.Object {
				return []client.Object{
					&v2pb.RayJob{
						ObjectMeta: metav1.ObjectMeta{
							Name:       rayJobName,
							Namespace:  testNamespace,
							Generation: 1,
						},
						Spec: v2pb.RayJobSpec{
							Cluster: &apipb.ResourceIdentifier{
								Name:      "existing-cluster",
								Namespace: testNamespace,
							},
							Entrypoint: "echo Hello World",
						},
					},
					&v2pb.RayCluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:       "existing-cluster",
							Namespace:  testNamespace,
							Generation: 1,
						},
						Status: v2pb.RayClusterStatus{
							State: v2pb.RAY_CLUSTER_STATE_READY,
							Assignment: &v2pb.AssignmentInfo{
								Cluster: assignedCluster,
							},
						},
					},
				}
			},
			setupMocks: func(ctrl *gomock.Controller, mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache) {
				mcc.addCluster(assignedCluster, &v2pb.Cluster{
					ObjectMeta: metav1.ObjectMeta{Name: assignedCluster},
				})
				mfc.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("failed to create job"))
			},
			expectedState:   v2pb.RAY_JOB_STATE_FAILED,
			expectedMessage: "failed to create ray job",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, requeueAfter, res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, job *v2pb.RayJob) {},
		},
		{
			name: "job already exists - should not fail",
			setup: func() []client.Object {
				objects := make([]client.Object, 0)
				rayJob := &v2pb.RayJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:       rayJobName,
						Namespace:  testNamespace,
						Generation: 1,
					},
					Spec: v2pb.RayJobSpec{
						Cluster: &apipb.ResourceIdentifier{
							Name:      "existing-cluster",
							Namespace: testNamespace,
						},
						Entrypoint: "echo Hello World",
					},
				}
				cluster := &v2pb.RayCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "existing-cluster",
						Namespace:  testNamespace,
						Generation: 1,
					},
					Status: v2pb.RayClusterStatus{
						State: v2pb.RAY_CLUSTER_STATE_READY,
						Assignment: &v2pb.AssignmentInfo{
							Cluster: assignedCluster,
						},
					},
				}
				objects = append(objects, rayJob)
				objects = append(objects, cluster)
				return objects
			},
			setupMocks: func(ctrl *gomock.Controller, mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache) {
				mcc.addCluster(assignedCluster, &v2pb.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: assignedCluster,
					},
				})
				mfc.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(apiErrors.NewAlreadyExists(schema.GroupResource{Group: "ray.io", Resource: "rayjobs"}, rayJobName))
			},
			expectedState:   v2pb.RAY_JOB_STATE_INITIALIZING,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, requeueAfter, res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, job *v2pb.RayJob) {
				// Should still mark as launched even if already exists
				var launchedCond *apipb.Condition
				for _, cond := range job.Status.StatusConditions {
					if cond.Type == "Launched" {
						launchedCond = cond
						break
					}
				}
				assert.NotNil(t, launchedCond, "LaunchedCondition should exist")
				assert.Equal(t, apipb.CONDITION_STATUS_TRUE, launchedCond.Status)
			},
		},
		{
			name: "job already launched - requeues for watcher updates",
			setup: func() []client.Object {
				return []client.Object{
					&v2pb.RayJob{
						ObjectMeta: metav1.ObjectMeta{
							Name:       rayJobName,
							Namespace:  testNamespace,
							Generation: 1,
						},
						Spec: v2pb.RayJobSpec{
							Cluster: &apipb.ResourceIdentifier{
								Name:      "existing-cluster",
								Namespace: testNamespace,
							},
							Entrypoint: "echo Hello World",
						},
						Status: v2pb.RayJobStatus{
							State: v2pb.RAY_JOB_STATE_INITIALIZING,
							StatusConditions: []*apipb.Condition{
								{
									Type:   "Launched",
									Status: apipb.CONDITION_STATUS_TRUE,
								},
							},
						},
					},
					&v2pb.RayCluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:       "existing-cluster",
							Namespace:  testNamespace,
							Generation: 1,
						},
						Status: v2pb.RayClusterStatus{
							State: v2pb.RAY_CLUSTER_STATE_READY,
							Assignment: &v2pb.AssignmentInfo{
								Cluster: assignedCluster,
							},
						},
					},
				}
			},
			setupMocks: func(ctrl *gomock.Controller, mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache) {
				mcc.addCluster(assignedCluster, &v2pb.Cluster{
					ObjectMeta: metav1.ObjectMeta{Name: assignedCluster},
				})
			},
			expectedState:   v2pb.RAY_JOB_STATE_INITIALIZING,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, requeueAfter, res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, job *v2pb.RayJob) {},
		},
		{
			name: "job in terminal state - does not requeue",
			setup: func() []client.Object {
				return []client.Object{
					&v2pb.RayJob{
						ObjectMeta: metav1.ObjectMeta{
							Name:       rayJobName,
							Namespace:  testNamespace,
							Generation: 1,
						},
						Spec: v2pb.RayJobSpec{
							Cluster: &apipb.ResourceIdentifier{
								Name:      "existing-cluster",
								Namespace: testNamespace,
							},
							Entrypoint: "echo Hello World",
						},
						Status: v2pb.RayJobStatus{
							State: v2pb.RAY_JOB_STATE_SUCCEEDED,
							StatusConditions: []*apipb.Condition{
								{
									Type:   "Launched",
									Status: apipb.CONDITION_STATUS_TRUE,
								},
							},
						},
					},
					&v2pb.RayCluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:       "existing-cluster",
							Namespace:  testNamespace,
							Generation: 1,
						},
						Status: v2pb.RayClusterStatus{
							State: v2pb.RAY_CLUSTER_STATE_READY,
							Assignment: &v2pb.AssignmentInfo{
								Cluster: assignedCluster,
							},
						},
					},
				}
			},
			setupMocks: func(ctrl *gomock.Controller, mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache) {
				mcc.addCluster(assignedCluster, &v2pb.Cluster{
					ObjectMeta: metav1.ObjectMeta{Name: assignedCluster},
				})
			},
			expectedState:   v2pb.RAY_JOB_STATE_SUCCEEDED,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, time.Duration(0), res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, job *v2pb.RayJob) {
				assert.True(t, utils.IsImmutable(job), "terminal job should be marked immutable by reconciler")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			objects := tc.setup()
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).WithStatusSubresource(objects...).Build()

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockFedClient := clientmocks.NewMockFederatedClient(mockCtrl)
			mockCache := newMockClusterCache()
			tc.setupMocks(mockCtrl, mockFedClient, mockCache)

			r := &Reconciler{
				Client:          fakeClient,
				federatedClient: mockFedClient,
				clusterCache:    mockCache,
			}

			requestRayJob := types.NamespacedName{
				Name:      rayJobName,
				Namespace: testNamespace,
			}

			res, err := r.Reconcile(ctx, ctrl.Request{
				NamespacedName: requestRayJob,
			})

			tc.errorAssertion(t, err)
			tc.postCheck(res)

			var updatedRayJob v2pb.RayJob
			_ = r.Get(ctx, requestRayJob, &updatedRayJob)
			if updatedRayJob.Name != "" {
				assert.Equal(t, tc.expectedState, updatedRayJob.Status.State)
				assert.Contains(t, updatedRayJob.Status.Message, tc.expectedMessage)
				tc.verifyConditions(t, &updatedRayJob)
			}
		})
	}
}

// TestReconciler_Reconcile_NoOpSkipsStatusWrite verifies that a reconcile pass
// which produces no actual status change performs no write at all. The
// original dirty-check compared a *v2pb.RayJob to a v2pb.RayJob with
// reflect.DeepEqual, which reports distinct types as unequal regardless of
// content -- so status was rewritten, and a new watch event fired, on every
// single reconcile, even when nothing changed.
func TestReconciler_Reconcile_NoOpSkipsStatusWrite(t *testing.T) {
	ctx := context.Background()

	scheme := runtime.NewScheme()
	kubescheme.AddToScheme(scheme)
	v2pb.AddToScheme(scheme)

	rayJob := &v2pb.RayJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:       rayJobName,
			Namespace:  testNamespace,
			Generation: 1,
		},
		Spec: v2pb.RayJobSpec{
			Cluster: &apipb.ResourceIdentifier{
				Name:      testClusterName,
				Namespace: testNamespace,
			},
			Entrypoint: "echo Hello World",
		},
		Status: v2pb.RayJobStatus{
			State:        v2pb.RAY_JOB_STATE_RUNNING,
			JobStatus:    string(rayv1.JobStatusRunning),
			DashboardUrl: "http://dashboard.example.com",
			StatusConditions: []*apipb.Condition{
				{
					Type:   "Launched",
					Status: apipb.CONDITION_STATUS_TRUE,
				},
			},
		},
	}
	cluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:       testClusterName,
			Namespace:  testNamespace,
			Generation: 1,
		},
		Status: v2pb.RayClusterStatus{
			State: v2pb.RAY_CLUSTER_STATE_READY,
			Assignment: &v2pb.AssignmentInfo{
				Cluster: assignedCluster,
			},
		},
	}

	objects := make([]client.Object, 0)
	objects = append(objects, rayJob)
	objects = append(objects, cluster)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).WithStatusSubresource(objects...).Build()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockFedClient := clientmocks.NewMockFederatedClient(mockCtrl)
	mockCache := newMockClusterCache()
	mockCache.addCluster(assignedCluster, &v2pb.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: assignedCluster,
		},
	})
	// Status now arrives from the federated watcher rather than from a poll
	// inside Reconcile, so the stored status is already current and this pass has
	// nothing to persist. No federated call is expected at all.

	r := &Reconciler{
		Client:          fakeClient,
		federatedClient: mockFedClient,
		clusterCache:    mockCache,
	}

	requestRayJob := types.NamespacedName{
		Name:      rayJobName,
		Namespace: testNamespace,
	}

	var before v2pb.RayJob
	require.NoError(t, r.Get(ctx, requestRayJob, &before))

	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: requestRayJob})
	require.NoError(t, err)

	var after v2pb.RayJob
	require.NoError(t, r.Get(ctx, requestRayJob, &after))

	// A changed resourceVersion means either Status().Update or the plain
	// Update fired -- and neither should, since nothing actually changed.
	assert.Equal(t, before.ResourceVersion, after.ResourceVersion, "no-op reconcile must not write the RayJob")
}

// TestReconciler_Reconcile_RetriesImmutableAnnotationWhenStatusUnchanged
// covers a RayJob that is already terminal but not yet immutable -- e.g. a
// prior reconcile persisted the terminal status but its immutable-annotation
// write failed -- and whose status happens not to change on this pass. The
// immutable check must not be gated behind the status dirty-check, or a job
// like this would stay terminal-but-mutable forever once its status stopped
// changing.
func TestReconciler_Reconcile_RetriesImmutableAnnotationWhenStatusUnchanged(t *testing.T) {
	ctx := context.Background()

	scheme := runtime.NewScheme()
	kubescheme.AddToScheme(scheme)
	v2pb.AddToScheme(scheme)

	rayJob := &v2pb.RayJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:       rayJobName,
			Namespace:  testNamespace,
			Generation: 1,
		},
		Spec: v2pb.RayJobSpec{
			Cluster: &apipb.ResourceIdentifier{
				Name:      testClusterName,
				Namespace: testNamespace,
			},
			Entrypoint: "echo Hello World",
		},
		Status: v2pb.RayJobStatus{
			// Already terminal, and deliberately missing the immutable
			// annotation -- simulating a prior reconcile whose status write
			// succeeded but whose annotation write did not.
			State:     v2pb.RAY_JOB_STATE_SUCCEEDED,
			JobStatus: "SUCCEEDED",
			StatusConditions: []*apipb.Condition{
				{
					Type:   "Launched",
					Status: apipb.CONDITION_STATUS_TRUE,
				},
			},
		},
	}
	cluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:       testClusterName,
			Namespace:  testNamespace,
			Generation: 1,
		},
		Status: v2pb.RayClusterStatus{
			State: v2pb.RAY_CLUSTER_STATE_READY,
			Assignment: &v2pb.AssignmentInfo{
				Cluster: assignedCluster,
			},
		},
	}

	objects := make([]client.Object, 0)
	objects = append(objects, rayJob)
	objects = append(objects, cluster)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).WithStatusSubresource(objects...).Build()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockFedClient := clientmocks.NewMockFederatedClient(mockCtrl)
	mockCache := newMockClusterCache()
	mockCache.addCluster(assignedCluster, &v2pb.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: assignedCluster,
		},
	})
	// The watcher already wrote the terminal status, so there is nothing new for
	// the status dirty-check to catch -- only the pending immutable annotation.

	r := &Reconciler{
		Client:          fakeClient,
		federatedClient: mockFedClient,
		clusterCache:    mockCache,
	}

	requestRayJob := types.NamespacedName{
		Name:      rayJobName,
		Namespace: testNamespace,
	}

	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: requestRayJob})
	require.NoError(t, err)

	var after v2pb.RayJob
	require.NoError(t, r.Get(ctx, requestRayJob, &after))
	assert.True(t, utils.IsImmutable(&after), "already-terminal RayJob must still be marked immutable even when this pass made no status change")
}

// TestRayJobEventHandler verifies KubeRay RayJob execution status is mapped onto
// the global RayJob.
func TestRayJobEventHandler(t *testing.T) {
	scheme := newTestScheme()

	newGlobalRayJob := func() *v2pb.RayJob {
		return &v2pb.RayJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:       rayJobName,
				Namespace:  testNamespace,
				Generation: 1,
			},
			Status: v2pb.RayJobStatus{
				StatusConditions: make([]*apipb.Condition, 0),
			},
		}
	}

	newLocalRayJob := func(status rayv1.JobStatus, deployStatus rayv1.JobDeploymentStatus, msg string) *rayv1.RayJob {
		return &rayv1.RayJob{
			ObjectMeta: metav1.ObjectMeta{
				Name: rayJobName,
				Labels: map[string]string{
					constants.ProjectNameLabelKey: testNamespace,
				},
			},
			Status: rayv1.RayJobStatus{
				JobStatus:           status,
				JobDeploymentStatus: deployStatus,
				Message:             msg,
			},
		}
	}

	newReconciler := func(global *v2pb.RayJob) *Reconciler {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(global).
			WithStatusSubresource(global).
			Build()
		return &Reconciler{Client: fakeClient, logger: ctrl.Log.WithName("test")}
	}

	getUpdated := func(t *testing.T, r *Reconciler) v2pb.RayJob {
		var updated v2pb.RayJob
		require.NoError(t, r.Get(context.Background(),
			types.NamespacedName{Namespace: testNamespace, Name: rayJobName}, &updated))
		return updated
	}

	t.Run("running status maps to RUNNING state", func(t *testing.T) {
		r := newReconciler(newGlobalRayJob())

		r.rayJobEventHandler(newLocalRayJob(rayv1.JobStatusRunning, rayv1.JobDeploymentStatusRunning, "job is running"))

		updated := getUpdated(t, r)
		assert.Equal(t, v2pb.RAY_JOB_STATE_RUNNING, updated.Status.State)
		assert.Equal(t, string(rayv1.JobStatusRunning), updated.Status.JobStatus)
		assert.Equal(t, string(rayv1.JobDeploymentStatusRunning), updated.Status.JobDeploymentStatus)
		assert.Equal(t, "job is running", updated.Status.Message)
	})

	t.Run("succeeded status maps to SUCCEEDED state", func(t *testing.T) {
		r := newReconciler(newGlobalRayJob())

		r.rayJobEventHandler(newLocalRayJob(rayv1.JobStatusSucceeded, rayv1.JobDeploymentStatusComplete, ""))

		updated := getUpdated(t, r)
		assert.Equal(t, v2pb.RAY_JOB_STATE_SUCCEEDED, updated.Status.State)
		assert.Equal(t, string(rayv1.JobStatusSucceeded), updated.Status.JobStatus)
	})

	t.Run("failed status maps to FAILED state", func(t *testing.T) {
		r := newReconciler(newGlobalRayJob())

		r.rayJobEventHandler(newLocalRayJob(rayv1.JobStatusFailed, rayv1.JobDeploymentStatusFailed, "boom"))

		updated := getUpdated(t, r)
		assert.Equal(t, v2pb.RAY_JOB_STATE_FAILED, updated.Status.State)
		assert.Equal(t, "boom", updated.Status.Message)
	})

	t.Run("immutable job is skipped", func(t *testing.T) {
		global := newGlobalRayJob()
		utils.MarkImmutable(global)
		r := newReconciler(global)

		r.rayJobEventHandler(newLocalRayJob(rayv1.JobStatusRunning, rayv1.JobDeploymentStatusRunning, ""))

		updated := getUpdated(t, r)
		assert.NotEqual(t, v2pb.RAY_JOB_STATE_RUNNING, updated.Status.State)
	})

	t.Run("ill-formed object is ignored", func(t *testing.T) {
		r := &Reconciler{logger: ctrl.Log.WithName("test")}
		// Should not panic on non-RayJob object.
		r.rayJobEventHandler("not-a-rayjob")
	})
}

// TestRayJobDeleteEventHandler verifies KubeRay RayJob deletions transition the
// global RayJob to a killed state.
func TestRayJobDeleteEventHandler(t *testing.T) {
	scheme := newTestScheme()

	newLocalRayJob := func() *rayv1.RayJob {
		return &rayv1.RayJob{
			ObjectMeta: metav1.ObjectMeta{
				Name: rayJobName,
				Labels: map[string]string{
					constants.ProjectNameLabelKey: testNamespace,
				},
			},
		}
	}

	t.Run("tombstone is handled gracefully", func(t *testing.T) {
		r := &Reconciler{logger: ctrl.Log.WithName("test")}
		// Unusable tombstone payload should not panic.
		r.rayJobDeleteEventHandler(cache.DeletedFinalStateUnknown{
			Key: "default/deleted-job",
			Obj: nil,
		})
	})

	t.Run("external deletion sets succeeded=FALSE and killed=TRUE", func(t *testing.T) {
		rayJob := &v2pb.RayJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:       rayJobName,
				Namespace:  testNamespace,
				Generation: 1,
			},
			Status: v2pb.RayJobStatus{
				StatusConditions: make([]*apipb.Condition, 0),
			},
		}
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rayJob).
			WithStatusSubresource(rayJob).
			Build()
		r := &Reconciler{Client: fakeClient, logger: ctrl.Log.WithName("test")}

		r.rayJobDeleteEventHandler(newLocalRayJob())

		var updated v2pb.RayJob
		require.NoError(t, r.Get(context.Background(),
			types.NamespacedName{Namespace: testNamespace, Name: rayJobName}, &updated))

		succeeded := jobsutils.GetCondition(&updated.Status.StatusConditions, constants.SucceededCondition, updated.Generation)
		assert.Equal(t, apipb.CONDITION_STATUS_FALSE, succeeded.Status)
		assert.Equal(t, constants.ClusterKilled, succeeded.Reason)

		killed := jobsutils.GetCondition(&updated.Status.StatusConditions, constants.KilledCondition, updated.Generation)
		assert.Equal(t, apipb.CONDITION_STATUS_TRUE, killed.Status)
	})

	t.Run("controller-initiated deletion clears killing and sets killed", func(t *testing.T) {
		rayJob := &v2pb.RayJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:       rayJobName,
				Namespace:  testNamespace,
				Generation: 1,
			},
			Status: v2pb.RayJobStatus{
				StatusConditions: []*apipb.Condition{
					{
						Type:   constants.KillingCondition,
						Status: apipb.CONDITION_STATUS_TRUE,
					},
				},
			},
		}
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rayJob).
			WithStatusSubresource(rayJob).
			Build()
		r := &Reconciler{Client: fakeClient, logger: ctrl.Log.WithName("test")}

		r.rayJobDeleteEventHandler(newLocalRayJob())

		var updated v2pb.RayJob
		require.NoError(t, r.Get(context.Background(),
			types.NamespacedName{Namespace: testNamespace, Name: rayJobName}, &updated))

		killing := jobsutils.GetCondition(&updated.Status.StatusConditions, constants.KillingCondition, updated.Generation)
		assert.Equal(t, apipb.CONDITION_STATUS_FALSE, killing.Status)

		killed := jobsutils.GetCondition(&updated.Status.StatusConditions, constants.KilledCondition, updated.Generation)
		assert.Equal(t, apipb.CONDITION_STATUS_TRUE, killed.Status)
	})
}
