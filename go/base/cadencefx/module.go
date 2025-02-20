// Package cadencefx configures and provides workers and clients for Cadence.
// The configuration for the module is specified in YAML. See Config for reference.
package cadencefx

import (
	"context"
	"fmt"

	"github.com/cadence-workflow/starlark-worker/cadstar"
	"github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/uber-go/tally"
	"go.uber.org/cadence/.gen/go/cadence/workflowserviceclient"
	"go.uber.org/cadence/client"
	"go.uber.org/cadence/worker"
	"go.uber.org/fx"
	"go.uber.org/yarpc"
	"go.uber.org/yarpc/api/transport"
	"go.uber.org/yarpc/transport/grpc"
	"go.uber.org/yarpc/transport/tchannel"
	"go.uber.org/zap"
)

// Module provides workers and clients for Cadence.
// See Config for the configuration reference.
var Module = fx.Options(
	fx.Provide(config.ProvideConfig[Config](configKey)),
	fx.Provide(provide),
	fx.Invoke(start),
)

type In struct {
	fx.In
	Config Config
	Logger *zap.Logger
}

type Out struct {
	fx.Out
	Client  client.Client
	Workers []worker.Worker
}

func provide(in In) (Out, error) {
	out := Out{}

	// TODO: andrii: Create FX module for Tally and inject metrics here.
	metrics := tally.NoopScope

	conf := in.Config

	// Create the Cadence client interface.
	inter, err := newInterface(conf)
	if err != nil {
		return out, err
	}

	// Create Cadence workers
	out.Workers = make([]worker.Worker, len(conf.Workers))
	for i, w := range conf.Workers {
		out.Workers[i] = worker.New(inter, w.Domain, w.TaskList, worker.Options{
			MetricsScope:  metrics,
			Logger:        in.Logger,
			DataConverter: &cadstar.DataConverter{Logger: in.Logger},
		})
	}

	// Create Cadence client
	out.Client = client.NewClient(inter, conf.Client.Domain, &client.Options{
		MetricsScope: metrics,
	})

	return out, nil
}

// start starts workers.
func start(lc fx.Lifecycle, workers []worker.Worker) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			for _, w := range workers {
				if err := w.Start(); err != nil {
					return err
				}
			}
			return nil
		},
		OnStop: func(context.Context) error {
			for _, w := range workers {
				w.Stop()
			}
			return nil
		},
	})
}

// newInterface creates a new Cadence client interface.
func newInterface(conf Config) (workflowserviceclient.Interface, error) {
	service := "cadence-frontend"

	var tran transport.UnaryOutbound
	switch conf.Transport {
	case "grpc":
		tran = grpc.NewTransport().NewSingleOutbound(conf.Host)
	case "tchannel":
		if t, err := tchannel.NewTransport(tchannel.ServiceName("tchannel")); err != nil {
			return nil, err
		} else {
			tran = t.NewSingleOutbound(conf.Host)
		}
	default:
		return nil, fmt.Errorf("unsupported transport: %s", conf.Transport)
	}
	dispatcher := yarpc.NewDispatcher(yarpc.Config{
		Name: service,
		Outbounds: yarpc.Outbounds{
			service: {
				Unary: tran,
			},
		},
	})
	if err := dispatcher.Start(); err != nil {
		return nil, err
	}
	return workflowserviceclient.New(dispatcher.ClientConfig(service)), nil
}
