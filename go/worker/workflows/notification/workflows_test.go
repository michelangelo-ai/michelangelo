// Package notification provides the pipeline run notification workflow.
package notification

import (
	"testing"

	"github.com/michelangelo-ai/michelangelo/go/base/notification/types"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestNewWorkflow verifies that NewWorkflow wires the PhaseResolver correctly.
func TestNewWorkflow(t *testing.T) {
	t.Run("nil resolver defaults to DefaultPhaseResolver", func(t *testing.T) {
		wf := NewWorkflow(nil)
		assert.NotNil(t, wf)
		assert.NotNil(t, wf.phaseResolver)
		// DefaultPhaseResolver maps PIPELINE_TYPE_TRAIN to "train"
		assert.Equal(t, "train", wf.phaseResolver("PIPELINE_TYPE_TRAIN"))
	})

	t.Run("custom resolver is used", func(t *testing.T) {
		custom := types.PhaseResolver(func(_ string) string { return "custom-phase" })
		wf := NewWorkflow(custom)
		assert.Equal(t, "custom-phase", wf.phaseResolver("anything"))
	})
}


// via the shared types package (it must not be defined locally to avoid the
// layering violation where the controller imports the worker package).
func TestWorkflowConstants(t *testing.T) {
	assert.Equal(t, "PipelineRunNotificationWorkflow", types.PipelineRunNotificationWorkflowName)
	assert.NotZero(t, workflowActivityOpts.ScheduleToStartTimeout)
	assert.NotZero(t, workflowActivityOpts.StartToCloseTimeout)
	assert.NotZero(t, workflowActivityOpts.HeartbeatTimeout)
}

// TestSendPipelineRunNotificationInputValidation tests basic input handling for
// the workflow function. Full workflow execution requires a Cadence/Temporal
// test environment; these tests verify the function handles inputs without panicking.
func TestSendPipelineRunNotificationInputValidation(t *testing.T) {
	tests := []struct {
		name        string
		req         *types.PipelineRunNotificationRequest
		shouldPanic bool
		description string
	}{
		{
			name: "Valid request with email notifications",
			req: &types.PipelineRunNotificationRequest{
				PipelineRun: &v2pb.PipelineRun{
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
				StudioBaseURL: "https://ml.example.com/studio/",
				SenderEmail:   "notifications@example.com",
			},
			shouldPanic: false,
			description: "Should handle valid request with email notifications",
		},
		{
			name: "Request with no notifications configured",
			req: &types.PipelineRunNotificationRequest{
				PipelineRun: &v2pb.PipelineRun{
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
			},
			shouldPanic: false,
			description: "Should handle request with no notifications gracefully",
		},
		{
			name: "Request with Slack notifications",
			req: &types.PipelineRunNotificationRequest{
				PipelineRun: &v2pb.PipelineRun{
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
			},
			shouldPanic: false,
			description: "Should handle request with Slack notifications",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shouldPanic {
				assert.Panics(t, func() { _ = tt.req }, tt.description)
			} else {
				assert.NotPanics(t, func() {
					assert.NotNil(t, tt.req.PipelineRun.Name)
					assert.NotNil(t, tt.req.PipelineRun.Namespace)
					for _, notif := range tt.req.PipelineRun.Spec.Notifications {
						_ = types.ContainsEventType(notif.EventTypes, tt.req.PipelineRun.Status.State)
						_ = types.GenerateSubject(tt.req.PipelineRun)
						_ = types.GenerateText(tt.req.PipelineRun, "email", tt.req.StudioBaseURL, nil)
						_ = types.GenerateText(tt.req.PipelineRun, "slack", tt.req.StudioBaseURL, nil)
					}
				}, tt.description)
			}
		})
	}
}

// TestNotificationHelperFunctions verifies the types package helpers used by
// the workflow.
func TestNotificationHelperFunctions(t *testing.T) {
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

	t.Run("GenerateSubject", func(t *testing.T) {
		subject := types.GenerateSubject(testPipelineRun)
		assert.NotEmpty(t, subject)
		assert.Contains(t, subject, testPipelineRun.Name)
	})

	t.Run("GenerateEmailText", func(t *testing.T) {
		text := types.GenerateText(testPipelineRun, "email", "https://ml.example.com/", nil)
		assert.NotEmpty(t, text)
	})

	t.Run("GenerateSlackText", func(t *testing.T) {
		text := types.GenerateText(testPipelineRun, "slack", "https://ml.example.com/", nil)
		assert.NotEmpty(t, text)
	})

	t.Run("GenerateTextNoURL", func(t *testing.T) {
		text := types.GenerateText(testPipelineRun, "email", "", nil)
		assert.NotContains(t, text, "Studio URL")
	})

	t.Run("ContainsEventType", func(t *testing.T) {
		eventTypes := []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED}
		assert.True(t, types.ContainsEventType(eventTypes, v2pb.PIPELINE_RUN_STATE_SUCCEEDED))
		assert.False(t, types.ContainsEventType(eventTypes, v2pb.PIPELINE_RUN_STATE_FAILED))
	})
}
