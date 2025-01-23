package zapfx

const configKey = "logging"

// Config is the configuration for the module. YAML look like this:
//
//	logging:
//	  level: info
//	  development: false
//	  encoding: json
type Config struct {
	Level       string `yaml:"level"`
	Development bool   `yaml:"development"`
	Encoding    string `yaml:"encoding"`
}
