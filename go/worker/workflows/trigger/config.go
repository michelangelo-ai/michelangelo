package trigger

import "go.uber.org/config"

// configKey is the config-provider path for Config. It reads from the same
// Helm/ConfigMap key as go/api/handler.Config so that the API server and
// worker share the same operator-configured defaults.
const configKey = "apiserver.pipelineRunDefaults"

// Config holds the subset of operator-configured defaults that the
// trigger-fire path consumes when generating PipelineRun requests.
type Config struct {
	// DefaultEnvironment is the environment label value used when a
	// trigger-fired PipelineRun's source TriggerRun lacks the label.
	// Empty means the operator configured no default; callers fall back
	// to api.UnspecifiedEnvironment.
	DefaultEnvironment string `yaml:"environment"`
}

// NewConfig populates a Config from the given provider.
func NewConfig(provider config.Provider) (Config, error) {
	var conf Config
	err := provider.Get(configKey).Populate(&conf)
	return conf, err
}
