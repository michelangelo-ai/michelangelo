package notification

import (
	"context"
	"testing"

	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap/zaptest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NotificationActivitiesTestSuite struct {
	suite.Suite
	activities *activities
}

func (suite *NotificationActivitiesTestSuite) SetupTest() {
	logger := zaptest.NewLogger(suite.T())
	suite.activities = &activities{
		logger: logger,
	}
}

func (suite *NotificationActivitiesTestSuite) TestSendEmailActivity() {
	request := SendEmailActivityRequest{
		To:      []string{"test@uber.com"},
		Subject: "Test Notification",
		Text:    "This is a test email.",
		SendAs:  "michelangelo-noreply@uber.com",
	}

	err := suite.activities.SendEmailActivity(context.Background(), request)
	assert.NoError(suite.T(), err)
}

func (suite *NotificationActivitiesTestSuite) TestSendSlackActivity() {
	request := SendSlackActivityRequest{
		Channel: "#test-alerts",
		Text:    "Test Slack notification",
	}

	err := suite.activities.SendSlackActivity(context.Background(), request)
	assert.NoError(suite.T(), err)
}

func (suite *NotificationActivitiesTestSuite) TestGenerateEmailActivity() {
	pipelineRun := &v2pb.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline-run",
			Namespace: "test-namespace",
		},
		Spec: v2pb.PipelineRunSpec{
			Pipeline: &apipb.ResourceIdentifier{
				Namespace: "test-namespace",
				Name:      "test-pipeline",
			},
		},
		Status: v2pb.PipelineRunStatus{
			State:  v2pb.PIPELINE_RUN_STATE_SUCCEEDED,
			LogUrl: "https://cadence.example.com/run/123",
		},
	}

	result, err := suite.activities.GenerateEmailActivity(context.Background(), pipelineRun)
	assert.NoError(suite.T(), err)
	assert.NotEmpty(suite.T(), result.Subject)
	assert.NotEmpty(suite.T(), result.Text)
	assert.Equal(suite.T(), "michelangelo-noreply@uber.com", result.SendAs)
}

func (suite *NotificationActivitiesTestSuite) TestGenerateSlackActivity() {
	pipelineRun := &v2pb.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline-run",
			Namespace: "test-namespace",
		},
		Spec: v2pb.PipelineRunSpec{
			Pipeline: &apipb.ResourceIdentifier{
				Namespace: "test-namespace",
				Name:      "test-pipeline",
			},
		},
		Status: v2pb.PipelineRunStatus{
			State:  v2pb.PIPELINE_RUN_STATE_FAILED,
			LogUrl: "https://cadence.example.com/run/456",
		},
	}

	result, err := suite.activities.GenerateSlackActivity(context.Background(), pipelineRun)
	assert.NoError(suite.T(), err)
	assert.NotEmpty(suite.T(), result)
	assert.Contains(suite.T(), result, "test-pipeline-run")
	assert.Contains(suite.T(), result, "PIPELINE_RUN_STATE_FAILED")
}

func TestNotificationActivitiesTestSuite(t *testing.T) {
	suite.Run(t, new(NotificationActivitiesTestSuite))
}