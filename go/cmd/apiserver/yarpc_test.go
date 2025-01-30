package main

import (
	"testing"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx/fxtest"
	"go.uber.org/yarpc/api/transport"
	"go.uber.org/zap"
)

func TestProvideDispatcher(t *testing.T) {
	conf := YARPCConfig{
		Host: "localhost",
		Port: 12345,
	}
	dispatcher, err := provideDispatcher(conf, zap.NewNop())
	assert.NoError(t, err)
	assert.NotNil(t, dispatcher)
}

func TestRegisterProcedures(t *testing.T) {
	dispatcher, err := provideDispatcher(YARPCConfig{
		Host: "localhost",
		Port: 12345,
	}, zap.NewNop())
	assert.NoError(t, err)
	assert.NotNil(t, dispatcher)
	params := RegisterParams{
		Dispatcher: dispatcher,
		ProcedureLists: [][]transport.Procedure{
			v2pb.BuildProjectServiceYARPCProcedures(nil),
			v2pb.BuildRayClusterServiceYARPCProcedures(nil),
		},
	}
	registerProcedures(params)
	r := dispatcher.Router()
	procedures := r.Procedures()
	assert.Len(t, procedures, 24)
}

func TestStartYARPCServer(t *testing.T) {
	dispatcher, err := provideDispatcher(YARPCConfig{
		Host: "localhost",
		Port: 0,
	}, zap.NewNop())
	assert.NoError(t, err)
	assert.NotNil(t, dispatcher)
	lc := fxtest.NewLifecycle(t)
	startYARPCServer(lc, dispatcher)
	lc.RequireStart()
	lc.RequireStop()
}
