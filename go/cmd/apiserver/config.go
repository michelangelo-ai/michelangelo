package main

import (
	"github.com/michelangelo-ai/michelangelo/go/storage"
	"go.uber.org/config"
	"k8s.io/client-go/rest"
	baseconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
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
)

// getK8sRestConfig parses the configuration file and returns the k8s REST client configuration
// for Michelangelo API server.
func getK8sRestConfig(provider config.Provider) (*rest.Config, error) {
	return baseconfig.GetK8sConfig(provider, k8sConfigKey)
}

// getYARPCConfig parses the configuration file and returns the YARPC server configuration
// for Michelangelo API server.
func getYARPCConfig(provider config.Provider) (YARPCConfig, error) {
	yarpcConfig := YARPCConfig{}
	err := provider.Get(yarpcConfigKey).Populate(&yarpcConfig)
	return yarpcConfig, err
}

func getMetadataStorageConfig(provider config.Provider) (storage.MetadataStorageConfig, error) {
	return baseconfig.GetMetadataStorageConfig(provider, storageConfigKey)
}
