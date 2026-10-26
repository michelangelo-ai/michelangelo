package cadenceclient

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/cadence-workflow/starlark-worker/cadence"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/cadence/.gen/go/shared"
	cadenceClient "go.uber.org/cadence/client"
	"go.uber.org/cadence/encoded"
	cadencemocks "go.uber.org/cadence/mocks"
	cadenceworkflow "go.uber.org/cadence/workflow"
	"go.uber.org/zap"
)

func TestStartWorkflow(t *testing.T) {
	workflowID := "testWorkflowID"
	runID := "testRunID"

	testCases := []struct {
		name     string
		mockFunc func(mockClient *cadencemocks.Client)
		errMsg   string
	}{
		{
			name: "StartWorkflow Succeeded",
			mockFunc: func(mockClient *cadencemocks.Client) {
				mockClient.On("StartWorkflow", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
					&cadenceworkflow.Execution{
						ID:    workflowID,
						RunID: runID,
					},
					nil,
				)
			},
			errMsg: "",
		},
		{
			name: "StartWorkflow Failed",
			mockFunc: func(mockClient *cadencemocks.Client) {
				mockClient.On("StartWorkflow", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
					nil,
					fmt.Errorf("test error"),
				)
			},
			errMsg: "test error",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mockClient := &cadencemocks.Client{}
			testCase.mockFunc(mockClient)
			client := &CadenceClient{
				Client: mockClient,
			}
			_, err := client.StartWorkflow(context.Background(), clientInterface.StartWorkflowOptions{}, "testWorkflow", "testWorkflow")
			if testCase.errMsg != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), testCase.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetWorkflowExecutionInfo(t *testing.T) {
	workflowID := "testWorkflowID"
	runID := "testRunID"

	testCases := []struct {
		name                   string
		mockFunc               func(mockClient *cadencemocks.Client)
		expectedStatus         clientInterface.WorkflowExecutionStatus
		expectedExecution      *clientInterface.WorkflowExecution
		expectedFailureMessage string
		errMsg                 string
	}{
		{
			name: "GetWorkflowExecutionInfo Succeeded -- workflow completed",
			mockFunc: func(mockClient *cadencemocks.Client) {
				mockClient.On("DescribeWorkflowExecution", mock.Anything, mock.Anything, mock.Anything).Return(
					&shared.DescribeWorkflowExecutionResponse{
						WorkflowExecutionInfo: &shared.WorkflowExecutionInfo{
							CloseStatus: shared.WorkflowExecutionCloseStatusCompleted.Ptr(),
						},
					}, nil)
			},
			expectedStatus:    clientInterface.WorkflowExecutionStatusCompleted,
			expectedExecution: &clientInterface.WorkflowExecution{},
			errMsg:            "",
		},
		{
			name: "GetWorkflowExecutionInfo Succeeded -- workflow failed",
			mockFunc: func(mockClient *cadencemocks.Client) {
				mockClient.On("DescribeWorkflowExecution", mock.Anything, mock.Anything, mock.Anything).Return(
					&shared.DescribeWorkflowExecutionResponse{
						WorkflowExecutionInfo: &shared.WorkflowExecutionInfo{
							CloseStatus: shared.WorkflowExecutionCloseStatusFailed.Ptr(),
						},
					}, nil)
				iter := cadencemocks.NewHistoryEventIterator(t)
				iter.On("HasNext").Return(true).Once()
				iter.On("Next").Return(&shared.HistoryEvent{
					EventType: shared.EventTypeWorkflowExecutionFailed.Ptr(),
					WorkflowExecutionFailedEventAttributes: &shared.WorkflowExecutionFailedEventAttributes{
						Reason: ptrString("got an unexpected keyword argument"),
					},
				}, nil).Once()
				iter.On("HasNext").Return(false)
				mockClient.On("GetWorkflowHistory", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(iter)
			},
			expectedStatus:         clientInterface.WorkflowExecutionStatusFailed,
			expectedExecution:      &clientInterface.WorkflowExecution{},
			expectedFailureMessage: "got an unexpected keyword argument",
			errMsg:                 "",
		},
		{
			// The starlark-worker library (used by our pipeline workflows) reports
			// application errors via workflow.NewCustomError(ctx, code, err.Error()),
			// which stores the generic code (e.g. "invalid-argument") in Reason and the
			// actual descriptive text in Details, encoded with its own custom
			// DataConverter. GetWorkflowExecutionInfo must prefer that decoded detail
			// over the generic Reason.
			name: "GetWorkflowExecutionInfo Succeeded -- workflow failed with encoded details",
			mockFunc: func(mockClient *cadencemocks.Client) {
				mockClient.On("DescribeWorkflowExecution", mock.Anything, mock.Anything, mock.Anything).Return(
					&shared.DescribeWorkflowExecutionResponse{
						WorkflowExecutionInfo: &shared.WorkflowExecutionInfo{
							CloseStatus: shared.WorkflowExecutionCloseStatusFailed.Ptr(),
						},
					}, nil)
				detailBytes, err := (&cadence.DataConverter{Logger: zap.NewNop()}).ToData(
					`function pipeline_220 got an unexpected keyword argument "task_1_d"`,
				)
				require.NoError(t, err)
				iter := cadencemocks.NewHistoryEventIterator(t)
				iter.On("HasNext").Return(true).Once()
				iter.On("Next").Return(&shared.HistoryEvent{
					EventType: shared.EventTypeWorkflowExecutionFailed.Ptr(),
					WorkflowExecutionFailedEventAttributes: &shared.WorkflowExecutionFailedEventAttributes{
						Reason:  ptrString("invalid-argument"),
						Details: detailBytes,
					},
				}, nil).Once()
				iter.On("HasNext").Return(false)
				mockClient.On("GetWorkflowHistory", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(iter)
			},
			expectedStatus:         clientInterface.WorkflowExecutionStatusFailed,
			expectedExecution:      &clientInterface.WorkflowExecution{},
			expectedFailureMessage: `function pipeline_220 got an unexpected keyword argument "task_1_d"`,
			errMsg:                 "",
		},
		{
			name: "GetWorkflowExecutionInfo Succeeded -- workflow timed out",
			mockFunc: func(mockClient *cadencemocks.Client) {
				mockClient.On("DescribeWorkflowExecution", mock.Anything, mock.Anything, mock.Anything).Return(
					&shared.DescribeWorkflowExecutionResponse{
						WorkflowExecutionInfo: &shared.WorkflowExecutionInfo{
							CloseStatus: shared.WorkflowExecutionCloseStatusTimedOut.Ptr(),
						},
					}, nil)
			},
			expectedStatus:         clientInterface.WorkflowExecutionStatusTimedOut,
			expectedExecution:      &clientInterface.WorkflowExecution{},
			expectedFailureMessage: "Workflow execution timed out",
			errMsg:                 "",
		},
		{
			name: "GetWorkflowExecutionInfo Succeeded -- workflow running",
			mockFunc: func(mockClient *cadencemocks.Client) {
				mockClient.On("DescribeWorkflowExecution", mock.Anything, mock.Anything, mock.Anything).Return(
					&shared.DescribeWorkflowExecutionResponse{
						WorkflowExecutionInfo: &shared.WorkflowExecutionInfo{
							CloseStatus: nil,
							Execution: &shared.WorkflowExecution{
								WorkflowId: ptrString(workflowID),
								RunId:      ptrString(runID),
							},
						},
					}, nil)
			},
			expectedStatus: clientInterface.WorkflowExecutionStatusRunning,
			expectedExecution: &clientInterface.WorkflowExecution{
				ID:    workflowID,
				RunID: runID,
			},
			errMsg: "",
		},
		{
			name: "GetWorkflowExecutionInfo Failed",
			mockFunc: func(mockClient *cadencemocks.Client) {
				mockClient.On("DescribeWorkflowExecution", mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("test error"))
			},
			expectedStatus: clientInterface.WorkflowExecutionStatusUnSpecified,
			errMsg:         "test error",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mockClient := &cadencemocks.Client{}
			testCase.mockFunc(mockClient)
			client := &CadenceClient{
				Client: mockClient,
			}
			workflowExecutionInfo, err := client.GetWorkflowExecutionInfo(context.Background(), workflowID, runID)
			if testCase.errMsg != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), testCase.errMsg)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, testCase.expectedStatus, workflowExecutionInfo.Status)
				assert.Equal(t, testCase.expectedExecution, workflowExecutionInfo.Execution)
				assert.Equal(t, testCase.expectedFailureMessage, workflowExecutionInfo.FailureMessage)
			}
		})
	}
}

func TestCancelWorkflow(t *testing.T) {
	workflowID := "testWorkflowID"
	runID := "testRunID"

	testCases := []struct {
		name     string
		mockFunc func(mockClient *cadencemocks.Client)
		errMsg   string
	}{
		{
			name: "CancelWorkflow Succeeded",
			mockFunc: func(mockClient *cadencemocks.Client) {
				mockClient.On("CancelWorkflow", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
			},
			errMsg: "",
		},
		{
			name: "CancelWorkflow Failed",
			mockFunc: func(mockClient *cadencemocks.Client) {
				mockClient.On("CancelWorkflow", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("test error"))
			},
			errMsg: "test error",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mockClient := &cadencemocks.Client{}
			testCase.mockFunc(mockClient)
			client := &CadenceClient{
				Client: mockClient,
			}
			err := client.CancelWorkflow(context.Background(), workflowID, runID, "test reason")
			if testCase.errMsg != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), testCase.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func newEncodedValue[T any](value *T, err error) encoded.Value {
	return &fakeEncodedValue[T]{value: value, err: err}
}

type fakeEncodedValue[T any] struct {
	value *T
	err   error
}

var _ encoded.Value = &fakeEncodedValue[any]{}

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
	workflowID := "testWorkflowID"
	runID := "testRunID"
	queryHandlerKey := "testQueryHandlerKey"
	queryResult := "test result"
	queryResultWrongFormat := map[string]string{"test": "result"}

	testCases := []struct {
		name     string
		mockFunc func(mockClient *cadencemocks.Client)
		errMsg   string
	}{
		{
			name: "QueryWorkflow Succeeded",
			mockFunc: func(mockClient *cadencemocks.Client) {
				mockClient.On("QueryWorkflowWithOptions", mock.Anything, mock.Anything).Return(
					&cadenceClient.QueryWorkflowWithOptionsResponse{
						QueryResult: newEncodedValue(&queryResult, nil),
					}, nil)
			},
			errMsg: "",
		},
		{
			name: "QueryWorkflow Failed",
			mockFunc: func(mockClient *cadencemocks.Client) {
				mockClient.On("QueryWorkflowWithOptions", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("test error"))
			},
			errMsg: "test error",
		},
		{
			name: "QueryWorkflow Failed with wrong query result format",
			mockFunc: func(mockClient *cadencemocks.Client) {
				mockClient.On("QueryWorkflowWithOptions", mock.Anything, mock.Anything).Return(
					&cadenceClient.QueryWorkflowWithOptionsResponse{
						QueryResult: newEncodedValue(&queryResultWrongFormat, nil),
					}, nil)
			},
			errMsg: "error getting query result",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mockClient := &cadencemocks.Client{}
			testCase.mockFunc(mockClient)
			client := &CadenceClient{
				Client: mockClient,
			}
			var result string
			err := client.QueryWorkflow(context.Background(), workflowID, runID, queryHandlerKey, &result)
			if testCase.errMsg != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), testCase.errMsg)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, "test result", result)
			}
		})
	}
}

func TestGetProvider(t *testing.T) {
	client := &CadenceClient{
		Client:   &cadencemocks.Client{},
		Provider: "cadence",
	}
	assert.Equal(t, "cadence", client.GetProvider())
}

func TestGetDomain(t *testing.T) {
	client := &CadenceClient{
		Client:   &cadencemocks.Client{},
		Provider: "cadence",
		Domain:   "default",
	}
	assert.Equal(t, "default", client.GetDomain())
}

func TestTerminateWorkflow(t *testing.T) {
	workflowID := "testWorkflowID"
	runID := "testRunID"
	reason := "test termination reason"

	testCases := []struct {
		name     string
		mockFunc func(mockClient *cadencemocks.Client)
		errMsg   string
	}{
		{
			name: "TerminateWorkflow Succeeded",
			mockFunc: func(mockClient *cadencemocks.Client) {
				mockClient.On("TerminateWorkflow", mock.Anything, workflowID, runID, reason, mock.Anything).Return(nil)
			},
			errMsg: "",
		},
		{
			name: "TerminateWorkflow Failed",
			mockFunc: func(mockClient *cadencemocks.Client) {
				mockClient.On("TerminateWorkflow", mock.Anything, workflowID, runID, reason, mock.Anything).Return(fmt.Errorf("test error"))
			},
			errMsg: "test error",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mockClient := &cadencemocks.Client{}
			testCase.mockFunc(mockClient)
			client := &CadenceClient{
				Client: mockClient,
			}
			err := client.TerminateWorkflow(context.Background(), workflowID, runID, reason)
			if testCase.errMsg != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), testCase.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
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
	workflowID := "testWorkflowID"
	runID := "testRunID"
	executionTime := time.Now().UnixNano()
	expectedResponse := &shared.ListOpenWorkflowExecutionsResponse{
		Executions: []*shared.WorkflowExecutionInfo{
			{
				Execution: &shared.WorkflowExecution{
					WorkflowId: &workflowID,
					RunId:      &runID,
				},
				ExecutionTime: &executionTime,
			},
		},
		NextPageToken: []byte("nextToken"),
	}
	testCases := []struct {
		name     string
		mockFunc func(mockClient *cadencemocks.Client)
		errMsg   string
	}{
		{
			name: "ListOpenWorkflow Succeeded",
			mockFunc: func(mockClient *cadencemocks.Client) {
				mockClient.On("ListOpenWorkflow", mock.Anything, mock.Anything).Return(expectedResponse, nil)
			},
			errMsg: "",
		},
		{
			name: "ListOpenWorkflow Failed",
			mockFunc: func(mockClient *cadencemocks.Client) {
				mockClient.On("ListOpenWorkflow", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("test error"))
			},
			errMsg: "test error",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mockClient := &cadencemocks.Client{}
			testCase.mockFunc(mockClient)
			client := &CadenceClient{
				Client: mockClient,
			}
			response, err := client.ListOpenWorkflow(context.Background(), request)
			if testCase.errMsg != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), testCase.errMsg)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				assert.Equal(t, 1, len(response.Executions))
				assert.Equal(t, "testWorkflowID", response.Executions[0].Execution.ID)
				assert.Equal(t, "testRunID", response.Executions[0].Execution.RunID)
			}
		})
	}
}

func TestResetWorkflow(t *testing.T) {
	workflowID := "testWorkflowID"
	runID := "testRunID"
	newRunID := "newTestRunID"
	eventID := int64(123)
	reason := "test reset reason"
	requestID := "test-request-id"

	testCases := []struct {
		name     string
		options  clientInterface.ResetWorkflowOptions
		mockFunc func(mockClient *cadencemocks.Client)
		errMsg   string
	}{
		{
			name: "ResetWorkflow Succeeded",
			options: clientInterface.ResetWorkflowOptions{
				WorkflowID: workflowID,
				RunID:      runID,
				EventID:    eventID,
				Reason:     reason,
				RequestID:  requestID,
			},
			mockFunc: func(mockClient *cadencemocks.Client) {
				mockClient.On("ResetWorkflow", mock.Anything, mock.MatchedBy(func(req *shared.ResetWorkflowExecutionRequest) bool {
					return *req.Domain == "" &&
						*req.WorkflowExecution.WorkflowId == workflowID &&
						*req.WorkflowExecution.RunId == runID &&
						*req.DecisionFinishEventId == eventID &&
						*req.Reason == reason &&
						*req.RequestId == requestID
				})).Return(
					&shared.ResetWorkflowExecutionResponse{
						RunId: &newRunID,
					}, nil)
			},
			errMsg: "",
		},
		{
			name: "ResetWorkflow Succeeded without RequestID",
			options: clientInterface.ResetWorkflowOptions{
				WorkflowID: workflowID,
				RunID:      runID,
				EventID:    eventID,
				Reason:     reason,
				RequestID:  "", // Empty request ID
			},
			mockFunc: func(mockClient *cadencemocks.Client) {
				mockClient.On("ResetWorkflow", mock.Anything, mock.MatchedBy(func(req *shared.ResetWorkflowExecutionRequest) bool {
					return *req.Domain == "" &&
						*req.WorkflowExecution.WorkflowId == workflowID &&
						*req.WorkflowExecution.RunId == runID &&
						*req.DecisionFinishEventId == eventID &&
						*req.Reason == reason &&
						req.RequestId == nil
				})).Return(
					&shared.ResetWorkflowExecutionResponse{
						RunId: &newRunID,
					}, nil)
			},
			errMsg: "",
		},
		{
			name: "ResetWorkflow Failed",
			options: clientInterface.ResetWorkflowOptions{
				WorkflowID: workflowID,
				RunID:      runID,
				EventID:    eventID,
				Reason:     reason,
			},
			mockFunc: func(mockClient *cadencemocks.Client) {
				mockClient.On("ResetWorkflow", mock.Anything, mock.Anything).Return(
					nil, fmt.Errorf("reset failed"))
			},
			errMsg: "failed to reset workflow: reset failed",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mockClient := &cadencemocks.Client{}
			testCase.mockFunc(mockClient)
			client := &CadenceClient{
				Client: mockClient,
				Domain: "",
			}

			execution, err := client.ResetWorkflow(context.Background(), testCase.options)

			if testCase.errMsg != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), testCase.errMsg)
				assert.Nil(t, execution)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, execution)
				assert.Equal(t, workflowID, execution.ID)
				assert.Equal(t, newRunID, execution.RunID)
			}
		})
	}
}

func TestGetWorkflowExecutionHistory(t *testing.T) {
	workflowID := "testWorkflowID"
	runID := "testRunID"

	// Mock history iterator
	eventID1 := int64(1)
	eventID2 := int64(2)
	eventID3 := int64(3)
	eventID4 := int64(4)
	timestamp := time.Now().UnixNano()
	workflowName := "test-workflow"
	tasklistName := "test-tasklist"
	identity := "test-identity"
	activityID := "test-activity-1"
	activityTypeName := "test-activity-type"
	reason := "activity failed"

	mockIterator := &mockHistoryIterator{
		events: []*shared.HistoryEvent{
			{
				EventId:   &eventID1,
				EventType: shared.EventTypeWorkflowExecutionStarted.Ptr(),
				Timestamp: &timestamp,
				WorkflowExecutionStartedEventAttributes: &shared.WorkflowExecutionStartedEventAttributes{
					WorkflowType: &shared.WorkflowType{Name: &workflowName},
					TaskList:     &shared.TaskList{Name: &tasklistName},
				},
			},
			{
				EventId:   &eventID2,
				EventType: shared.EventTypeDecisionTaskCompleted.Ptr(),
				Timestamp: &timestamp,
				DecisionTaskCompletedEventAttributes: &shared.DecisionTaskCompletedEventAttributes{
					Identity:         &identity,
					ScheduledEventId: &eventID1,
				},
			},
			{
				EventId:   &eventID3,
				EventType: shared.EventTypeActivityTaskScheduled.Ptr(),
				Timestamp: &timestamp,
				ActivityTaskScheduledEventAttributes: &shared.ActivityTaskScheduledEventAttributes{
					ActivityId:   &activityID,
					ActivityType: &shared.ActivityType{Name: &activityTypeName},
				},
			},
			{
				EventId:   &eventID4,
				EventType: shared.EventTypeActivityTaskFailed.Ptr(),
				Timestamp: &timestamp,
				ActivityTaskFailedEventAttributes: &shared.ActivityTaskFailedEventAttributes{
					Reason:   &reason,
					Details:  []byte("failure details"),
					Identity: &identity,
				},
			},
		},
		currentIndex: 0,
	}

	testCases := []struct {
		name           string
		pageSize       int32
		mockFunc       func(mockClient *cadencemocks.Client)
		expectedEvents int
		errMsg         string
	}{
		{
			name:     "GetWorkflowExecutionHistory Succeeded with no page limit",
			pageSize: 0,
			mockFunc: func(mockClient *cadencemocks.Client) {
				// Reset iterator for each test
				mockIterator.currentIndex = 0
				mockClient.On("GetWorkflowHistory", mock.Anything, workflowID, runID, false, shared.HistoryEventFilterTypeAllEvent).Return(mockIterator)
			},
			expectedEvents: 4,
			errMsg:         "",
		},
		{
			name:     "GetWorkflowExecutionHistory Succeeded with page limit",
			pageSize: 2,
			mockFunc: func(mockClient *cadencemocks.Client) {
				// Reset iterator for each test
				mockIterator.currentIndex = 0
				mockClient.On("GetWorkflowHistory", mock.Anything, workflowID, runID, false, shared.HistoryEventFilterTypeAllEvent).Return(mockIterator)
			},
			expectedEvents: 2,
			errMsg:         "",
		},
		{
			name:     "GetWorkflowExecutionHistory with error during iteration",
			pageSize: 0,
			mockFunc: func(mockClient *cadencemocks.Client) {
				errorIterator := &mockHistoryIterator{
					events:       []*shared.HistoryEvent{},
					currentIndex: 0,
					shouldError:  true,
				}
				mockClient.On("GetWorkflowHistory", mock.Anything, workflowID, runID, false, shared.HistoryEventFilterTypeAllEvent).Return(errorIterator)
			},
			expectedEvents: 0,
			errMsg:         "failed to get next history event: iterator error",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mockClient := &cadencemocks.Client{}
			testCase.mockFunc(mockClient)
			client := &CadenceClient{
				Client: mockClient,
			}

			history, err := client.GetWorkflowExecutionHistory(context.Background(), workflowID, runID, nil, testCase.pageSize)

			if testCase.errMsg != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), testCase.errMsg)
				assert.Nil(t, history)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, history)
				assert.Equal(t, testCase.expectedEvents, len(history.Events))
				assert.Nil(t, history.NextPageToken) // Iterator API doesn't provide page tokens

				// Verify event details are populated correctly
				if len(history.Events) > 0 {
					firstEvent := history.Events[0]
					assert.Equal(t, int64(1), firstEvent.EventID)
					assert.Equal(t, "WorkflowExecutionStarted", firstEvent.EventType)
					assert.Contains(t, firstEvent.Details, "workflow_type")
					assert.Contains(t, firstEvent.Details, "task_list")
				}

				if len(history.Events) > 2 {
					activityEvent := history.Events[2]
					assert.Equal(t, int64(3), activityEvent.EventID)
					assert.Equal(t, "ActivityTaskScheduled", activityEvent.EventType)
					assert.Contains(t, activityEvent.Details, "activity_id")
					assert.Contains(t, activityEvent.Details, "activity_type")
				}
			}
		})
	}
}

func scheduleMocks() (*cadencemocks.Client, *cadencemocks.ScheduleClient) {
	mockClient := &cadencemocks.Client{}
	mockScheduleClient := &cadencemocks.ScheduleClient{}
	mockClient.On("ScheduleClient").Return(mockScheduleClient)
	return mockClient, mockScheduleClient
}

func encodedInput(t *testing.T, args ...interface{}) []byte {
	t.Helper()
	input, err := encoded.GetDefaultDataConverter().ToData(args...)
	require.NoError(t, err)
	return input
}

func TestStartWorkflowCreatesSchedule(t *testing.T) {
	now := time.Now()
	start := now.Add(-3 * time.Hour).UTC()
	end := now.Add(24 * time.Hour).UTC()
	pastEnd := now.Add(-time.Hour).UTC()
	notFound := &shared.EntityNotExistsError{Message: "schedule not found"}
	alreadyExists := &shared.BadRequestError{Message: "schedule already exists"}

	testCases := []struct {
		name       string
		options    clientInterface.StartWorkflowOptions
		mockFunc   func(sc *cadencemocks.ScheduleClient)
		errMsg     string
		catchUpErr bool
	}{
		{
			name: "window, policies, paused state and input are carried onto the schedule",
			options: clientInterface.StartWorkflowOptions{
				ID:                              "ns.trigger",
				TaskList:                        "trigger_run",
				ExecutionStartToCloseTimeout:    24 * time.Hour,
				DecisionTaskStartToCloseTimeout: 30 * time.Second,
				CronSchedule:                    "0 * * * *",
				StartPaused:                     true,
				ScheduleStartAt:                 start,
				ScheduleEndAt:                   end,
				OverlapPolicy:                   clientInterface.OverlapPolicyBuffer,
				CatchUpPolicy:                   clientInterface.CatchUpPolicyAll,
			},
			mockFunc: func(sc *cadencemocks.ScheduleClient) {
				sc.On("Create", mock.Anything, mock.MatchedBy(func(req *cadenceClient.CreateScheduleRequest) bool {
					action := req.Action.StartWorkflow
					return req.ScheduleID == "ns.trigger-schedule" &&
						req.Spec.CronExpression == "0 * * * *" &&
						req.Spec.StartTime.Equal(start) && req.Spec.EndTime.Equal(end) &&
						action.WorkflowType == "trigger.CronTrigger" &&
						action.TaskList == "trigger_run" &&
						action.WorkflowIDPrefix == "ns.trigger" &&
						action.ExecutionStartToCloseTimeout == 24*time.Hour &&
						action.DecisionTaskStartToCloseTimeout == 30*time.Second &&
						string(action.Input) == string(encodedInput(t, "arg1")) &&
						req.Policies.OverlapPolicy == cadenceClient.ScheduleOverlapPolicyBuffer &&
						req.Policies.CatchUpPolicy == cadenceClient.ScheduleCatchUpPolicyAll &&
						req.State != nil && req.State.Paused
				})).Return("ns.trigger-schedule", nil)
				// No Backfill expectation: a paused schedule takes no catch-up.
			},
		},
		{
			name: "defaults are SkipNew, Skip and a 10s decision task timeout",
			options: clientInterface.StartWorkflowOptions{
				ID:                           "ns.trigger",
				TaskList:                     "trigger_run",
				ExecutionStartToCloseTimeout: time.Hour,
				CronSchedule:                 "0 0 * * *",
			},
			mockFunc: func(sc *cadencemocks.ScheduleClient) {
				sc.On("Create", mock.Anything, mock.MatchedBy(func(req *cadenceClient.CreateScheduleRequest) bool {
					return req.Spec.StartTime.IsZero() && req.Spec.EndTime.IsZero() &&
						req.Action.StartWorkflow.DecisionTaskStartToCloseTimeout == 10*time.Second &&
						req.Policies.OverlapPolicy == cadenceClient.ScheduleOverlapPolicySkipNew &&
						req.Policies.CatchUpPolicy == cadenceClient.ScheduleCatchUpPolicySkip &&
						req.State == nil
				})).Return("ns.trigger-schedule", nil)
			},
		},
		{
			name: "existing schedule is adopted when create fails",
			options: clientInterface.StartWorkflowOptions{
				ID: "ns.trigger", TaskList: "trigger_run", CronSchedule: "0 0 * * *",
			},
			mockFunc: func(sc *cadencemocks.ScheduleClient) {
				sc.On("Create", mock.Anything, mock.Anything).Return("", alreadyExists)
				sc.On("Describe", mock.Anything, "ns.trigger-schedule").Return(&cadenceClient.DescribeScheduleResponse{}, nil)
			},
		},
		{
			name: "existing schedule is paused when the start asks for it",
			options: clientInterface.StartWorkflowOptions{
				ID: "ns.trigger", TaskList: "trigger_run", CronSchedule: "0 0 * * *", StartPaused: true,
			},
			mockFunc: func(sc *cadencemocks.ScheduleClient) {
				sc.On("Create", mock.Anything, mock.Anything).Return("", alreadyExists)
				sc.On("Describe", mock.Anything, "ns.trigger-schedule").Return(&cadenceClient.DescribeScheduleResponse{}, nil)
				sc.On("Pause", mock.Anything, "ns.trigger-schedule", "paused by michelangelo").Return(nil)
			},
		},
		{
			name: "create failure with no schedule behind it is an error",
			options: clientInterface.StartWorkflowOptions{
				ID: "ns.trigger", TaskList: "trigger_run", CronSchedule: "0 0 * * *",
			},
			mockFunc: func(sc *cadencemocks.ScheduleClient) {
				sc.On("Create", mock.Anything, mock.Anything).Return("", fmt.Errorf("frontend unavailable"))
				sc.On("Describe", mock.Anything, "ns.trigger-schedule").Return(nil, notFound)
			},
			errMsg: "failed to create Cadence schedule ns.trigger-schedule: frontend unavailable",
		},
		{
			name: "catch-up backfills the missed window with the Buffer overlap policy",
			options: clientInterface.StartWorkflowOptions{
				ID: "ns.trigger", TaskList: "trigger_run", CronSchedule: "0 * * * *",
				ScheduleStartAt: start, CatchUpFrom: start,
			},
			mockFunc: func(sc *cadencemocks.ScheduleClient) {
				sc.On("Create", mock.Anything, mock.Anything).Return("ns.trigger-schedule", nil)
				sc.On("Backfill", mock.Anything, "ns.trigger-schedule", mock.MatchedBy(func(req *cadenceClient.BackfillRequest) bool {
					return req.StartTime.Equal(start) && req.EndTime.After(now.Add(-time.Second)) &&
						req.OverlapPolicy == cadenceClient.ScheduleOverlapPolicyBuffer
				})).Return(nil)
			},
		},
		{
			name: "catch-up stops at a schedule end that already passed",
			options: clientInterface.StartWorkflowOptions{
				ID: "ns.trigger", TaskList: "trigger_run", CronSchedule: "0 * * * *",
				ScheduleStartAt: start, ScheduleEndAt: pastEnd, CatchUpFrom: start,
			},
			mockFunc: func(sc *cadencemocks.ScheduleClient) {
				sc.On("Create", mock.Anything, mock.Anything).Return("ns.trigger-schedule", nil)
				sc.On("Backfill", mock.Anything, "ns.trigger-schedule", mock.MatchedBy(func(req *cadenceClient.BackfillRequest) bool {
					return req.StartTime.Equal(start) && req.EndTime.Equal(pastEnd)
				})).Return(nil)
			},
		},
		{
			name: "catch-up failure leaves the schedule live and reports CatchUpError",
			options: clientInterface.StartWorkflowOptions{
				ID: "ns.trigger", TaskList: "trigger_run", CronSchedule: "0 * * * *",
				ScheduleStartAt: start, CatchUpFrom: start,
			},
			mockFunc: func(sc *cadencemocks.ScheduleClient) {
				sc.On("Create", mock.Anything, mock.Anything).Return("ns.trigger-schedule", nil)
				sc.On("Backfill", mock.Anything, "ns.trigger-schedule", mock.Anything).Return(fmt.Errorf("backfill rejected"))
			},
			errMsg:     "backfill rejected",
			catchUpErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mockClient, mockScheduleClient := scheduleMocks()
			testCase.mockFunc(mockScheduleClient)
			client := &CadenceClient{Client: mockClient}

			exec, err := client.StartWorkflow(context.Background(), testCase.options, "trigger.CronTrigger", "arg1")

			if testCase.catchUpErr {
				var catchUpErr *clientInterface.CatchUpError
				require.ErrorAs(t, err, &catchUpErr)
				assert.Equal(t, "ns.trigger-schedule", catchUpErr.ScheduleID)
				assert.True(t, catchUpErr.CatchUpFrom.Equal(start))
				require.NotNil(t, exec, "a live schedule must be returned alongside CatchUpError")
				assert.Equal(t, "ns.trigger-schedule", exec.ID)
			} else if testCase.errMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), testCase.errMsg)
				assert.Nil(t, exec)
			} else {
				require.NoError(t, err)
				require.NotNil(t, exec)
				assert.Equal(t, "ns.trigger-schedule", exec.ID)
				assert.Empty(t, exec.RunID, "schedules have no run ID")
			}
			mockScheduleClient.AssertExpectations(t)
		})
	}
}

func TestStartWorkflowWithoutCronDoesNotTouchSchedules(t *testing.T) {
	mockClient := &cadencemocks.Client{}
	mockClient.On("StartWorkflow", mock.Anything, mock.MatchedBy(func(opts cadenceClient.StartWorkflowOptions) bool {
		return opts.ID == "ns.backfill" && opts.CronSchedule == ""
	}), "trigger.BackfillTrigger", mock.Anything).Return(&cadenceworkflow.Execution{ID: "ns.backfill", RunID: "run-1"}, nil)
	client := &CadenceClient{Client: mockClient}

	exec, err := client.StartWorkflow(context.Background(), clientInterface.StartWorkflowOptions{
		ID: "ns.backfill", TaskList: "trigger_run",
	}, "trigger.BackfillTrigger", "arg1")

	require.NoError(t, err)
	assert.Equal(t, "run-1", exec.RunID)
	mockClient.AssertNotCalled(t, "ScheduleClient")
}

func TestStartWorkflowWithoutCronWhenAlreadyStarted(t *testing.T) {
	runID := "run-0"
	testCases := []struct {
		name     string
		startErr error
		wantRun  string
		wantErr  bool
	}{
		{
			name:     "returns the open execution like the Temporal client",
			startErr: &shared.WorkflowExecutionAlreadyStartedError{RunId: &runID},
			wantRun:  "run-0",
		},
		{
			name:     "already-started without a run ID is still an error",
			startErr: &shared.WorkflowExecutionAlreadyStartedError{},
			wantErr:  true,
		},
		{
			name:     "other errors propagate",
			startErr: &shared.BadRequestError{Message: "bad"},
			wantErr:  true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := &cadencemocks.Client{}
			mockClient.On("StartWorkflow", mock.Anything, mock.Anything, "trigger.BackfillTrigger", mock.Anything).
				Return(nil, tc.startErr)
			client := &CadenceClient{Client: mockClient}

			exec, err := client.StartWorkflow(context.Background(), clientInterface.StartWorkflowOptions{
				ID: "ns.backfill", TaskList: "trigger_run",
			}, "trigger.BackfillTrigger", "arg1")

			if tc.wantErr {
				require.Error(t, err)
				assert.Nil(t, exec)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "ns.backfill", exec.ID)
			assert.Equal(t, tc.wantRun, exec.RunID)
		})
	}
}

func TestPauseTrigger(t *testing.T) {
	testCases := []struct {
		name     string
		pauseErr error
		errMsg   string
	}{
		{name: "pauses the schedule"},
		{
			name:     "missing schedule explains the legacy cron workflow",
			pauseErr: &shared.EntityNotExistsError{Message: "schedule not found"},
			errMsg:   "must be recreated",
		},
		{
			name:     "other errors are passed through",
			pauseErr: fmt.Errorf("frontend unavailable"),
			errMsg:   "pause trigger ns.trigger: frontend unavailable",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mockClient, mockScheduleClient := scheduleMocks()
			mockScheduleClient.On("Pause", mock.Anything, "ns.trigger-schedule", "paused by michelangelo").Return(testCase.pauseErr)
			client := &CadenceClient{Client: mockClient}

			err := client.PauseTrigger(context.Background(), "ns.trigger")

			if testCase.errMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), testCase.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUnpauseTrigger(t *testing.T) {
	testCases := []struct {
		name       string
		unpauseErr error
		errMsg     string
	}{
		{name: "unpauses the schedule with its configured catch-up policy"},
		{
			name:       "missing schedule explains the legacy cron workflow",
			unpauseErr: &shared.EntityNotExistsError{Message: "schedule not found"},
			errMsg:     "must be recreated",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mockClient, mockScheduleClient := scheduleMocks()
			mockScheduleClient.On("Unpause", mock.Anything, "ns.trigger-schedule", "unpaused by michelangelo",
				cadenceClient.ScheduleCatchUpPolicyUnspecified).Return(testCase.unpauseErr)
			client := &CadenceClient{Client: mockClient}

			err := client.UnpauseTrigger(context.Background(), "ns.trigger")

			if testCase.errMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), testCase.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDeleteTrigger(t *testing.T) {
	workflowID := "ns.trigger"
	scheduleID := "ns.trigger-schedule"
	runID := "legacy-run"

	testCases := []struct {
		name     string
		runID    string
		mockFunc func(mockClient *cadencemocks.Client, sc *cadencemocks.ScheduleClient)
		errMsg   string
	}{
		{
			name:  "deletes the schedule; nothing to terminate",
			runID: "",
			mockFunc: func(_ *cadencemocks.Client, sc *cadencemocks.ScheduleClient) {
				sc.On("Delete", mock.Anything, scheduleID).Return(nil)
			},
		},
		{
			name:  "deletes the schedule and terminates a legacy cron run",
			runID: runID,
			mockFunc: func(mockClient *cadencemocks.Client, sc *cadencemocks.ScheduleClient) {
				sc.On("Delete", mock.Anything, scheduleID).Return(nil)
				mockClient.On("TerminateWorkflow", mock.Anything, workflowID, runID, "trigger killed", mock.Anything).Return(nil)
			},
		},
		{
			name:  "a legacy trigger has no schedule; its run is still terminated",
			runID: runID,
			mockFunc: func(mockClient *cadencemocks.Client, sc *cadencemocks.ScheduleClient) {
				sc.On("Delete", mock.Anything, scheduleID).Return(&shared.EntityNotExistsError{Message: "schedule not found"})
				mockClient.On("TerminateWorkflow", mock.Anything, workflowID, runID, "trigger killed", mock.Anything).Return(nil)
			},
		},
		{
			name:  "an already-completed run is not an error",
			runID: runID,
			mockFunc: func(mockClient *cadencemocks.Client, sc *cadencemocks.ScheduleClient) {
				sc.On("Delete", mock.Anything, scheduleID).Return(nil)
				mockClient.On("TerminateWorkflow", mock.Anything, workflowID, runID, "trigger killed", mock.Anything).
					Return(&shared.WorkflowExecutionAlreadyCompletedError{Message: "already completed"})
			},
		},
		{
			name:  "schedule delete failure is surfaced",
			runID: runID,
			mockFunc: func(_ *cadencemocks.Client, sc *cadencemocks.ScheduleClient) {
				sc.On("Delete", mock.Anything, scheduleID).Return(fmt.Errorf("frontend unavailable"))
			},
			errMsg: "delete Cadence schedule ns.trigger-schedule: frontend unavailable",
		},
		{
			name:  "terminate failure is surfaced",
			runID: runID,
			mockFunc: func(mockClient *cadencemocks.Client, sc *cadencemocks.ScheduleClient) {
				sc.On("Delete", mock.Anything, scheduleID).Return(nil)
				mockClient.On("TerminateWorkflow", mock.Anything, workflowID, runID, "trigger killed", mock.Anything).Return(fmt.Errorf("terminate error"))
			},
			errMsg: "terminate error",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mockClient, mockScheduleClient := scheduleMocks()
			testCase.mockFunc(mockClient, mockScheduleClient)
			client := &CadenceClient{Client: mockClient}

			err := client.DeleteTrigger(context.Background(), workflowID, testCase.runID)

			if testCase.errMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), testCase.errMsg)
			} else {
				assert.NoError(t, err)
			}
			mockClient.AssertExpectations(t)
			mockScheduleClient.AssertExpectations(t)
		})
	}
}

func TestUpdateTrigger(t *testing.T) {
	scheduleID := "ns.trigger-schedule"
	newCron := "0 6 * * *"
	newArgs := []interface{}{"new-input"}
	pausedTrue, pausedFalse := true, false

	// runMutate applies the mutate callback UpdateTrigger passed to a describe-shaped
	// ScheduleUpdate and returns the result, so the test can see what would be sent.
	runMutate := func(t *testing.T, args mock.Arguments, withAction bool) (*cadenceClient.ScheduleUpdate, error) {
		t.Helper()
		mutate, ok := args.Get(2).(func(*cadenceClient.ScheduleUpdate) error)
		require.True(t, ok, "mutate callback has the ScheduleClient.Update signature")
		update := &cadenceClient.ScheduleUpdate{Spec: &cadenceClient.ScheduleSpec{CronExpression: "0 0 * * *"}}
		if withAction {
			update.Action = &cadenceClient.ScheduleActionDescription{
				StartWorkflow: &cadenceClient.ScheduleStartWorkflowActionDescription{Input: []byte("old-input")},
			}
		}
		return update, mutate(update)
	}

	t.Run("cron and args are changed in one schedule update", func(t *testing.T) {
		mockClient, sc := scheduleMocks()
		var applied *cadenceClient.ScheduleUpdate
		sc.On("Update", mock.Anything, scheduleID, mock.Anything).Run(func(args mock.Arguments) {
			update, err := runMutate(t, args, true)
			require.NoError(t, err)
			applied = update
		}).Return(nil)
		client := &CadenceClient{Client: mockClient}

		require.NoError(t, client.UpdateTrigger(context.Background(), "ns.trigger", newCron, nil, newArgs))

		require.NotNil(t, applied)
		assert.Equal(t, newCron, applied.Spec.CronExpression)
		assert.Equal(t, encodedInput(t, "new-input"), applied.Action.StartWorkflow.Input)
		sc.AssertNotCalled(t, "Pause", mock.Anything, mock.Anything, mock.Anything)
		sc.AssertNotCalled(t, "Unpause", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("args only leaves the cron untouched", func(t *testing.T) {
		mockClient, sc := scheduleMocks()
		sc.On("Update", mock.Anything, scheduleID, mock.Anything).Run(func(args mock.Arguments) {
			update, err := runMutate(t, args, true)
			require.NoError(t, err)
			assert.Equal(t, "0 0 * * *", update.Spec.CronExpression)
			assert.Equal(t, encodedInput(t, "new-input"), update.Action.StartWorkflow.Input)
		}).Return(nil)
		client := &CadenceClient{Client: mockClient}

		require.NoError(t, client.UpdateTrigger(context.Background(), "ns.trigger", "", nil, newArgs))
		sc.AssertExpectations(t)
	})

	t.Run("pause happens before the spec change", func(t *testing.T) {
		mockClient, sc := scheduleMocks()
		var order []string
		sc.On("Pause", mock.Anything, scheduleID, "paused by michelangelo").Run(func(mock.Arguments) {
			order = append(order, "pause")
		}).Return(nil)
		sc.On("Update", mock.Anything, scheduleID, mock.Anything).Run(func(args mock.Arguments) {
			_, err := runMutate(t, args, true)
			require.NoError(t, err)
			order = append(order, "update")
		}).Return(nil)
		client := &CadenceClient{Client: mockClient}

		require.NoError(t, client.UpdateTrigger(context.Background(), "ns.trigger", newCron, &pausedTrue, nil))
		assert.Equal(t, []string{"pause", "update"}, order)
	})

	t.Run("unpause happens after the spec change", func(t *testing.T) {
		mockClient, sc := scheduleMocks()
		var order []string
		sc.On("Update", mock.Anything, scheduleID, mock.Anything).Run(func(args mock.Arguments) {
			_, err := runMutate(t, args, true)
			require.NoError(t, err)
			order = append(order, "update")
		}).Return(nil)
		sc.On("Unpause", mock.Anything, scheduleID, "unpaused by michelangelo", cadenceClient.ScheduleCatchUpPolicyUnspecified).
			Run(func(mock.Arguments) { order = append(order, "unpause") }).Return(nil)
		client := &CadenceClient{Client: mockClient}

		require.NoError(t, client.UpdateTrigger(context.Background(), "ns.trigger", "", &pausedFalse, newArgs))
		assert.Equal(t, []string{"update", "unpause"}, order)
	})

	t.Run("paused state alone needs no schedule update", func(t *testing.T) {
		mockClient, sc := scheduleMocks()
		sc.On("Pause", mock.Anything, scheduleID, "paused by michelangelo").Return(nil)
		client := &CadenceClient{Client: mockClient}

		require.NoError(t, client.UpdateTrigger(context.Background(), "ns.trigger", "", &pausedTrue, nil))
		sc.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("nothing to change is a no-op", func(t *testing.T) {
		mockClient, sc := scheduleMocks()
		client := &CadenceClient{Client: mockClient}

		require.NoError(t, client.UpdateTrigger(context.Background(), "ns.trigger", "", nil, nil))
		sc.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("a schedule without a start-workflow action cannot take new args", func(t *testing.T) {
		mockClient, sc := scheduleMocks()
		sc.On("Update", mock.Anything, scheduleID, mock.Anything).Return(func(_ context.Context, _ string, mutate func(*cadenceClient.ScheduleUpdate) error) error {
			return mutate(&cadenceClient.ScheduleUpdate{Spec: &cadenceClient.ScheduleSpec{CronExpression: "0 0 * * *"}})
		})
		client := &CadenceClient{Client: mockClient}

		err := client.UpdateTrigger(context.Background(), "ns.trigger", "", nil, newArgs)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no start-workflow action")
	})

	t.Run("missing schedule explains the legacy cron workflow", func(t *testing.T) {
		mockClient, sc := scheduleMocks()
		sc.On("Update", mock.Anything, scheduleID, mock.Anything).Return(&shared.EntityNotExistsError{Message: "schedule not found"})
		client := &CadenceClient{Client: mockClient}

		err := client.UpdateTrigger(context.Background(), "ns.trigger", newCron, nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "update trigger ns.trigger")
		assert.Contains(t, err.Error(), "must be recreated")
	})

	t.Run("pause failure stops before the spec change", func(t *testing.T) {
		mockClient, sc := scheduleMocks()
		sc.On("Pause", mock.Anything, scheduleID, "paused by michelangelo").Return(fmt.Errorf("frontend unavailable"))
		client := &CadenceClient{Client: mockClient}

		err := client.UpdateTrigger(context.Background(), "ns.trigger", newCron, &pausedTrue, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "pause trigger ns.trigger: frontend unavailable")
		sc.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything)
	})
}

func TestCatchUpEnd(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	assert.Equal(t, now, catchUpEnd(time.Time{}, now), "no schedule end bounds the replay at now")
	assert.Equal(t, now, catchUpEnd(now.Add(time.Hour), now), "a future schedule end does not shorten the replay")
	assert.Equal(t, now.Add(-time.Hour), catchUpEnd(now.Add(-time.Hour), now), "a past schedule end bounds the replay")
}

func TestMapPolicies(t *testing.T) {
	overlap := map[clientInterface.OverlapPolicy]cadenceClient.ScheduleOverlapPolicy{
		clientInterface.OverlapPolicyUnspecified:       cadenceClient.ScheduleOverlapPolicySkipNew,
		clientInterface.OverlapPolicySkipNew:           cadenceClient.ScheduleOverlapPolicySkipNew,
		clientInterface.OverlapPolicyBuffer:            cadenceClient.ScheduleOverlapPolicyBuffer,
		clientInterface.OverlapPolicyConcurrent:        cadenceClient.ScheduleOverlapPolicyConcurrent,
		clientInterface.OverlapPolicyCancelPrevious:    cadenceClient.ScheduleOverlapPolicyCancelPrevious,
		clientInterface.OverlapPolicyTerminatePrevious: cadenceClient.ScheduleOverlapPolicyTerminatePrevious,
	}
	for in, want := range overlap {
		assert.Equal(t, want, mapOverlapPolicy(in), "overlap policy %d", in)
	}
	catchUp := map[clientInterface.CatchUpPolicy]cadenceClient.ScheduleCatchUpPolicy{
		clientInterface.CatchUpPolicyUnspecified: cadenceClient.ScheduleCatchUpPolicySkip,
		clientInterface.CatchUpPolicySkip:        cadenceClient.ScheduleCatchUpPolicySkip,
		clientInterface.CatchUpPolicyOne:         cadenceClient.ScheduleCatchUpPolicyOne,
		clientInterface.CatchUpPolicyAll:         cadenceClient.ScheduleCatchUpPolicyAll,
	}
	for in, want := range catchUp {
		assert.Equal(t, want, mapCatchUpPolicy(in), "catch-up policy %d", in)
	}
}

// Mock implementation of cadence history iterator
type mockHistoryIterator struct {
	events       []*shared.HistoryEvent
	currentIndex int
	shouldError  bool
}

func (m *mockHistoryIterator) HasNext() bool {
	if m.shouldError && m.currentIndex == 0 {
		return true // Return true to trigger the Next() call that will error
	}
	return m.currentIndex < len(m.events)
}

func (m *mockHistoryIterator) Next() (*shared.HistoryEvent, error) {
	if m.shouldError {
		return nil, fmt.Errorf("iterator error")
	}

	if m.currentIndex >= len(m.events) {
		return nil, fmt.Errorf("no more events")
	}

	event := m.events[m.currentIndex]
	m.currentIndex++
	return event, nil
}

func ptrString(s string) *string {
	return &s
}
