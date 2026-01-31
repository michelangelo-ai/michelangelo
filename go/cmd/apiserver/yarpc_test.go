package main

import (
	"testing"

	"go.uber.org/yarpc/encoding/protobuf/reflection"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx/fxtest"
	"go.uber.org/yarpc/api/transport"
	"go.uber.org/zap"
)

func TestProvideDispatcher(t *testing.T) {
	conf := YARPCConfig{
		Host: "localhost",
		Port: 0,
	}
	dispatcher, err := provideDispatcher(conf, zap.NewNop())
	assert.NoError(t, err)
	assert.NotNil(t, dispatcher)

	conf = YARPCConfig{
		Host: "fake-host",
		Port: 0,
	}
	dispatcher, err = provideDispatcher(conf, zap.NewNop())
	assert.Error(t, err)
}

func TestRegisterProcedures(t *testing.T) {
	dispatcher, err := provideDispatcher(YARPCConfig{
		Host: "localhost",
		Port: 0,
	}, zap.NewNop())
	assert.NoError(t, err)
	assert.NotNil(t, dispatcher)
	params := RegisterParams{
		Dispatcher: dispatcher,
		ProcedureLists: [][]transport.Procedure{
			v2pb.BuildProjectServiceYARPCProcedures(nil),
			v2pb.BuildRayClusterServiceYARPCProcedures(nil),
		},
		ProtoReflectionMetas: []reflection.ServerMeta{
			v2pb.ProjectServiceReflectionMeta,
			v2pb.RayClusterServiceReflectionMeta,
		},
	}
	registerProcedures(params)
	r := dispatcher.Router()
	procedures := r.Procedures()
	assert.Len(t, procedures, 26)
	proceduresMap := make(map[string]transport.Procedure, len(procedures))
	for _, p := range procedures {
		proceduresMap[p.Name] = p
	}
	assert.NotNil(t, proceduresMap["ProjectService.GetProject"])
	assert.NotNil(t, proceduresMap["RayClusterService.GetRayCluster"])
	assert.NotNil(t, proceduresMap["grpc.reflection.v1alpha.ServerReflection::ServerReflectionInfo"])
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
