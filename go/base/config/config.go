package config

import (
	"flag"

	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"go.uber.org/config"

	"os"
	"strings"

	"github.com/michelangelo-ai/michelangelo/go/storage"
	"go.uber.org/fx"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	_configKeySeparator       = ":"
	_defaultConfigDir         = "config"
	_k8sConfigKey             = "k8s"
	_metadataStorageConfigKey = "metadataStorage"
	_workflowClientConfigKey  = "workflowClient"
	_mysqlConfigKey           = "mysql"
	_ingesterConfigKey        = "ingester"
	_inferenceServerConfigKey = "inferenceServer"
)

// K8sConfig is the configuration for k8s REST client.
type K8sConfig struct {
	QPS   float32 `yaml:"qps"`
	Burst int     `yaml:"burst"`
}

type WorkflowClientConfig struct {
	Service            string `yaml:"service"`
	Host               string `yaml:"host"`
	Transport          string `yaml:"transport"`
	Domain             string `yaml:"domain"`
	TaskList           string `yaml:"taskList"`
	Provider           string `yaml:"provider"`
	UseTLS             bool   `yaml:"useTLS"`
	ExecutionUrlFormat string `yaml:"executionUrlFormat"`
}

// Params defines the dependencies of the config fx module.
type Params struct {
	fx.In

	Environment env.Context
}

// Result defines the objects that the config fx module provides.
type Result struct {
	fx.Out

	Provider config.Provider
}

// Module load config.Provider based on the environment context.
var Module = fx.Module("config",
	fx.Provide(New),
)

// New exports functionality similar to Module, but allows the caller to wrap
// or modify Result. Most users should use Module instead.
func New(p Params) (Result, error) {
	// use os.LookupEnv to look up environment variables
	lookupFun := os.LookupEnv
	cfg, err := newYAML(p.Environment, lookupFun)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Provider: cfg,
	}, nil
}

// getConfigDirs extract config dirs from env if ConfigPath was set as environment variable,
// otherwise use default config dir
func getConfigDirs(env env.Context) []string {
	// Allow overriding the directory where config is loaded from
	if env.ConfigPath != "" {
		return strings.Split(env.ConfigPath, _configKeySeparator)
	}
	return []string{_defaultConfigDir}
}

// GetK8sConfig parses the configuration file and returns the k8s REST client configuration
func GetK8sConfig(provider config.Provider) (*rest.Config, error) {
	flag.Parse()
	conf, err := ctrl.GetConfig()
	if err != nil {
		return nil, err
	}
	k8sConfig := K8sConfig{}
	err = provider.Get(_k8sConfigKey).Populate(&k8sConfig)
	if err != nil {
		return nil, err
	}
	conf.QPS = k8sConfig.QPS
	conf.Burst = k8sConfig.Burst
	return conf, nil
}

// GetMetadataStorageConfig parses the configuration file and returns the metadata storage configuration
func GetMetadataStorageConfig(provider config.Provider) (storage.MetadataStorageConfig, error) {
	storageConfig := storage.MetadataStorageConfig{}
	err := provider.Get(_metadataStorageConfigKey).Populate(&storageConfig)
	return storageConfig, err
}

// GetWorkflowClientConfig parses the configuration file and returns the workflow client configuration
func GetWorkflowClientConfig(provider config.Provider) (WorkflowClientConfig, error) {
	workflowClientConfig := WorkflowClientConfig{}
	err := provider.Get(_workflowClientConfigKey).Populate(&workflowClientConfig)
	return workflowClientConfig, err
}

// GetMySQLConfig parses the configuration file and returns the MySQL configuration.
func GetMySQLConfig(provider config.Provider) (MySQLConfig, error) {
	mysqlConfig := MySQLConfig{}
	err := provider.Get(_mysqlConfigKey).Populate(&mysqlConfig)
	return mysqlConfig, err
}

// GetIngesterConfig parses the configuration file and returns the ingester configuration.
func GetIngesterConfig(provider config.Provider) (IngesterConfig, error) {
	ingesterConfig := IngesterConfig{}
	err := provider.Get(_ingesterConfigKey).Populate(&ingesterConfig)
	return ingesterConfig, err
}

// InferenceServerConfig is the controller-side configuration for the inference
// server controller.
type InferenceServerConfig struct {
	Gateway GatewayConfig `yaml:"gateway"`
}

// GatewayConfig describes how the inference server controller locates a
// cluster's ingress Gateway Service. The Service is set up out-of-band (e.g.,
// by `ma sandbox create` for sandbox); this config tells the EndpointSource
// where to find it.
type GatewayConfig struct {
	ServiceName      string `yaml:"serviceName"`
	ServiceNamespace string `yaml:"serviceNamespace"`
	PortName         string `yaml:"portName"`
}

// GetInferenceServerConfig parses the configuration file and returns the
// inference server controller configuration.
func GetInferenceServerConfig(provider config.Provider) (InferenceServerConfig, error) {
	inferenceServerConfig := InferenceServerConfig{}
	err := provider.Get(_inferenceServerConfigKey).Populate(&inferenceServerConfig)
	return inferenceServerConfig, err
}
