package deployment

import (
	"mock/github.com/michelangelo-ai/michelangelo/proto-go/api/v2/v2mock"
	"testing"

	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestSensorDeploymentRetriesStaleCompletedRevision(t *testing.T) {
	ctrl := gomock.NewController(t)
	deploymentService := v2mock.NewMockDeploymentServiceYARPCClient(ctrl)
	activity := &activities{deploymentService: deploymentService}
	activitySuite := service.NewCadTestActivitySuite()
	activitySuite.RegisterActivity(activity)

	deploymentService.EXPECT().GetDeployment(gomock.Any(), gomock.Any()).Return(
		&v2pb.GetDeploymentResponse{Deployment: &v2pb.Deployment{
			Spec: v2pb.DeploymentSpec{
				DesiredRevision: &apipb.ResourceIdentifier{Name: "revision-2"},
			},
			Status: v2pb.DeploymentStatus{
				Stage:           v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
				CurrentRevision: &apipb.ResourceIdentifier{Name: "revision-1"},
			},
		}}, nil,
	)

	_, err := activitySuite.ExecuteActivity(Activities.SensorDeployment, SensorDeploymentRequest{
		Namespace:             "default",
		DeploymentName:        "retrain-example",
		ExpectedModelRevision: "revision-2",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed-precondition")
}

func TestSensorDeploymentAcceptsExpectedCompletedRevision(t *testing.T) {
	ctrl := gomock.NewController(t)
	deploymentService := v2mock.NewMockDeploymentServiceYARPCClient(ctrl)
	activity := &activities{deploymentService: deploymentService}
	activitySuite := service.NewCadTestActivitySuite()
	activitySuite.RegisterActivity(activity)

	deploymentService.EXPECT().GetDeployment(gomock.Any(), gomock.Any()).Return(
		&v2pb.GetDeploymentResponse{Deployment: &v2pb.Deployment{
			Spec: v2pb.DeploymentSpec{
				DesiredRevision: &apipb.ResourceIdentifier{Name: "revision-2"},
			},
			Status: v2pb.DeploymentStatus{
				Stage:           v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
				CurrentRevision: &apipb.ResourceIdentifier{Name: "revision-2"},
			},
		}}, nil,
	)

	value, err := activitySuite.ExecuteActivity(Activities.SensorDeployment, SensorDeploymentRequest{
		Namespace:             "default",
		DeploymentName:        "retrain-example",
		ExpectedModelRevision: "revision-2",
	})
	assert.NoError(t, err)

	var result *v2pb.Deployment
	err = value.Get(&result)
	assert.NoError(t, err)
	assert.Equal(t, "revision-2", result.Status.CurrentRevision.Name)
}
