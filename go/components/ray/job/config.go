package job

import (
	"go.uber.org/config"
)

const configKey = "controllers.rayJob"

// Config holds tunables for the RayJob controller.
type Config struct {
	// FinishedJobTtlSeconds is how long a terminal RayJob is retained in the
	// compute cluster before the controller deletes the KubeRay RayJob. A
	// non-positive value falls back to the controller default.
	FinishedJobTtlSeconds int64 `yaml:"finishedJobTtlSeconds"`
}

func newConfig(provider config.Provider) (Config, error) {
	conf := Config{}
	if err := provider.Get(configKey).Populate(&conf); err != nil {
		return Config{}, err
	}
	return conf, nil
}
