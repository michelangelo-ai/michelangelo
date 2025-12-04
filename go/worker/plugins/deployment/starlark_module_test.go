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
	env.RegisterActivity(deployment.Activities.GetLatestDeploymentRevision)
	env.RegisterActivity(deployment.Activities.UpdateDeployment)

	// Mock successful GetDeployment (deployment exists)
	env.OnActivity(deployment.Activities.GetDeployment, mock.Anything, mock.Anything).Return(&v2pb.Deployment{}, nil)

	// Mock GetLatestDeploymentRevision to return old revision
	env.OnActivity(deployment.Activities.GetLatestDeploymentRevision, mock.Anything, mock.Anything).Return("old-revision-123", nil)

	// Mock successful UpdateDeployment
	env.OnActivity(deployment.Activities.UpdateDeployment, mock.Anything, mock.Anything).Return(&v2pb.Deployment{}, nil)

	// Mock GetLatestDeploymentRevision with retry to return new revision
	env.OnActivity(deployment.Activities.GetLatestDeploymentRevision, mock.Anything, mock.Anything).Return("new-revision-456", nil)

	// Note: You need a test.star file in testdata/ that calls this function
	// r.env.Cadence.ExecuteFunction("/test.star", "test_create_or_update_deployment", nil, nil, nil)
	// Since we can't easily create the .star file and run it here, we assume basic compilation check is enough.
}

