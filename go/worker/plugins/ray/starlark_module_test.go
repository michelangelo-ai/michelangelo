package ray

import (
	"context"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/ray"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/stretchr/testify/mock"
	"go.uber.org/cadence"
	"go.uber.org/yarpc/yarpcerrors"
	"testing"

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

	rayCluster := &v2pb.RayCluster{}
	env.OnActivity(ray.Activities.CreateRayCluster, mock.Anything, mock.Anything).Once().
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
