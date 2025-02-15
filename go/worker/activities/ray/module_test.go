package ray

import (
	"context"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/cadence/worker"
	"mock/github.com/michelangelo-ai/michelangelo/proto/api/v2/v2mock"
	"testing"
)

func Test_Module(t *testing.T) {
	ctrl := gomock.NewController(t)
	mock := worker.New
	workers := make([]worker.Worker, 0)
	register(workers, v2mock.NewMockRayJobServiceYARPCClient(ctrl), v2mock.NewMockRayClusterServiceYARPCClient(ctrl))
	assert.Equal(t, len(workers[0].RegisterActivity), 1)
}
