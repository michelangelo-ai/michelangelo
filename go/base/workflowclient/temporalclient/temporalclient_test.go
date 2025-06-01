package temporalclient

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

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
