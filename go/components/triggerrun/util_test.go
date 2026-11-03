package triggerrun

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/zapr"
	pbtypes "github.com/gogo/protobuf/types"
	"github.com/golang/mock/gomock"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	interfaceMock "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface/interface_mock"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetTriggerType(t *testing.T) {
	t.Run("cron trigger", func(t *testing.T) {
		triggerRun := &v2pb.TriggerRun{
			Spec: v2pb.TriggerRunSpec{
				Trigger: &v2pb.Trigger{
					TriggerType: &v2pb.Trigger_CronSchedule{
						CronSchedule: &v2pb.CronSchedule{Cron: "0 0 * * *"},
					},
				},
			},
		}
		result := GetTriggerType(triggerRun)
		assert.Equal(t, TriggerTypeCron, result)
	})

	t.Run("batch rerun trigger", func(t *testing.T) {
		triggerRun := &v2pb.TriggerRun{
			Spec: v2pb.TriggerRunSpec{
				Trigger: &v2pb.Trigger{
					TriggerType: &v2pb.Trigger_BatchRerun{
						BatchRerun: &v2pb.BatchRerun{},
					},
				},
			},
		}
		result := GetTriggerType(triggerRun)
		assert.Equal(t, TriggerTypeBatchRerun, result)
	})

	t.Run("unknown trigger", func(t *testing.T) {
		triggerRun := &v2pb.TriggerRun{
			Spec: v2pb.TriggerRunSpec{
				Trigger: &v2pb.Trigger{},
			},
		}
		result := GetTriggerType(triggerRun)
		assert.Equal(t, TriggerTypeUnknown, result)
	})

	cronWithWindow := func(catchup bool) *v2pb.TriggerRun {
		return &v2pb.TriggerRun{
			Spec: v2pb.TriggerRunSpec{
				Trigger: &v2pb.Trigger{
					TriggerType: &v2pb.Trigger_CronSchedule{
						CronSchedule: &v2pb.CronSchedule{Cron: "0 0 * * *"},
					},
				},
				StartTimestamp: &pbtypes.Timestamp{Seconds: 1_700_000_000},
				EndTimestamp:   &pbtypes.Timestamp{Seconds: 1_700_086_400},
				Catchup:        catchup,
			},
		}
	}

	t.Run("closed window without catchup keeps the legacy backfill runner", func(t *testing.T) {
		assert.Equal(t, TriggerTypeBackfill, GetTriggerType(cronWithWindow(false)))
	})

	// The type must follow the trigger oneof once catchup is set, so that adding or
	// removing an end date on a running schedule never flips it to a different runner.
	t.Run("closed window with catchup stays a cron trigger", func(t *testing.T) {
		assert.Equal(t, TriggerTypeCron, GetTriggerType(cronWithWindow(true)))
	})

	t.Run("open window is a cron trigger regardless of catchup", func(t *testing.T) {
		tr := cronWithWindow(false)
		tr.Spec.EndTimestamp = nil
		assert.Equal(t, TriggerTypeCron, GetTriggerType(tr))
		tr.Spec.Catchup = true
		assert.Equal(t, TriggerTypeCron, GetTriggerType(tr))
	})
}

func TestKillWorkflow(t *testing.T) {
	tests := []struct {
		name                  string
		triggerRun            *v2pb.TriggerRun
		setupMock             func(mockClient *interfaceMock.MockWorkflowClient)
		expectedState         v2pb.TriggerRunState
		expectError           bool
		expectedErrorContains string
	}{
		{
			name: "successful kill with open workflow",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Status: v2pb.TriggerRunStatus{
					State: v2pb.TRIGGER_RUN_STATE_RUNNING,
				},
			},
			setupMock: func(mockClient *interfaceMock.MockWorkflowClient) {
				runID := "test-run-id"
				mockClient.EXPECT().GetDomain().Return("test-domain")
				mockClient.EXPECT().ListOpenWorkflow(gomock.Any(), gomock.Any()).Return(&clientInterface.ListOpenWorkflowExecutionsResponse{
					Executions: []clientInterface.WorkflowExecutionInfo{
						{Execution: &clientInterface.WorkflowExecution{ID: "test-namespace.test-triggerrun", RunID: runID}},
					},
				}, nil)
				mockClient.EXPECT().DeleteTrigger(gomock.Any(), "test-namespace.test-triggerrun", runID).Return(nil)
			},
			expectedState: v2pb.TRIGGER_RUN_STATE_KILLED,
			expectError:   false,
		},
		{
			name: "no open execution - idempotent kill",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Status: v2pb.TriggerRunStatus{
					State: v2pb.TRIGGER_RUN_STATE_RUNNING,
				},
			},
			setupMock: func(mockClient *interfaceMock.MockWorkflowClient) {
				mockClient.EXPECT().GetDomain().Return("test-domain")
				mockClient.EXPECT().ListOpenWorkflow(gomock.Any(), gomock.Any()).Return(&clientInterface.ListOpenWorkflowExecutionsResponse{
					Executions: []clientInterface.WorkflowExecutionInfo{},
				}, nil)
				mockClient.EXPECT().DeleteTrigger(gomock.Any(), "test-namespace.test-triggerrun", "").Return(nil)
			},
			expectedState: v2pb.TRIGGER_RUN_STATE_KILLED,
			expectError:   false,
		},
		{
			name: "error getting workflow execution info",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Status: v2pb.TriggerRunStatus{
					State: v2pb.TRIGGER_RUN_STATE_RUNNING,
				},
			},
			setupMock: func(mockClient *interfaceMock.MockWorkflowClient) {
				mockClient.EXPECT().GetDomain().Return("test-domain")
				mockClient.EXPECT().ListOpenWorkflow(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("failed to list workflows")).AnyTimes()
			},
			expectedState:         v2pb.TRIGGER_RUN_STATE_RUNNING,
			expectError:           true,
			expectedErrorContains: "get workflow execution info for trigger test-namespace/test-triggerrun",
		},
		{
			name: "error deleting trigger",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Status: v2pb.TriggerRunStatus{
					State: v2pb.TRIGGER_RUN_STATE_RUNNING,
				},
			},
			setupMock: func(mockClient *interfaceMock.MockWorkflowClient) {
				runID := "test-run-id"
				mockClient.EXPECT().GetDomain().Return("test-domain")
				mockClient.EXPECT().ListOpenWorkflow(gomock.Any(), gomock.Any()).Return(&clientInterface.ListOpenWorkflowExecutionsResponse{
					Executions: []clientInterface.WorkflowExecutionInfo{
						{Execution: &clientInterface.WorkflowExecution{ID: "test-namespace.test-triggerrun", RunID: runID}},
					},
				}, nil)
				mockClient.EXPECT().DeleteTrigger(gomock.Any(), "test-namespace.test-triggerrun", runID).Return(fmt.Errorf("failed to delete trigger"))
			},
			expectedState:         v2pb.TRIGGER_RUN_STATE_RUNNING,
			expectError:           true,
			expectedErrorContains: "delete trigger for test-namespace/test-triggerrun",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock client
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
			tt.setupMock(mockClient)

			// Create logger
			logger := zapr.NewLogger(zaptest.NewLogger(t))

			// Execute the function
			result, err := killWorkflow(context.Background(), tt.triggerRun, logger, mockClient)

			// Verify results
			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedErrorContains != "" {
					assert.Contains(t, err.Error(), tt.expectedErrorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedState, result.State)
		})
	}
}

func TestGetRecurringRunWorkflowStatus(t *testing.T) {
	tests := []struct {
		name                  string
		triggerRun            *v2pb.TriggerRun
		setupMock             func(mockClient *interfaceMock.MockWorkflowClient)
		expectedState         v2pb.TriggerRunState
		expectError           bool
		expectedErrorContains string
	}{
		{
			name: "running workflow",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Status: v2pb.TriggerRunStatus{
					State: v2pb.TRIGGER_RUN_STATE_RUNNING,
				},
			},
			setupMock: func(mockClient *interfaceMock.MockWorkflowClient) {
				// Mock ListOpenWorkflow to return a running execution
				mockClient.EXPECT().ListOpenWorkflow(gomock.Any(), gomock.Any()).Return(&clientInterface.ListOpenWorkflowExecutionsResponse{
					Executions: []clientInterface.WorkflowExecutionInfo{
						{
							Execution: &clientInterface.WorkflowExecution{
								ID:    "test-namespace.test-triggerrun",
								RunID: "test-run-id",
							},
							ExecutionTime: time.Now(),
							Status:        clientInterface.WorkflowExecutionStatusRunning,
						},
					},
				}, nil)
			},
			expectedState: v2pb.TRIGGER_RUN_STATE_RUNNING,
			expectError:   false,
		},
		{
			name: "failed workflow",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Status: v2pb.TriggerRunStatus{
					State: v2pb.TRIGGER_RUN_STATE_RUNNING,
				},
			},
			setupMock: func(mockClient *interfaceMock.MockWorkflowClient) {
				// Mock ListOpenWorkflow to return a failed execution
				mockClient.EXPECT().ListOpenWorkflow(gomock.Any(), gomock.Any()).Return(&clientInterface.ListOpenWorkflowExecutionsResponse{
					Executions: []clientInterface.WorkflowExecutionInfo{
						{
							Execution: &clientInterface.WorkflowExecution{
								ID:    "test-namespace.test-triggerrun",
								RunID: "test-run-id",
							},
							ExecutionTime: time.Now(),
							Status:        clientInterface.WorkflowExecutionStatusFailed,
						},
					},
				}, nil)
			},
			expectedState:         v2pb.TRIGGER_RUN_STATE_FAILED,
			expectError:           true,
			expectedErrorContains: "workflow failed with state:",
		},
		{
			name: "canceled workflow",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Status: v2pb.TriggerRunStatus{
					State: v2pb.TRIGGER_RUN_STATE_RUNNING,
				},
			},
			setupMock: func(mockClient *interfaceMock.MockWorkflowClient) {
				// Mock ListOpenWorkflow to return a canceled execution
				mockClient.EXPECT().ListOpenWorkflow(gomock.Any(), gomock.Any()).Return(&clientInterface.ListOpenWorkflowExecutionsResponse{
					Executions: []clientInterface.WorkflowExecutionInfo{
						{
							Execution: &clientInterface.WorkflowExecution{
								ID:    "test-namespace.test-triggerrun",
								RunID: "test-run-id",
							},
							ExecutionTime: time.Now(),
							Status:        clientInterface.WorkflowExecutionStatusCanceled,
						},
					},
				}, nil)
			},
			expectedState: v2pb.TRIGGER_RUN_STATE_KILLED,
			expectError:   false,
		},
		{
			name: "terminated workflow",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Status: v2pb.TriggerRunStatus{
					State: v2pb.TRIGGER_RUN_STATE_RUNNING,
				},
			},
			setupMock: func(mockClient *interfaceMock.MockWorkflowClient) {
				// Mock ListOpenWorkflow to return a terminated execution
				mockClient.EXPECT().ListOpenWorkflow(gomock.Any(), gomock.Any()).Return(&clientInterface.ListOpenWorkflowExecutionsResponse{
					Executions: []clientInterface.WorkflowExecutionInfo{
						{
							Execution: &clientInterface.WorkflowExecution{
								ID:    "test-namespace.test-triggerrun",
								RunID: "test-run-id",
							},
							ExecutionTime: time.Now(),
							Status:        clientInterface.WorkflowExecutionStatusTerminated,
						},
					},
				}, nil)
			},
			expectedState: v2pb.TRIGGER_RUN_STATE_KILLED,
			expectError:   false,
		},
		{
			name: "timed out workflow",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Status: v2pb.TriggerRunStatus{
					State: v2pb.TRIGGER_RUN_STATE_RUNNING,
				},
			},
			setupMock: func(mockClient *interfaceMock.MockWorkflowClient) {
				// Mock ListOpenWorkflow to return a timed out execution
				mockClient.EXPECT().ListOpenWorkflow(gomock.Any(), gomock.Any()).Return(&clientInterface.ListOpenWorkflowExecutionsResponse{
					Executions: []clientInterface.WorkflowExecutionInfo{
						{
							Execution: &clientInterface.WorkflowExecution{
								ID:    "test-namespace.test-triggerrun",
								RunID: "test-run-id",
							},
							ExecutionTime: time.Now(),
							Status:        clientInterface.WorkflowExecutionStatusTimedOut,
						},
					},
				}, nil)
			},
			expectedState:         v2pb.TRIGGER_RUN_STATE_FAILED,
			expectError:           true,
			expectedErrorContains: "workflow failed with state:",
		},
		{
			name: "no open workflows",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Status: v2pb.TriggerRunStatus{
					State: v2pb.TRIGGER_RUN_STATE_RUNNING,
				},
			},
			setupMock: func(mockClient *interfaceMock.MockWorkflowClient) {
				// Mock ListOpenWorkflow to return empty result
				mockClient.EXPECT().ListOpenWorkflow(gomock.Any(), gomock.Any()).Return(&clientInterface.ListOpenWorkflowExecutionsResponse{
					Executions: []clientInterface.WorkflowExecutionInfo{},
				}, nil)
			},
			expectedState: v2pb.TRIGGER_RUN_STATE_RUNNING,
			expectError:   false,
		},
		{
			name: "execution with zero time",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Status: v2pb.TriggerRunStatus{
					State: v2pb.TRIGGER_RUN_STATE_RUNNING,
				},
			},
			setupMock: func(mockClient *interfaceMock.MockWorkflowClient) {
				// Mock ListOpenWorkflow to return execution with zero execution time
				mockClient.EXPECT().ListOpenWorkflow(gomock.Any(), gomock.Any()).Return(&clientInterface.ListOpenWorkflowExecutionsResponse{
					Executions: []clientInterface.WorkflowExecutionInfo{
						{
							Execution: &clientInterface.WorkflowExecution{
								ID:    "test-namespace.test-triggerrun",
								RunID: "test-run-id",
							},
							ExecutionTime: time.Time{}, // Zero time
							Status:        clientInterface.WorkflowExecutionStatusRunning,
						},
					},
				}, nil)
			},
			expectedState: v2pb.TRIGGER_RUN_STATE_RUNNING,
			expectError:   false,
		},
		{
			name: "error listing workflows",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Status: v2pb.TriggerRunStatus{
					State: v2pb.TRIGGER_RUN_STATE_RUNNING,
				},
			},
			setupMock: func(mockClient *interfaceMock.MockWorkflowClient) {
				// Mock ListOpenWorkflow to return error
				mockClient.EXPECT().ListOpenWorkflow(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("failed to list workflows")).AnyTimes()
			},
			expectedState:         v2pb.TRIGGER_RUN_STATE_RUNNING, // Should keep original state
			expectError:           true,
			expectedErrorContains: "list open workflow for trigger test-namespace/test-triggerrun",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock client
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
			tt.setupMock(mockClient)

			// Create logger
			logger := zapr.NewLogger(zaptest.NewLogger(t))

			// Execute the function
			result, err := getRecurringRunWorkflowStatus(context.Background(), tt.triggerRun, logger, mockClient, "test-domain")

			// Verify results
			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedErrorContains != "" {
					assert.Contains(t, err.Error(), tt.expectedErrorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedState, result.State)
		})
	}
}

func TestGetAdhocRunWorkflowStatus(t *testing.T) {
	tests := []struct {
		name                  string
		triggerRun            *v2pb.TriggerRun
		setupMock             func(mockClient *interfaceMock.MockWorkflowClient)
		expectedState         v2pb.TriggerRunState
		expectError           bool
		expectedErrorContains string
	}{
		{
			name: "empty execution workflow ID",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Status: v2pb.TriggerRunStatus{
					State:               v2pb.TRIGGER_RUN_STATE_RUNNING,
					ExecutionWorkflowId: "", // Empty workflow ID
				},
			},
			setupMock: func(mockClient *interfaceMock.MockWorkflowClient) {
				// No mock calls expected since we error out early
			},
			expectedState:         v2pb.TRIGGER_RUN_STATE_FAILED,
			expectError:           true,
			expectedErrorContains: "execution workflow id is empty",
		},
		{
			name: "running workflow",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Status: v2pb.TriggerRunStatus{
					State:               v2pb.TRIGGER_RUN_STATE_RUNNING,
					ExecutionWorkflowId: "test-workflow-id",
				},
			},
			setupMock: func(mockClient *interfaceMock.MockWorkflowClient) {
				mockClient.EXPECT().GetWorkflowExecutionInfo(gomock.Any(), "test-workflow-id", "").Return(&clientInterface.WorkflowExecutionInfo{
					Execution: &clientInterface.WorkflowExecution{
						ID:    "test-workflow-id",
						RunID: "test-run-id",
					},
					Status: clientInterface.WorkflowExecutionStatusRunning,
				}, nil)
			},
			expectedState: v2pb.TRIGGER_RUN_STATE_RUNNING,
			expectError:   false,
		},
		{
			name: "completed workflow",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Status: v2pb.TriggerRunStatus{
					State:               v2pb.TRIGGER_RUN_STATE_RUNNING,
					ExecutionWorkflowId: "test-workflow-id",
				},
			},
			setupMock: func(mockClient *interfaceMock.MockWorkflowClient) {
				mockClient.EXPECT().GetWorkflowExecutionInfo(gomock.Any(), "test-workflow-id", "").Return(&clientInterface.WorkflowExecutionInfo{
					Execution: &clientInterface.WorkflowExecution{
						ID:    "test-workflow-id",
						RunID: "test-run-id",
					},
					Status: clientInterface.WorkflowExecutionStatusCompleted,
				}, nil)
			},
			expectedState: v2pb.TRIGGER_RUN_STATE_SUCCEEDED,
			expectError:   false,
		},
		{
			name: "failed workflow",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Status: v2pb.TriggerRunStatus{
					State:               v2pb.TRIGGER_RUN_STATE_RUNNING,
					ExecutionWorkflowId: "test-workflow-id",
				},
			},
			setupMock: func(mockClient *interfaceMock.MockWorkflowClient) {
				mockClient.EXPECT().GetWorkflowExecutionInfo(gomock.Any(), "test-workflow-id", "").Return(&clientInterface.WorkflowExecutionInfo{
					Execution: &clientInterface.WorkflowExecution{
						ID:    "test-workflow-id",
						RunID: "test-run-id",
					},
					Status: clientInterface.WorkflowExecutionStatusFailed,
				}, nil)
			},
			expectedState:         v2pb.TRIGGER_RUN_STATE_FAILED,
			expectError:           true,
			expectedErrorContains: "workflow is terminated with state:",
		},
		{
			name: "timed out workflow",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Status: v2pb.TriggerRunStatus{
					State:               v2pb.TRIGGER_RUN_STATE_RUNNING,
					ExecutionWorkflowId: "test-workflow-id",
				},
			},
			setupMock: func(mockClient *interfaceMock.MockWorkflowClient) {
				mockClient.EXPECT().GetWorkflowExecutionInfo(gomock.Any(), "test-workflow-id", "").Return(&clientInterface.WorkflowExecutionInfo{
					Execution: &clientInterface.WorkflowExecution{
						ID:    "test-workflow-id",
						RunID: "test-run-id",
					},
					Status: clientInterface.WorkflowExecutionStatusTimedOut,
				}, nil)
			},
			expectedState:         v2pb.TRIGGER_RUN_STATE_FAILED,
			expectError:           true,
			expectedErrorContains: "workflow is terminated with state:",
		},
		{
			name: "canceled workflow",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Status: v2pb.TriggerRunStatus{
					State:               v2pb.TRIGGER_RUN_STATE_RUNNING,
					ExecutionWorkflowId: "test-workflow-id",
				},
			},
			setupMock: func(mockClient *interfaceMock.MockWorkflowClient) {
				mockClient.EXPECT().GetWorkflowExecutionInfo(gomock.Any(), "test-workflow-id", "").Return(&clientInterface.WorkflowExecutionInfo{
					Execution: &clientInterface.WorkflowExecution{
						ID:    "test-workflow-id",
						RunID: "test-run-id",
					},
					Status: clientInterface.WorkflowExecutionStatusCanceled,
				}, nil)
			},
			expectedState:         v2pb.TRIGGER_RUN_STATE_FAILED,
			expectError:           true,
			expectedErrorContains: "workflow is terminated with state:",
		},
		{
			name: "terminated workflow",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Status: v2pb.TriggerRunStatus{
					State:               v2pb.TRIGGER_RUN_STATE_RUNNING,
					ExecutionWorkflowId: "test-workflow-id",
				},
			},
			setupMock: func(mockClient *interfaceMock.MockWorkflowClient) {
				mockClient.EXPECT().GetWorkflowExecutionInfo(gomock.Any(), "test-workflow-id", "").Return(&clientInterface.WorkflowExecutionInfo{
					Execution: &clientInterface.WorkflowExecution{
						ID:    "test-workflow-id",
						RunID: "test-run-id",
					},
					Status: clientInterface.WorkflowExecutionStatusTerminated,
				}, nil)
			},
			expectedState:         v2pb.TRIGGER_RUN_STATE_FAILED,
			expectError:           true,
			expectedErrorContains: "workflow is terminated with state:",
		},
		{
			name: "unknown workflow status",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Status: v2pb.TriggerRunStatus{
					State:               v2pb.TRIGGER_RUN_STATE_RUNNING,
					ExecutionWorkflowId: "test-workflow-id",
				},
			},
			setupMock: func(mockClient *interfaceMock.MockWorkflowClient) {
				mockClient.EXPECT().GetWorkflowExecutionInfo(gomock.Any(), "test-workflow-id", "").Return(&clientInterface.WorkflowExecutionInfo{
					Execution: &clientInterface.WorkflowExecution{
						ID:    "test-workflow-id",
						RunID: "test-run-id",
					},
					Status: clientInterface.WorkflowExecutionStatus(999), // Unknown status
				}, nil)
			},
			expectedState:         v2pb.TRIGGER_RUN_STATE_FAILED,
			expectError:           true,
			expectedErrorContains: "workflow is terminated with unknown state:",
		},
		{
			name: "error getting workflow execution info",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Status: v2pb.TriggerRunStatus{
					State:               v2pb.TRIGGER_RUN_STATE_RUNNING,
					ExecutionWorkflowId: "test-workflow-id",
				},
			},
			setupMock: func(mockClient *interfaceMock.MockWorkflowClient) {
				mockClient.EXPECT().GetWorkflowExecutionInfo(gomock.Any(), "test-workflow-id", "").Return(nil, fmt.Errorf("failed to get workflow execution info"))
			},
			expectedState:         v2pb.TRIGGER_RUN_STATE_FAILED,
			expectError:           true,
			expectedErrorContains: "failed to get workflow execution info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock client
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
			tt.setupMock(mockClient)

			// Create logger
			logger := zapr.NewLogger(zaptest.NewLogger(t))

			// Execute the function
			result, err := getAdhocRunWorkflowStatus(context.Background(), tt.triggerRun, logger, mockClient, "test-domain")

			// Verify results
			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedErrorContains != "" {
					assert.Contains(t, err.Error(), tt.expectedErrorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedState, result.State)
		})
	}
}

func TestIsTerminalState(t *testing.T) {
	for state, want := range map[v2pb.TriggerRunState]bool{
		v2pb.TRIGGER_RUN_STATE_INVALID:      false,
		v2pb.TRIGGER_RUN_STATE_RUNNING:      false,
		v2pb.TRIGGER_RUN_STATE_PENDING_KILL: false,
		v2pb.TRIGGER_RUN_STATE_PAUSED:       false,
		v2pb.TRIGGER_RUN_STATE_FAILED:       true,
		v2pb.TRIGGER_RUN_STATE_KILLED:       true,
		v2pb.TRIGGER_RUN_STATE_SUCCEEDED:    true,
	} {
		assert.Equal(t, want, IsTerminalState(state), state.String())
	}
}

func TestBackfillRunTimestamps(t *testing.T) {
	day := func(d, h int) time.Time { return time.Date(2024, 1, d, h, 0, 0, 0, time.UTC) }

	tests := []struct {
		name    string
		cron    string
		start   time.Time
		end     time.Time
		want    []time.Time
		wantErr string
	}{
		{
			name:  "every occurrence after start up to and including end",
			cron:  "0 8 * * *",
			start: day(1, 7),
			end:   day(3, 8),
			want:  []time.Time{day(1, 8), day(2, 8), day(3, 8)},
		},
		{
			name:  "start on the schedule is itself an occurrence",
			cron:  "0 8 * * *",
			start: day(1, 8),
			end:   day(2, 7),
			want:  []time.Time{day(1, 8)},
		},
		{
			name:  "window between two occurrences is empty",
			cron:  "0 8 * * *",
			start: day(1, 9),
			end:   day(1, 10),
		},
		{
			name:  "end before start is empty",
			cron:  "0 * * * *",
			start: day(2, 0).Add(30 * time.Minute),
			end:   day(1, 0),
		},
		{
			name:  "start on the schedule counts even when end is before it",
			cron:  "0 * * * *",
			start: day(2, 0),
			end:   day(1, 0),
			want:  []time.Time{day(2, 0)},
		},
		{
			name:  "cron that never fires is empty",
			cron:  "0 0 30 2 *",
			start: day(1, 0),
			end:   day(2, 0),
		},
		{
			name:  "cron that never fires is empty over a multi-year window",
			cron:  "0 0 30 2 *",
			start: day(1, 0),
			end:   day(1, 0).AddDate(10, 0, 0),
		},
		{
			name:    "unparsable cron",
			cron:    "every day",
			start:   day(1, 0),
			end:     day(2, 0),
			wantErr: "failed to parse cron schedule",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BackfillRunTimestamps(tt.start, tt.end, tt.cron)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestBackfillWindowHasRun(t *testing.T) {
	day := func(d, h int) time.Time { return time.Date(2024, 1, d, h, 0, 0, 0, time.UTC) }

	tests := []struct {
		name    string
		cron    string
		start   time.Time
		end     time.Time
		want    bool
		wantErr string
	}{
		{name: "occurrence inside the window", cron: "0 8 * * *", start: day(1, 7), end: day(1, 9), want: true},
		{name: "start on the schedule", cron: "0 8 * * *", start: day(1, 8), end: day(1, 8), want: true},
		{name: "window between two occurrences", cron: "0 8 * * *", start: day(1, 9), end: day(1, 10)},
		{name: "end before start", cron: "0 * * * *", start: day(2, 0).Add(30 * time.Minute), end: day(1, 0)},
		{name: "cron that never fires", cron: "0 0 30 2 *", start: day(1, 0), end: day(1, 0).AddDate(10, 0, 0)},
		{
			name:  "large frequent window does not enumerate",
			cron:  "* * * * *",
			start: day(1, 0),
			end:   day(1, 0).AddDate(100, 0, 0),
			want:  true,
		},
		{name: "unparsable cron", cron: "every day", start: day(1, 0), end: day(2, 0), wantErr: "failed to parse cron schedule"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BackfillWindowHasRun(tt.start, tt.end, tt.cron)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
			// The two helpers must agree on whether the window runs at all.
			if tt.end.Sub(tt.start) < 24*time.Hour*366 {
				runs, err := BackfillRunTimestamps(tt.start, tt.end, tt.cron)
				require.NoError(t, err)
				require.Equal(t, tt.want, len(runs) > 0)
			}
		})
	}
}
