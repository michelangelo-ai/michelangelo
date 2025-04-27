package main

import (
	"flag"

	"github.com/michelangelo-ai/michelangelo/go/storage"
	"go.uber.org/config"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"

)

const (
	k8sConfigKey     = "controllermgr.k8s"
	storageConfigKey = "controllermgr.metadataStorage"
)

type (
	K8sConfig struct {
		QPS   float32 `yaml:"qps"`
		Burst int     `yaml:"burst"`
	}
)

// getK8sRestConfig parses the configuration file and returns the k8s REST client configuration
// for Michelangelo Controller Mananger.
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

func getMetadataStorageConfig(provider config.Provider) (storage.MetadataStorageConfig, error) {
	storageConfig := storage.MetadataStorageConfig{}
	err := provider.Get(storageConfigKey).Populate(&storageConfig)
	return storageConfig, err
}
