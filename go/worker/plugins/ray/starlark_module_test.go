package ray

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/workflow"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/ray"
	"github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/mock"
	"go.uber.org/yarpc/yarpcerrors"
)

type Test struct {
	suite.Suite
	service.TestSuite
	env *service.TestEnvironment
}

func TestSuite(t *testing.T) { suite.Run(t, new(Test)) }

func (r *Test) SetupTest() {
	r.env = r.NewTestEnvironment(r.T(), &service.TestEnvironmentParams{
		RootDirectory: "testdata",
		Plugins: map[string]service.IPlugin{
			"ray": Plugin,
		},
	})
}

func (r *Test) TearDownTest() {
	r.env.Cadence.AssertExpectations(r.T())
	r.env.Temporal.AssertExpectations(r.T())
}

func (r *Test) TestCreateClusterSuccessfully() {
	env := r.env.Cadence.GetTestWorkflowEnvironment()
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
		Return(func(ctx context.Context, req v2pb.CreateRayClusterRequest) (*v2pb.CreateRayClusterResponse, error) {
			return &v2pb.CreateRayClusterResponse{
				RayCluster: rayCluster,
			}, nil
		})

	var sensorClusterReq v2pb.GetRayClusterRequest
	env.OnActivity(ray.Activities.SensorRayClusterReadiness, mock.Anything, mock.Anything).Once().
		Run(func(args mock.Arguments) {
			sensorClusterReq = args.Get(1).(v2pb.GetRayClusterRequest) // Capture the request argument
		}).
		Return(func(ctx context.Context, req v2pb.GetRayClusterRequest) (*ray.SensorRayClusterReadinessResponse, error) {
			return &ray.SensorRayClusterReadinessResponse{
				RayCluster: rayCluster,
				Ready:      true,
			}, nil
		})

	r.env.Cadence.ExecuteFunction("/test.star", "test_create_cluster", nil, nil, nil)
	require := r.Require()
	var res any
	err := r.env.Cadence.GetResult(&res)
	require.NoError(err)
	require.EqualValues(rayCluster, createClusterReq.RayCluster)
	require.EqualValues(rayCluster.Name, sensorClusterReq.Name)
	require.EqualValues(rayCluster.Namespace, sensorClusterReq.Namespace)
	require.NotNil(res.(map[string]interface{}))
}

func (r *Test) TestCreateClusterFailed() {
	env := r.env.Cadence.GetTestWorkflowEnvironment()
	env.RegisterActivity(ray.Activities.CreateRayCluster)
	env.RegisterActivity(ray.Activities.TerminateCluster)
	env.RegisterActivity(ray.Activities.SensorRayClusterReadiness)

	rayCluster := &v2pb.RayCluster{}
	env.OnActivity(ray.Activities.CreateRayCluster, mock.Anything, mock.Anything).Once().
		Return(func(ctx context.Context, req v2pb.CreateRayClusterRequest) (*v2pb.CreateRayClusterResponse, error) {
			return &v2pb.CreateRayClusterResponse{
				RayCluster: rayCluster,
			}, nil
		})

	env.OnActivity(ray.Activities.TerminateCluster, mock.Anything, mock.Anything).Once().
		Return(func(ctx context.Context, req ray.TerminateClusterRequest) (*v2pb.UpdateRayClusterResponse, error) {
			return &v2pb.UpdateRayClusterResponse{
				RayCluster: rayCluster,
			}, nil
		})

	env.OnActivity(ray.Activities.SensorRayClusterReadiness, mock.Anything, mock.Anything).Once().
		Return(func(ctx context.Context, req v2pb.GetRayClusterRequest) (*ray.SensorRayClusterReadinessResponse, error) {
			return nil, workflow.NewCustomError(ctx, yarpcerrors.CodeInternal.String(), "failed")
		})

	r.env.Cadence.ExecuteFunction("/test.star", "test_create_cluster", nil, nil, nil)
	require := r.Require()
	var res any
	err := r.env.Cadence.GetResult(&res)
	require.Error(err)
	require.Nil(res)
}

func (r *Test) TestCreateRayJobSuccessfully() {
	env := r.env.Cadence.GetTestWorkflowEnvironment()
	env.RegisterActivity(ray.Activities.CreateRayJob)
	env.RegisterActivity(ray.Activities.SensorRayJob)

	rayJob := &v2pb.RayJob{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "uf-rj-test-ray-job-",
			Namespace:    "default",
		},
		Spec: v2pb.RayJobSpec{
			User:                   nil,
			Entrypoint:             "python3 -m michelangelo.uniflow.core.run_task --task 'examples.bert_cola.data.load_data' --args '[\"glue\",\"cola\"]' --kwargs '{\"tokenizer_max_length\":128}' --result-url 's3://default/d47efe2f682f4965bcf119f9d9a06eb1.json'",
			ObjectStoreMemoryRatio: 0,
			JobId:                  "",
			Cluster: &api.ResourceIdentifier{
				Namespace: "default",
				Name:      "test-ray-job",
			},
		},
	}

	var createdRayJob v2pb.CreateRayJobRequest
	env.OnActivity(ray.Activities.CreateRayJob, mock.Anything, mock.Anything).Once().
		Run(func(args mock.Arguments) {
			createdRayJob = args.Get(1).(v2pb.CreateRayJobRequest) // Capture the request argument
		}).
		Return(func(ctx context.Context, req v2pb.CreateRayJobRequest) (*v2pb.CreateRayJobResponse, error) {
			return &v2pb.CreateRayJobResponse{
				RayJob: rayJob,
			}, nil
		})

	var sensorJobReq v2pb.GetRayJobRequest
	env.OnActivity(ray.Activities.SensorRayJob, mock.Anything, mock.Anything).Once().
		Run(func(args mock.Arguments) {
			sensorJobReq = args.Get(1).(v2pb.GetRayJobRequest) // Capture the request argument
		}).
		Return(func(ctx context.Context, req v2pb.GetRayJobRequest) (*ray.SensorRayJobResponse, error) {
			return &ray.SensorRayJobResponse{
				RayJob:   rayJob,
				JobURL:   "",
				Terminal: false,
			}, nil
		})

	r.env.Cadence.ExecuteFunction("/test.star", "test_create_job", nil, nil, nil)
	require := r.Require()
	var res any
	err := r.env.Cadence.GetResult(&res)
	require.NoError(err)
	require.EqualValues(createdRayJob.RayJob, rayJob)
	require.EqualValues(rayJob.Name, sensorJobReq.Name)
	require.EqualValues(rayJob.Namespace, sensorJobReq.Namespace)
	require.NotNil(res.(map[string]interface{}))
}

func (r *Test) TestCreateRayJobFailed() {
	env := r.env.Cadence.GetTestWorkflowEnvironment()
	env.RegisterActivity(ray.Activities.CreateRayJob)
	env.RegisterActivity(ray.Activities.SensorRayJob)

	rayJob := &v2pb.RayJob{}
	env.OnActivity(ray.Activities.CreateRayJob, mock.Anything, mock.Anything).Once().
		Return(func(ctx context.Context, req v2pb.CreateRayJobRequest) (*v2pb.CreateRayJobResponse, error) {
			return &v2pb.CreateRayJobResponse{
				RayJob: rayJob,
			}, nil
		})

	env.OnActivity(ray.Activities.SensorRayJob, mock.Anything, mock.Anything).Once().
		Return(func(ctx context.Context, req v2pb.GetRayJobRequest) (*ray.SensorRayJobResponse, error) {
			return nil, workflow.NewCustomError(ctx, yarpcerrors.CodeInternal.String(), "failed")
		})

	r.env.Cadence.ExecuteFunction("/test.star", "test_create_job", nil, nil, nil)
	require := r.Require()
	var res any
	err := r.env.Cadence.GetResult(&res)
	require.Error(err)
	require.Nil(res)
}
