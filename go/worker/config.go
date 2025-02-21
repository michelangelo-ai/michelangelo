package worker

import (
	"go.uber.org/config"
	"go.uber.org/yarpc"
	"go.uber.org/yarpc/transport/grpc"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

const (
	configKey = "worker"
)

type (
	// Config is the configuration for YARPC server.
	Config struct {
		MaAPIServiceName string `yaml:"maApiServiceName"`
		Address          string `yaml:"address"`
	}
)

func getYARPCClients(provider config.Provider) (v2pb.RayClusterServiceYARPCClient, v2pb.RayJobServiceYARPCClient, error) {
	conf := Config{}
	err := provider.Get(configKey).Populate(&conf)
	if err != nil {
		return nil, nil, err
	}
	tran := grpc.NewTransport().NewSingleOutbound(conf.Address)
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
