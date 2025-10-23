package azure

import (
	"testing"

	"go.uber.org/config"
)

// TestNewConfig_Success verifies that newConfig correctly populates the Config struct
// when valid YAML configuration data is provided.
func TestNewConfig_Success(t *testing.T) {
	// YAML content with the "azure" key and its multi-provider configuration.
	const yamlContent = `
azure:
  storageProviders:
    azure-dev:
      type: "azure"
      azureStorageAccount: "testaccount"
      azureSASToken: "sv=2022-11-02&ss=b&srt=sco&sp=rwdlacupx&se=2023-12-31T23:59:59Z"
    azure-prod:
      type: "azure"
      azureStorageAccount: "prodaccount"
      azureSASToken: "sv=2023-01-01&ss=b&srt=sco&sp=rwdlacupx&se=2024-12-31T23:59:59Z"
      azureEndpoint: "https://custom.endpoint.net"
  defaultProvider: "azure-dev"
`

	// Create a new YAML provider using the YAML configuration.
	provider, err := config.NewYAMLProviderFromBytes([]byte(yamlContent))
	if err != nil {
		t.Fatalf("failed to create YAML provider: %v", err)
	}

	// Call newConfig with the provider.
	conf, err := newConfig(provider)
	if err != nil {
		t.Fatalf("newConfig returned error: %v", err)
	}

	// Validate that the configuration values are correctly populated.
	if len(conf.StorageProviders) != 2 {
		t.Errorf("expected 2 storage providers, got %d", len(conf.StorageProviders))
	}

	// Check azure-dev provider
	azureDevProvider, exists := conf.StorageProviders["azure-dev"]
	if !exists {
		t.Errorf("expected azure-dev provider to exist")
	}
	if azureDevProvider.Type != "azure" {
		t.Errorf("expected Type 'azure', got %q", azureDevProvider.Type)
	}
	if azureDevProvider.AzureStorageAccount != "testaccount" {
		t.Errorf("expected AzureStorageAccount 'testaccount', got %q", azureDevProvider.AzureStorageAccount)
	}

	// Check azure-prod provider
	azureProdProvider, exists := conf.StorageProviders["azure-prod"]
	if !exists {
		t.Errorf("expected azure-prod provider to exist")
	}
	if azureProdProvider.Type != "azure" {
		t.Errorf("expected Type 'azure', got %q", azureProdProvider.Type)
	}
	if azureProdProvider.AzureStorageAccount != "prodaccount" {
		t.Errorf("expected AzureStorageAccount 'prodaccount', got %q", azureProdProvider.AzureStorageAccount)
	}
	if azureProdProvider.AzureEndpoint != "https://custom.endpoint.net" {
		t.Errorf("expected AzureEndpoint 'https://custom.endpoint.net', got %q", azureProdProvider.AzureEndpoint)
	}

	// Check default provider
	if conf.DefaultProvider != "azure-dev" {
		t.Errorf("expected DefaultProvider 'azure-dev', got %q", conf.DefaultProvider)
	}
}

// TestNewConfig_MissingKey verifies that newConfig returns an empty Config struct
// when the required "azure" key is missing from the YAML configuration.
func TestNewConfig_MissingKey(t *testing.T) {
	// YAML content without the "azure" key.
	const yamlContent = `
notazure:
  someKey: "value"
`

	provider, err := config.NewYAMLProviderFromBytes([]byte(yamlContent))
	if err != nil {
		t.Fatalf("failed to create YAML provider: %v", err)
	}

	conf, err := newConfig(provider)
	if err != nil {
		t.Fatalf("newConfig returned error: %v", err)
	}

	// Since the "azure" key is missing, StorageProviders should be empty.
	if len(conf.StorageProviders) != 0 {
		t.Errorf("expected empty StorageProviders when key is missing, got %+v", conf.StorageProviders)
	}
	if conf.DefaultProvider != "" {
		t.Errorf("expected empty DefaultProvider when key is missing, got %q", conf.DefaultProvider)
	}
}