package workflowfx

const (
	ConfigKey        = "workflow-engine"
	ProviderCadence  = "cadence"
	ProviderTemporal = "temporal"
)

// Config is the configuration for the module. YAML looks like this:
//
//	cadence:
//	  host: 127.0.0.1:7833
//	  transport: grpc
//	  workers:
//	    - domain: default
//	      taskList: default
//	  client:
//	    domain: default
type Config struct {
	Host      string         `yaml:"host"`
	Transport string         `yaml:"transport"`
	Workers   []WorkerConfig `yaml:"workers"`
	Client    ClientConfig   `yaml:"client"`
	Provider  string         `yaml:"provider"`
}

type WorkerConfig struct {
	Domain   string `yaml:"domain"`
	TaskList string `yaml:"taskList"`
}

type ClientConfig struct {
	Domain string `yaml:"domain"`
}
