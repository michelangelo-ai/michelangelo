// Package workflowfx configures and provides workers and clients for Cadence and Temporal.
// The configuration for the module is specified in YAML. See Config for reference.
package workflowfx

import (
	"context"
	"fmt"
	"time"

	"github.com/cadence-workflow/starlark-worker/cadence"
	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/temporal"
	sworker "github.com/cadence-workflow/starlark-worker/worker"
	"github.com/cadence-workflow/starlark-worker/workflow"
	tallyv4 "github.com/uber-go/tally/v4"

	"github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/uber-go/tally"
	tempclient "go.temporal.io/sdk/client"
	temptally "go.temporal.io/sdk/contrib/tally"
	tempworker "go.temporal.io/sdk/worker"
	"go.uber.org/cadence/.gen/go/cadence/workflowserviceclient"
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
	fx.Provide(config.ProvideConfig[Config](ConfigKey)),
	fx.Provide(func() TemporalClientFactory { return DefaultTemporalClientFactory{} }),
	fx.Provide(func() CadenceClientFactory { return DefaultCadenceClientFactory{} }),
	fx.Provide(provide),
	fx.Invoke(start),
)

// TemporalClientFactory creates Temporal clients.
type TemporalClientFactory interface {
	NewTemporalClient(tempclient.Options) (tempclient.Client, error)
}

// DefaultTemporalClientFactory implements TemporalClientFactory.
type DefaultTemporalClientFactory struct{}

func (f DefaultTemporalClientFactory) NewTemporalClient(opts tempclient.Options) (tempclient.Client, error) {
	return tempclient.Dial(opts)
}

// CadenceClientFactory creates Cadence clients.
type CadenceClientFactory interface {
	NewCadenceClient(conf Config) (workflowserviceclient.Interface, error)
}

// DefaultCadenceClientFactory implements CadenceClientFactory.
type DefaultCadenceClientFactory struct{}

func (f DefaultCadenceClientFactory) NewCadenceClient(conf Config) (workflowserviceclient.Interface, error) {
	return newCadenceClient(conf)
}

type In struct {
	fx.In
	Config Config
	Logger *zap.Logger

	CadenceFactory  CadenceClientFactory  `optional:"true"`
	TemporalFactory TemporalClientFactory `optional:"true"`
}

type Out struct {
	fx.Out
	Backend service.BackendType
	Workers []sworker.Worker
}

// provide provides workers and clients for either Cadence or Temporal.
func provide(in In) (Out, error) {
	out := Out{}

	conf := in.Config
	out.Backend = service.BackendType(conf.Provider)
	if conf.Provider == ProviderCadence {
		var err error
		out.Workers, err = newCadenceWorker(in.CadenceFactory, in.Config, in.Logger)
		if err != nil {
			return out, err
		}
	} else if conf.Provider == ProviderTemporal {
		var err error
		out.Workers, err = newTemporalWorker(in.TemporalFactory, in.Config, in.Logger)
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

// newCadenceWorker creates a new Cadence worker.
func newCadenceWorker(factory CadenceClientFactory, conf Config, log *zap.Logger) ([]sworker.Worker, error) {
	metrics := tally.NoopScope
	ctx := context.Background()
	ctx = context.WithValue(ctx, workflow.BackendContextKey, cadence.NewWorkflow())
	workerOptions := worker.Options{
		MetricsScope:              metrics,
		Logger:                    log,
		DataConverter:             &cadence.DataConverter{Logger: log},
		BackgroundActivityContext: ctx,
	}
	// Create the Cadence client interface.
	inter, err := factory.NewCadenceClient(conf)
	if err != nil {
		return nil, err
	}

	// Create Cadence workers
	workers := make([]sworker.Worker, len(conf.Workers))
	for i, w := range conf.Workers {
		workers[i] = cadence.NewWorker(worker.New(inter, w.Domain, w.TaskList, workerOptions))
	}

	return workers, nil
}

// newCadenceClient creates a new Cadence client interface.
func newCadenceClient(conf Config) (workflowserviceclient.Interface, error) {
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

// newTemporalWorker creates a new Temporal worker.
func newTemporalWorker(factory TemporalClientFactory, conf Config, log *zap.Logger) ([]sworker.Worker, error) {
	scope, _ := tallyv4.NewRootScope(tallyv4.ScopeOptions{
		Prefix: "temporal",
	}, time.Second)
	// Create Temporal client
	c, err := factory.NewTemporalClient(tempclient.Options{
		HostPort:       conf.Host,
		Namespace:      conf.Client.Domain,
		DataConverter:  &temporal.DataConverter{Logger: log},
		MetricsHandler: temptally.NewMetricsHandler(scope),
		Logger:         temporal.NewZapLoggerAdapter(log),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create temporal client: %w", err)
	}

	// Create workers
	workers := make([]sworker.Worker, len(conf.Workers))
	for i, w := range conf.Workers {
		ctx := context.Background()
		ctx = context.WithValue(ctx, workflow.BackendContextKey, temporal.NewWorkflow())
		workers[i] = temporal.NewWorker(tempworker.New(c, w.TaskList, tempworker.Options{
			BackgroundActivityContext: ctx,
		}))
	}

	return workers, nil
}

// start starts workers.
func start(lc fx.Lifecycle, workers []sworker.Worker) {
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
