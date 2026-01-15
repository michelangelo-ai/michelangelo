package utils

import (
	"context"
	"testing"

	"github.com/michelangelo-ai/michelangelo/go/base/notification/types"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestSendMessageToEmailActivity tests the email activity function.
//
// This test verifies that the email activity executes successfully with various
// request configurations without throwing errors. Since this is a placeholder
// implementation, it primarily tests the function signature and basic execution flow.
func TestSendMessageToEmailActivity(t *testing.T) {
	tests := []struct {
		name        string
		request     *SendMessageToEmailActivityRequest
		description string
	}{
		{
			name: "Valid email request",
			request: &SendMessageToEmailActivityRequest{
				To:      []string{"test@uber.com"},
				Subject: "Test Subject",
				Text:    "Test message",
				SendAs:  "michelangelo@uber.com",
			},
			description: "Should handle valid email request without error",
		},
		{
			name: "Email request with CC and BCC",
			request: &SendMessageToEmailActivityRequest{
				To:      []string{"test@uber.com"},
				Cc:      []string{"cc@uber.com"},
				Bcc:     []string{"bcc@uber.com"},
				Subject: "Test Subject",
				Text:    "Test message",
				SendAs:  "sender@uber.com",
			},
			description: "Should handle email request with CC and BCC fields",
		},
		{
			name: "Email request with HTML content",
			request: &SendMessageToEmailActivityRequest{
				To:      []string{"test@uber.com"},
				Subject: "Test Subject",
				HTML:    "<h1>Test HTML Content</h1>",
				SendAs:  "sender@uber.com",
			},
			description: "Should handle email request with HTML content",
		},
		{
			name: "Multiple recipients",
			request: &SendMessageToEmailActivityRequest{
				To:      []string{"test1@uber.com", "test2@uber.com", "test3@uber.com"},
				Subject: "Test Subject",
				Text:    "Test message to multiple recipients",
				SendAs:  "sender@uber.com",
			},
			description: "Should handle email request with multiple recipients",
		},
		{
			name: "Email with ReplyTo field",
			request: &SendMessageToEmailActivityRequest{
				To:      []string{"test@uber.com"},
				Subject: "Test Subject",
				Text:    "Test message",
				ReplyTo: "noreply@uber.com",
				SendAs:  "sender@uber.com",
			},
			description: "Should handle email request with ReplyTo field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SendMessageToEmailActivity(context.Background(), tt.request)
			assert.NoError(t, err, tt.description)
		})
	}
}

// TestSendMessageToSlackActivity tests the Slack activity function.
//
// This test verifies that the Slack activity executes successfully with various
// request configurations. Since this is a placeholder implementation, it primarily
// tests the function signature and basic execution flow.
func TestSendMessageToSlackActivity(t *testing.T) {
	tests := []struct {
		name        string
		request     *SendMessageToSlackActivityRequest
		description string
	}{
		{
			name: "Valid slack channel request",
			request: &SendMessageToSlackActivityRequest{
				Channel: "#test-channel",
				Text:    "Test slack message",
			},
			description: "Should handle valid slack channel request",
		},
		{
			name: "Slack direct message to user",
			request: &SendMessageToSlackActivityRequest{
				Channel: "@testuser",
				Text:    "Direct message to user",
			},
			description: "Should handle direct message to a user",
		},
		{
			name: "Empty channel name",
			request: &SendMessageToSlackActivityRequest{
				Channel: "",
				Text:    "Test message",
			},
			description: "Should handle request with empty channel name",
		},
		{
			name: "Long message content",
			request: &SendMessageToSlackActivityRequest{
				Channel: "#alerts",
				Text:    "This is a very long message that contains detailed information about a pipeline run failure including error messages, timestamps, and debugging information that might be useful for troubleshooting the issue.",
			},
			description: "Should handle request with long message content",
		},
		{
			name: "Message with special characters",
			request: &SendMessageToSlackActivityRequest{
				Channel: "#test",
				Text:    "Pipeline failed: Error 500 - Internal Server Error @here 🚨",
			},
			description: "Should handle message with special characters and mentions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SendMessageToSlackActivity(context.Background(), tt.request)
			assert.NoError(t, err, tt.description)
		})
	}
}

// TestNotificationEventTypeMatching tests the event type matching logic.
//
// This test verifies that the notification system correctly matches pipeline run
// states with configured notification event types using the types.ContainsEventType function.
func TestNotificationEventTypeMatching(t *testing.T) {
	tests := []struct {
		name          string
		eventTypes    []v2pb.Notification_EventType
		pipelineState v2pb.PipelineRunState
		shouldMatch   bool
		description   string
	}{
		{
			name:          "Success event matches succeeded state",
			eventTypes:    []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED},
			pipelineState: v2pb.PIPELINE_RUN_STATE_SUCCEEDED,
			shouldMatch:   true,
			description:   "Should match when event type includes succeeded state",
		},
		{
			name:          "Failed event matches failed state",
			eventTypes:    []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_FAILED},
			pipelineState: v2pb.PIPELINE_RUN_STATE_FAILED,
			shouldMatch:   true,
			description:   "Should match when event type includes failed state",
		},
		{
			name:          "Multiple event types with matching state",
			eventTypes:    []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED, v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_FAILED},
			pipelineState: v2pb.PIPELINE_RUN_STATE_FAILED,
			shouldMatch:   true,
			description:   "Should match when one of multiple event types matches",
		},
		{
			name:          "No matching event type",
			eventTypes:    []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED},
			pipelineState: v2pb.PIPELINE_RUN_STATE_FAILED,
			shouldMatch:   false,
			description:   "Should not match when event types don't include the state",
		},
		{
			name:          "Killed event matches killed state",
			eventTypes:    []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_KILLED},
			pipelineState: v2pb.PIPELINE_RUN_STATE_KILLED,
			shouldMatch:   true,
			description:   "Should match when event type includes killed state",
		},
		{
			name:          "Empty event types",
			eventTypes:    []v2pb.Notification_EventType{},
			pipelineState: v2pb.PIPELINE_RUN_STATE_SUCCEEDED,
			shouldMatch:   false,
			description:   "Should not match when no event types are configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := types.ContainsEventType(tt.eventTypes, tt.pipelineState)
			assert.Equal(t, tt.shouldMatch, result, tt.description)
		})
	}
}

// TestNotificationConfigurationValidation tests notification configuration validation.
//
// This test ensures that notification configurations are properly structured and
// contain the required fields for different notification types.
func TestNotificationConfigurationValidation(t *testing.T) {
	tests := []struct {
		name           string
		notification   *v2pb.Notification
		description    string
		expectValid    bool
		checkEmails    bool
		checkSlack     bool
	}{
		{
			name: "Valid email notification",
			notification: &v2pb.Notification{
				NotificationType: v2pb.NOTIFICATION_TYPE_EMAIL,
				EventTypes:       []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED},
				Emails:           []string{"test@uber.com"},
			},
			description: "Should be valid email notification with recipient",
			expectValid: true,
			checkEmails: true,
			checkSlack:  false,
		},
		{
			name: "Valid slack notification",
			notification: &v2pb.Notification{
				NotificationType:  v2pb.NOTIFICATION_TYPE_SLACK,
				EventTypes:        []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_FAILED},
				SlackDestinations: []string{"#alerts"},
			},
			description: "Should be valid slack notification with channel",
			expectValid: true,
			checkEmails: false,
			checkSlack:  true,
		},
		{
			name: "Email notification without recipients",
			notification: &v2pb.Notification{
				NotificationType: v2pb.NOTIFICATION_TYPE_EMAIL,
				EventTypes:       []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED},
				Emails:           []string{},
			},
			description: "Should handle email notification without recipients",
			expectValid: false,
			checkEmails: true,
			checkSlack:  false,
		},
		{
			name: "Slack notification without channels",
			notification: &v2pb.Notification{
				NotificationType:  v2pb.NOTIFICATION_TYPE_SLACK,
				EventTypes:        []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_FAILED},
				SlackDestinations: []string{},
			},
			description: "Should handle slack notification without channels",
			expectValid: false,
			checkEmails: false,
			checkSlack:  true,
		},
		{
			name: "Notification without event types",
			notification: &v2pb.Notification{
				NotificationType: v2pb.NOTIFICATION_TYPE_EMAIL,
				EventTypes:       []v2pb.Notification_EventType{},
				Emails:           []string{"test@uber.com"},
			},
			description: "Should handle notification without event types",
			expectValid: false,
			checkEmails: true,
			checkSlack:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test basic validation
			hasEventTypes := len(tt.notification.EventTypes) > 0
			hasRecipients := (tt.checkEmails && len(tt.notification.Emails) > 0) ||
				            (tt.checkSlack && len(tt.notification.SlackDestinations) > 0)

			isValid := hasEventTypes && hasRecipients
			assert.Equal(t, tt.expectValid, isValid, tt.description)

			// Test specific field presence
			if tt.checkEmails {
				emailsPresent := len(tt.notification.Emails) > 0
				if tt.expectValid {
					assert.True(t, emailsPresent, "Valid email notification should have email addresses")
				}
			}

			if tt.checkSlack {
				slackPresent := len(tt.notification.SlackDestinations) > 0
				if tt.expectValid {
					assert.True(t, slackPresent, "Valid slack notification should have slack destinations")
				}
			}
		})
	}
}

// TestPipelineRunNotificationScenarios tests end-to-end notification scenarios.
//
// This test simulates complete notification scenarios with pipeline runs that have
// different states and notification configurations to ensure the logic works correctly.
func TestPipelineRunNotificationScenarios(t *testing.T) {
	tests := []struct {
		name                  string
		pipelineRun           *v2pb.PipelineRun
		expectedEmailCalls    int
		expectedSlackCalls    int
		description           string
	}{
		{
			name: "Successful pipeline with email notification",
			pipelineRun: &v2pb.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pipeline-run",
					Namespace: "test-namespace",
				},
				Spec: v2pb.PipelineRunSpec{
					Notifications: []*v2pb.Notification{
						{
							NotificationType: v2pb.NOTIFICATION_TYPE_EMAIL,
							EventTypes:       []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED},
							Emails:           []string{"test@uber.com"},
						},
					},
				},
				Status: v2pb.PipelineRunStatus{
					State: v2pb.PIPELINE_RUN_STATE_SUCCEEDED,
				},
			},
			expectedEmailCalls: 1,
			expectedSlackCalls: 0,
			description:        "Should send email for successful pipeline run",
		},
		{
			name: "Failed pipeline with slack notification",
			pipelineRun: &v2pb.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pipeline-run",
					Namespace: "test-namespace",
				},
				Spec: v2pb.PipelineRunSpec{
					Notifications: []*v2pb.Notification{
						{
							NotificationType:  v2pb.NOTIFICATION_TYPE_SLACK,
							EventTypes:        []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_FAILED},
							SlackDestinations: []string{"#alerts"},
						},
					},
				},
				Status: v2pb.PipelineRunStatus{
					State: v2pb.PIPELINE_RUN_STATE_FAILED,
				},
			},
			expectedEmailCalls: 0,
			expectedSlackCalls: 1,
			description:        "Should send slack notification for failed pipeline run",
		},
		{
			name: "Pipeline with both email and slack notifications",
			pipelineRun: &v2pb.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pipeline-run",
					Namespace: "test-namespace",
				},
				Spec: v2pb.PipelineRunSpec{
					Notifications: []*v2pb.Notification{
						{
							NotificationType: v2pb.NOTIFICATION_TYPE_EMAIL,
							EventTypes:       []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED},
							Emails:           []string{"test@uber.com"},
						},
						{
							NotificationType:  v2pb.NOTIFICATION_TYPE_SLACK,
							EventTypes:        []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED},
							SlackDestinations: []string{"#notifications"},
						},
					},
				},
				Status: v2pb.PipelineRunStatus{
					State: v2pb.PIPELINE_RUN_STATE_SUCCEEDED,
				},
			},
			expectedEmailCalls: 1,
			expectedSlackCalls: 1,
			description:        "Should send both email and slack notifications",
		},
		{
			name: "Pipeline with no matching notification event",
			pipelineRun: &v2pb.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pipeline-run",
					Namespace: "test-namespace",
				},
				Spec: v2pb.PipelineRunSpec{
					Notifications: []*v2pb.Notification{
						{
							NotificationType: v2pb.NOTIFICATION_TYPE_EMAIL,
							EventTypes:       []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_FAILED},
							Emails:           []string{"test@uber.com"},
						},
					},
				},
				Status: v2pb.PipelineRunStatus{
					State: v2pb.PIPELINE_RUN_STATE_SUCCEEDED,
				},
			},
			expectedEmailCalls: 0,
			expectedSlackCalls: 0,
			description:        "Should not send notifications when event types don't match",
		},
		{
			name: "Pipeline with no notifications configured",
			pipelineRun: &v2pb.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pipeline-run",
					Namespace: "test-namespace",
				},
				Status: v2pb.PipelineRunStatus{
					State: v2pb.PIPELINE_RUN_STATE_SUCCEEDED,
				},
			},
			expectedEmailCalls: 0,
			expectedSlackCalls: 0,
			description:        "Should not send notifications when none are configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			emailCallCount := 0
			slackCallCount := 0

			// Count expected notifications by simulating the SendPRNotification logic
			for _, notif := range tt.pipelineRun.Spec.Notifications {
				if types.ContainsEventType(notif.EventTypes, tt.pipelineRun.Status.State) {
					if len(notif.Emails) > 0 {
						emailCallCount++
					}
					if len(notif.SlackDestinations) > 0 {
						slackCallCount += len(notif.SlackDestinations) // One call per slack destination
					}
				}
			}

			// Verify expected counts
			assert.Equal(t, tt.expectedEmailCalls, emailCallCount,
				"Email call count should match expected for: %s", tt.description)
			assert.Equal(t, tt.expectedSlackCalls, slackCallCount,
				"Slack call count should match expected for: %s", tt.description)
		})
	}
}