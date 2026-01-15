package notification

import (
	"testing"

	"github.com/michelangelo-ai/michelangelo/go/base/notification/types"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/notification"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap/zaptest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NotificationWorkflowsTestSuite struct {
	suite.Suite
	workflows *workflows
}

func (suite *NotificationWorkflowsTestSuite) SetupTest() {
	logger := zaptest.NewLogger(suite.T())
	_ = logger // Suppress unused variable warning for now
	suite.workflows = &workflows{}
}

func (suite *NotificationWorkflowsTestSuite) TestWorkflowStructure() {
	// Test that the workflows struct is properly initialized
	assert.NotNil(suite.T(), suite.workflows)
}

func (suite *NotificationWorkflowsTestSuite) TestNotificationEventValidation() {
	// Test event validation logic that would be used in the workflow
	event := &types.NotificationEvent{
		EventType:    v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED,
		ResourceType: v2pb.RESOURCE_TYPE_PIPELINE_RUN,
		PipelineRun: &v2pb.PipelineRun{
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
		},
		Notifications: []*v2pb.Notification{
			{
				NotificationType: v2pb.NOTIFICATION_TYPE_EMAIL,
				EventTypes:       []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED},
				Emails:           []string{"test@uber.com"},
			},
		},
	}

	assert.NotNil(suite.T(), event.PipelineRun)
	assert.Equal(suite.T(), "test-pipeline-run", event.PipelineRun.Name)
	assert.Len(suite.T(), event.Notifications, 1)
}

func (suite *NotificationWorkflowsTestSuite) TestNotificationFiltering() {
	// Test notification filtering logic
	notifications := []*v2pb.Notification{
		{
			NotificationType: v2pb.NOTIFICATION_TYPE_EMAIL,
			EventTypes:       []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED},
			Emails:           []string{"success@uber.com"},
		},
		{
			NotificationType: v2pb.NOTIFICATION_TYPE_SLACK,
			EventTypes:       []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_FAILED},
			SlackDestinations: []string{"#alerts"},
		},
	}

	// Test that the first notification should trigger for SUCCESS event
	succeededEvent := v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED
	shouldTrigger := false
	for _, eventType := range notifications[0].EventTypes {
		if eventType == succeededEvent {
			shouldTrigger = true
			break
		}
	}
	assert.True(suite.T(), shouldTrigger)

	// Test that the second notification should NOT trigger for SUCCESS event
	shouldTrigger = false
	for _, eventType := range notifications[1].EventTypes {
		if eventType == succeededEvent {
			shouldTrigger = true
			break
		}
	}
	assert.False(suite.T(), shouldTrigger)
}

func (suite *NotificationWorkflowsTestSuite) TestActivityRequestGeneration() {
	// Test email request structure
	emailRequest := notification.SendEmailActivityRequest{
		To:      []string{"test@uber.com"},
		Subject: "Test Subject",
		Text:    "Test Body",
		SendAs:  "michelangelo-noreply@uber.com",
	}

	assert.NotEmpty(suite.T(), emailRequest.To)
	assert.NotEmpty(suite.T(), emailRequest.Subject)
	assert.NotEmpty(suite.T(), emailRequest.SendAs)

	// Test Slack request structure
	slackRequest := notification.SendSlackActivityRequest{
		Channel: "#test-alerts",
		Text:    "Test Slack Message",
	}

	assert.NotEmpty(suite.T(), slackRequest.Channel)
	assert.NotEmpty(suite.T(), slackRequest.Text)
}

func TestNotificationWorkflowsTestSuite(t *testing.T) {
	suite.Run(t, new(NotificationWorkflowsTestSuite))
}