package job

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/zapr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
	"go.uber.org/zap/zaptest/observer"

	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/clientmocks"
	jobscluster "github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	jobtypes "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
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
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
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
			// The watcher owns non-terminal states; the safety net only polls for a
			// terminal one it may have missed.
			name: "job already launched - safety net ignores a non-terminal status",
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
				mfc.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(&jobtypes.JobStatus{
					Ray: &v2pb.RayJobStatus{
						JobStatus: string(rayv1.JobStatusRunning),
						State:     v2pb.RAY_JOB_STATE_RUNNING,
						Message:   "job is running",
					},
				}, nil)
			},
			expectedState:   v2pb.RAY_JOB_STATE_INITIALIZING,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, _safetyNetPeriod, res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, job *v2pb.RayJob) {
				assert.Empty(t, job.Status.JobStatus, "a non-terminal status is the watcher's to write")
			},
		},
		{
			// GetJobStatus maps a KubeRay RayJob missing from the compute cluster to
			// FAILED, e.g. a delete the watcher never saw because the controller
			// was down.
			name: "job already launched - safety net records a missing KubeRay RayJob as failed",
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
							State:     v2pb.RAY_JOB_STATE_RUNNING,
							JobStatus: string(rayv1.JobStatusRunning),
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
				mfc.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(&jobtypes.JobStatus{
					Ray: &v2pb.RayJobStatus{
						JobStatus: "Unknown",
						State:     v2pb.RAY_JOB_STATE_FAILED,
						Message:   "Resource not found in compute cluster",
					},
				}, nil)
			},
			expectedState:   v2pb.RAY_JOB_STATE_FAILED,
			expectedMessage: "Resource not found in compute cluster",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, time.Duration(0), res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, job *v2pb.RayJob) {
				assert.Equal(t, "Unknown", job.Status.JobStatus)
				assert.True(t, utils.IsImmutable(job), "terminal job should be marked immutable by reconciler")
			},
		},
		{
			name: "job already launched - safety net applies a terminal status",
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
							State:     v2pb.RAY_JOB_STATE_RUNNING,
							JobStatus: string(rayv1.JobStatusRunning),
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
				mfc.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(&jobtypes.JobStatus{
					Ray: &v2pb.RayJobStatus{
						JobStatus:           string(rayv1.JobStatusSucceeded),
						JobDeploymentStatus: string(rayv1.JobDeploymentStatusComplete),
						State:               v2pb.RAY_JOB_STATE_SUCCEEDED,
						Message:             "Job finished successfully.",
					},
				}, nil)
			},
			expectedState:   v2pb.RAY_JOB_STATE_SUCCEEDED,
			expectedMessage: "Job finished successfully.",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, time.Duration(0), res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, job *v2pb.RayJob) {
				assert.Equal(t, string(rayv1.JobStatusSucceeded), job.Status.JobStatus)
				assert.Equal(t, string(rayv1.JobDeploymentStatusComplete), job.Status.JobDeploymentStatus)
				assert.True(t, utils.IsImmutable(job), "terminal job should be marked immutable by reconciler")
			},
		},
		{
			name: "job already launched - safety net retries later when the status check fails",
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
							State:   v2pb.RAY_JOB_STATE_RUNNING,
							Message: "job is running",
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
				mfc.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("backend down"))
			},
			expectedState:   v2pb.RAY_JOB_STATE_RUNNING,
			expectedMessage: "job is running",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, _safetyNetPeriod, res.RequeueAfter)
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
		{
			// The watcher writes the terminal state and the next reconcile freezes it,
			// by which time Uniflow may already be tearing the cluster down. A
			// terminal state is final: it must be frozen as recorded, not re-derived
			// from a cluster that is no longer READY.
			name: "terminal job is frozen when its cluster is no longer ready",
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
							State:   v2pb.RAY_JOB_STATE_SUCCEEDED,
							Message: "Job finished successfully.",
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
							State: v2pb.RAY_CLUSTER_STATE_TERMINATED,
							Assignment: &v2pb.AssignmentInfo{
								Cluster: assignedCluster,
							},
						},
					},
				}
			},
			setupMocks:      func(ctrl *gomock.Controller, mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache) {},
			expectedState:   v2pb.RAY_JOB_STATE_SUCCEEDED,
			expectedMessage: "Job finished successfully.",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, time.Duration(0), res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, job *v2pb.RayJob) {
				assert.True(t, utils.IsImmutable(job), "terminal job should be marked immutable by reconciler")
			},
		},
		{
			// Same as above, once the torn-down cluster has also been archived.
			name: "terminal job is frozen when its cluster is gone",
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
							Entrypoint: "echo Hello World",
						},
						Status: v2pb.RayJobStatus{
							State:   v2pb.RAY_JOB_STATE_SUCCEEDED,
							Message: "Job finished successfully.",
							StatusConditions: []*apipb.Condition{
								{
									Type:   "Launched",
									Status: apipb.CONDITION_STATUS_TRUE,
								},
							},
						},
					},
				}
			},
			setupMocks:      func(ctrl *gomock.Controller, mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache) {},
			expectedState:   v2pb.RAY_JOB_STATE_SUCCEEDED,
			expectedMessage: "Job finished successfully.",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, time.Duration(0), res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, job *v2pb.RayJob) {
				assert.True(t, utils.IsImmutable(job), "terminal job should be marked immutable by reconciler")
			},
		},
		{
			// After launch the watcher owns State and Message. Cluster readiness
			// gates only the launch, so a cluster leaving READY must not flip a
			// running job back to INITIALIZING.
			name: "launched job keeps its state when its cluster is no longer ready",
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
							State:     v2pb.RAY_JOB_STATE_RUNNING,
							JobStatus: string(rayv1.JobStatusRunning),
							Message:   "job is running",
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
							State: v2pb.RAY_CLUSTER_STATE_UNHEALTHY,
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
				mfc.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(&jobtypes.JobStatus{
					Ray: &v2pb.RayJobStatus{
						JobStatus: string(rayv1.JobStatusRunning),
						State:     v2pb.RAY_JOB_STATE_RUNNING,
						Message:   "job is running",
					},
				}, nil)
			},
			expectedState:   v2pb.RAY_JOB_STATE_RUNNING,
			expectedMessage: "job is running",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, _safetyNetPeriod, res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, job *v2pb.RayJob) {},
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
	// Status arrives from the federated watcher, so the stored status is already
	// current and this pass has nothing to persist. The safety net still polls,
	// but ignores the non-terminal result.
	mockFedClient.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(&jobtypes.JobStatus{
		Ray: &v2pb.RayJobStatus{
			JobStatus: string(rayv1.JobStatusRunning),
			State:     v2pb.RAY_JOB_STATE_RUNNING,
		},
	}, nil)

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

// TestReconcile_StatusWriteConflictIsRequeuedQuietly covers Reconcile's status
// write losing a race with another writer. That is routine -- the other write
// triggers a fresh pass, which works from the current object -- so it is
// requeued without an error or an error log. Any other failed write is still
// reported.
func TestReconcile_StatusWriteConflictIsRequeuedQuietly(t *testing.T) {
	tests := []struct {
		name          string
		writeErr      error
		wantResult    ctrl.Result
		wantErr       bool
		wantErrorLogs []string
	}{
		{
			name: "conflict",
			writeErr: apiErrors.NewConflict(schema.GroupResource{Group: "michelangelo.api", Resource: "rayjobs"},
				rayJobName, fmt.Errorf("the object has been modified")),
			wantResult: ctrl.Result{Requeue: true},
		},
		{
			name:          "other error",
			writeErr:      apiErrors.NewInternalError(fmt.Errorf("etcdserver: leader changed")),
			wantResult:    ctrl.Result{RequeueAfter: requeueAfter},
			wantErr:       true,
			wantErrorLogs: []string{"failed to update status"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
			fakeClient := fake.NewClientBuilder().
				WithScheme(newTestScheme()).
				WithObjects(rayJob, cluster).
				WithStatusSubresource(rayJob, cluster).
				WithInterceptorFuncs(interceptor.Funcs{
					SubResourceUpdate: func(context.Context, client.Client, string, client.Object, ...client.SubResourceUpdateOption) error {
						return tt.writeErr
					},
				}).
				Build()

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockFedClient := clientmocks.NewMockFederatedClient(mockCtrl)
			mockFedClient.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			mockCache := newMockClusterCache()
			mockCache.addCluster(assignedCluster, &v2pb.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: assignedCluster},
			})
			zapLogger, logs := newObservedLogger(t)
			r := &Reconciler{
				Client:          fakeClient,
				logger:          zapr.NewLogger(zapLogger),
				federatedClient: mockFedClient,
				clusterCache:    mockCache,
			}

			res, err := r.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: rayJobName},
			})

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantResult, res)
			assert.Equal(t, tt.wantErrorLogs, errorLogs(logs))
		})
	}
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
				StatusConditions: []*apipb.Condition{
					{
						Type:   constants.LaunchedCondition,
						Status: apipb.CONDITION_STATUS_TRUE,
					},
				},
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

	// A terminal state is final even before the job is frozen, e.g. a FAILED
	// that Reconcile recorded when the job's RayCluster disappeared.
	t.Run("terminal job is skipped", func(t *testing.T) {
		global := newGlobalRayJob()
		global.Status.State = v2pb.RAY_JOB_STATE_FAILED
		global.Status.Message = "failed to find cluster default/existing-cluster"
		r := newReconciler(global)

		r.rayJobEventHandler(newLocalRayJob(rayv1.JobStatusRunning, rayv1.JobDeploymentStatusRunning, "job is running"))

		updated := getUpdated(t, r)
		assert.Equal(t, v2pb.RAY_JOB_STATE_FAILED, updated.Status.State)
		assert.Equal(t, "failed to find cluster default/existing-cluster", updated.Status.Message)
	})

	t.Run("ill-formed object is ignored", func(t *testing.T) {
		r := &Reconciler{logger: ctrl.Log.WithName("test")}
		// Should not panic on non-RayJob object.
		r.rayJobEventHandler("not-a-rayjob")
	})
}

// TestRayJobDeleteEventHandler verifies KubeRay RayJob deletions transition the
// global RayJob to a terminal state.
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
				State:     v2pb.RAY_JOB_STATE_RUNNING,
				JobStatus: string(rayv1.JobStatusRunning),
				Message:   "job is running",
				StatusConditions: []*apipb.Condition{
					{
						Type:   constants.LaunchedCondition,
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

		succeeded := jobsutils.GetCondition(&updated.Status.StatusConditions, constants.SucceededCondition, updated.Generation)
		assert.Equal(t, apipb.CONDITION_STATUS_FALSE, succeeded.Status)
		assert.Equal(t, constants.ClusterKilled, succeeded.Reason)

		killed := jobsutils.GetCondition(&updated.Status.StatusConditions, constants.KilledCondition, updated.Generation)
		assert.Equal(t, apipb.CONDITION_STATUS_TRUE, killed.Status)

		// FAILED, not KILLED: Uniflow retries FAILED but not KILLED, and the usual
		// cause is the cluster being torn down, not a user kill.
		assert.Equal(t, v2pb.RAY_JOB_STATE_FAILED, updated.Status.State,
			"a job whose backing object is gone must reach a terminal state")
		assert.Equal(t, "Unknown", updated.Status.JobStatus)
		assert.Equal(t, "ray job was deleted from the compute cluster", updated.Status.Message)
		assert.True(t, utils.IsImmutable(&updated),
			"a terminal job must be frozen so the ingester can archive it")
	})

	// An unlaunched job has no KubeRay RayJob of its own, so the deleted object
	// belongs to something else, e.g. an earlier job with the same name.
	t.Run("external deletion leaves an unlaunched job untouched", func(t *testing.T) {
		rayJob := &v2pb.RayJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:       rayJobName,
				Namespace:  testNamespace,
				Generation: 1,
			},
			Status: v2pb.RayJobStatus{
				State:            v2pb.RAY_JOB_STATE_INITIALIZING,
				Message:          "cluster default/existing-cluster is not ready",
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

		assert.Equal(t, v2pb.RAY_JOB_STATE_INITIALIZING, updated.Status.State)
		assert.Equal(t, "cluster default/existing-cluster is not ready", updated.Status.Message)
		assert.Empty(t, updated.Status.StatusConditions,
			"delete handler must not add conditions to a job it should skip")
		assert.False(t, utils.IsImmutable(&updated))
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

		assert.Equal(t, v2pb.RAY_JOB_STATE_KILLED, updated.Status.State,
			"a job whose backing object is gone must reach a terminal state")
		assert.True(t, utils.IsImmutable(&updated),
			"a terminal job must be frozen so the ingester can archive it")
	})

	// A job whose outcome is already recorded must survive KubeRay removing the
	// backing object (owner GC at cluster teardown, an operator sweep): that is
	// garbage collection, not a kill, and must not overwrite the terminal record.
	for _, tc := range []struct {
		name      string
		state     v2pb.RayJobState
		immutable bool
	}{
		{name: "succeeded job", state: v2pb.RAY_JOB_STATE_SUCCEEDED},
		{name: "failed job", state: v2pb.RAY_JOB_STATE_FAILED},
		{name: "killed job", state: v2pb.RAY_JOB_STATE_KILLED},
		{name: "immutable non-terminal job", state: v2pb.RAY_JOB_STATE_RUNNING, immutable: true},
	} {
		t.Run("external deletion leaves "+tc.name+" untouched", func(t *testing.T) {
			rayJob := &v2pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:       rayJobName,
					Namespace:  testNamespace,
					Generation: 1,
				},
				Status: v2pb.RayJobStatus{
					State: tc.state,
					StatusConditions: []*apipb.Condition{
						{
							Type:   constants.LaunchedCondition,
							Status: apipb.CONDITION_STATUS_TRUE,
						},
					},
				},
			}
			if tc.immutable {
				utils.MarkImmutable(rayJob)
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

			assert.Equal(t, tc.state, updated.Status.State)
			assert.Len(t, updated.Status.StatusConditions, 1,
				"delete handler must not add conditions to a job it should skip")

			succeeded := jobsutils.GetCondition(&updated.Status.StatusConditions, constants.SucceededCondition, updated.Generation)
			assert.NotEqual(t, apipb.CONDITION_STATUS_FALSE, succeeded.Status,
				"a job that already finished must not be reported as failed")

			killed := jobsutils.GetCondition(&updated.Status.StatusConditions, constants.KilledCondition, updated.Generation)
			assert.NotEqual(t, apipb.CONDITION_STATUS_TRUE, killed.Status,
				"garbage collection of the backing object is not a kill")
		})
	}
}

// setupJobWatcherTestWithoutGlobal builds a Reconciler whose store holds no
// global RayJob, and returns a counter of every write the handlers attempt.
//
// Counting the writes is what makes the assertion meaningful: asserting only
// that the handler did not error would also pass if it went on to resurrect the
// job, since the fake client happily creates an object on write.
func setupJobWatcherTestWithoutGlobal(t *testing.T) (*Reconciler, *int, *observer.ObservedLogs) {
	t.Helper()
	return setupJobWatcherTest(t)
}

// setupJobWatcherTest builds a Reconciler whose store holds objs, and returns a
// counter of every write the handlers attempt along with the logs they emit.
func setupJobWatcherTest(t *testing.T, objs ...client.Object) (*Reconciler, *int, *observer.ObservedLogs) {
	t.Helper()

	writes := 0
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(objs...).
		WithStatusSubresource(&v2pb.RayJob{}).
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

	zapLogger, logs := newObservedLogger(t)
	return &Reconciler{Client: fakeClient, logger: zapr.NewLogger(zapLogger)}, &writes, logs
}

// newObservedLogger tees the test logger into an observer so the assertions can
// inspect what the handlers logged and at which severity, not just whether they
// wrote. V(1) entries are captured at debug level.
func newObservedLogger(t *testing.T) (*zap.Logger, *observer.ObservedLogs) {
	observed, logs := observer.New(zapcore.DebugLevel)
	zapLogger := zaptest.NewLogger(t).WithOptions(
		zap.WrapCore(func(core zapcore.Core) zapcore.Core {
			return zapcore.NewTee(core, observed)
		}))
	return zapLogger, logs
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

// TestJobWatcherHandlersIgnoreMissingGlobalJob asserts that both handlers treat
// a missing global RayJob as an expected, silent no-op.
//
// The KubeRay RayJob outlives the global CR: the ingester archives a terminal
// job out of etcd as soon as it finishes, while KubeRay keeps its own object
// until owner GC removes it with its KubeRay RayCluster (KubeRay skips TTL
// cleanup for jobs that target an existing cluster). Every event in that window
// -- the trailing status updates and the eventual GC delete -- arrives with
// nothing left to record it on.
func TestJobWatcherHandlersIgnoreMissingGlobalJob(t *testing.T) {
	newLocal := func() *rayv1.RayJob {
		return &rayv1.RayJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rayJobName,
				Namespace: testNamespace,
				Labels:    map[string]string{constants.ProjectNameLabelKey: testNamespace},
			},
			Status: rayv1.RayJobStatus{
				JobStatus:           rayv1.JobStatusSucceeded,
				JobDeploymentStatus: rayv1.JobDeploymentStatusComplete,
			},
		}
	}

	tests := []struct {
		name  string
		event func(r *Reconciler)
	}{
		{
			name:  "status update",
			event: func(r *Reconciler) { r.rayJobEventHandler(newLocal()) },
		},
		{
			name:  "delete",
			event: func(r *Reconciler) { r.rayJobDeleteEventHandler(newLocal()) },
		},
		{
			name: "delete via tombstone",
			event: func(r *Reconciler) {
				r.rayJobDeleteEventHandler(cache.DeletedFinalStateUnknown{
					Key: testNamespace + "/" + rayJobName,
					Obj: newLocal(),
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, writes, logs := setupJobWatcherTestWithoutGlobal(t)

			require.NotPanics(t, func() { tt.event(r) })

			assert.Zero(t, *writes, "a global job that no longer exists must not be written to")
			assert.Empty(t, errorLogs(logs),
				"losing the race against teardown is routine and must not be logged as an error")
		})
	}
}

// setupJobWatcherTestWithConcurrentWriter builds a Reconciler whose first
// status write loses a race: before failing it with a conflict, the fake client
// records concurrentState on the stored job, as a concurrent writer would. It
// returns a counter of every status write the handlers attempt along with the
// logs they emit.
func setupJobWatcherTestWithConcurrentWriter(t *testing.T, global *v2pb.RayJob, concurrentState v2pb.RayJobState) (*Reconciler, *int, *observer.ObservedLogs) {
	t.Helper()
	return setupJobWatcherTestWithConcurrentUpdate(t, global, func(stored *v2pb.RayJob) {
		stored.Status.State = concurrentState
	})
}

// setupJobWatcherTestWithConcurrentUpdate is setupJobWatcherTestWithConcurrentWriter
// for a concurrent writer that applies concurrentUpdate to the stored job.
func setupJobWatcherTestWithConcurrentUpdate(t *testing.T, global *v2pb.RayJob, concurrentUpdate func(stored *v2pb.RayJob)) (*Reconciler, *int, *observer.ObservedLogs) {
	t.Helper()

	statusWrites := 0
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(global).
		WithStatusSubresource(global).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceUpdate: func(ctx context.Context, c client.Client, subResource string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				statusWrites++
				if statusWrites > 1 {
					return c.SubResource(subResource).Update(ctx, obj, opts...)
				}
				var stored v2pb.RayJob
				if err := c.Get(ctx, client.ObjectKeyFromObject(obj), &stored); err != nil {
					return err
				}
				concurrentUpdate(&stored)
				if err := c.SubResource(subResource).Update(ctx, &stored); err != nil {
					return err
				}
				return apiErrors.NewConflict(schema.GroupResource{Group: "michelangelo.api", Resource: "rayjobs"},
					obj.GetName(), fmt.Errorf("the object has been modified"))
			},
		}).
		Build()

	zapLogger, logs := newObservedLogger(t)
	return &Reconciler{Client: fakeClient, logger: zapr.NewLogger(zapLogger)}, &statusWrites, logs
}

// TestJobWatcherHandlersRecheckGuardsAfterConflict asserts that both handlers
// re-check their guards on the object they re-fetch after a conflict. A
// conflict means another writer got in first, possibly finishing the job, and a
// terminal state is final.
func TestJobWatcherHandlersRecheckGuardsAfterConflict(t *testing.T) {
	newGlobal := func() *v2pb.RayJob {
		return &v2pb.RayJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:       rayJobName,
				Namespace:  testNamespace,
				Generation: 1,
			},
			Status: v2pb.RayJobStatus{
				State: v2pb.RAY_JOB_STATE_INITIALIZING,
				StatusConditions: []*apipb.Condition{
					{
						Type:   constants.LaunchedCondition,
						Status: apipb.CONDITION_STATUS_TRUE,
					},
				},
			},
		}
	}
	newLocal := func() *rayv1.RayJob {
		return &rayv1.RayJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:   rayJobName,
				Labels: map[string]string{constants.ProjectNameLabelKey: testNamespace},
			},
			Status: rayv1.RayJobStatus{
				JobStatus:           rayv1.JobStatusRunning,
				JobDeploymentStatus: rayv1.JobDeploymentStatusRunning,
			},
		}
	}

	tests := []struct {
		name  string
		event func(r *Reconciler)
	}{
		{
			name:  "status update",
			event: func(r *Reconciler) { r.rayJobEventHandler(newLocal()) },
		},
		{
			name:  "delete",
			event: func(r *Reconciler) { r.rayJobDeleteEventHandler(newLocal()) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, statusWrites, _ := setupJobWatcherTestWithConcurrentWriter(t, newGlobal(), v2pb.RAY_JOB_STATE_SUCCEEDED)

			tt.event(r)

			var updated v2pb.RayJob
			require.NoError(t, r.Get(context.Background(),
				types.NamespacedName{Namespace: testNamespace, Name: rayJobName}, &updated))

			assert.Equal(t, 1, *statusWrites, "the retry must not write over a job that turned terminal")
			assert.Equal(t, v2pb.RAY_JOB_STATE_SUCCEEDED, updated.Status.State)
			killed := jobsutils.GetCondition(&updated.Status.StatusConditions, constants.KilledCondition, updated.Generation)
			assert.NotEqual(t, apipb.CONDITION_STATUS_TRUE, killed.Status,
				"a job that already finished must not be reported as killed")
		})
	}
}

// TestRayJobEventHandlerRechecksLaunchAfterConflict covers a status write that
// conflicts because the job was replaced by an unlaunched one of the same name.
// The retry must leave the new job alone, as an event that found it so would.
func TestRayJobEventHandlerRechecksLaunchAfterConflict(t *testing.T) {
	global := &v2pb.RayJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:       rayJobName,
			Namespace:  testNamespace,
			Generation: 1,
		},
		Status: v2pb.RayJobStatus{
			State: v2pb.RAY_JOB_STATE_INITIALIZING,
			StatusConditions: []*apipb.Condition{
				{
					Type:   constants.LaunchedCondition,
					Status: apipb.CONDITION_STATUS_TRUE,
				},
			},
		},
	}
	r, statusWrites, _ := setupJobWatcherTestWithConcurrentUpdate(t, global, func(stored *v2pb.RayJob) {
		stored.Status = v2pb.RayJobStatus{}
	})

	r.rayJobEventHandler(&rayv1.RayJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:   rayJobName,
			Labels: map[string]string{constants.ProjectNameLabelKey: testNamespace},
		},
		Status: rayv1.RayJobStatus{
			JobStatus:           rayv1.JobStatusRunning,
			JobDeploymentStatus: rayv1.JobDeploymentStatusRunning,
		},
	})

	var updated v2pb.RayJob
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Namespace: testNamespace, Name: rayJobName}, &updated))
	assert.Equal(t, 1, *statusWrites, "the retry must not write to a job that has not launched")
	assert.Equal(t, v2pb.RAY_JOB_STATE_INVALID, updated.Status.State)
}

// TestRayJobDeleteEventHandlerLogsConcurrentSkip covers a delete whose status
// write loses the race to another writer that finishes the job first. The
// retry then writes nothing, so the handler must say it skipped rather than
// report the job as failed or killed.
func TestRayJobDeleteEventHandlerLogsConcurrentSkip(t *testing.T) {
	newGlobal := func(conditions ...*apipb.Condition) *v2pb.RayJob {
		return &v2pb.RayJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:       rayJobName,
				Namespace:  testNamespace,
				Generation: 1,
			},
			Status: v2pb.RayJobStatus{
				State: v2pb.RAY_JOB_STATE_RUNNING,
				StatusConditions: append([]*apipb.Condition{
					{
						Type:   constants.LaunchedCondition,
						Status: apipb.CONDITION_STATUS_TRUE,
					},
				}, conditions...),
			},
		}
	}

	tests := []struct {
		name       string
		global     *v2pb.RayJob
		successLog string
	}{
		{
			name:       "external deletion",
			global:     newGlobal(),
			successLog: "job externally deleted, marked as failed",
		},
		{
			name: "controller-initiated deletion",
			global: newGlobal(&apipb.Condition{
				Type:   constants.KillingCondition,
				Status: apipb.CONDITION_STATUS_TRUE,
			}),
			successLog: "job killed successfully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, statusWrites, logs := setupJobWatcherTestWithConcurrentWriter(t, tt.global, v2pb.RAY_JOB_STATE_SUCCEEDED)

			r.rayJobDeleteEventHandler(&rayv1.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:   rayJobName,
					Labels: map[string]string{constants.ProjectNameLabelKey: testNamespace},
				},
			})

			require.Equal(t, 1, *statusWrites, "the retry must not write over a job that turned terminal")
			skipped := logs.FilterMessage("skipping: ray job reached a terminal state concurrently").All()
			require.Len(t, skipped, 1, "the handler must report that it skipped")
			assert.Equal(t, zapcore.DebugLevel, skipped[0].Level, "a lost race is routine and logged at V(1)")
			assert.Zero(t, logs.FilterMessage(tt.successLog).Len(),
				"the handler must not report an outcome it did not record")
			assert.Empty(t, errorLogs(logs))
		})
	}
}

// TestJobWatcherHandlersRetryTransientErrors asserts that a status write which
// fails with a transient API server error is retried, not dropped. The delete
// path in particular gets a single event: the object is gone from the store, so
// nothing re-delivers it.
func TestJobWatcherHandlersRetryTransientErrors(t *testing.T) {
	newGlobal := func() *v2pb.RayJob {
		return &v2pb.RayJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:       rayJobName,
				Namespace:  testNamespace,
				Generation: 1,
			},
			Status: v2pb.RayJobStatus{
				State: v2pb.RAY_JOB_STATE_INITIALIZING,
				StatusConditions: []*apipb.Condition{
					{
						Type:   constants.LaunchedCondition,
						Status: apipb.CONDITION_STATUS_TRUE,
					},
				},
			},
		}
	}
	newLocal := func() *rayv1.RayJob {
		return &rayv1.RayJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:   rayJobName,
				Labels: map[string]string{constants.ProjectNameLabelKey: testNamespace},
			},
			Status: rayv1.RayJobStatus{
				JobStatus:           rayv1.JobStatusRunning,
				JobDeploymentStatus: rayv1.JobDeploymentStatusRunning,
			},
		}
	}

	tests := []struct {
		name      string
		event     func(r *Reconciler)
		wantState v2pb.RayJobState
	}{
		{
			name:      "status update",
			event:     func(r *Reconciler) { r.rayJobEventHandler(newLocal()) },
			wantState: v2pb.RAY_JOB_STATE_RUNNING,
		},
		{
			name:      "delete",
			event:     func(r *Reconciler) { r.rayJobDeleteEventHandler(newLocal()) },
			wantState: v2pb.RAY_JOB_STATE_FAILED,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			global := newGlobal()
			statusWrites := 0
			fakeClient := fake.NewClientBuilder().
				WithScheme(newTestScheme()).
				WithObjects(global).
				WithStatusSubresource(global).
				WithInterceptorFuncs(interceptor.Funcs{
					SubResourceUpdate: func(ctx context.Context, c client.Client, subResource string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
						statusWrites++
						if statusWrites == 1 {
							return apiErrors.NewInternalError(fmt.Errorf("etcdserver: request timed out"))
						}
						return c.SubResource(subResource).Update(ctx, obj, opts...)
					},
				}).
				Build()
			r := &Reconciler{Client: fakeClient, logger: zapr.NewLogger(zaptest.NewLogger(t))}

			tt.event(r)

			var updated v2pb.RayJob
			require.NoError(t, r.Get(context.Background(),
				types.NamespacedName{Namespace: testNamespace, Name: rayJobName}, &updated))
			assert.Equal(t, 2, statusWrites, "the failed write must be retried once")
			assert.Equal(t, tt.wantState, updated.Status.State)
		})
	}
}

// TestRayJobConvergesAfterDeleteHandlerExhaustsRetries covers a delete whose
// status write keeps conflicting -- e.g. against a stale cache -- until the
// handler gives up. The handler gets no second event, so the job must still
// reach a terminal state through Reconcile's safety net, which reports the
// missing KubeRay RayJob as FAILED.
func TestRayJobConvergesAfterDeleteHandlerExhaustsRetries(t *testing.T) {
	ctx := context.Background()

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
			State:     v2pb.RAY_JOB_STATE_RUNNING,
			JobStatus: string(rayv1.JobStatusRunning),
			StatusConditions: []*apipb.Condition{
				{
					Type:   constants.LaunchedCondition,
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
			State: v2pb.RAY_CLUSTER_STATE_TERMINATED,
			Assignment: &v2pb.AssignmentInfo{
				Cluster: assignedCluster,
			},
		},
	}

	const conflicts = 6
	statusWrites := 0
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(rayJob, cluster).
		WithStatusSubresource(rayJob, cluster).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceUpdate: func(ctx context.Context, c client.Client, subResource string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				statusWrites++
				if statusWrites <= conflicts {
					return apiErrors.NewConflict(schema.GroupResource{Group: "michelangelo.api", Resource: "rayjobs"},
						obj.GetName(), fmt.Errorf("the object has been modified"))
				}
				return c.SubResource(subResource).Update(ctx, obj, opts...)
			},
		}).
		Build()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockFedClient := clientmocks.NewMockFederatedClient(mockCtrl)
	mockFedClient.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(&jobtypes.JobStatus{
		Ray: &v2pb.RayJobStatus{
			JobStatus: "Unknown",
			State:     v2pb.RAY_JOB_STATE_FAILED,
			Message:   "Resource not found in compute cluster",
		},
	}, nil).AnyTimes()
	mockCache := newMockClusterCache()
	mockCache.addCluster(assignedCluster, &v2pb.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: assignedCluster},
	})

	r := &Reconciler{
		Client:          fakeClient,
		logger:          zapr.NewLogger(zaptest.NewLogger(t)),
		federatedClient: mockFedClient,
		clusterCache:    mockCache,
	}

	r.rayJobDeleteEventHandler(&rayv1.RayJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:   rayJobName,
			Labels: map[string]string{constants.ProjectNameLabelKey: testNamespace},
		},
	})

	request := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: rayJobName}}
	var stored v2pb.RayJob
	require.NoError(t, r.Get(ctx, request.NamespacedName, &stored))
	require.Equal(t, v2pb.RAY_JOB_STATE_RUNNING, stored.Status.State, "the handler should have given up")

	// The workqueue runs a reconcile again when it returns an error or asks for
	// a requeue; model that by reconciling until one pass does neither.
	var res ctrl.Result
	var err error
	for i := 0; i < conflicts; i++ {
		if res, err = r.Reconcile(ctx, request); err == nil && !res.Requeue {
			break
		}
	}
	require.NoError(t, err)
	require.False(t, res.Requeue, "the conflicts should have run out")

	require.NoError(t, r.Get(ctx, request.NamespacedName, &stored))
	assert.Equal(t, v2pb.RAY_JOB_STATE_FAILED, stored.Status.State)
	assert.Equal(t, "Resource not found in compute cluster", stored.Status.Message)
	assert.True(t, utils.IsImmutable(&stored), "a terminal job must be frozen so the ingester can archive it")
}

// TestNewReconciler_NilMetricsScope asserts that a Reconciler built without a
// metrics scope can still build its watcher, which derives its own scope from
// the reconciler's.
func TestNewReconciler_NilMetricsScope(t *testing.T) {
	r := NewReconciler(zapr.NewLogger(zap.NewNop()), nil, env.Context{}, nil, newMockClusterCache(), nil)

	require.NotNil(t, r.metricsScope)
	assert.NotPanics(t, func() { r.getFederatedWatcher() })
}

// TestJobWatcherHandlersSkipWithoutWriting asserts the events the handlers must
// drop without writing to the global RayJob.
func TestJobWatcherHandlersSkipWithoutWriting(t *testing.T) {
	newGlobal := func() *v2pb.RayJob {
		return &v2pb.RayJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:       rayJobName,
				Namespace:  testNamespace,
				Generation: 1,
			},
			Status: v2pb.RayJobStatus{
				State:               v2pb.RAY_JOB_STATE_RUNNING,
				JobStatus:           string(rayv1.JobStatusRunning),
				JobDeploymentStatus: string(rayv1.JobDeploymentStatusRunning),
				Message:             "job is running",
				StatusConditions: []*apipb.Condition{
					{
						Type:   constants.LaunchedCondition,
						Status: apipb.CONDITION_STATUS_TRUE,
					},
				},
			},
		}
	}
	newLocal := func(jobStatus rayv1.JobStatus) *rayv1.RayJob {
		return &rayv1.RayJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:   rayJobName,
				Labels: map[string]string{constants.ProjectNameLabelKey: testNamespace},
			},
			Status: rayv1.RayJobStatus{
				JobStatus:           jobStatus,
				JobDeploymentStatus: rayv1.JobDeploymentStatusRunning,
				Message:             "job is running",
			},
		}
	}

	// Reconcile owns a job until it records the launch: a write before then
	// would race the launch write.
	notLaunched := func() *v2pb.RayJob {
		global := newGlobal()
		global.Status = v2pb.RayJobStatus{
			State:   v2pb.RAY_JOB_STATE_INITIALIZING,
			Message: "cluster default/test-cluster is not ready",
			StatusConditions: []*apipb.Condition{
				{
					Type:   constants.LaunchedCondition,
					Status: apipb.CONDITION_STATUS_UNKNOWN,
				},
			},
		}
		return global
	}

	tests := []struct {
		name   string
		global func() *v2pb.RayJob
		event  func(r *Reconciler)
	}{
		{
			// The informer re-delivers every object on each resync, so an unchanged
			// status must not turn into a write.
			name:   "status update with nothing changed",
			global: newGlobal,
			event:  func(r *Reconciler) { r.rayJobEventHandler(newLocal(rayv1.JobStatusRunning)) },
		},
		{
			name:   "add event for a job not launched yet",
			global: notLaunched,
			event:  func(r *Reconciler) { r.rayJobAddEventHandler(newLocal(rayv1.JobStatusRunning)) },
		},
		{
			name:   "update event for a job not launched yet",
			global: notLaunched,
			event:  func(r *Reconciler) { r.rayJobUpdateEventHandler(nil, newLocal(rayv1.JobStatusRunning)) },
		},
		{
			name: "status update for a job being deleted",
			global: func() *v2pb.RayJob {
				global := newGlobal()
				global.DeletionTimestamp = &metav1.Time{Time: time.Now()}
				global.Finalizers = []string{"test-finalizer"}
				return global
			},
			event: func(r *Reconciler) { r.rayJobEventHandler(newLocal(rayv1.JobStatusSucceeded)) },
		},
		{
			name:   "status update without a project label",
			global: newGlobal,
			event: func(r *Reconciler) {
				local := newLocal(rayv1.JobStatusSucceeded)
				local.Labels = nil
				r.rayJobEventHandler(local)
			},
		},
		{
			name:   "delete without a project label",
			global: newGlobal,
			event: func(r *Reconciler) {
				local := newLocal(rayv1.JobStatusRunning)
				local.Labels = nil
				r.rayJobDeleteEventHandler(local)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, writes, _ := setupJobWatcherTest(t, tt.global())

			tt.event(r)

			assert.Zero(t, *writes, "the event must not be written to the global job")
		})
	}
}

// TestRayJobStoppedIsKilledAndFrozen follows a stopped KubeRay RayJob through
// the watcher and the reconcile its status write triggers: the watcher records
// KILLED, and Reconcile freezes it. The job's RayCluster is already gone, so a
// reconcile that looked at the cluster would record FAILED instead.
func TestRayJobStoppedIsKilledAndFrozen(t *testing.T) {
	ctx := context.Background()

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
			State:     v2pb.RAY_JOB_STATE_RUNNING,
			JobStatus: string(rayv1.JobStatusRunning),
			StatusConditions: []*apipb.Condition{
				{
					Type:   constants.LaunchedCondition,
					Status: apipb.CONDITION_STATUS_TRUE,
				},
			},
		},
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(rayJob).
		WithStatusSubresource(rayJob).
		Build()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	// No federated call is expected: a terminal job is frozen before the safety
	// net runs.
	r := &Reconciler{
		Client:          fakeClient,
		logger:          zapr.NewLogger(zaptest.NewLogger(t)),
		federatedClient: clientmocks.NewMockFederatedClient(mockCtrl),
		clusterCache:    newMockClusterCache(),
	}

	r.rayJobEventHandler(&rayv1.RayJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:   rayJobName,
			Labels: map[string]string{constants.ProjectNameLabelKey: testNamespace},
		},
		Status: rayv1.RayJobStatus{
			JobStatus:           rayv1.JobStatusStopped,
			JobDeploymentStatus: rayv1.JobDeploymentStatusComplete,
			Message:             "Job was stopped",
		},
	})

	request := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: rayJobName}}
	var stored v2pb.RayJob
	require.NoError(t, r.Get(ctx, request.NamespacedName, &stored))
	require.Equal(t, v2pb.RAY_JOB_STATE_KILLED, stored.Status.State)
	require.False(t, utils.IsImmutable(&stored), "the watcher records the state; the reconciler freezes it")

	res, err := r.Reconcile(ctx, request)
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), res.RequeueAfter)

	require.NoError(t, r.Get(ctx, request.NamespacedName, &stored))
	assert.Equal(t, v2pb.RAY_JOB_STATE_KILLED, stored.Status.State)
	assert.Equal(t, string(rayv1.JobStatusStopped), stored.Status.JobStatus)
	assert.Equal(t, "Job was stopped", stored.Status.Message)
	assert.True(t, utils.IsImmutable(&stored), "a terminal job must be frozen so the ingester can archive it")
}

// TestRayJobLaunchIsNotLostToAnEarlyKubeRayEvent covers KubeRay's first status
// update for a new RayJob arriving between CreateJob and Reconcile's launch
// write. Were the watcher to write it, the launch write would conflict and lose
// Launched=TRUE, and the next pass would call CreateJob again -- where a
// transient error fetching the remote RayCluster records FAILED while the
// KubeRay RayJob runs on. The watcher leaves the job alone until the launch is
// recorded and takes the status from the next KubeRay update.
func TestRayJobLaunchIsNotLostToAnEarlyKubeRayEvent(t *testing.T) {
	ctx := context.Background()

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
	var writtenStates []v2pb.RayJobState
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(rayJob, cluster).
		WithStatusSubresource(rayJob, cluster).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceUpdate: func(ctx context.Context, c client.Client, subResource string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				if job, ok := obj.(*v2pb.RayJob); ok {
					writtenStates = append(writtenStates, job.Status.State)
				}
				return c.SubResource(subResource).Update(ctx, obj, opts...)
			},
		}).
		Build()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockFedClient := clientmocks.NewMockFederatedClient(mockCtrl)
	mockCache := newMockClusterCache()
	mockCache.addCluster(assignedCluster, &v2pb.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: assignedCluster},
	})
	r := &Reconciler{
		Client:          fakeClient,
		logger:          zapr.NewLogger(zaptest.NewLogger(t)),
		federatedClient: mockFedClient,
		clusterCache:    mockCache,
	}

	newLocal := func(jobStatus rayv1.JobStatus, deploymentStatus rayv1.JobDeploymentStatus) *rayv1.RayJob {
		return &rayv1.RayJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:   rayJobName,
				Labels: map[string]string{constants.ProjectNameLabelKey: testNamespace},
			},
			Status: rayv1.RayJobStatus{
				JobStatus:           jobStatus,
				JobDeploymentStatus: deploymentStatus,
			},
		}
	}
	// KubeRay reports the RayJob it just created before CreateJob returns.
	mockFedClient.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, runtime.Object, runtime.Object, *v2pb.Cluster) error {
			r.rayJobAddEventHandler(newLocal(rayv1.JobStatusNew, rayv1.JobDeploymentStatusInitializing))
			return nil
		}).
		Times(1)
	mockFedClient.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(&jobtypes.JobStatus{
		Ray: &v2pb.RayJobStatus{
			JobStatus: string(rayv1.JobStatusRunning),
			State:     v2pb.RAY_JOB_STATE_RUNNING,
		},
	}, nil).AnyTimes()

	request := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: rayJobName}}
	_, err := r.Reconcile(ctx, request)
	require.NoError(t, err)
	// The launch write enqueues the job again.
	_, err = r.Reconcile(ctx, request)
	require.NoError(t, err)
	r.rayJobUpdateEventHandler(nil, newLocal(rayv1.JobStatusRunning, rayv1.JobDeploymentStatusRunning))

	var stored v2pb.RayJob
	require.NoError(t, r.Get(ctx, request.NamespacedName, &stored))
	launched := jobsutils.GetCondition(&stored.Status.StatusConditions, constants.LaunchedCondition, stored.Generation)
	assert.Equal(t, apipb.CONDITION_STATUS_TRUE, launched.Status, "the launch must be recorded")
	assert.Equal(t, v2pb.RAY_JOB_STATE_RUNNING, stored.Status.State)
	assert.NotContains(t, writtenStates, v2pb.RAY_JOB_STATE_FAILED, "a job that launched must never be recorded as failed")
}
