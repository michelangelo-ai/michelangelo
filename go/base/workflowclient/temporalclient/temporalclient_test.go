package temporalclient

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	temporalEnumsV1 "go.temporal.io/api/enums/v1"
	workflowV1 "go.temporal.io/api/workflow/v1"
	workflowserviceV1 "go.temporal.io/api/workflowservice/v1"
	temporalClient "go.temporal.io/sdk/client"
	temporalConverter "go.temporal.io/sdk/converter"
	temporalMocks "go.temporal.io/sdk/mocks"
)

func TestStartWorkflow(t *testing.T) {
	testCases := []struct {
		name     string
		mockFunc func(mockTemporalClient *temporalMocks.Client, mockWorkflowRun *temporalMocks.WorkflowRun)
		errMsg   string
	}{
		{
			name: "success",
			mockFunc: func(mockTemporalClient *temporalMocks.Client, mockWorkflowRun *temporalMocks.WorkflowRun) {
				mockWorkflowRun.On("GetID").Return("testWorkflow")
				mockWorkflowRun.On("GetRunID").Return("testRunID")
				mockTemporalClient.On("ExecuteWorkflow", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mockWorkflowRun, nil)
			},
			errMsg: "",
		},
		{
			name: "error",
			mockFunc: func(mockTemporalClient *temporalMocks.Client, mockWorkflowRun *temporalMocks.WorkflowRun) {
				mockTemporalClient.On("ExecuteWorkflow", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("error"))
			},
			errMsg: "error",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// create a new temporal client
			mockClient := temporalMocks.NewClient(t)
			client := &TemporalClient{
				Client: mockClient,
			}
			mockWorkflowRun := temporalMocks.NewWorkflowRun(t)
			testCase.mockFunc(mockClient, mockWorkflowRun)
			_, err := client.StartWorkflow(context.Background(), clientInterface.StartWorkflowOptions{}, "testWorkflow", "testArgs")
			if testCase.errMsg != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), testCase.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGetWorkflowExecutionInfo(t *testing.T) {
	testCases := []struct {
		name           string
		mockFunc       func(mockTemporalClient *temporalMocks.Client)
		errMsg         string
		expectedStatus clientInterface.WorkflowExecutionStatus
	}{
		{
			name: "unspecified",
			mockFunc: func(mockTemporalClient *temporalMocks.Client) {
				mockTemporalClient.On("DescribeWorkflowExecution", mock.Anything, mock.Anything, mock.Anything).Return(
					&workflowserviceV1.DescribeWorkflowExecutionResponse{
						WorkflowExecutionInfo: &workflowV1.WorkflowExecutionInfo{
							Status: temporalEnumsV1.WORKFLOW_EXECUTION_STATUS_UNSPECIFIED,
						},
					},
					nil,
				)
			},
			expectedStatus: clientInterface.WorkflowExecutionStatusUnSpecified,
		},
		{
			name: "success",
			mockFunc: func(mockTemporalClient *temporalMocks.Client) {
				mockTemporalClient.On("DescribeWorkflowExecution", mock.Anything, mock.Anything, mock.Anything).Return(
					&workflowserviceV1.DescribeWorkflowExecutionResponse{
						WorkflowExecutionInfo: &workflowV1.WorkflowExecutionInfo{
							Status: temporalEnumsV1.WORKFLOW_EXECUTION_STATUS_RUNNING,
						},
					},
					nil,
				)
			},
			expectedStatus: clientInterface.WorkflowExecutionStatusRunning,
		},
		{
			name: "failed",
			mockFunc: func(mockTemporalClient *temporalMocks.Client) {
				mockTemporalClient.On("DescribeWorkflowExecution", mock.Anything, mock.Anything, mock.Anything).Return(
					&workflowserviceV1.DescribeWorkflowExecutionResponse{
						WorkflowExecutionInfo: &workflowV1.WorkflowExecutionInfo{
							Status: temporalEnumsV1.WORKFLOW_EXECUTION_STATUS_FAILED,
						},
					},
					nil,
				)
			},
			expectedStatus: clientInterface.WorkflowExecutionStatusFailed,
		},
		{
			name: "error",
			mockFunc: func(mockTemporalClient *temporalMocks.Client) {
				mockTemporalClient.On("DescribeWorkflowExecution", mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("error"))
			},
			errMsg: "error",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// create a new temporal client
			mockClient := temporalMocks.NewClient(t)
			client := &TemporalClient{
				Client: mockClient,
			}
			testCase.mockFunc(mockClient)
			status, err := client.GetWorkflowExecutionInfo(context.Background(), "testWorkflow", "testRunID")
			if testCase.errMsg != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), testCase.errMsg)
			} else {
				require.NoError(t, err)
				require.Equal(t, testCase.expectedStatus, status.Status)
			}
		})
	}
}

func newEncodedValue[T any](value *T, err error) temporalConverter.EncodedValue {
	return &fakeEncodedValue[T]{value: value, err: err}
}

type fakeEncodedValue[T any] struct {
	value *T
	err   error
}

var _ temporalConverter.EncodedValue = &fakeEncodedValue[any]{}

// HasValue return whether there is value encoded.
func (v *fakeEncodedValue[T]) HasValue() bool {
	return v.value != nil
}

// Get extract the encoded value into strong typed value pointer.
func (v *fakeEncodedValue[T]) Get(valuePtr interface{}) error {
	if v.err != nil {
		return v.err
	}
	marshalled, err := json.Marshal(v.value)
	if err != nil {
		return err
	}
	err = json.Unmarshal(marshalled, valuePtr)
	return err
}

func TestQueryWorkflow(t *testing.T) {
	queryResult := "testResult"
	queryResultWrongFormat := map[string]string{"test": "result"}
	testCases := []struct {
		name           string
		mockFunc       func(mockTemporalClient *temporalMocks.Client)
		errMsg         string
		expectedResult string
	}{
		{
			name: "success",
			mockFunc: func(mockTemporalClient *temporalMocks.Client) {
				mockTemporalClient.On("QueryWorkflowWithOptions", mock.Anything, mock.Anything).Return(
					&temporalClient.QueryWorkflowWithOptionsResponse{
						QueryResult: newEncodedValue(&queryResult, nil),
					}, nil)
			},
			expectedResult: "testResult",
		},
		{
			name: "error",
			mockFunc: func(mockTemporalClient *temporalMocks.Client) {
				mockTemporalClient.On("QueryWorkflowWithOptions", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("error"))
			},
			errMsg: "error",
		},
		{
			name: "nil query result",
			mockFunc: func(mockTemporalClient *temporalMocks.Client) {
				mockTemporalClient.On("QueryWorkflowWithOptions", mock.Anything, mock.Anything).Return(nil, nil)
			},
			errMsg: "queryResult is nil",
		},
		{
			name: "wrong format",
			mockFunc: func(mockTemporalClient *temporalMocks.Client) {
				mockTemporalClient.On("QueryWorkflowWithOptions", mock.Anything, mock.Anything).Return(
					&temporalClient.QueryWorkflowWithOptionsResponse{
						QueryResult: newEncodedValue(&queryResultWrongFormat, nil),
					}, nil)
			},
			errMsg: "failed to decode query result",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// create a new temporal client
			mockClient := temporalMocks.NewClient(t)
			client := &TemporalClient{
				Client: mockClient,
			}
			testCase.mockFunc(mockClient)
			var result string
			err := client.QueryWorkflow(context.Background(), "testWorkflow", "testRunID", "testQuery", &result)
			if testCase.errMsg != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), testCase.errMsg)
			} else {
				require.NoError(t, err)
				require.Equal(t, testCase.expectedResult, result)
			}
		})
	}
}

func TestCancelWorkflow(t *testing.T) {
	testCases := []struct {
		name     string
		mockFunc func(mockTemporalClient *temporalMocks.Client)
		errMsg   string
	}{
		{
			name: "success",
			mockFunc: func(mockTemporalClient *temporalMocks.Client) {
				mockTemporalClient.On("CancelWorkflow", mock.Anything, mock.Anything, mock.Anything).Return(nil)
			},
			errMsg: "",
		},
		{
			name: "error",
			mockFunc: func(mockTemporalClient *temporalMocks.Client) {
				mockTemporalClient.On("CancelWorkflow", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("error"))
			},
			errMsg: "error",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mockClient := temporalMocks.NewClient(t)
			client := &TemporalClient{
				Client: mockClient,
			}
			testCase.mockFunc(mockClient)
			err := client.CancelWorkflow(context.Background(), "testWorkflow", "testRunID", "testReason")
			if testCase.errMsg != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), testCase.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGetProvider(t *testing.T) {
	mockClient := temporalMocks.NewClient(t)
	client := &TemporalClient{
		Client:   mockClient,
		Provider: "temporal",
	}
	provider := client.GetProvider()
	require.Equal(t, "temporal", provider)
}

func TestGetDomain(t *testing.T) {
	mockClient := temporalMocks.NewClient(t)
	client := &TemporalClient{
		Client:   mockClient,
		Provider: "temporal",
		Domain:   "default",
	}
	domain := client.GetDomain()
	require.Equal(t, "default", domain)
}

func TestListOpenWorkflow(t *testing.T) {
	request := clientInterface.ListOpenWorkflowExecutionsRequest{
		Domain:        "default",
		NextPageToken: []byte("testPageToken"),
		ExecutionFilter: &clientInterface.ExecutionFilter{
			WorkflowID: "testWorkflowID",
			RunID:      "testRunID",
		},
	}

	testCases := []struct {
		name     string
		mockFunc func(mockClient *temporalMocks.Client)
		errMsg   string
	}{
		{
			name: "ListOpenWorkflow Succeeded",
			mockFunc: func(mockClient *temporalMocks.Client) {
				mockResponse := &workflowserviceV1.ListOpenWorkflowExecutionsResponse{
					Executions:    []*workflowV1.WorkflowExecutionInfo{},
					NextPageToken: []byte("nextToken"),
				}
				mockClient.On("ListOpenWorkflow", mock.Anything, mock.Anything).Return(mockResponse, nil)
			},
			errMsg: "",
		},
		{
			name: "ListOpenWorkflow Failed",
			mockFunc: func(mockClient *temporalMocks.Client) {
				mockClient.On("ListOpenWorkflow", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("test error"))
			},
			errMsg: "test error",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mockClient := temporalMocks.NewClient(t)
			testCase.mockFunc(mockClient)
			client := &TemporalClient{
				Client: mockClient,
			}
			response, err := client.ListOpenWorkflow(context.Background(), request)
			if testCase.errMsg != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), testCase.errMsg)
			} else {
				require.NoError(t, err)
				require.NotNil(t, response)
			}
		})
	}
}

func TestTerminateWorkflow(t *testing.T) {
	workflowID := "testWorkflowID"
	runID := "testRunID"
	reason := "test termination reason"

	testCases := []struct {
		name     string
		mockFunc func(mockClient *temporalMocks.Client)
		errMsg   string
	}{
		{
			name: "TerminateWorkflow Succeeded",
			mockFunc: func(mockClient *temporalMocks.Client) {
				mockClient.On("TerminateWorkflow", mock.Anything, workflowID, runID, reason).Return(nil)
			},
			errMsg: "",
		},
		{
			name: "TerminateWorkflow Failed",
			mockFunc: func(mockClient *temporalMocks.Client) {
				mockClient.On("TerminateWorkflow", mock.Anything, workflowID, runID, reason).Return(fmt.Errorf("test error"))
			},
			errMsg: "test error",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mockClient := temporalMocks.NewClient(t)
			testCase.mockFunc(mockClient)
			client := &TemporalClient{
				Client: mockClient,
			}
			err := client.TerminateWorkflow(context.Background(), workflowID, runID, reason)
			if testCase.errMsg != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), testCase.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCreateScheduleForCron(t *testing.T) {
	testCases := []struct {
		name          string
		options       clientInterface.StartWorkflowOptions
		workflowName  string
		args          []interface{}
		mockFunc      func(mockClient *temporalMocks.Client, mockScheduleClient *temporalMocks.ScheduleClient, mockScheduleHandle *temporalMocks.ScheduleHandle)
		expectedID    string
		expectedRunID string
		errMsg        string
	}{
		{
			name: "success - schedule created",
			options: clientInterface.StartWorkflowOptions{
				ID:                              "test-workflow",
				TaskList:                        "test-task-list",
				ExecutionStartToCloseTimeout:    time.Hour,
				DecisionTaskStartToCloseTimeout: 30 * time.Second,
				CronSchedule:                    "0 0 * * *",
			},
			workflowName: "test-workflow-name",
			args:         []interface{}{"arg1", "arg2"},
			mockFunc: func(mockClient *temporalMocks.Client, mockScheduleClient *temporalMocks.ScheduleClient, mockScheduleHandle *temporalMocks.ScheduleHandle) {
				// Mock ScheduleClient() method
				mockClient.On("ScheduleClient").Return(mockScheduleClient)

				// Mock GetHandle - schedule doesn't exist yet
				mockScheduleClient.On("GetHandle", mock.Anything, "test-workflow-schedule").Return(mockScheduleHandle)

				// Mock Describe - schedule doesn't exist (returns error)
				mockScheduleHandle.On("Describe", mock.Anything).Return(nil, fmt.Errorf("schedule not found"))

				// Mock Create - successfully creates schedule
				mockScheduleClient.On("Create", mock.Anything, mock.MatchedBy(func(options temporalClient.ScheduleOptions) bool {
					return options.ID == "test-workflow-schedule" &&
						len(options.Spec.CronExpressions) == 1 &&
						options.Spec.CronExpressions[0] == "0 0 * * *" &&
						options.Action != nil
				})).Return(mockScheduleHandle, nil)
			},
			expectedID:    "test-workflow-schedule",
			expectedRunID: "",
			errMsg:        "",
		},
		{
			name: "success - schedule already exists",
			options: clientInterface.StartWorkflowOptions{
				ID:           "existing-workflow",
				TaskList:     "test-task-list",
				CronSchedule: "0 0 * * *",
			},
			workflowName: "test-workflow-name",
			args:         []interface{}{"arg1"},
			mockFunc: func(mockClient *temporalMocks.Client, mockScheduleClient *temporalMocks.ScheduleClient, mockScheduleHandle *temporalMocks.ScheduleHandle) {
				// Mock ScheduleClient() method
				mockClient.On("ScheduleClient").Return(mockScheduleClient)

				// Mock GetHandle - schedule exists
				mockScheduleClient.On("GetHandle", mock.Anything, "existing-workflow-schedule").Return(mockScheduleHandle)

				// Mock Describe - schedule exists (returns successfully)
				mockScheduleHandle.On("Describe", mock.Anything).Return(&temporalClient.ScheduleDescription{}, nil)

				// Create should not be called in this case
			},
			expectedID:    "existing-workflow-schedule",
			expectedRunID: "",
			errMsg:        "",
		},
		{
			name: "error - create schedule fails",
			options: clientInterface.StartWorkflowOptions{
				ID:           "test-workflow",
				TaskList:     "test-task-list",
				CronSchedule: "0 0 * * *",
			},
			workflowName: "test-workflow-name",
			args:         []interface{}{"arg1"},
			mockFunc: func(mockClient *temporalMocks.Client, mockScheduleClient *temporalMocks.ScheduleClient, mockScheduleHandle *temporalMocks.ScheduleHandle) {
				// Mock ScheduleClient() method
				mockClient.On("ScheduleClient").Return(mockScheduleClient)

				// Mock GetHandle - schedule doesn't exist
				mockScheduleClient.On("GetHandle", mock.Anything, "test-workflow-schedule").Return(mockScheduleHandle)

				// Mock Describe - schedule doesn't exist (returns error)
				mockScheduleHandle.On("Describe", mock.Anything).Return(nil, fmt.Errorf("schedule not found"))

				// Mock Create - fails to create schedule
				mockScheduleClient.On("Create", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("failed to create schedule"))
			},
			expectedID:    "",
			expectedRunID: "",
			errMsg:        "failed to create Temporal schedule",
		},
		{
			name: "success - describe fails but create succeeds",
			options: clientInterface.StartWorkflowOptions{
				ID:                           "test-workflow",
				TaskList:                     "test-task-list",
				CronSchedule:                 "0 0 * * *",
				ExecutionStartToCloseTimeout: 2 * time.Hour,
			},
			workflowName: "test-workflow-name",
			args:         []interface{}{"arg1"},
			mockFunc: func(mockClient *temporalMocks.Client, mockScheduleClient *temporalMocks.ScheduleClient, mockScheduleHandle *temporalMocks.ScheduleHandle) {
				// Mock ScheduleClient() method
				mockClient.On("ScheduleClient").Return(mockScheduleClient)

				// Mock GetHandle - returns handle but describe will fail
				mockScheduleClient.On("GetHandle", mock.Anything, "test-workflow-schedule").Return(mockScheduleHandle)

				// Mock Describe - fails (maybe schedule was deleted)
				mockScheduleHandle.On("Describe", mock.Anything).Return(nil, fmt.Errorf("describe failed"))

				// Mock Create - successfully creates schedule with timeout set
				mockScheduleClient.On("Create", mock.Anything, mock.MatchedBy(func(options temporalClient.ScheduleOptions) bool {
					action, ok := options.Action.(*temporalClient.ScheduleWorkflowAction)
					return ok && action.WorkflowExecutionTimeout == 2*time.Hour
				})).Return(mockScheduleHandle, nil)
			},
			expectedID:    "test-workflow-schedule",
			expectedRunID: "",
			errMsg:        "",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mockClient := temporalMocks.NewClient(t)
			mockScheduleClient := temporalMocks.NewScheduleClient(t)
			mockScheduleHandle := temporalMocks.NewScheduleHandle(t)

			client := &TemporalClient{
				Client:   mockClient,
				Provider: "temporal",
				Domain:   "default",
			}

			testCase.mockFunc(mockClient, mockScheduleClient, mockScheduleHandle)

			result, err := client.createScheduleForCron(
				context.Background(),
				testCase.options,
				testCase.workflowName,
				testCase.args...,
			)

			if testCase.errMsg != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), testCase.errMsg)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, testCase.expectedID, result.ID)
				require.Equal(t, testCase.expectedRunID, result.RunID)
			}
		})
	}
}
