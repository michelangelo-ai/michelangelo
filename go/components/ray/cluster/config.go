package cluster

import (
	"go.uber.org/config"
)

const (
	// configKey is the configuration path for RayCluster controller settings.
	configKey = "controllers.rayCluster"
)

// Config defines configuration parameters for the RayCluster controller.
//
// These settings control the rate limiting behavior when the controller interacts
// with the Kubernetes API server. Proper tuning of these values ensures optimal
// performance without overwhelming the API server.
type (
	Config struct {
		QPS   float32 `yaml:"k8sQps"`   // Maximum queries per second to Kubernetes API
		Burst int     `yaml:"k8sBurst"` // Maximum burst for throttle (number of requests)
	}
)

// newConfig creates a new Config by loading values from the configuration provider.
//
// The configuration is loaded from the path specified by configKey. If the configuration
// path does not exist or the values cannot be populated, an error is returned.
func newConfig(provider config.Provider) (Config, error) {
	conf := Config{}
	err := provider.Get(configKey).Populate(&conf)
	return conf, err
}
