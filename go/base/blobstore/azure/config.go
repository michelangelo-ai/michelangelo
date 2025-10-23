package azure

import "go.uber.org/config"

const (
	configKey = "azure"
)

type (
	// StorageProvider defines configuration for Azure storage provider
	StorageProvider struct {
		// Provider type: should be "azure"
		Type string `yaml:"type"`

		// Azure Blob Storage configuration
		AzureStorageAccount string `yaml:"azureStorageAccount"`
		AzureSASToken       string `yaml:"azureSASToken"`
		AzureEndpoint       string `yaml:"azureEndpoint,omitempty"`
	}

	// Config defines customization parameters for the Azure Module
	Config struct {
		// Map of Azure storage providers with keys like "azure-dev", "azure-prod", etc.
		StorageProviders map[string]StorageProvider `yaml:"storageProviders"`

		// Default provider key to use when none specified
		DefaultProvider string `yaml:"defaultProvider,omitempty"`
	}
)

func newConfig(provider config.Provider) (Config, error) {
	conf := Config{}
	err := provider.Get(configKey).Populate(&conf)
	return conf, err
}