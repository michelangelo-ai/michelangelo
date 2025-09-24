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
