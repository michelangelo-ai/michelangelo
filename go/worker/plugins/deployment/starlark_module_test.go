package deployment

import (
	"testing"

	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/deployment"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
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

