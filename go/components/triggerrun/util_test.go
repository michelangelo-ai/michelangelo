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

func TestSyncCronScheduleToTemporal(t *testing.T) {
	tests := []struct {
		name        string
		triggerRun  *v2pb.TriggerRun
		desiredCron string
		setupMock   func(mockClient *interfaceMock.MockWorkflowClient)
		expectError bool
	}{
		{
			name: "no sync needed - schedules already match",
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
			},
			desiredCron: "0 6 * * *",
			setupMock: func(mockClient *interfaceMock.MockWorkflowClient) {
				mockClient.EXPECT().GetTriggerSchedule(gomock.Any(), "test-namespace.test-triggerrun").Return("0 6 * * *", nil)
				// UpdateTrigger should NOT be called since schedules match
			},
			expectError: false,
		},
		{
			name: "sync needed - schedules differ",
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
			},
			desiredCron: "0 0 * * *",
			setupMock: func(mockClient *interfaceMock.MockWorkflowClient) {
				mockClient.EXPECT().GetTriggerSchedule(gomock.Any(), "test-namespace.test-triggerrun").Return("0 6 * * *", nil)
				mockClient.EXPECT().UpdateTrigger(gomock.Any(), "test-namespace.test-triggerrun", "0 0 * * *").Return(nil)
			},
			expectError: false,
		},
		{
			name: "not a cron trigger - skip sync",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-triggerrun",
				},
				Spec: v2pb.TriggerRunSpec{
					Trigger: &v2pb.Trigger{
						TriggerType: &v2pb.Trigger_BatchRerun{
							BatchRerun: &v2pb.BatchRerun{},
						},
					},
				},
			},
			desiredCron: "0 6 * * *",
			setupMock: func(mockClient *interfaceMock.MockWorkflowClient) {
				// No calls expected
			},
			expectError: false,
		},
		{
			name: "empty desired cron - skip sync",
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
			},
			desiredCron: "",
			setupMock: func(mockClient *interfaceMock.MockWorkflowClient) {
				// No calls expected
			},
			expectError: false,
		},
		{
			name: "get trigger schedule fails",
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
			},
			desiredCron: "0 6 * * *",
			setupMock: func(mockClient *interfaceMock.MockWorkflowClient) {
				mockClient.EXPECT().GetTriggerSchedule(gomock.Any(), "test-namespace.test-triggerrun").Return("", fmt.Errorf("failed to get schedule"))
			},
			expectError: true,
		},
		{
			name: "update trigger fails",
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
			},
			desiredCron: "0 0 * * *",
			setupMock: func(mockClient *interfaceMock.MockWorkflowClient) {
				mockClient.EXPECT().GetTriggerSchedule(gomock.Any(), "test-namespace.test-triggerrun").Return("0 6 * * *", nil)
				mockClient.EXPECT().UpdateTrigger(gomock.Any(), "test-namespace.test-triggerrun", "0 0 * * *").Return(fmt.Errorf("update failed"))
			},
			expectError: true,
		},
		{
			name: "Temporal returns empty string - triggers update",
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
			},
			desiredCron: "0 6 * * *",
			setupMock: func(mockClient *interfaceMock.MockWorkflowClient) {
				mockClient.EXPECT().GetTriggerSchedule(gomock.Any(), "test-namespace.test-triggerrun").Return("", nil)
				mockClient.EXPECT().UpdateTrigger(gomock.Any(), "test-namespace.test-triggerrun", "0 6 * * *").Return(nil)
			},
			expectError: false,
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
			err := syncCronScheduleToTemporal(context.Background(), tt.triggerRun, tt.desiredCron, logger, mockClient)

			// Verify results
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
