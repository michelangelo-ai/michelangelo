package main

import (
	"context"
	"fmt"
	"github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/auth"
	"github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/logging"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/uber-go/tally"
	uber_config "go.uber.org/config"
	"go.uber.org/fx"
	"go.uber.org/yarpc"
	"go.uber.org/yarpc/transport/http"
)

// Module provides the YARPC dispatcher and server.
var Module = fx.Options(
	handler.APIServerModule,
	config.Module,
	fx.Provide(newScope),
	fx.Provide(newAuth),
	fx.Provide(v2pb.GetK8sClient),
	v2pb.RayJobSvcModule,
	v2pb.RayClusterSvcModule,
	v2pb.ProjectSvcModule,
	v2pb.SparkJobSvcModule,
	fx.Provide(newConfig),
	fx.Provide(provideDispatcher),
	fx.Invoke(startYARPCServer),
)

const configKey = "apiserver"

type (
	// Config defines the server's configuration.
	Config struct {
		Port int    `yaml:"port"`
		Host string `yaml:"host"`
	}

	In struct {
		fx.In

		Metrics         tally.Scope
		Config Config
		RayCluster v2pb.RayClusterServiceYARPCServer
		RayJob v2pb.RayJobServiceYARPCServer
		Project v2pb.ProjectServiceYARPCServer
		SparkJob v2pb.SparkJobServiceYARPCServer
	}

	Out struct {
		fx.Out
		Dispatcher *yarpc.Dispatcher
	}
)

// provideDispatcher creates and configures a YARPC dispatcher.
func provideDispatcher(in In) (Out, error) {
	dispatcher := yarpc.NewDispatcher(yarpc.Config{
		Name: "michelangelo-apiserver",
		Outbounds: yarpc.Outbounds{
			"michelangelo-apiserver": {
				Unary: http.NewTransport().NewSingleOutbound(fmt.Sprintf("http://%s:%d", in.Config.Host, in.Config.Port)),
			},
		},
		Inbounds: yarpc.Inbounds{
			http.NewTransport().NewInbound(fmt.Sprintf(":%d", in.Config.Port)),
		},
	})
	dispatcher.Register(v2pb.BuildRayClusterServiceYARPCProcedures(in.RayCluster))
	dispatcher.Register(v2pb.BuildRayJobServiceYARPCProcedures(in.RayJob))
	dispatcher.Register(v2pb.BuildProjectServiceYARPCProcedures(in.Project))
	dispatcher.Register(v2pb.BuildSparkJobServiceYARPCProcedures(in.SparkJob))
	return Out{Dispatcher: dispatcher}, nil
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

func main() {
	fx.New(
		fx.Options(
			env.Module,
			Module,
		),
	).Run()
}


func newConfig(provider uber_config.Provider) (Config, error) {
	conf := Config{}
	err := provider.Get(configKey).Populate(&conf)
	return conf, err
}

func newAuth() (auth.Auth, error) {
	return auth.DummyAuth{}, nil
}

func newScope() (tally.Scope, error) {
	s, _ := tally.NewRootScope(tally.ScopeOptions{
		Tags:            nil,
		Prefix:          "",
		Reporter:        nil,
		CachedReporter:  nil,
		Separator:       "",
		DefaultBuckets:  nil,
		SanitizeOptions: nil,
		MetricsOption:   0,
	}, 0)
	return s, nil
}

func newLogging() (logging.AuditLog, error) {
	return logging.GetLogrLoggerOrPanic(), nil
}