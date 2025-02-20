package ray

import (
	"context"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"

	"github.com/michelangelo-ai/michelangelo/go/worker/activities/ray"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/stretchr/testify/mock"
	"go.uber.org/cadence"
	"go.uber.org/yarpc/yarpcerrors"

	"github.com/cadence-workflow/starlark-worker/cadstar"
	"github.com/stretchr/testify/suite"
)

type Test struct {
	suite.Suite
	cadstar.StarTestSuite
	env *cadstar.StarTestEnvironment
}

func TestSuite(t *testing.T) { suite.Run(t, new(Test)) }

func (r *Test) SetupTest() {
	r.env = r.NewEnvironment(r.T(), &cadstar.StarTestEnvironmentParams{
		RootDirectory: "testdata",
		Plugins: map[string]cadstar.IPlugin{
			"ray": Plugin,
		},
	})
}

func (r *Test) TearDownTest() { r.env.AssertExpectations(r.T()) }

func (r *Test) TestCreateClusterSuccessfully() {
	env := r.env.GetTestWorkflowEnvironment()
	env.RegisterActivity(ray.Activities.CreateRayCluster)
	env.RegisterActivity(ray.Activities.TerminateCluster)
	env.RegisterActivity(ray.Activities.SensorRayClusterReadiness)

	rayCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "uf-ray-test",
			Namespace: "default",
		},
		Spec: v2pb.RayClusterSpec{
			User: &v2pb.UserInfo{
				Name:      "test-user",
				ProxyUser: "",
			},
			RayVersion: "2.3.1",
			Head: &v2pb.RayHeadSpec{
				ServiceType: "ClusterIP",
				Pod: &v1.PodTemplateSpec{
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{
								Name:  "head",
								Image: "test-image",
								EnvFrom: []v1.EnvFromSource{
									{
										Prefix: "",
										ConfigMapRef: &v1.ConfigMapEnvSource{
											LocalObjectReference: v1.LocalObjectReference{
												Name: "michelangelo-config",
											},
										},
									},
								},
								Lifecycle: &v1.Lifecycle{
									PostStart: &v1.LifecycleHandler{
										Exec: &v1.ExecAction{
											Command: []string{"/bin/sh", "-c", "echo", "'Initializing Ray Head'"},
										},
									},
								},
							},
						},
					},
				},
				RayStartParams: map[string]string{
					"block":          "true",
					"dashboard-host": "0.0.0.0",
				},
			},
			Workers: []*v2pb.RayWorkerSpec{
				{
					Pod: &v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Name:  "worker",
									Image: "test-image",
									EnvFrom: []v1.EnvFromSource{
										{
											Prefix: "",
											ConfigMapRef: &v1.ConfigMapEnvSource{
												LocalObjectReference: v1.LocalObjectReference{
													Name: "michelangelo-config",
												},
											},
										},
									},
									Lifecycle: &v1.Lifecycle{
										PostStart: &v1.LifecycleHandler{
											Exec: &v1.ExecAction{
												Command: []string{"/bin/sh", "-c", "echo", "'Initializing Ray Worker'"},
											},
										},
									},
								},
							},
						},
					},
					RayStartParams: map[string]string{
						"block":          "true",
						"dashboard-host": "0.0.0.0",
					},
					NodeType:     "worker-group-1",
					MinInstances: 1,
					MaxInstances: 2,
				},
			},
			RayConf: map[string]string{},
		},
		Status: v2pb.RayClusterStatus{},
	}
	var createClusterReq v2pb.CreateRayClusterRequest
	env.OnActivity(ray.Activities.CreateRayCluster, mock.Anything, mock.Anything).Once().
		Run(func(args mock.Arguments) {
			createClusterReq = args.Get(1).(v2pb.CreateRayClusterRequest) // Capture the request argument
		}).
		Return(func(ctx context.Context, req v2pb.CreateRayClusterRequest) (*v2pb.CreateRayClusterResponse, *cadence.CustomError) {
			return &v2pb.CreateRayClusterResponse{
				RayCluster: rayCluster,
			}, nil
		})

	env.OnActivity(ray.Activities.SensorRayClusterReadiness, mock.Anything, mock.Anything).Once().
		Return(func(ctx context.Context, req v2pb.GetRayClusterRequest) (*ray.SensorRayClusterReadinessResponse, *cadence.CustomError) {
			return &ray.SensorRayClusterReadinessResponse{
				RayCluster: rayCluster,
				Ready:      true,
			}, nil
		})

	r.env.ExecuteFunction("/test.star", "test_create_cluster", nil, nil, nil)
	require := r.Require()
	var res any
	err := r.env.GetResult(&res)
	require.NoError(err)
	require.EqualValues(rayCluster, createClusterReq.RayCluster)
	require.NotNil(res.(map[string]interface{}))
}

func (r *Test) TestCreateClusterFailed() {
	env := r.env.GetTestWorkflowEnvironment()
	env.RegisterActivity(ray.Activities.CreateRayCluster)
	env.RegisterActivity(ray.Activities.TerminateCluster)
	env.RegisterActivity(ray.Activities.SensorRayClusterReadiness)

	rayCluster := &v2pb.RayCluster{}
	env.OnActivity(ray.Activities.CreateRayCluster, mock.Anything, mock.Anything).Once().
		Return(func(ctx context.Context, req v2pb.CreateRayClusterRequest) (*v2pb.CreateRayClusterResponse, *cadence.CustomError) {
			return &v2pb.CreateRayClusterResponse{
				RayCluster: rayCluster,
			}, nil
		})

	env.OnActivity(ray.Activities.TerminateCluster, mock.Anything, mock.Anything).Once().
		Return(func(ctx context.Context, req ray.TerminateClusterRequest) (*v2pb.UpdateRayClusterResponse, *cadence.CustomError) {
			return &v2pb.UpdateRayClusterResponse{
				RayCluster: rayCluster,
			}, nil
		})

	env.OnActivity(ray.Activities.SensorRayClusterReadiness, mock.Anything, mock.Anything).Once().
		Return(func(ctx context.Context, req v2pb.GetRayClusterRequest) (*ray.SensorRayClusterReadinessResponse, *cadence.CustomError) {
			return nil, cadence.NewCustomError(yarpcerrors.CodeInternal.String(), "failed")
		})

	r.env.ExecuteFunction("/test.star", "test_create_cluster", nil, nil, nil)
	require := r.Require()
	var res any
	err := r.env.GetResult(&res)
	require.Error(err)
	require.Nil(res)
}

func (r *Test) TestCreateRayJobSuccessfully() {
	env := r.env.GetTestWorkflowEnvironment()
	env.RegisterActivity(ray.Activities.CreateRayJob)
	env.RegisterActivity(ray.Activities.SensorRayJob)

	rayJob := &v2pb.RayJob{}
	env.OnActivity(ray.Activities.CreateRayJob, mock.Anything, mock.Anything).Once().
		Return(func(ctx context.Context, req v2pb.CreateRayJobRequest) (*v2pb.CreateRayJobResponse, *cadence.CustomError) {
			return &v2pb.CreateRayJobResponse{
				RayJob: rayJob,
			}, nil
		})

	env.OnActivity(ray.Activities.SensorRayJob, mock.Anything, mock.Anything).Once().
		Return(func(ctx context.Context, req v2pb.GetRayJobRequest) (*ray.SensorRayJobResponse, *cadence.CustomError) {
			return &ray.SensorRayJobResponse{
				RayJob:   rayJob,
				JobURL:   "",
				Terminal: false,
			}, nil
		})

	r.env.ExecuteFunction("/test.star", "test_create_job", nil, nil, nil)
	require := r.Require()
	var res any
	err := r.env.GetResult(&res)
	require.NoError(err)
	require.NotNil(res.(map[string]interface{}))
}

func (r *Test) TestCreateRayJobFailed() {
	env := r.env.GetTestWorkflowEnvironment()
	env.RegisterActivity(ray.Activities.CreateRayJob)
	env.RegisterActivity(ray.Activities.SensorRayJob)

	rayJob := &v2pb.RayJob{}
	env.OnActivity(ray.Activities.CreateRayJob, mock.Anything, mock.Anything).Once().
		Return(func(ctx context.Context, req v2pb.CreateRayJobRequest) (*v2pb.CreateRayJobResponse, *cadence.CustomError) {
			return &v2pb.CreateRayJobResponse{
				RayJob: rayJob,
			}, nil
		})

	env.OnActivity(ray.Activities.SensorRayJob, mock.Anything, mock.Anything).Once().
		Return(func(ctx context.Context, req v2pb.GetRayJobRequest) (*ray.SensorRayJobResponse, *cadence.CustomError) {
			return nil, cadence.NewCustomError(yarpcerrors.CodeInternal.String(), "failed")
		})

	r.env.ExecuteFunction("/test.star", "test_create_job", nil, nil, nil)
	require := r.Require()
	var res any
	err := r.env.GetResult(&res)
	require.Error(err)
	require.Nil(res)
}
