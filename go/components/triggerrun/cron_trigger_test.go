package triggerrun

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	workflowclientInterfacemocks "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface/interface_mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCronTrigger(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := workflowclientInterfacemocks.NewMockWorkflowClient(ctrl)
	logger := logr.Discard()

	runner := NewCronTrigger(logger, mockClient)

	assert.NotNil(t, runner)
	cronTrigger, ok := runner.(*cronTrigger)
	require.True(t, ok)
	assert.Equal(t, logger, cronTrigger.Log)
	assert.Equal(t, mockClient, cronTrigger.WorkflowClient)
}
