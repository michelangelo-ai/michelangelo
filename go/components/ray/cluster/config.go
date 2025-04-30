package cluster

import (
	"go.uber.org/config"
)

const (
	configKey = "controllermgr.k8s"
)

type (
	Config struct {
		QPS   float32 `yaml:"Qps"`
		Burst int     `yaml:"Burst"`
	}
)

func newConfig(provider config.Provider) (Config, error) {
	conf := Config{}
	err := provider.Get(configKey).Populate(&conf)
	return conf, err
}
