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

		// Log persistence platform-level config (for computing log_url on cluster status)
		LogPersistence LogPersistenceConfig `yaml:"logPersistence"`
	}

	LogPersistenceConfig struct {
		Enabled           bool   `yaml:"enabled"`
		Bucket            string `yaml:"bucket"`             // S3 bucket name (e.g. "ray-history")
		PathPrefix        string `yaml:"pathPrefix"`         // Key prefix under the bucket (e.g. "clusters/")
		StorageEndpoint   string `yaml:"storageEndpoint"`   // S3-compatible endpoint (used by mapper)
		Region            string `yaml:"region"`             // S3 region (used by mapper)
		CredentialsSecret string `yaml:"credentialsSecret"`  // K8s Secret name (used by mapper)
		CollectorImage    string `yaml:"collectorImage"`     // Collector sidecar image (used by mapper)
	}
)

func newConfig(provider config.Provider) (Config, error) {
	conf := Config{}
	err := provider.Get(configKey).Populate(&conf)
	return conf, err
}
