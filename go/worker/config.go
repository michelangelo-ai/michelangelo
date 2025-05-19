package worker

import (
	"go.uber.org/config"
	"go.uber.org/fx"
	"go.uber.org/yarpc"
	"go.uber.org/yarpc/transport/grpc"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

const configKey = "worker"

// Config represents the worker YARPC configuration.
type Config struct {
	MaAPIServiceName string `yaml:"maApiServiceName"`
	Address          string `yaml:"address"`
}

// Params provides dependencies for YARPC dispatcher.
type Params struct {
	fx.In

	Config Config
}

// ClientParams provides dependencies for creating YARPC clients.
type ClientParams struct {
	fx.In

	Dispatcher *yarpc.Dispatcher
	Config     Config
}

// NewConfig creates a new Config from a provider.
func NewConfig(provider config.Provider) (Config, error) {
	var conf Config
	err := provider.Get(configKey).Populate(&conf)
	return conf, err
}

// NewYARPCDispatcher creates and starts a new YARPC dispatcher.
func NewYARPCDispatcher(p Params) (*yarpc.Dispatcher, error) {
	tran := grpc.NewTransport().NewSingleOutbound(p.Config.Address)
	dispatcher := yarpc.NewDispatcher(yarpc.Config{
		Name:      p.Config.MaAPIServiceName,
		Outbounds: yarpc.Outbounds{p.Config.MaAPIServiceName: {Unary: tran}},
	})

	if err := dispatcher.Start(); err != nil {
		return nil, err
	}

	return dispatcher, nil
}

// NewRayClusterServiceClient creates a RayClusterService YARPC client.
func NewRayClusterServiceClient(p ClientParams) v2pb.RayClusterServiceYARPCClient {
	return v2pb.NewRayClusterServiceYARPCClient(p.Dispatcher.ClientConfig(p.Config.MaAPIServiceName))
}

// NewRayJobServiceClient creates a RayJobService YARPC client.
func NewRayJobServiceClient(p ClientParams) v2pb.RayJobServiceYARPCClient {
	return v2pb.NewRayJobServiceYARPCClient(p.Dispatcher.ClientConfig(p.Config.MaAPIServiceName))
}

// NewSparkJobServiceClient creates a SparkJobService YARPC client.
func NewSparkJobServiceClient(p ClientParams) v2pb.SparkJobServiceYARPCClient {
	return v2pb.NewSparkJobServiceYARPCClient(p.Dispatcher.ClientConfig(p.Config.MaAPIServiceName))
}

// NewCachedOutputServiceClient creates a CachedOutputService YARPC client.
func NewCachedOutputServiceClient(p ClientParams) v2pb.CachedOutputServiceYARPCClient {
	return v2pb.NewCachedOutputServiceYARPCClient(p.Dispatcher.ClientConfig(p.Config.MaAPIServiceName))
}

// NewDeploymentServiceClient creates a DeploymentService YARPC client.
func NewDeploymentServiceClient(p ClientParams) v2pb.DeploymentServiceYARPCClient {
	return v2pb.NewDeploymentServiceYARPCClient(p.Dispatcher.ClientConfig(p.Config.MaAPIServiceName))
}
