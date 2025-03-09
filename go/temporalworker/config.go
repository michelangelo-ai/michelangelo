package worker

import (
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/config"
	"go.uber.org/yarpc"
	"go.uber.org/yarpc/transport/grpc"
	"go.uber.org/zap"

	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/ext/cadence"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

const (
	configKey = "worker"
)

type (
	// Config is the configuration for the Temporal worker.
	Config struct {
		Namespace           string `yaml:"namespace"`
		Address             string `yaml:"address"`
		TaskQueue           string `yaml:"taskQueue"`
		MaAPIServiceName    string `yaml:"maApiServiceName"`
		MaAPIServiceAddress string `yaml:"maApiServiceAddress"`
	}
)

func getYARPCClients(provider config.Provider) (v2pb.RayClusterServiceYARPCClient, v2pb.RayJobServiceYARPCClient, error) {
	conf := Config{}
	err := provider.Get(configKey).Populate(&conf)
	if err != nil {
		return nil, nil, err
	}
	tran := grpc.NewTransport().NewSingleOutbound(conf.MaAPIServiceAddress)
	dispatcher := yarpc.NewDispatcher(yarpc.Config{
		Name:      conf.MaAPIServiceName,
		Outbounds: yarpc.Outbounds{conf.MaAPIServiceName: {Unary: tran}},
	})
	if err := dispatcher.Start(); err != nil {
		return nil, nil, err
	}

	return v2pb.NewRayClusterServiceYARPCClient(dispatcher.ClientConfig(conf.MaAPIServiceName)),
		v2pb.NewRayJobServiceYARPCClient(dispatcher.ClientConfig(conf.MaAPIServiceName)), nil
}

// InitWorker initializes the Temporal client and worker.
func initWorker(provider config.Provider, logger *zap.Logger) (client.Client, worker.Worker, error) {
	var conf Config
	if err := provider.Get(configKey).Populate(&conf); err != nil {
		logger.Error("Failed to read worker config", zap.Error(err))
		return nil, nil, err
	}

	// Create Temporal client using the provided helper function.
	c := cadence.CreateClient(conf.Address, conf.Namespace)

	// Create Temporal worker using the provided helper function.
	w := cadence.CreateWorker(c, conf.TaskQueue)

	return c, w, nil
}
