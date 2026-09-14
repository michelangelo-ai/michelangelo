package deployment

import (
	"errors"
	"testing"

	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/deployment"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"go.starlark.net/starlark"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
			pluginID: Plugin,
		},
	})
}

func (r *Test) TearDownTest() {
	r.env.Cadence.AssertExpectations(r.T())
	r.env.Temporal.AssertExpectations(r.T())
}

func (r *Test) TestCreateOrUpdateDeployment_Update() {
	env := r.env.Cadence.GetTestWorkflowEnvironment()
	env.RegisterActivity(deployment.Activities.GetDeployment)
	env.RegisterActivity(deployment.Activities.UpdateDeployment)

	// Mock successful GetDeployment (deployment exists)
	existingDeployment := &v2pb.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-deployment",
			Namespace:       "test-namespace",
			ResourceVersion: "12345",
		},
		Spec: v2pb.DeploymentSpec{},
	}
	env.OnActivity(deployment.Activities.GetDeployment, mock.Anything, mock.Anything).Return(existingDeployment, nil)

	// Mock successful UpdateDeployment
	env.OnActivity(deployment.Activities.UpdateDeployment, mock.Anything, mock.Anything).Return(&v2pb.Deployment{}, nil)

	r.env.Cadence.ExecuteFunction("/test.star", "test_update_deployment", nil, nil, nil)
	require := r.Require()
	var res any
	err := r.env.Cadence.GetResult(&res)
	require.NoError(err)
	resMap := res.(map[string]interface{})
	require.Equal("test-deployment-1", resMap["deployment_name"])
	require.Equal("test-model-revision-2", resMap["model_revision_name"])
}

func (r *Test) TestCreateOrUpdateDeployment_Create() {
	env := r.env.Cadence.GetTestWorkflowEnvironment()
	env.RegisterActivity(deployment.Activities.GetDeployment)
	env.RegisterActivity(deployment.Activities.CreateDeployment)

	// The target deployment doesn't exist yet: GetDeployment reports this as (nil, nil).
	env.OnActivity(deployment.Activities.GetDeployment, mock.Anything, mock.MatchedBy(func(req *v2pb.GetDeploymentRequest) bool {
		return req.Name == "test-deployment-1"
	})).Return(nil, nil)

	// The template deployment exists and is used as the base for the new one.
	template := &v2pb.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-template-deployment",
			Namespace: "ma-dev-test",
			Labels:    map[string]string{"team": "michelangelo"},
		},
		Spec: v2pb.DeploymentSpec{},
	}
	env.OnActivity(deployment.Activities.GetDeployment, mock.Anything, mock.MatchedBy(func(req *v2pb.GetDeploymentRequest) bool {
		return req.Name == "test-template-deployment"
	})).Return(template, nil)

	env.OnActivity(deployment.Activities.CreateDeployment, mock.Anything, mock.Anything).Return(&v2pb.Deployment{}, nil)

	r.env.Cadence.ExecuteFunction("/test.star", "test_create_deployment", nil, nil, nil)
	require := r.Require()
	var res any
	err := r.env.Cadence.GetResult(&res)
	require.NoError(err)
	resMap := res.(map[string]interface{})
	require.Equal("test-deployment-1", resMap["deployment_name"])
	require.Equal("test-model-revision-1", resMap["model_revision_name"])
}

func (r *Test) TestCreateOrUpdateDeployment_MissingTemplate() {
	env := r.env.Cadence.GetTestWorkflowEnvironment()
	env.RegisterActivity(deployment.Activities.GetDeployment)

	// The target deployment doesn't exist, and no template was provided.
	env.OnActivity(deployment.Activities.GetDeployment, mock.Anything, mock.Anything).Return(nil, nil)

	r.env.Cadence.ExecuteFunction("/test.star", "test_create_deployment_without_template", nil, nil, nil)
	require := r.Require()
	var res any
	err := r.env.Cadence.GetResult(&res)
	require.Error(err)
}

func (r *Test) TestCreateOrUpdateDeployment_HardFailure() {
	env := r.env.Cadence.GetTestWorkflowEnvironment()
	env.RegisterActivity(deployment.Activities.GetDeployment)

	// A genuine (non-not-found) failure on the existence check must be
	// propagated, not silently treated as "deployment doesn't exist".
	env.OnActivity(deployment.Activities.GetDeployment, mock.Anything, mock.Anything).
		Return(nil, errors.New("apiserver unavailable"))

	r.env.Cadence.ExecuteFunction("/test.star", "test_create_or_update_deployment_hard_failure", nil, nil, nil)
	require := r.Require()
	var res any
	err := r.env.Cadence.GetResult(&res)
	require.Error(err)
}

func (r *Test) TestWaitForDeployment_Success() {
	env := r.env.Cadence.GetTestWorkflowEnvironment()
	env.RegisterActivity(deployment.Activities.SensorDeployment)

	finalDeployment := &v2pb.Deployment{
		Spec: v2pb.DeploymentSpec{
			DesiredRevision: &apipb.ResourceIdentifier{Name: "test-model-revision-2"},
		},
		Status: v2pb.DeploymentStatus{
			Stage:           v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
			CurrentRevision: &apipb.ResourceIdentifier{Name: "test-model-revision-2"},
		},
	}
	env.OnActivity(deployment.Activities.SensorDeployment, mock.Anything, mock.Anything).Once().Return(finalDeployment, nil)

	r.env.Cadence.ExecuteFunction("/test.star", "test_wait_for_deployment",
		starlark.Tuple{starlark.String("test-model-revision-2")}, nil, nil)
	require := r.Require()
	var res any
	err := r.env.Cadence.GetResult(&res)
	require.NoError(err)
	resMap := res.(map[string]interface{})
	require.Equal("DEPLOYMENT_STAGE_ROLLOUT_COMPLETE", resMap["stage"])
	require.Equal("test-model-revision-2", resMap["current_revision"])
	require.Equal("test-model-revision-2", resMap["desired_revision"])
}

func (r *Test) TestWaitForDeployment_FailedStageIsError() {
	env := r.env.Cadence.GetTestWorkflowEnvironment()
	env.RegisterActivity(deployment.Activities.SensorDeployment)

	// SensorDeployment reports any terminal stage (success or failure) without
	// erroring; waitForDeployment must translate a failed terminal stage into
	// an error rather than reporting it as a successful result.
	finalDeployment := &v2pb.Deployment{
		Spec: v2pb.DeploymentSpec{
			DesiredRevision: &apipb.ResourceIdentifier{Name: "test-model-revision-2"},
		},
		Status: v2pb.DeploymentStatus{
			Stage: v2pb.DEPLOYMENT_STAGE_ROLLOUT_FAILED,
		},
	}
	env.OnActivity(deployment.Activities.SensorDeployment, mock.Anything, mock.Anything).Once().Return(finalDeployment, nil)

	r.env.Cadence.ExecuteFunction("/test.star", "test_wait_for_deployment",
		starlark.Tuple{starlark.String("test-model-revision-2")}, nil, nil)
	require := r.Require()
	var res any
	err := r.env.Cadence.GetResult(&res)
	require.Error(err)
}
