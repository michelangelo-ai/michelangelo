package minio

import "go.uber.org/config"

const (
	configKey = "minio"
)

type (
	// StorageProvider defines configuration for a single storage provider
	StorageProvider struct {
		// Provider type: "s3", "azure", "gcp"
		Type string `yaml:"type"`

		// S3/MinIO specific configuration
		AwsRegion          string `yaml:"awsRegion,omitempty"`
		AwsAccessKeyId     string `yaml:"awsAccessKeyId,omitempty"`
		AwsSecretAccessKey string `yaml:"awsSecretAccessKey,omitempty"`
		AwsEndpointUrl     string `yaml:"awsEndpointUrl,omitempty"`
		UseEnvAws          bool   `yaml:"useEnvAws,omitempty"`
		UseIAM             bool   `yaml:"useIam,omitempty"`

		// Azure Blob Storage configuration
		AzureStorageAccount string `yaml:"azureStorageAccount,omitempty"`
		AzureSASToken       string `yaml:"azureSASToken,omitempty"`
		AzureEndpoint       string `yaml:"azureEndpoint,omitempty"`
	}

	// Config defines customization parameters for the Module
	Config struct {
		// Map of storage providers with keys like "aws", "azure", "aws-2", etc.
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
