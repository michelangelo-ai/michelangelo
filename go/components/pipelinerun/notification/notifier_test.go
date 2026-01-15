package notification

import (
	"context"
	"testing"

	"github.com/michelangelo-ai/michelangelo/go/base/notification/types"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MockNotificationProvider is a mock implementation of NotificationProvider for testing
type MockNotificationProvider struct {
	mock.Mock
}

func (m *MockNotificationProvider) SendPipelineRunNotification(ctx context.Context, event *types.NotificationEvent) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

func TestPipelineRunNotifier_NotifyOnStateChange(t *testing.T) {
	tests := []struct {
		name           string
		oldPipelineRun *v2pb.PipelineRun
		newPipelineRun *v2pb.PipelineRun
		shouldNotify   bool
		expectedError  bool
		providerError  error
	}{
		{
			name: "No state change - should not notify",
			oldPipelineRun: &v2pb.PipelineRun{
				Status: v2pb.PipelineRunStatus{
					State: v2pb.PIPELINE_RUN_STATE_RUNNING,
				},
			},
			newPipelineRun: &v2pb.PipelineRun{
				Status: v2pb.PipelineRunStatus{
					State: v2pb.PIPELINE_RUN_STATE_RUNNING,
				},
			},
			shouldNotify:  false,
			expectedError: false,
		},
		{
			name: "State change to succeeded - should notify",
			oldPipelineRun: &v2pb.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pipeline-run",
					Namespace: "test-namespace",
				},
				Status: v2pb.PipelineRunStatus{
					State: v2pb.PIPELINE_RUN_STATE_RUNNING,
				},
			},
			newPipelineRun: &v2pb.PipelineRun{
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
			shouldNotify:  true,
			expectedError: false,
		},
		{
			name: "State change to failed - should notify",
			oldPipelineRun: &v2pb.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pipeline-run",
					Namespace: "test-namespace",
				},
				Status: v2pb.PipelineRunStatus{
					State: v2pb.PIPELINE_RUN_STATE_RUNNING,
				},
			},
			newPipelineRun: &v2pb.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pipeline-run",
					Namespace: "test-namespace",
				},
				Spec: v2pb.PipelineRunSpec{
					Notifications: []*v2pb.Notification{
						{
							NotificationType: v2pb.NOTIFICATION_TYPE_SLACK,
							EventTypes:       []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_FAILED},
							SlackDestinations: []string{"#alerts"},
						},
					},
				},
				Status: v2pb.PipelineRunStatus{
					State: v2pb.PIPELINE_RUN_STATE_FAILED,
				},
			},
			shouldNotify:  true,
			expectedError: false,
		},
		{
			name: "No notifications configured - should not notify",
			oldPipelineRun: &v2pb.PipelineRun{
				Status: v2pb.PipelineRunStatus{
					State: v2pb.PIPELINE_RUN_STATE_RUNNING,
				},
			},
			newPipelineRun: &v2pb.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pipeline-run",
					Namespace: "test-namespace",
				},
				Status: v2pb.PipelineRunStatus{
					State: v2pb.PIPELINE_RUN_STATE_SUCCEEDED,
				},
			},
			shouldNotify:  false,
			expectedError: false,
		},
		{
			name: "Provider error - should not fail reconciliation",
			oldPipelineRun: &v2pb.PipelineRun{
				Status: v2pb.PipelineRunStatus{
					State: v2pb.PIPELINE_RUN_STATE_RUNNING,
				},
			},
			newPipelineRun: &v2pb.PipelineRun{
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
			shouldNotify:  true,
			expectedError: false, // Should not propagate provider errors
			providerError: assert.AnError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock provider
			mockProvider := &MockNotificationProvider{}

			// Set up expectations based on shouldNotify
			if tt.shouldNotify {
				mockProvider.On("SendPipelineRunNotification", mock.Anything, mock.Anything).Return(tt.providerError)
			}

			// Create notifier with mock provider
			logger := zap.NewNop() // Use no-op logger for tests
			notifier := NewPipelineRunNotifier(mockProvider, logger)

			// Execute the method under test
			err := notifier.NotifyOnStateChange(context.Background(), tt.oldPipelineRun, tt.newPipelineRun)

			// Verify results
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Verify mock expectations
			mockProvider.AssertExpectations(t)
		})
	}
}

func TestPipelineRunNotifier_ShouldNotify(t *testing.T) {
	tests := []struct {
		name           string
		oldPipelineRun *v2pb.PipelineRun
		newPipelineRun *v2pb.PipelineRun
		expected       bool
	}{
		{
			name: "No state change",
			oldPipelineRun: &v2pb.PipelineRun{
				Status: v2pb.PipelineRunStatus{
					State: v2pb.PIPELINE_RUN_STATE_RUNNING,
				},
			},
			newPipelineRun: &v2pb.PipelineRun{
				Status: v2pb.PipelineRunStatus{
					State: v2pb.PIPELINE_RUN_STATE_RUNNING,
				},
			},
			expected: false,
		},
		{
			name: "State change with notifications configured",
			oldPipelineRun: &v2pb.PipelineRun{
				Status: v2pb.PipelineRunStatus{
					State: v2pb.PIPELINE_RUN_STATE_RUNNING,
				},
			},
			newPipelineRun: &v2pb.PipelineRun{
				Spec: v2pb.PipelineRunSpec{
					Notifications: []*v2pb.Notification{
						{
							EventTypes: []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED},
							Emails:     []string{"test@uber.com"},
						},
					},
				},
				Status: v2pb.PipelineRunStatus{
					State: v2pb.PIPELINE_RUN_STATE_SUCCEEDED,
				},
			},
			expected: true,
		},
		{
			name: "State change without notifications configured",
			oldPipelineRun: &v2pb.PipelineRun{
				Status: v2pb.PipelineRunStatus{
					State: v2pb.PIPELINE_RUN_STATE_RUNNING,
				},
			},
			newPipelineRun: &v2pb.PipelineRun{
				Status: v2pb.PipelineRunStatus{
					State: v2pb.PIPELINE_RUN_STATE_SUCCEEDED,
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create notifier (provider not used in ShouldNotify)
			logger := zap.NewNop()
			notifier := NewPipelineRunNotifier(nil, logger)

			result := notifier.ShouldNotify(tt.oldPipelineRun, tt.newPipelineRun)
			assert.Equal(t, tt.expected, result)
		})
	}
}