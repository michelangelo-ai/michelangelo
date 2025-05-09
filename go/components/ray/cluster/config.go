package cluster

import (
	"go.uber.org/config"
)

const (
	configKey = "controllers.rayCluster"
)

type (
	Config struct {
		QPS   float32 `yaml:"k8sQps"`
		Burst int     `yaml:"k8sBurst"`
	}
)

func newConfig(provider config.Provider) (Config, error) {
	conf := Config{}
	err := provider.Get(configKey).Populate(&conf)
	return conf, err
}
