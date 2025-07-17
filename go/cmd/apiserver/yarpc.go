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
	"go.uber.org/yarpc/transport/http"
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

// provideDispatcher creates and configures a YARPC dispatcher with gRPC and/or HTTP support.
func provideDispatcher(conf YARPCConfig, zapLogger *zap.Logger) (*yarpc.Dispatcher, error) {
	var inbounds []transport.Inbound
	
	// Default transport is gRPC if not specified
	transport := conf.Transport
	if transport == "" {
		transport = "grpc"
	}
	
	// Always add gRPC inbound (for backward compatibility)
	if transport == "grpc" || transport == "both" {
		grpcListener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", conf.Host, conf.Port))
		if err != nil {
			return nil, fmt.Errorf("failed to create gRPC listener: %w", err)
		}
		grpcTransport := grpc.NewTransport()
		grpcInbound := grpcTransport.NewInbound(grpcListener)
		inbounds = append(inbounds, grpcInbound)
		zapLogger.Info("gRPC server listening", zap.String("address", fmt.Sprintf("%s:%d", conf.Host, conf.Port)))
	}
	
	// Add HTTP inbound if requested
	if transport == "http" || transport == "both" {
		httpPort := conf.HTTPPort
		if httpPort == 0 {
			if transport == "http" {
				// If only HTTP is requested and no specific port, use the main port
				httpPort = conf.Port
			} else {
				// If both transports, default HTTP to main port + 1
				httpPort = conf.Port + 1
			}
		}
		
		httpTransport := http.NewTransport()
		httpInbound := httpTransport.NewInbound(fmt.Sprintf("%s:%d", conf.Host, httpPort))
		inbounds = append(inbounds, httpInbound)
		zapLogger.Info("HTTP server listening", zap.String("address", fmt.Sprintf("%s:%d", conf.Host, httpPort)))
	}
	
	if len(inbounds) == 0 {
		return nil, fmt.Errorf("no valid transport configured (transport: %s)", transport)
	}

	dispatcher := yarpc.NewDispatcher(yarpc.Config{
		Name:     serverName,
		Inbounds: inbounds,
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
