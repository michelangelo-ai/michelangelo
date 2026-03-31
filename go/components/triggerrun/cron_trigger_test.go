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
)

var (
	_runID            = "test-run-id"
	_workflowID       = "test-workflow-id"
	_execTime   int64 = 1683616260555000000
	_logURL           = "http://localhost:8088/domains/default/workflows/test-namespace.test-triggerrun-name"
)

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
			expectedStatus: v2pb.TriggerRunStatus{LogUrl: _logURL, State: v2pb.TRIGGER_RUN_STATE_RUNNING},
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
			expectedStatus: v2pb.TriggerRunStatus{LogUrl: _logURL, State: v2pb.TRIGGER_RUN_STATE_RUNNING},
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
