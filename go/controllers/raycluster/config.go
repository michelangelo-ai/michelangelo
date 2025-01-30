package raycluster

import (
	"go.uber.org/config"
)

const (
	configKey = "controllers.rayCluster"
)

type (
	Config struct {
	}
)

func newConfig(provider config.Provider) (Config, error) {
	conf := Config{}
	err := provider.Get(configKey).Populate(&conf)
	return conf, err
}
