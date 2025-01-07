package raycluster

import "go.uber.org/config"

const (
	configKey = "controllers.rayCluster"
)

type (
	Config struct {
		CadenceTaskList string `yaml:"cadenceTaskList"`
	}
)

func newConfig(provider config.Provider) (Config, error) {
	conf := Config{}
	err := provider.Get(configKey).Populate(&conf)
	return conf, err
}
