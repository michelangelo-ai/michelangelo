package cadenceclient

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/cadence/.gen/go/shared"
	cadenceClient "go.uber.org/cadence/client"
	"go.uber.org/cadence/encoded"
	cadencemocks "go.uber.org/cadence/mocks"
	cadenceworkflow "go.uber.org/cadence/workflow"
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
		name           string
		mockFunc       func(mockClient *cadencemocks.Client)
		expectedStatus clientInterface.WorkflowExecutionStatus
		errMsg         string
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
			expectedStatus: clientInterface.WorkflowExecutionStatusCompleted,
			errMsg:         "",
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
			},
			expectedStatus: clientInterface.WorkflowExecutionStatusFailed,
			errMsg:         "",
		},
		{
			name: "GetWorkflowExecutionInfo Succeeded -- workflow running",
			mockFunc: func(mockClient *cadencemocks.Client) {
				mockClient.On("DescribeWorkflowExecution", mock.Anything, mock.Anything, mock.Anything).Return(
					&shared.DescribeWorkflowExecutionResponse{
						WorkflowExecutionInfo: &shared.WorkflowExecutionInfo{
							CloseStatus: nil,
						},
					}, nil)
			},
			expectedStatus: clientInterface.WorkflowExecutionStatusRunning,
			errMsg:         "",
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

func TestDeleteTrigger(t *testing.T) {
	workflowID := "testWorkflowID"

	testCases := []struct {
		name     string
		mockFunc func(mockClient *cadencemocks.Client)
		errMsg   string
	}{
		{
			name: "DeleteTrigger Succeeded",
			mockFunc: func(mockClient *cadencemocks.Client) {
				mockClient.On("TerminateWorkflow", mock.Anything, workflowID, "", "trigger killed", mock.Anything).Return(nil)
			},
			errMsg: "",
		},
		{
			name: "DeleteTrigger Failed",
			mockFunc: func(mockClient *cadencemocks.Client) {
				mockClient.On("TerminateWorkflow", mock.Anything, workflowID, "", "trigger killed", mock.Anything).Return(fmt.Errorf("terminate error"))
			},
			errMsg: "terminate error",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mockClient := &cadencemocks.Client{}
			testCase.mockFunc(mockClient)
			client := &CadenceClient{
				Client: mockClient,
			}
			err := client.DeleteTrigger(context.Background(), workflowID)
			if testCase.errMsg != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), testCase.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPauseTrigger(t *testing.T) {
	workflowID := "testWorkflowID"
	client := &CadenceClient{
		Client: &cadencemocks.Client{},
	}
	err := client.PauseTrigger(context.Background(), workflowID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

func TestUnpauseTrigger(t *testing.T) {
	workflowID := "testWorkflowID"
	client := &CadenceClient{
		Client: &cadencemocks.Client{},
	}
	err := client.UnpauseTrigger(context.Background(), workflowID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
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
