package triggerrun

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/zapr"
	"github.com/golang/mock/gomock"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	interfaceMock "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface/interface_mock"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_runID            = "test-run-id"
	_workflowID       = "test-workflow-id"
	_execTime   int64 = 1683616260555000000
	_logURL           = "http://localhost:8088/domains/default/workflows/test-namespace.test-triggerrun-name"
)

// getCronFromActualTrigger extracts the cron expression from a Trigger, returns empty string if not a cron trigger
func getCronFromActualTrigger(trigger *v2pb.Trigger) string {
	if trigger == nil || trigger.GetCronSchedule() == nil {
		return ""
	}
	return trigger.GetCronSchedule().GetCron()
}

func TestRun(t *testing.T) {
	tests := []struct {
		name                   string
		workflowClientProvider func(t *testing.T) clientInterface.WorkflowClient
		expectedStatus         v2pb.TriggerRunStatus
		expectError            bool
	}{
		{
			name: "already started",
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().GetDomain().Return("test-domain")
				mockClient.EXPECT().GetProvider().Return("test-provider").AnyTimes()
				mockClient.EXPECT().ListOpenWorkflow(
					gomock.Any(),
					gomock.Any(),
				).Return(
					&clientInterface.ListOpenWorkflowExecutionsResponse{
						Executions: []clientInterface.WorkflowExecutionInfo{
							{
								Execution: &clientInterface.WorkflowExecution{RunID: _runID},
							},
						},
					}, nil)
				return mockClient
			},
			expectedStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING},
			expectError:    false,
		},
		{
			name: "ListOpenWorkflow failed and start succeeded",
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().GetDomain().Return("test-domain")
				mockClient.EXPECT().GetProvider().Return("test-provider").AnyTimes()
				mockClient.EXPECT().ListOpenWorkflow(gomock.Any(), gomock.Any()).AnyTimes().Return(nil, fmt.Errorf("failed to list open workflow"))
				mockClient.EXPECT().StartWorkflow(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&clientInterface.WorkflowExecution{ID: _workflowID, RunID: _runID}, nil)
				return mockClient
			},
			expectedStatus: v2pb.TriggerRunStatus{LogUrl: _logURL, State: v2pb.TRIGGER_RUN_STATE_RUNNING, ActualTrigger: &v2pb.Trigger{TriggerType: &v2pb.Trigger_CronSchedule{CronSchedule: &v2pb.CronSchedule{Cron: "0 0 * * *"}}}},
			expectError:    false,
		},
		{
			name: "empty open workflow and start succeeded",
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().GetDomain().Return("test-domain")
				mockClient.EXPECT().GetProvider().Return("test-provider").AnyTimes()
				mockClient.EXPECT().ListOpenWorkflow(gomock.Any(), gomock.Any()).AnyTimes().Return(
					&clientInterface.ListOpenWorkflowExecutionsResponse{
						Executions: []clientInterface.WorkflowExecutionInfo{
							{Execution: &clientInterface.WorkflowExecution{RunID: ""}},
						},
					}, nil)
				mockClient.EXPECT().StartWorkflow(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&clientInterface.WorkflowExecution{ID: _workflowID, RunID: _runID}, nil)
				return mockClient
			},
			expectedStatus: v2pb.TriggerRunStatus{LogUrl: _logURL, State: v2pb.TRIGGER_RUN_STATE_RUNNING, ActualTrigger: &v2pb.Trigger{TriggerType: &v2pb.Trigger_CronSchedule{CronSchedule: &v2pb.CronSchedule{Cron: "0 0 * * *"}}}},
			expectError:    false,
		},
		{
			name: "start failed",
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().GetDomain().Return("test-domain")
				mockClient.EXPECT().GetProvider().Return("test-provider").AnyTimes()
				mockClient.EXPECT().ListOpenWorkflow(gomock.Any(), gomock.Any()).AnyTimes().Return(
					&clientInterface.ListOpenWorkflowExecutionsResponse{
						Executions: []clientInterface.WorkflowExecutionInfo{
							{Execution: &clientInterface.WorkflowExecution{RunID: ""}},
						},
					}, nil)
				mockClient.EXPECT().StartWorkflow(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("failed to start workflow"))
				return mockClient
			},
			expectedStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_FAILED, ErrorMessage: "failed to start workflow"},
			expectError:    true,
		},
	}

	for _, test := range tests {
		ct := setupCronTrigger(t, test.workflowClientProvider(t))
		trStatus, err := ct.Run(context.Background(), _triggerRun.DeepCopy())
		if test.expectError {
			assert.Error(t, err, test.name)
		} else {
			assert.NoError(t, err, test.name)
		}
		assert.Equal(t, test.expectedStatus, trStatus, test.name)
	}
}

func TestRunStoresActualNotifications(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
	mockClient.EXPECT().GetDomain().Return("test-domain")
	mockClient.EXPECT().GetProvider().Return("test-provider").AnyTimes()
	mockClient.EXPECT().ListOpenWorkflow(gomock.Any(), gomock.Any()).Return(
		&clientInterface.ListOpenWorkflowExecutionsResponse{}, nil)
	mockClient.EXPECT().StartWorkflow(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&clientInterface.WorkflowExecution{ID: _workflowID, RunID: _runID}, nil)

	triggerRun := _triggerRun.DeepCopy()
	triggerRun.Spec.Notifications = []*v2pb.Notification{
		{
			NotificationType: v2pb.NOTIFICATION_TYPE_EMAIL,
			EventTypes:       []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_STARTED},
			ResourceType:     v2pb.RESOURCE_TYPE_PIPELINE_RUN,
			Emails:           []string{"test@example.com"},
		},
	}

	status, err := setupCronTrigger(t, mockClient).Run(context.Background(), triggerRun)

	assert.NoError(t, err)
	assert.Equal(t, triggerRun.Spec.Notifications, status.ActualNotifications)
}

func TestKill(t *testing.T) {
	tests := []struct {
		name                   string
		workflowClientProvider func(t *testing.T) clientInterface.WorkflowClient
		expectedStatus         v2pb.TriggerRunStatus
		expectError            bool
	}{
		{
			name: "delete trigger succeeded",
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().GetDomain().Return("test-domain")
				mockClient.EXPECT().GetProvider().Return("test-provider").AnyTimes()
				mockClient.EXPECT().ListOpenWorkflow(gomock.Any(), gomock.Any()).Return(&clientInterface.ListOpenWorkflowExecutionsResponse{
					Executions: []clientInterface.WorkflowExecutionInfo{
						{Execution: &clientInterface.WorkflowExecution{RunID: _runID}},
					},
				}, nil)
				mockClient.EXPECT().DeleteTrigger(gomock.Any(), gomock.Any(), _runID).Return(nil)
				return mockClient
			},
			expectedStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_KILLED},
			expectError:    false,
		},
		{
			name: "delete trigger failed",
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().GetDomain().Return("test-domain")
				mockClient.EXPECT().GetProvider().Return("test-provider").AnyTimes()
				mockClient.EXPECT().ListOpenWorkflow(gomock.Any(), gomock.Any()).Return(&clientInterface.ListOpenWorkflowExecutionsResponse{
					Executions: []clientInterface.WorkflowExecutionInfo{
						{Execution: &clientInterface.WorkflowExecution{RunID: _runID}},
					},
				}, nil)
				mockClient.EXPECT().DeleteTrigger(gomock.Any(), gomock.Any(), _runID).Return(fmt.Errorf("failed to delete trigger"))
				return mockClient
			},
			expectedStatus: v2pb.TriggerRunStatus{State: _triggerRun.Status.State},
			expectError:    true,
		},
	}

	for _, test := range tests {
		ct := setupCronTrigger(t, test.workflowClientProvider(t))
		trStatus, err := ct.Kill(context.Background(), _triggerRun.DeepCopy())
		if test.expectError {
			assert.Error(t, err, test.name)
		} else {
			assert.NoError(t, err, test.name)
		}
		assert.Equal(t, test.expectedStatus, trStatus, test.name)
	}
}

func TestGetStatus(t *testing.T) {
	tests := []struct {
		name                   string
		workflowClientProvider func(t *testing.T) clientInterface.WorkflowClient
		expectedStatus         v2pb.TriggerRunStatus
		expectError            bool
	}{
		{
			name: "get status succeeded",
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().GetDomain().Return("test-domain")
				mockClient.EXPECT().GetProvider().Return("test-provider").AnyTimes()
				mockClient.EXPECT().ListOpenWorkflow(gomock.Any(), gomock.Any()).AnyTimes().Return(
					&clientInterface.ListOpenWorkflowExecutionsResponse{
						Executions: []clientInterface.WorkflowExecutionInfo{
							{
								Execution:     &clientInterface.WorkflowExecution{RunID: _runID},
								ExecutionTime: time.Unix(0, _execTime),
							},
						},
					}, nil)
				return mockClient
			},
			expectedStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING},
			expectError:    false,
		},
		{
			name: "list open workflow failed",
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().GetDomain().Return("test-domain")
				mockClient.EXPECT().GetProvider().Return("test-provider").AnyTimes()
				mockClient.EXPECT().ListOpenWorkflow(gomock.Any(), gomock.Any()).AnyTimes().Return(
					nil, fmt.Errorf("bad connection"))
				return mockClient
			},
			expectedStatus: v2pb.TriggerRunStatus{
				State:        _triggerRun.Status.State,
				ErrorMessage: "failed to list open workflow: bad connection",
			},
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ct := setupCronTrigger(t, test.workflowClientProvider(t))
			trStatus, err := ct.GetStatus(context.Background(), _triggerRun.DeepCopy())
			if test.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, test.expectedStatus, trStatus)
		})
	}
}

func TestPause(t *testing.T) {
	tests := []struct {
		name                   string
		workflowClientProvider func(t *testing.T) clientInterface.WorkflowClient
		expectedStatus         v2pb.TriggerRunStatus
		expectError            bool
	}{
		{
			name: "pause succeeded",
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().GetDomain().Return("test-domain")
				mockClient.EXPECT().PauseTrigger(gomock.Any(), gomock.Any()).Return(nil)
				return mockClient
			},
			expectedStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_PAUSED},
			expectError:    false,
		},
		{
			name: "pause failed",
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().GetDomain().Return("test-domain")
				mockClient.EXPECT().PauseTrigger(gomock.Any(), gomock.Any()).Return(fmt.Errorf("failed to pause"))
				return mockClient
			},
			expectedStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_FAILED, ErrorMessage: "failed to pause"},
			expectError:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ct := setupCronTrigger(t, test.workflowClientProvider(t))
			trStatus, err := ct.Pause(context.Background(), _triggerRun.DeepCopy())
			if test.expectError {
				assert.Error(t, err, test.name)
			} else {
				assert.NoError(t, err, test.name)
			}
			assert.Equal(t, test.expectedStatus, trStatus, test.name)
		})
	}
}

func TestResume(t *testing.T) {
	tests := []struct {
		name                   string
		workflowClientProvider func(t *testing.T) clientInterface.WorkflowClient
		expectedStatus         v2pb.TriggerRunStatus
		expectError            bool
	}{
		{
			name: "resume succeeded",
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().GetDomain().Return("test-domain")
				mockClient.EXPECT().UnpauseTrigger(gomock.Any(), gomock.Any()).Return(nil)
				return mockClient
			},
			expectedStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING},
			expectError:    false,
		},
		{
			name: "resume failed",
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().GetDomain().Return("test-domain")
				mockClient.EXPECT().UnpauseTrigger(gomock.Any(), gomock.Any()).Return(fmt.Errorf("failed to resume"))
				return mockClient
			},
			expectedStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_FAILED, ErrorMessage: "failed to resume"},
			expectError:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ct := setupCronTrigger(t, test.workflowClientProvider(t))
			trStatus, err := ct.Resume(context.Background(), _triggerRun.DeepCopy())
			if test.expectError {
				assert.Error(t, err, test.name)
			} else {
				assert.NoError(t, err, test.name)
			}
			assert.Equal(t, test.expectedStatus, trStatus, test.name)
		})
	}
}

func setupCronTrigger(t *testing.T, workflowClient clientInterface.WorkflowClient) *cronTrigger {
	trigger := NewCronTrigger(
		zapr.NewLogger(zap.NewNop()),
		workflowClient,
	).(*cronTrigger)
	assert.NotNil(t, trigger)
	return trigger
}

func TestCronTrigger_Update(t *testing.T) {
	tests := []struct {
		name                   string
		triggerRun             *v2pb.TriggerRun
		action                 v2pb.TriggerRunAction
		workflowClientProvider func(t *testing.T) clientInterface.WorkflowClient
		expectError            bool
		expectedActualCron     string
		expectedState          v2pb.TriggerRunState
	}{
		{
			name: "no update needed - schedules match",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Spec: v2pb.TriggerRunSpec{
					Trigger: &v2pb.Trigger{
						TriggerType: &v2pb.Trigger_CronSchedule{
							CronSchedule: &v2pb.CronSchedule{Cron: "0 6 * * *"},
						},
					},
				},
				Status: v2pb.TriggerRunStatus{
					State:         v2pb.TRIGGER_RUN_STATE_RUNNING,
					ActualTrigger: &v2pb.Trigger{TriggerType: &v2pb.Trigger_CronSchedule{CronSchedule: &v2pb.CronSchedule{Cron: "0 6 * * *"}}},
				},
			},
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				// No calls expected when schedules match
				return mockClient
			},
			expectError:        false,
			expectedActualCron: "0 6 * * *",
		},
		{
			name: "update needed - schedules differ",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Spec: v2pb.TriggerRunSpec{
					Trigger: &v2pb.Trigger{
						TriggerType: &v2pb.Trigger_CronSchedule{
							CronSchedule: &v2pb.CronSchedule{Cron: "0 0 * * *"},
						},
					},
				},
				Status: v2pb.TriggerRunStatus{
					State:         v2pb.TRIGGER_RUN_STATE_RUNNING,
					ActualTrigger: &v2pb.Trigger{TriggerType: &v2pb.Trigger_CronSchedule{CronSchedule: &v2pb.CronSchedule{Cron: "0 6 * * *"}}},
				},
			},
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().UpdateTrigger(gomock.Any(), "test-namespace.test-triggerrun", "0 0 * * *", gomock.Nil(), gomock.Nil()).Return(nil)
				return mockClient
			},
			expectError:        false,
			expectedActualCron: "0 0 * * *",
		},
		{
			name: "empty cron - skip update",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Spec: v2pb.TriggerRunSpec{
					Trigger: &v2pb.Trigger{
						TriggerType: &v2pb.Trigger_CronSchedule{
							CronSchedule: &v2pb.CronSchedule{Cron: ""},
						},
					},
				},
				Status: v2pb.TriggerRunStatus{
					State:         v2pb.TRIGGER_RUN_STATE_RUNNING,
					ActualTrigger: nil,
				},
			},
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				// No calls expected for empty cron
				return mockClient
			},
			expectError:        false,
			expectedActualCron: "",
		},
		{
			name: "update trigger fails with non-not-found error",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Spec: v2pb.TriggerRunSpec{
					Trigger: &v2pb.Trigger{
						TriggerType: &v2pb.Trigger_CronSchedule{
							CronSchedule: &v2pb.CronSchedule{Cron: "0 0 * * *"},
						},
					},
				},
				Status: v2pb.TriggerRunStatus{
					State:         v2pb.TRIGGER_RUN_STATE_RUNNING,
					ActualTrigger: &v2pb.Trigger{TriggerType: &v2pb.Trigger_CronSchedule{CronSchedule: &v2pb.CronSchedule{Cron: "0 6 * * *"}}},
				},
			},
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().UpdateTrigger(gomock.Any(), "test-namespace.test-triggerrun", "0 0 * * *", gomock.Nil(), gomock.Nil()).Return(fmt.Errorf("update failed"))
				return mockClient
			},
			expectError:        true,
			expectedActualCron: "0 6 * * *", // Existing status is preserved on error
		},
		{
			name: "pause action - success",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Spec: v2pb.TriggerRunSpec{
					Trigger: &v2pb.Trigger{
						TriggerType: &v2pb.Trigger_CronSchedule{
							CronSchedule: &v2pb.CronSchedule{Cron: "0 0 * * *"},
						},
					},
				},
				Status: v2pb.TriggerRunStatus{
					State:         v2pb.TRIGGER_RUN_STATE_RUNNING,
					ActualTrigger: &v2pb.Trigger{TriggerType: &v2pb.Trigger_CronSchedule{CronSchedule: &v2pb.CronSchedule{Cron: "0 0 * * *"}}},
				},
			},
			action: v2pb.TRIGGER_RUN_ACTION_PAUSE,
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().UpdateTrigger(gomock.Any(), "test-namespace.test-triggerrun", "", gomock.Any(), gomock.Nil()).Return(nil)
				return mockClient
			},
			expectError:        false,
			expectedActualCron: "0 0 * * *",
			expectedState:      v2pb.TRIGGER_RUN_STATE_PAUSED,
		},
		{
			name: "pause action - fails",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Spec: v2pb.TriggerRunSpec{
					Trigger: &v2pb.Trigger{
						TriggerType: &v2pb.Trigger_CronSchedule{
							CronSchedule: &v2pb.CronSchedule{Cron: "0 0 * * *"},
						},
					},
				},
				Status: v2pb.TriggerRunStatus{
					State:         v2pb.TRIGGER_RUN_STATE_RUNNING,
					ActualTrigger: &v2pb.Trigger{TriggerType: &v2pb.Trigger_CronSchedule{CronSchedule: &v2pb.CronSchedule{Cron: "0 0 * * *"}}},
				},
			},
			action: v2pb.TRIGGER_RUN_ACTION_PAUSE,
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().UpdateTrigger(gomock.Any(), "test-namespace.test-triggerrun", "", gomock.Any(), gomock.Nil()).Return(fmt.Errorf("pause failed"))
				return mockClient
			},
			expectError:        true,
			expectedActualCron: "0 0 * * *",                    // Existing status is preserved on error
			expectedState:      v2pb.TRIGGER_RUN_STATE_RUNNING, // State unchanged on error
		},
		{
			name: "resume action - success",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Spec: v2pb.TriggerRunSpec{
					Trigger: &v2pb.Trigger{
						TriggerType: &v2pb.Trigger_CronSchedule{
							CronSchedule: &v2pb.CronSchedule{Cron: "0 0 * * *"},
						},
					},
				},
				Status: v2pb.TriggerRunStatus{
					State:         v2pb.TRIGGER_RUN_STATE_PAUSED,
					ActualTrigger: &v2pb.Trigger{TriggerType: &v2pb.Trigger_CronSchedule{CronSchedule: &v2pb.CronSchedule{Cron: "0 0 * * *"}}},
				},
			},
			action: v2pb.TRIGGER_RUN_ACTION_RESUME,
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().UpdateTrigger(gomock.Any(), "test-namespace.test-triggerrun", "", gomock.Any(), gomock.Nil()).Return(nil)
				return mockClient
			},
			expectError:        false,
			expectedActualCron: "0 0 * * *",
			expectedState:      v2pb.TRIGGER_RUN_STATE_RUNNING,
		},
		{
			name: "resume action - fails",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Spec: v2pb.TriggerRunSpec{
					Trigger: &v2pb.Trigger{
						TriggerType: &v2pb.Trigger_CronSchedule{
							CronSchedule: &v2pb.CronSchedule{Cron: "0 0 * * *"},
						},
					},
				},
				Status: v2pb.TriggerRunStatus{
					State:         v2pb.TRIGGER_RUN_STATE_PAUSED,
					ActualTrigger: &v2pb.Trigger{TriggerType: &v2pb.Trigger_CronSchedule{CronSchedule: &v2pb.CronSchedule{Cron: "0 0 * * *"}}},
				},
			},
			action: v2pb.TRIGGER_RUN_ACTION_RESUME,
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().UpdateTrigger(gomock.Any(), "test-namespace.test-triggerrun", "", gomock.Any(), gomock.Nil()).Return(fmt.Errorf("resume failed"))
				return mockClient
			},
			expectError:        true,
			expectedActualCron: "0 0 * * *",                   // Existing status is preserved on error
			expectedState:      v2pb.TRIGGER_RUN_STATE_PAUSED, // State unchanged on error
		},
		{
			name: "pause action with cron drift - atomic update succeeds",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Spec: v2pb.TriggerRunSpec{
					Trigger: &v2pb.Trigger{
						TriggerType: &v2pb.Trigger_CronSchedule{
							CronSchedule: &v2pb.CronSchedule{Cron: "0 0 * * *"},
						},
					},
				},
				Status: v2pb.TriggerRunStatus{
					State:         v2pb.TRIGGER_RUN_STATE_RUNNING,
					ActualTrigger: &v2pb.Trigger{TriggerType: &v2pb.Trigger_CronSchedule{CronSchedule: &v2pb.CronSchedule{Cron: "0 6 * * *"}}},
				},
			},
			action: v2pb.TRIGGER_RUN_ACTION_PAUSE,
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().UpdateTrigger(gomock.Any(), "test-namespace.test-triggerrun", "0 0 * * *", gomock.Any(), gomock.Nil()).Return(nil)
				return mockClient
			},
			expectError:        false,
			expectedActualCron: "0 0 * * *",
			expectedState:      v2pb.TRIGGER_RUN_STATE_PAUSED,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logger := zapr.NewLogger(zap.NewNop())
			workflowClient := test.workflowClientProvider(t)
			cronTrigger := NewCronTrigger(logger, workflowClient)

			action := v2pb.TRIGGER_RUN_ACTION_NO_ACTION
			if test.action != 0 {
				action = test.action
			}
			status, _, err := cronTrigger.Update(context.Background(), test.triggerRun, action)

			if test.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Check state - if expectedState is set, use it; otherwise check it's unchanged
			if test.expectedState != 0 {
				assert.Equal(t, test.expectedState, status.State)
			} else {
				assert.Equal(t, test.triggerRun.Status.State, status.State)
			}
			assert.Equal(t, test.expectedActualCron, getCronFromActualTrigger(status.ActualTrigger))
		})
	}
}

func TestCronTrigger_UpdateNotifications(t *testing.T) {
	notification := func(events ...v2pb.Notification_EventType) *v2pb.Notification {
		return &v2pb.Notification{
			NotificationType: v2pb.NOTIFICATION_TYPE_EMAIL,
			EventTypes:       events,
			ResourceType:     v2pb.RESOURCE_TYPE_PIPELINE_RUN,
			Emails:           []string{"test@example.com"},
		}
	}

	tests := []struct {
		name                string
		desired             []*v2pb.Notification
		actual              []*v2pb.Notification
		expectUpdate        bool
		expectedActual      []*v2pb.Notification
		workflowUpdateError error
		expectError         bool
	}{
		{
			name:           "notifications unchanged",
			desired:        []*v2pb.Notification{notification(v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_FAILED)},
			actual:         []*v2pb.Notification{notification(v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_FAILED)},
			expectedActual: []*v2pb.Notification{notification(v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_FAILED)},
		},
		{
			name:           "notification event added",
			desired:        []*v2pb.Notification{notification(v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_FAILED, v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_STARTED)},
			actual:         []*v2pb.Notification{notification(v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_FAILED)},
			expectUpdate:   true,
			expectedActual: []*v2pb.Notification{notification(v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_FAILED, v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_STARTED)},
		},
		{
			name:           "notifications cleared",
			actual:         []*v2pb.Notification{notification(v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_FAILED)},
			expectUpdate:   true,
			expectedActual: nil,
		},
		{
			name:                "workflow update fails",
			desired:             []*v2pb.Notification{notification(v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_STARTED)},
			actual:              []*v2pb.Notification{notification(v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_FAILED)},
			expectUpdate:        true,
			workflowUpdateError: fmt.Errorf("update failed"),
			expectError:         true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
			triggerRun := &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Spec: v2pb.TriggerRunSpec{
					Trigger: &v2pb.Trigger{
						TriggerType: &v2pb.Trigger_CronSchedule{
							CronSchedule: &v2pb.CronSchedule{Cron: "0 0 * * *"},
						},
					},
					Notifications: test.desired,
				},
				Status: v2pb.TriggerRunStatus{
					State:               v2pb.TRIGGER_RUN_STATE_RUNNING,
					ActualTrigger:       &v2pb.Trigger{TriggerType: &v2pb.Trigger_CronSchedule{CronSchedule: &v2pb.CronSchedule{Cron: "0 0 * * *"}}},
					ActualNotifications: test.actual,
				},
			}
			if test.expectUpdate {
				mockClient.EXPECT().UpdateTrigger(
					gomock.Any(),
					"test-namespace.test-triggerrun",
					"",
					gomock.Nil(),
					gomock.Eq([]interface{}{CreateTriggerRequest{TriggerRun: triggerRun}}),
				).Return(test.workflowUpdateError)
			}

			status, handled, err := NewCronTrigger(zapr.NewLogger(zap.NewNop()), mockClient).Update(
				context.Background(), triggerRun, v2pb.TRIGGER_RUN_ACTION_NO_ACTION)

			if test.expectError {
				assert.Error(t, err)
				assert.False(t, handled)
				return
			}
			assert.NoError(t, err)
			assert.False(t, handled)
			assert.Equal(t, test.expectedActual, status.ActualNotifications)
			assert.Equal(t, "0 0 * * *", getCronFromActualTrigger(status.ActualTrigger))
		})
	}
}

// TestCronTrigger_UpdateNotifications_SignalLimitError verifies that when
// UpdateTrigger fails with a signal limit error, ActualNotifications is still
// updated to prevent infinite retry loops. This is the fix for the STG incident
// where 2 triggers in cauldron-admin hit "exceeded workflow execution limit for
// signal events" and retried 920+ times because ActualNotifications was never synced.
func TestCronTrigger_UpdateNotifications_SignalLimitError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := interfaceMock.NewMockWorkflowClient(ctrl)

	oldNotif := &v2pb.Notification{
		NotificationType: v2pb.NOTIFICATION_TYPE_EMAIL,
		EventTypes:       []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_FAILED},
		ResourceType:     v2pb.RESOURCE_TYPE_PIPELINE_RUN,
		Emails:           []string{"old@example.com"},
	}
	newNotif := &v2pb.Notification{
		NotificationType: v2pb.NOTIFICATION_TYPE_EMAIL,
		EventTypes:       []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_STARTED},
		ResourceType:     v2pb.RESOURCE_TYPE_PIPELINE_RUN,
		Emails:           []string{"new@example.com"},
	}

	triggerRun := &v2pb.TriggerRun{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "cauldron-admin",
			Name:      "trigger-1778756734-1370990",
		},
		Spec: v2pb.TriggerRunSpec{
			Trigger: &v2pb.Trigger{
				TriggerType: &v2pb.Trigger_CronSchedule{
					CronSchedule: &v2pb.CronSchedule{Cron: "30 * * * *"},
				},
			},
			Notifications: []*v2pb.Notification{newNotif},
		},
		Status: v2pb.TriggerRunStatus{
			State: v2pb.TRIGGER_RUN_STATE_RUNNING,
			ActualTrigger: &v2pb.Trigger{
				TriggerType: &v2pb.Trigger_CronSchedule{
					CronSchedule: &v2pb.CronSchedule{Cron: "30 * * * *"},
				},
			},
			ActualNotifications: []*v2pb.Notification{oldNotif},
		},
	}

	signalLimitErr := fmt.Errorf("exceeded workflow execution limit for signal events")
	mockClient.EXPECT().UpdateTrigger(
		gomock.Any(),
		"cauldron-admin.trigger-1778756734-1370990",
		"",
		gomock.Nil(),
		gomock.Eq([]interface{}{CreateTriggerRequest{TriggerRun: triggerRun}}),
	).Return(signalLimitErr)

	status, handled, err := NewCronTrigger(zapr.NewLogger(zap.NewNop()), mockClient).Update(
		context.Background(), triggerRun, v2pb.TRIGGER_RUN_ACTION_NO_ACTION)

	assert.Error(t, err, "expected error to be returned")
	assert.Contains(t, err.Error(), "exceeded workflow execution limit for signal events")
	assert.False(t, handled, "no action should be marked as handled on error")

	// CRITICAL: ActualNotifications must be synced even though the update failed,
	// to prevent the reconciler from retrying the same doomed update infinitely.
	assert.Equal(t, []*v2pb.Notification{newNotif}, status.ActualNotifications,
		"ActualNotifications must be updated to spec even on UpdateTrigger failure")

	// Cron should remain unchanged (not drifted in this test)
	assert.Equal(t, "30 * * * *", getCronFromActualTrigger(status.ActualTrigger))

	// Error message should be preserved
	assert.Contains(t, status.ErrorMessage, "exceeded workflow execution limit for signal events")
}
