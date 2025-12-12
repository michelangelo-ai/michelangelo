package azure

import "go.uber.org/config"

const (
	configKey = "azure"
)

type (
	// Config defines customization parameters for the Azure Blob Storage Module
	Config struct {
		// Azure Blob Storage configuration
		StorageAccount string `yaml:"storageAccount"`
		SASToken       string `yaml:"sasToken"`
		Endpoint       string `yaml:"endpoint"`
	}
)

func newConfig(provider config.Provider) (Config, error) {
	conf := Config{}
	err := provider.Get(configKey).Populate(&conf)
	return conf, err
}
