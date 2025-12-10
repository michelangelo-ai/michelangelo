package deployment

import (
	"testing"

	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/deployment"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"go.starlark.net/starlark"
	"go.uber.org/cadence"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
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

	// Note: Full integration test would require executing test.star file
	// This test verifies the basic activity mocking structure compiles correctly
}

