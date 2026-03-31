package triggerrun

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-logr/zapr"
	"github.com/golang/mock/gomock"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	interfaceMock "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface/interface_mock"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestRunBackfill(t *testing.T) {
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
			expectedStatus: v2pb.TriggerRunStatus{
				State:               v2pb.TRIGGER_RUN_STATE_RUNNING,
				ExecutionWorkflowId: _runID,
				LogUrl:              _logURL,
			},
			expectError: false,
		},
		{
			name: "list open workflow failed and start succeeded",
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().GetDomain().Return("test-domain")
				mockClient.EXPECT().GetProvider().Return("test-provider").AnyTimes()
				mockClient.EXPECT().ListOpenWorkflow(gomock.Any(), gomock.Any()).AnyTimes().Return(nil, fmt.Errorf("failed to list open workflow"))
				mockClient.EXPECT().StartWorkflow(
					gomock.Any(),
					gomock.Any(),
					"trigger.BackfillTrigger",
					gomock.Any(),
				).Return(&clientInterface.WorkflowExecution{ID: _workflowID, RunID: _runID}, nil)
				return mockClient
			},
			expectedStatus: v2pb.TriggerRunStatus{
				LogUrl:              _logURL,
				State:               v2pb.TRIGGER_RUN_STATE_RUNNING,
				ExecutionWorkflowId: _workflowID,
			},
			expectError: false,
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
				mockClient.EXPECT().StartWorkflow(
					gomock.Any(),
					gomock.Any(),
					"trigger.BackfillTrigger",
					gomock.Any(),
				).Return(&clientInterface.WorkflowExecution{ID: _workflowID, RunID: _runID}, nil)
				return mockClient
			},
			expectedStatus: v2pb.TriggerRunStatus{
				LogUrl:              _logURL,
				State:               v2pb.TRIGGER_RUN_STATE_RUNNING,
				ExecutionWorkflowId: _workflowID,
			},
			expectError: false,
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
				mockClient.EXPECT().StartWorkflow(
					gomock.Any(),
					gomock.Any(),
					"trigger.BackfillTrigger",
					gomock.Any(),
				).Return(nil, fmt.Errorf("failed to start workflow"))
				return mockClient
			},
			expectedStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_FAILED, ErrorMessage: "failed to start workflow"},
			expectError:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ct := setupBackfillTrigger(t, test.workflowClientProvider(t))
			trStatus, err := ct.Run(context.Background(), _triggerRun.DeepCopy())
			if test.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, test.expectedStatus, trStatus)
		})
	}
}

func TestKillBackfill(t *testing.T) {
	tests := []struct {
		name                   string
		workflowClientProvider func(t *testing.T) clientInterface.WorkflowClient
		triggerRunStatus       v2pb.TriggerRunStatus
		expectedStatus         v2pb.TriggerRunStatus
		expectError            bool
	}{
		{
			name: "not running status and unable to kill",
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().GetProvider().Return("test-provider").AnyTimes()
				return mockClient
			},
			triggerRunStatus: v2pb.TriggerRunStatus{
				State:               v2pb.TRIGGER_RUN_STATE_FAILED,
				ExecutionWorkflowId: "test-namespace.test-triggerrun-name",
			},
			expectedStatus: v2pb.TriggerRunStatus{
				State:        v2pb.TRIGGER_RUN_STATE_FAILED,
				ErrorMessage: "cannot kill backfill trigger run in state: TRIGGER_RUN_STATE_FAILED",
			},
			expectError: true,
		},
		{
			name: "delete trigger failed",
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().GetProvider().Return("test-provider").AnyTimes()
				mockClient.EXPECT().DeleteTrigger(gomock.Any(), gomock.Any()).Return(fmt.Errorf("failed to delete trigger"))
				return mockClient
			},
			triggerRunStatus: v2pb.TriggerRunStatus{
				State:               v2pb.TRIGGER_RUN_STATE_RUNNING,
				ExecutionWorkflowId: "test-namespace.test-triggerrun-name",
			},
			expectedStatus: v2pb.TriggerRunStatus{
				State:               v2pb.TRIGGER_RUN_STATE_RUNNING,
				ExecutionWorkflowId: "test-namespace.test-triggerrun-name",
			},
			expectError: true,
		},
		{
			name: "kill workflow succeeded",
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().GetProvider().Return("test-provider").AnyTimes()
				mockClient.EXPECT().DeleteTrigger(gomock.Any(), gomock.Any()).Return(nil)
				return mockClient
			},
			triggerRunStatus: v2pb.TriggerRunStatus{
				State:               v2pb.TRIGGER_RUN_STATE_RUNNING,
				ExecutionWorkflowId: "test-namespace.test-triggerrun-name",
			},
			expectedStatus: v2pb.TriggerRunStatus{
				State:               v2pb.TRIGGER_RUN_STATE_KILLED,
				ExecutionWorkflowId: "test-namespace.test-triggerrun-name",
			},
			expectError: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ct := setupBackfillTrigger(t, test.workflowClientProvider(t))
			tr := _triggerRun.DeepCopy()
			tr.Status = test.triggerRunStatus
			trStatus, err := ct.Kill(context.Background(), tr)
			if test.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, test.expectedStatus, trStatus)
		})
	}
}

func TestGetStatusBackfill(t *testing.T) {
	tests := []struct {
		name                   string
		workflowID             string
		workflowClientProvider func(t *testing.T) clientInterface.WorkflowClient
		expectedStatus         v2pb.TriggerRunStatus
		expectError            bool
	}{
		{
			name:       "workflow id is empty and unable to get status",
			workflowID: "",
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().GetDomain().Return("test-domain")
				mockClient.EXPECT().GetProvider().Return("test-provider").AnyTimes()
				return mockClient
			},
			expectedStatus: v2pb.TriggerRunStatus{
				State:        v2pb.TRIGGER_RUN_STATE_FAILED,
				ErrorMessage: "failed to get workflow status: execution workflow id is empty",
			},
			expectError: true,
		},
		{
			name:       "get status running",
			workflowID: "test-namespace.test-triggerrun-name",
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().GetDomain().Return("test-domain")
				mockClient.EXPECT().GetProvider().Return("test-provider").AnyTimes()
				mockClient.EXPECT().GetWorkflowExecutionInfo(
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
				).Return(
					&clientInterface.WorkflowExecutionInfo{
						Status: clientInterface.WorkflowExecutionStatusRunning,
					}, nil)
				return mockClient
			},
			expectedStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING},
			expectError:    false,
		},
		{
			name:       "get status succeeded",
			workflowID: "test-namespace.test-triggerrun-name",
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().GetDomain().Return("test-domain")
				mockClient.EXPECT().GetProvider().Return("test-provider").AnyTimes()
				mockClient.EXPECT().GetWorkflowExecutionInfo(
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
				).Return(
					&clientInterface.WorkflowExecutionInfo{
						Status: clientInterface.WorkflowExecutionStatusCompleted,
					}, nil)
				return mockClient
			},
			expectedStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_SUCCEEDED},
			expectError:    false,
		},
		{
			name:       "get status failed",
			workflowID: "test-namespace.test-triggerrun-name",
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().GetDomain().Return("test-domain")
				mockClient.EXPECT().GetProvider().Return("test-provider").AnyTimes()
				mockClient.EXPECT().GetWorkflowExecutionInfo(
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
				).Return(
					&clientInterface.WorkflowExecutionInfo{
						Status: clientInterface.WorkflowExecutionStatusFailed,
					}, nil)
				return mockClient
			},
			expectedStatus: v2pb.TriggerRunStatus{
				State:        v2pb.TRIGGER_RUN_STATE_FAILED,
				ErrorMessage: "workflow is terminated with state: 3",
			},
			expectError: true,
		},
		{
			name:       "get status timed out",
			workflowID: "test-namespace.test-triggerrun-name",
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().GetDomain().Return("test-domain")
				mockClient.EXPECT().GetProvider().Return("test-provider").AnyTimes()
				mockClient.EXPECT().GetWorkflowExecutionInfo(
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
				).Return(
					&clientInterface.WorkflowExecutionInfo{
						Status: clientInterface.WorkflowExecutionStatusTimedOut,
					}, nil)
				return mockClient
			},
			expectedStatus: v2pb.TriggerRunStatus{
				State:        v2pb.TRIGGER_RUN_STATE_FAILED,
				ErrorMessage: "workflow is terminated with state: 7",
			},
			expectError: true,
		},
		{
			name:       "get status canceled",
			workflowID: "test-namespace.test-triggerrun-name",
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().GetDomain().Return("test-domain")
				mockClient.EXPECT().GetProvider().Return("test-provider").AnyTimes()
				mockClient.EXPECT().GetWorkflowExecutionInfo(
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
				).Return(
					&clientInterface.WorkflowExecutionInfo{
						Status: clientInterface.WorkflowExecutionStatusCanceled,
					}, nil)
				return mockClient
			},
			expectedStatus: v2pb.TriggerRunStatus{
				State:        v2pb.TRIGGER_RUN_STATE_FAILED,
				ErrorMessage: "workflow is terminated with state: 4",
			},
			expectError: true,
		},
		{
			name:       "get status terminated",
			workflowID: "test-namespace.test-triggerrun-name",
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().GetDomain().Return("test-domain")
				mockClient.EXPECT().GetProvider().Return("test-provider").AnyTimes()
				mockClient.EXPECT().GetWorkflowExecutionInfo(
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
				).Return(
					&clientInterface.WorkflowExecutionInfo{
						Status: clientInterface.WorkflowExecutionStatusTerminated,
					}, nil)
				return mockClient
			},
			expectedStatus: v2pb.TriggerRunStatus{
				State:        v2pb.TRIGGER_RUN_STATE_FAILED,
				ErrorMessage: "workflow is terminated with state: 5",
			},
			expectError: true,
		},
		{
			name:       "get status unknown",
			workflowID: "test-namespace.test-triggerrun-name",
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().GetDomain().Return("test-domain")
				mockClient.EXPECT().GetProvider().Return("test-provider").AnyTimes()
				mockClient.EXPECT().GetWorkflowExecutionInfo(
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
				).Return(
					&clientInterface.WorkflowExecutionInfo{
						Status: clientInterface.WorkflowExecutionStatusContinuedAsNew,
					}, nil)
				return mockClient
			},
			expectedStatus: v2pb.TriggerRunStatus{
				State:        v2pb.TRIGGER_RUN_STATE_FAILED,
				ErrorMessage: "workflow is terminated with unknown state: 6",
			},
			expectError: true,
		},
		{
			name:       "describe workflow execution failed",
			workflowID: "test-namespace.test-triggerrun-name",
			workflowClientProvider: func(t *testing.T) clientInterface.WorkflowClient {
				ctrl := gomock.NewController(t)
				mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
				mockClient.EXPECT().GetDomain().Return("test-domain")
				mockClient.EXPECT().GetProvider().Return("test-provider").AnyTimes()
				mockClient.EXPECT().GetWorkflowExecutionInfo(
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
				).Return(nil, fmt.Errorf("bad connection"))
				return mockClient
			},
			expectedStatus: v2pb.TriggerRunStatus{
				State:        v2pb.TRIGGER_RUN_STATE_FAILED,
				ErrorMessage: "failed to describe workflow execution: bad connection",
			},
			expectError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ct := setupBackfillTrigger(t, test.workflowClientProvider(t))
			tr := _triggerRun.DeepCopy()
			tr.Status.ExecutionWorkflowId = test.workflowID
			trStatus, err := ct.GetStatus(context.Background(), tr)
			if test.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, test.expectedStatus, trStatus)
		})
	}
}

func setupBackfillTrigger(t *testing.T, workflowClient clientInterface.WorkflowClient) *backfillTrigger {
	trigger := NewBackfillTrigger(
		zapr.NewLogger(zap.NewNop()),
		workflowClient,
	).(*backfillTrigger)
	assert.NotNil(t, trigger)
	return trigger
}
