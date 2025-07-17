package main

import (
	"go.uber.org/config"
)

const (
	yarpcConfigKey = "apiserver.yarpc"
)

type (
	// YARPCConfig is the configuration for YARPC server.
	YARPCConfig struct {
		Host      string `yaml:"host"`
		Port      int    `yaml:"port"`
		Transport string `yaml:"transport"` // "grpc", "http", or "both" (default: "grpc")
		HTTPPort  int    `yaml:"httpPort"`  // Optional separate HTTP port (default: Port + 1)
	}
)

// getYARPCConfig parses the configuration file and returns the YARPC server configuration
// for Michelangelo API server.
func getYARPCConfig(provider config.Provider) (YARPCConfig, error) {
	yarpcConfig := YARPCConfig{}
	err := provider.Get(yarpcConfigKey).Populate(&yarpcConfig)
	return yarpcConfig, err
}
