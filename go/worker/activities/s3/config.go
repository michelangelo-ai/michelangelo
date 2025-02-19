package s3

import "go.uber.org/config"

const (
	configKey = "s3"
)

type (
	// Config defines customization parameters for the Module
	Config struct {
		AwsRegion          string `yaml:"awsRegion"`
		AwsAccessKeyId     string `yaml:"awsAccessKeyId"`
		AwsSecretAccessKey string `yaml:"awsSecretAccessKey"`
		AwsEndpointUrl     string `yaml:"awsEndpointUrl"`
	}
)

func newConfig(provider config.Provider) (Config, error) {
	conf := Config{}
	err := provider.Get(configKey).Populate(&conf)
	return conf, err
}
