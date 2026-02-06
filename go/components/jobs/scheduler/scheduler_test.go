package scheduler

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/zapr"
	"github.com/golang/mock/gomock"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/api/handler/handlermocks"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster/clustermocks"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	sched "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/scheduler"
	matypes "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/framework"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/framework/frameworkmocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestRayClusterAssignment(t *testing.T) {
	tests := []struct {
		name           string
		rayCluster     *v2pb.RayCluster
		setupMock      func(g *gomock.Controller) *frameworkmocks.MockAssignmentEngine
		wantCondition  *apipb.Condition
		wantAssignment *v2pb.AssignmentInfo
		wantErr        bool
	}{
		{
			name: "successful ray cluster assignment",
			rayCluster: &v2pb.RayCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ray-cluster",
					Namespace: "test-namespace",
				},
				Spec: v2pb.RayClusterSpec{
					Head: &v2pb.RayHeadSpec{
						Pod: &corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: "ray-head",
										Resources: corev1.ResourceRequirements{
											Requests: corev1.ResourceList{
												corev1.ResourceCPU:    resource.MustParse("2"),
												corev1.ResourceMemory: resource.MustParse("4Gi"),
											},
										},
									},
								},
							},
						},
					},
					Workers: []*v2pb.RayWorkerSpec{
						{
							MinInstances: 2,
							Pod: &corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Name: "ray-worker",
											Resources: corev1.ResourceRequirements{
												Requests: corev1.ResourceList{
													corev1.ResourceCPU:    resource.MustParse("1"),
													corev1.ResourceMemory: resource.MustParse("2Gi"),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(g *gomock.Controller) *frameworkmocks.MockAssignmentEngine {
				mockEngine := frameworkmocks.NewMockAssignmentEngine(g)
				mockEngine.EXPECT().Select(gomock.Any(), gomock.Any()).Return(
					&v2pb.AssignmentInfo{
						ResourcePool: "test-pool",
						Cluster:      "test-cluster",
					},
					true,
					"ClusterMatched",
					nil,
				)
				return mockEngine
			},
			wantCondition: &apipb.Condition{
				Status: apipb.CONDITION_STATUS_TRUE,
				Reason: "ClusterMatched",
			},
			wantAssignment: &v2pb.AssignmentInfo{
				ResourcePool: "test-pool",
				Cluster:      "test-cluster",
			},
		},
		{
			name: "no cluster found for ray cluster",
			rayCluster: &v2pb.RayCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ray-cluster",
					Namespace: "test-namespace",
				},
				Spec: v2pb.RayClusterSpec{
					Head: &v2pb.RayHeadSpec{
						Pod: &corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: "ray-head",
										Resources: corev1.ResourceRequirements{
											Requests: corev1.ResourceList{
												corev1.ResourceCPU:    resource.MustParse("100"), // Very high CPU requirement
												corev1.ResourceMemory: resource.MustParse("1000Gi"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(g *gomock.Controller) *frameworkmocks.MockAssignmentEngine {
				mockEngine := frameworkmocks.NewMockAssignmentEngine(g)
				mockEngine.EXPECT().Select(gomock.Any(), gomock.Any()).Return(
					nil,
					false,
					"NoClustersFoundForAssignment",
					nil,
				)
				return mockEngine
			},
			wantCondition: &apipb.Condition{
				Status: apipb.CONDITION_STATUS_FALSE,
				Reason: "NoClustersFoundForAssignment",
			},
			wantAssignment: nil,
		},
		{
			name: "ray cluster already scheduled",
			rayCluster: &v2pb.RayCluster{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-ray-cluster",
				},
				Status: v2pb.RayClusterStatus{
					StatusConditions: []*apipb.Condition{
						{
							Type:   constants.ScheduledCondition,
							Status: apipb.CONDITION_STATUS_TRUE,
						},
					},
				},
			},
			setupMock: func(g *gomock.Controller) *frameworkmocks.MockAssignmentEngine {
				// No expectation since job is already scheduled
				return frameworkmocks.NewMockAssignmentEngine(g)
			},
			wantCondition: &apipb.Condition{
				Status: apipb.CONDITION_STATUS_TRUE,
			},
		},
		{
			name: "assignment engine error",
			rayCluster: &v2pb.RayCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ray-cluster",
					Namespace: "test-namespace",
				},
				Spec: v2pb.RayClusterSpec{
					Head: &v2pb.RayHeadSpec{
						Pod: &corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: "ray-head",
										Resources: corev1.ResourceRequirements{
											Requests: corev1.ResourceList{
												corev1.ResourceCPU:    resource.MustParse("2"),
												corev1.ResourceMemory: resource.MustParse("4Gi"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(g *gomock.Controller) *frameworkmocks.MockAssignmentEngine {
				mockEngine := frameworkmocks.NewMockAssignmentEngine(g)
				mockEngine.EXPECT().Select(gomock.Any(), gomock.Any()).Return(
					nil,
					false,
					"",
					errors.New("assignment engine error"),
				)
				return mockEngine
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomock.NewController(t)
			defer g.Finish()

			batchJob := framework.BatchRayCluster{RayCluster: tt.rayCluster}
			mockEngine := tt.setupMock(g)
			scheduler := setupTestScheduler(t, batchJob, mockEngine)

			err := scheduler.assignJob(context.Background(), batchJob)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// retrieve ray cluster object
			var rayCluster v2pb.RayCluster
			err = scheduler.Get(context.Background(), batchJob.GetNamespace(), batchJob.GetName(), &metav1.GetOptions{}, &rayCluster)
			require.NoError(t, err)

			// retrieve scheduler condition
			actualCondition := utils.GetCondition(&rayCluster.Status.StatusConditions, constants.ScheduledCondition, rayCluster.Generation)
			require.NotNil(t, actualCondition)
			require.Equal(t, tt.wantCondition.Status, actualCondition.Status)
			require.Equal(t, tt.wantCondition.Reason, actualCondition.Reason)
			require.Equal(t, tt.wantAssignment, rayCluster.Status.Assignment)
		})
	}
}

func TestFetchLatestRayCluster(t *testing.T) {
	tests := []struct {
		name      string
		job       matypes.SchedulableJob
		wantError bool
	}{
		{
			name: "fetch ray cluster successfully",
			job: framework.BatchRayCluster{
				RayCluster: &v2pb.RayCluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "test-ray-ns",
						Name:      "test-ray-cluster",
					},
				},
			},
			wantError: false,
		},
		{
			name: "fetch spark job successfully",
			job: framework.BatchSparkJob{
				SparkJob: &v2pb.SparkJob{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "test-spark-ns",
						Name:      "test-spark-job",
					},
				},
			},
			wantError: false,
		},
		{
			name: "unrecognized job type",
			job: matypes.NewSchedulableJob(matypes.SchedulableJobParams{
				Name:       "test-name",
				Namespace:  "test-ns",
				Generation: 1,
				JobType:    matypes.JobType(99), // Invalid job type
			}),
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			err := v2pb.AddToScheme(scheme)
			require.NoError(t, err)

			runTimeClient := fake.
				NewClientBuilder().
				WithScheme(scheme).
				Build()

			apiHandler := apiHandler.NewFakeAPIHandler(runTimeClient)
			if batchJob, ok := tt.job.(framework.BatchJob); ok {
				createErr := apiHandler.Create(context.Background(), batchJob.GetObject(), &metav1.CreateOptions{})
				require.NoError(t, createErr)
			}

			sc := &Scheduler{
				Handler: apiHandler,
			}

			var latest framework.BatchJob
			err = sc.fetchLatestJob(context.Background(), tt.job, &latest)
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.job.GetNamespace(), latest.GetNamespace())
				require.Equal(t, tt.job.GetName(), latest.GetName())
			}
		})
	}
}

func TestEnqueue(t *testing.T) {
	rayCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name:      "test-ray-cluster",
		},
		Spec: v2pb.RayClusterSpec{
			Head: &v2pb.RayHeadSpec{
				Pod: &corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "ray-head",
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("1"),
										corev1.ResourceMemory: resource.MustParse("2Gi"),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	testJob := framework.BatchRayCluster{RayCluster: rayCluster}
	g := gomock.NewController(t)
	defer g.Finish()

	mockEngine := frameworkmocks.NewMockAssignmentEngine(g)
	scheduler := setupTestScheduler(t, testJob, mockEngine)
	scheduler.initLock.Store(true)

	err := scheduler.Enqueue(context.Background(), testJob)
	require.NoError(t, err)

	testScope := scheduler.metrics.MetricsScope.(tally.TestScope)
	ta := testScope.Snapshot().Counters()
	assert.Equal(t, int64(1), ta["test.scheduler.job.enqueue_success_count+controller=scheduler"].Value())
}

func TestEnqueueNotInitialized(t *testing.T) {
	rayCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name:      "test-ray-cluster",
		},
	}

	testJob := framework.BatchRayCluster{RayCluster: rayCluster}
	g := gomock.NewController(t)
	defer g.Finish()

	mockEngine := frameworkmocks.NewMockAssignmentEngine(g)
	scheduler := setupTestScheduler(t, testJob, mockEngine)

	// Don't set initLock to true
	err := scheduler.Enqueue(context.Background(), testJob)
	require.Error(t, err)
	require.Contains(t, err.Error(), "scheduler_not_initialized")

	testScope := scheduler.metrics.MetricsScope.(tally.TestScope)
	ta := testScope.Snapshot().Counters()
	assert.Equal(t, int64(1), ta["test.scheduler.scheduler_not_initialized+controller=scheduler"].Value())
}

// Test Helpers
// -------------

func setupTestScheduler(t *testing.T, batchJob framework.BatchJob, assignmentStrategy framework.AssignmentStrategy) *Scheduler {
	scheme := runtime.NewScheme()
	err := v2pb.AddToScheme(scheme)
	require.NoError(t, err)

	// Create fake client with the job object
	mockClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(batchJob.GetObject()).
		WithStatusSubresource(&v2pb.RayCluster{}, &v2pb.SparkJob{}).
		Build()

	// Create real manager with fake config
	mgr, err := ctrl.NewManager(&rest.Config{}, ctrl.Options{
		Scheme: scheme,
		Logger: zapr.NewLogger(zaptest.NewLogger(t)),
	})
	require.NoError(t, err)

	// Set up dependencies
	testScope := tally.NewTestScope("test", map[string]string{})

	// Create mock API handler factory
	g := gomock.NewController(t)
	apiHandlerFactory := handlermocks.NewMockFactory(g)

	// Setup Mock Expectations
	apiHandlerFactory.EXPECT().
		GetAPIHandler(gomock.Any()).
		Return(apiHandler.NewFakeAPIHandler(mockClient), nil).
		AnyTimes()

	// Create the scheduler with real manager and mocked dependencies
	params := Params{
		Manager:            mgr,
		Queue:              sched.New().Queue,
		ClusterCache:       setupMockClusterCache(g),
		Scope:              testScope,
		APIHandlerFactory:  apiHandlerFactory,
		AssignmentStrategy: assignmentStrategy,
	}

	scheduler := NewScheduler(params)
	return scheduler
}

// setupMockClusterCache creates and configures a mock cluster cache for testing
func setupMockClusterCache(g *gomock.Controller) *clustermocks.MockRegisteredClustersCache {
	mockClusterCache := clustermocks.NewMockRegisteredClustersCache(g)

	// Set up default expectations
	testCluster := &v2pb.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-cluster",
		},
		Spec: v2pb.ClusterSpec{
			Region: "test-region",
		},
	}

	mockClusterCache.EXPECT().GetCluster(gomock.Any()).Return(testCluster).AnyTimes()

	return mockClusterCache
}
