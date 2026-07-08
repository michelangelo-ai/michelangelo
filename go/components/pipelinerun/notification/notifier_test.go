package notification

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	clientInterfaces "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	interface_mock "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface/interface_mock"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewPipelineRunNotifier_TaskListEmpty(t *testing.T) {
	logger := zap.NewNop()
	notifier, err := NewPipelineRunNotifier(Config{}, nil, logger)
	assert.NoError(t, err)
	assert.Nil(t, notifier, "empty TaskList should return nil notifier, not an error")
}

func TestPipelineRunNotifier_NotifyOnStateChange(t *testing.T) {
	tests := []struct {
		name           string
		oldPipelineRun *v2pb.PipelineRun
		newPipelineRun *v2pb.PipelineRun
		shouldNotify   bool
		expectedError  bool
		workflowError  error
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
							Emails:           []string{"test@example.com"},
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
			name: "Workflow start error is returned to caller",
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
							Emails:           []string{"test@example.com"},
						},
					},
				},
				Status: v2pb.PipelineRunStatus{
					State: v2pb.PIPELINE_RUN_STATE_SUCCEEDED,
				},
			},
			shouldNotify:  true,
			expectedError: true, // Error is returned; caller (reconciler) decides whether to block.
			workflowError: assert.AnError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock workflow client
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockClient := interface_mock.NewMockWorkflowClient(ctrl)

			// Set up expectations based on shouldNotify
			if tt.shouldNotify {
				mockExecution := &clientInterfaces.WorkflowExecution{RunID: "test-run-id"}
				mockClient.EXPECT().
					StartWorkflow(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(mockExecution, tt.workflowError).
					Times(1)
			}

			// Create notifier with mock workflow client
			logger := zap.NewNop() // Use no-op logger for tests
			notifier, err := NewPipelineRunNotifier(Config{TaskList: "notification-worker"}, mockClient, logger)
			require.NoError(t, err)

			// Execute the method under test
			notifyErr := notifier.NotifyOnStateChange(context.Background(), tt.oldPipelineRun, tt.newPipelineRun)

			// Verify results
			if tt.expectedError {
				assert.Error(t, notifyErr)
			} else {
				assert.NoError(t, notifyErr)
			}
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
							Emails:     []string{"test@example.com"},
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
			// Create notifier (workflow client not used in shouldNotify)
			logger := zap.NewNop()
			notifier, err := NewPipelineRunNotifier(Config{TaskList: "test-task-list"}, nil, logger)
			require.NoError(t, err)

			result := notifier.shouldNotify(tt.oldPipelineRun, tt.newPipelineRun, logger)
			assert.Equal(t, tt.expected, result)
		})
	}
}
