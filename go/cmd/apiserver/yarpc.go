package main

import (
	"context"
	"fmt"
	"net"

	"go.uber.org/fx"
	"go.uber.org/yarpc"
	"go.uber.org/yarpc/api/transport"
	"go.uber.org/yarpc/encoding/protobuf/reflection"
	"go.uber.org/yarpc/transport/grpc"
	yarpcreflection "go.uber.org/yarpc/x/reflection"
	"go.uber.org/zap"
)

// RegisterParams defines the parameters for procedure registration.
type RegisterParams struct {
	fx.In

	Dispatcher *yarpc.Dispatcher

	// We accept both, single transport.Procedures and collections of them
	// provided to the "yarpcfx" group.
	SingleProcedures []transport.Procedure   `group:"yarpcfx"`
	ProcedureLists   [][]transport.Procedure `group:"yarpcfx"`
	// ProtoReflectionMetas are the server metadata for gRPC reflection.
	ProtoReflectionMetas []reflection.ServerMeta `group:"yarpcfx"`
}

// provideDispatcher creates and configures a YARPC dispatcher.
func provideDispatcher(conf YARPCConfig, zapLogger *zap.Logger) (*yarpc.Dispatcher, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", conf.Host, conf.Port))
	if err != nil {
		return nil, err
	}
	grpcTransport := grpc.NewTransport()
	inbound := grpcTransport.NewInbound(
		listener,
	)

	dispatcher := yarpc.NewDispatcher(yarpc.Config{
		Name: serverName,
		Inbounds: yarpc.Inbounds{
			inbound,
		},
		Logging: yarpc.LoggingConfig{
			Zap: zapLogger,
		},
	})
	return dispatcher, nil
}

// registerProcedures registers procedures with a dispatcher.
func registerProcedures(p RegisterParams) {
	procs := make([]transport.Procedure, 0, len(p.SingleProcedures)+len(p.ProcedureLists))
	procs = append(procs, p.SingleProcedures...)
	for _, ps := range p.ProcedureLists {
		procs = append(procs, ps...)
	}
	refl, reflErr := yarpcreflection.NewServer(p.ProtoReflectionMetas)
	if reflErr == nil {
		procs = append(procs, refl...)
	}

	p.Dispatcher.Register(procs)
}

// startYARPCServer starts the YARPC dispatcher.
func startYARPCServer(lc fx.Lifecycle, dispatcher *yarpc.Dispatcher) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return dispatcher.Start()
		},
		OnStop: func(ctx context.Context) error {
			return dispatcher.Stop()
		},
	})
}
