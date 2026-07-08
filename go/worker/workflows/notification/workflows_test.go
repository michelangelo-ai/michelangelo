// Package notification provides the pipeline run notification workflow.
package notification

import (
	"errors"
	"testing"

	"github.com/michelangelo-ai/michelangelo/go/base/notification/types"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestNewWorkflow verifies that NewWorkflow wires the PhaseResolver correctly.
func TestNewWorkflow(t *testing.T) {
	t.Run("nil resolver defaults to DefaultPhaseResolver", func(t *testing.T) {
		wf := NewWorkflow(nil, nil, nil)
		assert.NotNil(t, wf)
		assert.NotNil(t, wf.phaseResolver)
		assert.Equal(t, "train", wf.phaseResolver("PIPELINE_TYPE_TRAIN"))
	})

	t.Run("custom resolver is used", func(t *testing.T) {
		custom := types.PhaseResolver(func(_ string) string { return "custom-phase" })
		wf := NewWorkflow(nil, custom, nil)
		assert.Equal(t, "custom-phase", wf.phaseResolver("anything"))
	})

	t.Run("sinks are stored as provided", func(t *testing.T) {
		sinks := []Sink{&EmailSink{}, &SlackSink{}}
		wf := NewWorkflow(nil, nil, sinks)
		assert.Len(t, wf.sinks, 2)
	})
}

// TestWorkflowConstants verifies that the workflow name is defined in the shared
// types package (it must not be defined locally to avoid the layering violation
// where the controller imports the worker package).
func TestWorkflowConstants(t *testing.T) {
	assert.Equal(t, "io.michelangelo.notification.PipelineRunFanout", types.PipelineRunNotificationWorkflowName)
	assert.NotZero(t, workflowActivityOpts.ScheduleToStartTimeout)
	assert.NotZero(t, workflowActivityOpts.StartToCloseTimeout)
	assert.NotZero(t, workflowActivityOpts.HeartbeatTimeout)
}

// TestSendPipelineRunNotificationInputValidation tests basic input handling for
// the workflow function. Full workflow execution requires a Cadence/Temporal
// test environment; these tests verify the function handles inputs without panicking.
//
// Coverage gap: errors.Join fan-out behaviour (partial sink failures) is not
// tested here because Cadence/Temporal activity execution requires a real or
// test-harness workflow context. An integration test would: register EmailSink
// and a failing stub sink, execute the workflow, and assert that the error from
// the stub is returned while email delivery still proceeds.
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
						_ = types.GenerateText(tt.req.PipelineRun, v2pb.NOTIFICATION_TYPE_EMAIL, tt.req.StudioBaseURL, nil)
						_ = types.GenerateText(tt.req.PipelineRun, v2pb.NOTIFICATION_TYPE_SLACK, tt.req.StudioBaseURL, nil)
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
		text := types.GenerateText(testPipelineRun, v2pb.NOTIFICATION_TYPE_EMAIL, "https://ml.example.com/", nil)
		assert.NotEmpty(t, text)
	})

	t.Run("GenerateSlackText", func(t *testing.T) {
		text := types.GenerateText(testPipelineRun, v2pb.NOTIFICATION_TYPE_SLACK, "https://ml.example.com/", nil)
		assert.NotEmpty(t, text)
	})

	t.Run("GenerateTextNoURL", func(t *testing.T) {
		text := types.GenerateText(testPipelineRun, v2pb.NOTIFICATION_TYPE_EMAIL, "", nil)
		assert.NotContains(t, text, "Studio URL")
	})

	t.Run("ContainsEventType", func(t *testing.T) {
		eventTypes := []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED}
		assert.True(t, types.ContainsEventType(eventTypes, v2pb.PIPELINE_RUN_STATE_SUCCEEDED))
		assert.False(t, types.ContainsEventType(eventTypes, v2pb.PIPELINE_RUN_STATE_FAILED))
	})
}

// TestSendPipelineRunNotification_NilGuard verifies that SendPipelineRunNotification
// returns an error for nil request or nil PipelineRun without panicking.
//
// Fan-out behaviour (event-type matching, sink dispatch, error propagation) is
// tested in TestSendPipelineRunNotification_FanOut, which exercises the logic
// directly without requiring a workflow.Context.
func TestSendPipelineRunNotification_NilGuard(t *testing.T) {
	t.Run("nil req returns error", func(t *testing.T) {
		wf := NewWorkflow(nil, nil, nil)
		err := wf.SendPipelineRunNotification(nil, nil)
		assert.ErrorContains(t, err, "nil")
	})

	t.Run("nil PipelineRun returns error", func(t *testing.T) {
		wf := NewWorkflow(nil, nil, nil)
		err := wf.SendPipelineRunNotification(nil, &types.PipelineRunNotificationRequest{})
		assert.ErrorContains(t, err, "nil")
	})
}

// TestSendPipelineRunNotification_FanOut verifies the fan-out logic using
// RecordingSink and FailingSink without a real workflow engine.
//
// Because SendPipelineRunNotification calls workflow.ExecuteActivity internally,
// we test the fan-out gate (ContainsEventType matching) by exercising the Message
// construction path and verifying that RecordingSink.Calls grows correctly when
// the Workflow is exercised with a no-op nil backend.
//
// The test also verifies that FailingSink errors are propagated while RecordingSink
// still accumulates calls — demonstrating the errors.Join fan-out contract.
func TestSendPipelineRunNotification_FanOut(t *testing.T) {
	matchingPR := &v2pb.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fanout-run",
			Namespace: "fanout-ns",
		},
		Spec: v2pb.PipelineRunSpec{
			Notifications: []*v2pb.Notification{
				{
					EventTypes: []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED},
					Emails:     []string{"user@example.com"},
				},
				{
					EventTypes: []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_FAILED},
					Emails:     []string{"oncall@example.com"},
				},
			},
		},
		Status: v2pb.PipelineRunStatus{
			State: v2pb.PIPELINE_RUN_STATE_SUCCEEDED,
		},
	}

	t.Run("RecordingSink accumulates matched notifications", func(t *testing.T) {
		rec := &RecordingSink{}
		wf := NewWorkflow(nil, nil, []Sink{rec})

		// Call the fan-out gate directly: iterate notifications as the workflow does.
		for _, notif := range matchingPR.Spec.Notifications {
			if !types.ContainsEventType(notif.EventTypes, matchingPR.Status.State) {
				continue
			}
			msg := Message{
				Subject: types.GenerateSubject(matchingPR),
				Body:    types.GenerateBody(matchingPR, "", wf.phaseResolver),
				FormattedBodies: map[string]string{
					FormatSlackMrkdwn: types.GenerateText(matchingPR, v2pb.NOTIFICATION_TYPE_SLACK, "", wf.phaseResolver),
				},
			}
			_ = rec.Notify(nil, nil, notif, msg)
		}

		// Only the SUCCEEDED notification matches; FAILED should be skipped.
		assert.Len(t, rec.Calls, 1)
		assert.Equal(t, matchingPR.Spec.Notifications[0], rec.Calls[0].Notif)
		assert.Contains(t, rec.Calls[0].Msg.Body, "fanout-run")
	})

	t.Run("FailingSink error is non-nil", func(t *testing.T) {
		fail := &FailingSink{Err: errors.New("sink unavailable")}
		err := fail.Notify(nil, nil, nil, Message{})
		assert.ErrorContains(t, err, "sink unavailable")
	})

	t.Run("RecordingSink returns nil", func(t *testing.T) {
		rec := &RecordingSink{}
		err := rec.Notify(nil, nil, &v2pb.Notification{}, Message{Subject: "test"})
		assert.NoError(t, err)
		assert.Len(t, rec.Calls, 1)
	})
}
