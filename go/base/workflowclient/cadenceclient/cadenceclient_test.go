package cadenceclient

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

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

func TestTerminateWorkflow(t *testing.T) {
	ctx := context.Background()
	workflowID := "test-workflow-id"
	runID := "test-run-id"
	reason := "test termination reason"

	t.Run("success", func(t *testing.T) {
		mockClient := &cadencemocks.Client{}
		client := &CadenceClient{
			Client:   mockClient,
			Provider: "cadence",
			Domain:   "test-domain",
		}

		mockClient.On("TerminateWorkflow", ctx, workflowID, runID, reason, []byte(nil)).Return(nil)

		err := client.TerminateWorkflow(ctx, workflowID, runID, reason)

		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("error", func(t *testing.T) {
		mockClient := &cadencemocks.Client{}
		client := &CadenceClient{
			Client:   mockClient,
			Provider: "cadence",
			Domain:   "test-domain",
		}

		expectedErr := fmt.Errorf("terminate failed")
		mockClient.On("TerminateWorkflow", ctx, workflowID, runID, reason, []byte(nil)).Return(expectedErr)

		err := client.TerminateWorkflow(ctx, workflowID, runID, reason)

		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
		mockClient.AssertExpectations(t)
	})
}
