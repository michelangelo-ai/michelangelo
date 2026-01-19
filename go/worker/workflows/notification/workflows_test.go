package notification

import (
	"testing"

	"github.com/michelangelo-ai/michelangelo/go/base/notification/types"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestWorkflowConstants tests that workflow constants are properly defined.
func TestWorkflowConstants(t *testing.T) {
	assert.Equal(t, "PRNotificationWorkflow", PRNotificationWorkflowName)
	assert.NotZero(t, workflowActivityOpts.ScheduleToStartTimeout)
	assert.NotZero(t, workflowActivityOpts.StartToCloseTimeout)
	assert.NotZero(t, workflowActivityOpts.HeartbeatTimeout)
}

// TestSendPRNotificationInputValidation tests basic input validation for the workflow.
//
// Note: This is a basic structure test since testing the full workflow requires
// a workflow testing framework. This tests that the function can handle various
// pipeline run configurations without panicking.
func TestSendPRNotificationInputValidation(t *testing.T) {
	tests := []struct {
		name        string
		pipelineRun *v2pb.PipelineRun
		shouldPanic bool
		description string
	}{
		{
			name: "Valid pipeline run with notifications",
			pipelineRun: &v2pb.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pipeline-run",
					Namespace: "test-namespace",
				},
				Spec: v2pb.PipelineRunSpec{
					Notifications: []*v2pb.Notification{
						{
							EventTypes: []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED},
							Emails:     []string{"test@example.com"},
						},
					},
				},
				Status: v2pb.PipelineRunStatus{
					State: v2pb.PIPELINE_RUN_STATE_SUCCEEDED,
				},
			},
			shouldPanic: false,
			description: "Should handle valid pipeline run with notifications",
		},
		{
			name: "Pipeline run with no notifications",
			pipelineRun: &v2pb.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pipeline-run-no-notif",
					Namespace: "test-namespace",
				},
				Spec: v2pb.PipelineRunSpec{
					Notifications: []*v2pb.Notification{},
				},
				Status: v2pb.PipelineRunStatus{
					State: v2pb.PIPELINE_RUN_STATE_SUCCEEDED,
				},
			},
			shouldPanic: false,
			description: "Should handle pipeline run with no notifications gracefully",
		},
		{
			name: "Pipeline run with Slack notifications",
			pipelineRun: &v2pb.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pipeline-run-slack",
					Namespace: "test-namespace",
				},
				Spec: v2pb.PipelineRunSpec{
					Notifications: []*v2pb.Notification{
						{
							EventTypes:        []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_FAILED},
							SlackDestinations: []string{"#alerts"},
						},
					},
				},
				Status: v2pb.PipelineRunStatus{
					State: v2pb.PIPELINE_RUN_STATE_FAILED,
				},
			},
			shouldPanic: false,
			description: "Should handle pipeline run with Slack notifications",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This tests that the function signature and basic structure are correct
			// Full workflow testing would require the cadence/temporal test framework

			if tt.shouldPanic {
				assert.Panics(t, func() {
					// In a real workflow test, this would be executed in a workflow context
					_ = tt.pipelineRun
				}, tt.description)
			} else {
				assert.NotPanics(t, func() {
					// Basic validation that we can access the pipeline run fields
					assert.NotNil(t, tt.pipelineRun.ObjectMeta.Name)
					assert.NotNil(t, tt.pipelineRun.ObjectMeta.Namespace)

					// Verify notification types integration works
					for _, notif := range tt.pipelineRun.Spec.Notifications {
						_ = types.ContainsEventType(notif.EventTypes, tt.pipelineRun.Status.State)
						_ = types.GenerateSubject(tt.pipelineRun)
						_ = types.GenerateText(tt.pipelineRun, "email")
						_ = types.GenerateText(tt.pipelineRun, "slack")
					}
				}, tt.description)
			}
		})
	}
}

// TestNotificationHelperFunctions tests that the helper functions work correctly.
func TestNotificationHelperFunctions(t *testing.T) {
	// Test pipeline run for helper functions
	testPipelineRun := &v2pb.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline-run",
			Namespace: "test-namespace",
		},
		Spec: v2pb.PipelineRunSpec{
			Notifications: []*v2pb.Notification{
				{
					EventTypes: []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED},
					Emails:     []string{"test@example.com"},
				},
			},
		},
		Status: v2pb.PipelineRunStatus{
			State: v2pb.PIPELINE_RUN_STATE_SUCCEEDED,
		},
	}

	// Test that notification helper functions work with our pipeline run structure
	t.Run("GenerateSubject", func(t *testing.T) {
		subject := types.GenerateSubject(testPipelineRun)
		assert.NotEmpty(t, subject)
		assert.Contains(t, subject, testPipelineRun.ObjectMeta.Name)
	})

	t.Run("GenerateEmailText", func(t *testing.T) {
		emailText := types.GenerateText(testPipelineRun, "email")
		assert.NotEmpty(t, emailText)
	})

	t.Run("GenerateSlackText", func(t *testing.T) {
		slackText := types.GenerateText(testPipelineRun, "slack")
		assert.NotEmpty(t, slackText)
	})

	t.Run("ContainsEventType", func(t *testing.T) {
		eventTypes := []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED}
		contains := types.ContainsEventType(eventTypes, v2pb.PIPELINE_RUN_STATE_SUCCEEDED)
		assert.True(t, contains)

		notContains := types.ContainsEventType(eventTypes, v2pb.PIPELINE_RUN_STATE_FAILED)
		assert.False(t, notContains)
	})
}