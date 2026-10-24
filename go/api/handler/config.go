package handler

import "go.uber.org/config"

// configKey is the config-provider path for Config, matching the apiserver
// ConfigMap's `apiserver.pipelineRunDefaults` YAML block.
const configKey = "apiserver.pipelineRunDefaults"

// Config holds apiserver-wide default values and restrictions applied when
// creating PipelineRun/TriggerRun/Model objects, configurable via the Helm
// value block apiserver.pipelineRunDefaults.
type Config struct {
	// PipelineRunDefaultEnvironment is the environment label value used when
	// a PipelineRun or Model is created without one. Empty means the operator
	// configured no default; callers fall back to api.UnspecifiedEnvironment
	// rather than treating empty as a valid label value.
	PipelineRunDefaultEnvironment string `yaml:"environment"`

	// RestrictedBranches maps an EnvironmentLabel value (e.g. "production")
	// to the list of git branch names allowed to run with that value. An
	// environment name absent from this map (or present with an empty/nil
	// slice) is unrestricted — the map's keys never need to be exhaustive
	// over every environment value in use; only environments the operator
	// has explicitly opted in are restricted. Empty/nil map (the default)
	// means the operator has not opted any environment into this
	// restriction at all, matching today's (pre-this-feature) behavior.
	// Example: {"production": ["main", "master"]}.
	RestrictedBranches map[string][]string `yaml:"restrictedBranches"`
}

// NewConfig populates a Config from the given provider.
func NewConfig(provider config.Provider) (Config, error) {
	var conf Config
	err := provider.Get(configKey).Populate(&conf)
	return conf, err
}
