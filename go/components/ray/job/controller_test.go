package job

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/clientmocks"
	jobscluster "github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	jobtypes "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	v1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
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

func TestReconciler_Reconcile(t *testing.T) {
	ctx := context.Background()

	// Mock environment
	scheme := runtime.NewScheme()
	kubescheme.AddToScheme(scheme)
	v2pb.AddToScheme(scheme)

	// Test cases
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
				objects := make([]client.Object, 0)
				return objects
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
				objects := make([]client.Object, 0)
				rayJob := &v2pb.RayJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:       rayJobName,
						Namespace:  testNamespace,
						Generation: 1,
					},
					Spec: v2pb.RayJobSpec{
						Cluster: nil,
					},
				}
				objects = append(objects, rayJob)
				return objects
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
			name: "Cluster not found",
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
							Name:      "missing-cluster",
							Namespace: testNamespace,
						},
					},
				}
				objects = append(objects, rayJob)
				return objects
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
						State: v2pb.RAY_CLUSTER_STATE_PROVISIONING,
					},
				}
				objects = append(objects, rayJob)
				objects = append(objects, cluster)
				return objects
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
			name: "job status unknown - requeue and initializing",
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
					Status: v2pb.RayJobStatus{
						State: v2pb.RAY_JOB_STATE_INITIALIZING,
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
				// Return an unknown/unsupported status string - mapper will set State to INVALID
				mfc.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(&jobtypes.JobStatus{
					Ray: &v2pb.RayJobStatus{
						JobStatus: "SOMETHING_WEIRD",
						State:     v2pb.RAY_JOB_STATE_INVALID,
						Message:   "",
					},
				}, nil)
			},
			expectedState:   v2pb.RAY_JOB_STATE_INVALID,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, requeueAfter, res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, job *v2pb.RayJob) {
				assert.Equal(t, "SOMETHING_WEIRD", job.Status.JobStatus)
			},
		},
		{
			name: "get job status error - requeue and state unchanged",
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
					Status: v2pb.RayJobStatus{
						State: v2pb.RAY_JOB_STATE_INITIALIZING,
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
				mfc.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("backend down"))
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
			name: "cluster is ready but not assigned",
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
						State:      v2pb.RAY_CLUSTER_STATE_READY,
						Assignment: nil,
					},
				}
				objects = append(objects, rayJob)
				objects = append(objects, cluster)
				return objects
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
					Spec: v2pb.RayClusterSpec{
						Head: &v2pb.RayHeadSpec{
							Pod: &corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{},
								},
							},
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
				mfc.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			},
			expectedState:   v2pb.RAY_JOB_STATE_INITIALIZING,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, requeueAfter, res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, job *v2pb.RayJob) {
				// Verify LaunchedCondition is TRUE
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
			name: "cluster assigned but not in cache",
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
							Cluster: "missing-cluster",
						},
					},
				}
				objects = append(objects, rayJob)
				objects = append(objects, cluster)
				return objects
			},
			setupMocks: func(ctrl *gomock.Controller, mfc *clientmocks.MockFederatedClient, mcc *mockClusterCache) {
				// Don't add cluster to cache
			},
			expectedState:   v2pb.RAY_JOB_STATE_INVALID,
			expectedMessage: "waiting for RayCluster assignment",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, requeueAfter, res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, job *v2pb.RayJob) {},
		},
		{
			name: "job already launched - check running status",
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
					Status: v2pb.RayJobStatus{
						State: v2pb.RAY_JOB_STATE_INITIALIZING,
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
				mfc.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(&jobtypes.JobStatus{
					Ray: &v2pb.RayJobStatus{
						JobStatus: string(v1.JobStatusRunning),
						State:     v2pb.RAY_JOB_STATE_RUNNING,
						Message:   "",
					},
				}, nil)
			},
			expectedState:   v2pb.RAY_JOB_STATE_RUNNING,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, requeueAfter, res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, job *v2pb.RayJob) {},
		},
		{
			name: "job succeeded",
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
					Status: v2pb.RayJobStatus{
						State: v2pb.RAY_JOB_STATE_RUNNING,
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
				mfc.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(&jobtypes.JobStatus{
					Ray: &v2pb.RayJobStatus{
						JobStatus: "SUCCEEDED",
						State:     v2pb.RAY_JOB_STATE_SUCCEEDED,
						Message:   "",
					},
				}, nil)
			},
			expectedState:   v2pb.RAY_JOB_STATE_SUCCEEDED,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				// Terminal state, should not requeue
				assert.Equal(t, time.Duration(0), res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, job *v2pb.RayJob) {
				assert.Equal(t, "SUCCEEDED", job.Status.JobStatus)
			},
		},
		{
			name: "job failed",
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
					Status: v2pb.RayJobStatus{
						State: v2pb.RAY_JOB_STATE_RUNNING,
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
				mfc.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(&jobtypes.JobStatus{
					Ray: &v2pb.RayJobStatus{
						JobStatus: "FAILED",
						State:     v2pb.RAY_JOB_STATE_FAILED,
						Message:   "Job execution failed",
					},
				}, nil)
			},
			expectedState:   v2pb.RAY_JOB_STATE_FAILED,
			expectedMessage: "Job execution failed",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, time.Duration(0), res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, job *v2pb.RayJob) {
				assert.Equal(t, "FAILED", job.Status.JobStatus)
			},
		},
		{
			name: "job stopped",
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
					Status: v2pb.RayJobStatus{
						State: v2pb.RAY_JOB_STATE_RUNNING,
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
				mfc.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(&jobtypes.JobStatus{
					Ray: &v2pb.RayJobStatus{
						JobStatus: "STOPPED",
						State:     v2pb.RAY_JOB_STATE_KILLED,
						Message:   "Job was stopped",
					},
				}, nil)
			},
			expectedState:   v2pb.RAY_JOB_STATE_KILLED,
			expectedMessage: "Job was stopped",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				assert.Equal(t, time.Duration(0), res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, job *v2pb.RayJob) {
				assert.Equal(t, "STOPPED", job.Status.JobStatus)
			},
		},
		{
			name: "job status empty - initializing",
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
					Status: v2pb.RayJobStatus{
						State: v2pb.RAY_JOB_STATE_INITIALIZING,
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
				// Empty status string returned when KubeRay hasn't populated JobStatus yet
				// Mapper will set State to INITIALIZING based on JobDeploymentStatus
				mfc.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(&jobtypes.JobStatus{
					Ray: &v2pb.RayJobStatus{
						JobStatus:           "",
						JobDeploymentStatus: string(v1.JobDeploymentStatusInitializing),
						State:               v2pb.RAY_JOB_STATE_INITIALIZING,
						Message:             "",
					},
				}, nil)
			},
			expectedState:   v2pb.RAY_JOB_STATE_INITIALIZING,
			expectedMessage: "",
			errorAssertion:  require.NoError,
			postCheck: func(res ctrl.Result) {
				// Should requeue to check again
				assert.Equal(t, requeueAfter, res.RequeueAfter)
			},
			verifyConditions: func(t *testing.T, job *v2pb.RayJob) {},
		},
		{
			name: "job creation fails",
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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
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

			// Act
			res, err := r.Reconcile(ctx, ctrl.Request{
				NamespacedName: requestRayJob,
			})

			// Assert
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
