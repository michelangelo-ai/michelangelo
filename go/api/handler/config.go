package handler

import "go.uber.org/config"

// configKey is the config-provider path for Config, matching the apiserver
// ConfigMap's `apiserver.pipelineRunDefaults` YAML block.
const configKey = "apiserver.pipelineRunDefaults"

// Config holds apiserver-wide default values applied when creating
// PipelineRun/Model objects, configurable via the Helm value
// apiserver.pipelineRunDefaults.environment.
type Config struct {
	// PipelineRunDefaultEnvironment is the environment label value used when
	// a PipelineRun or Model is created without one. Empty means the operator
	// configured no default; callers fall back to api.UnspecifiedEnvironment
	// rather than treating empty as a valid label value.
	PipelineRunDefaultEnvironment string `yaml:"environment"`
}

// NewConfig populates a Config from the given provider.
func NewConfig(provider config.Provider) (Config, error) {
	var conf Config
	err := provider.Get(configKey).Populate(&conf)
	return conf, err
}
