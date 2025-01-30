package main

import (
	"flag"

	"github.com/michelangelo-ai/michelangelo/go/storage"
	"go.uber.org/config"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	yarpcConfigKey   = "apiserver.yarpc"
	k8sConfigKey     = "apiserver.k8s"
	storageConfigKey = "apiserver.metadataStorage"
)

type (
	// YARPCConfig is the configuration for YARPC server.
	YARPCConfig struct {
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
	}
	// K8sConfig is the configuration for k8s REST client.
	K8sConfig struct {
		QPS   float32 `yaml:"qps"`
		Burst int     `yaml:"burst"`
	}
)

// getK8sRestConfig parses the configuration file and returns the k8s REST client configuration
// for Michelangelo API server.
func getK8sRestConfig(provider config.Provider) (*rest.Config, error) {
	flag.Parse()
	conf, err := ctrl.GetConfig()
	if err != nil {
		return nil, err
	}
	k8sConfig := K8sConfig{}
	err = provider.Get(k8sConfigKey).Populate(&k8sConfig)
	if err != nil {
		return nil, err
	}
	conf.QPS = k8sConfig.QPS
	conf.Burst = k8sConfig.Burst
	return conf, nil
}

// getYARPCConfig parses the configuration file and returns the YARPC server configuration
// for Michelangelo API server.
func getYARPCConfig(provider config.Provider) (YARPCConfig, error) {
	yarpcConfig := YARPCConfig{}
	err := provider.Get(yarpcConfigKey).Populate(&yarpcConfig)
	return yarpcConfig, err
}

func getMetadataStorageConfig(provider config.Provider) (storage.MetadataStorageConfig, error) {
	storageConfig := storage.MetadataStorageConfig{}
	err := provider.Get(storageConfigKey).Populate(&storageConfig)
	return storageConfig, err
}
