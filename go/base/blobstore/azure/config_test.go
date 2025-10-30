package azure

import (
	"testing"

	"go.uber.org/config"
)

// TestNewConfig_Success verifies that newConfig correctly populates the Config struct
// when valid YAML configuration data is provided.
func TestNewConfig_Success(t *testing.T) {
	// YAML content with the "azure" key and its array-based configuration.
	const yamlContent = `
azure:
  - name: "azure-dev"
    azureStorageAccount: "testaccount"
    azureSASToken: "sv=2022-11-02&ss=b&srt=sco&sp=rwdlacupx&se=2023-12-31T23:59:59Z"
    default: true
  - name: "azure-prod"
    azureStorageAccount: "prodaccount"
    azureSASToken: "sv=2023-01-01&ss=b&srt=sco&sp=rwdlacupx&se=2024-12-31T23:59:59Z"
    azureEndpoint: "https://custom.endpoint.net"
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
	if len(conf) != 2 {
		t.Errorf("expected 2 storage providers, got %d", len(conf))
	}

	// Check azure-dev provider (first in array)
	azureDevProvider := conf[0]
	if azureDevProvider.Name != "azure-dev" {
		t.Errorf("expected first provider name to be 'azure-dev', got '%s'", azureDevProvider.Name)
	}
	if !azureDevProvider.Default {
		t.Errorf("expected azure-dev to be default provider, got %v", azureDevProvider.Default)
	}
	if azureDevProvider.AzureStorageAccount != "testaccount" {
		t.Errorf("expected AzureStorageAccount 'testaccount', got %q", azureDevProvider.AzureStorageAccount)
	}

	// Check azure-prod provider (second in array)
	azureProdProvider := conf[1]
	if azureProdProvider.Name != "azure-prod" {
		t.Errorf("expected second provider name to be 'azure-prod', got '%s'", azureProdProvider.Name)
	}
	if azureProdProvider.Default {
		t.Errorf("expected azure-prod to not be default provider, got %v", azureProdProvider.Default)
	}
	if azureProdProvider.AzureStorageAccount != "prodaccount" {
		t.Errorf("expected AzureStorageAccount 'prodaccount', got %q", azureProdProvider.AzureStorageAccount)
	}
	if azureProdProvider.AzureEndpoint != "https://custom.endpoint.net" {
		t.Errorf("expected AzureEndpoint 'https://custom.endpoint.net', got %q", azureProdProvider.AzureEndpoint)
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

	// Since the "azure" key is missing, the array should be empty.
	if len(conf) != 0 {
		t.Errorf("expected empty array when key is missing, got %+v", conf)
	}
}
