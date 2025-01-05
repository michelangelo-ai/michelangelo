package controllermgr

import "go.uber.org/config"

const (
	configKey = "controllermgr"
)

type (
	// Config defines customization parameters for the Module
	Config struct {
		MetricsBindAddress     string `yaml:"metricsBindAddress"`
		HealthProbeBindAddress string `yaml:"healthProbeBindAddress"`
		LeaderElection         bool   `yaml:"leaderElection"`
		LeaderElectionID       string `yaml:"leaderElectionID"`
		Port                   int    `yaml:"port"`
	}
)

func newConfig(provider config.Provider) (Config, error) {
	conf := Config{}
	err := provider.Get(configKey).Populate(&conf)
	return conf, err
}
