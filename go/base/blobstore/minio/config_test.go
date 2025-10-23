package minio

import (
	"testing"

	"go.uber.org/config"
)

// TestNewConfig_Success verifies that newConfig correctly populates the Config struct
// when valid YAML configuration data is provided.
func TestNewConfig_Success(t *testing.T) {
	// YAML content with the "minio" key and its multi-provider configuration.
	const yamlContent = `
minio:
  storageProviders:
    aws-sandbox:
      type: "s3"
      awsRegion: "us-west-2"
      awsAccessKeyId: "testAccessKey"
      awsSecretAccessKey: "testSecretKey"
      awsEndpointUrl: "http://localhost:9000"
    azure-dev:
      type: "azure"
      azureStorageAccount: "testaccount"
      azureSASToken: "sv=2022-11-02&ss=b&srt=sco&sp=rwdlacupx&se=2023-12-31T23:59:59Z"
  defaultProvider: "aws-sandbox"
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

	// Check AWS provider
	awsProvider, exists := conf.StorageProviders["aws-sandbox"]
	if !exists {
		t.Errorf("expected aws-sandbox provider to exist")
	}
	if awsProvider.Type != "s3" {
		t.Errorf("expected Type 's3', got %q", awsProvider.Type)
	}
	if awsProvider.AwsRegion != "us-west-2" {
		t.Errorf("expected AwsRegion 'us-west-2', got %q", awsProvider.AwsRegion)
	}
	if awsProvider.AwsAccessKeyId != "testAccessKey" {
		t.Errorf("expected AwsAccessKeyId 'testAccessKey', got %q", awsProvider.AwsAccessKeyId)
	}

	// Check Azure provider
	azureProvider, exists := conf.StorageProviders["azure-dev"]
	if !exists {
		t.Errorf("expected azure-dev provider to exist")
	}
	if azureProvider.Type != "azure" {
		t.Errorf("expected Type 'azure', got %q", azureProvider.Type)
	}
	if azureProvider.AzureStorageAccount != "testaccount" {
		t.Errorf("expected AzureStorageAccount 'testaccount', got %q", azureProvider.AzureStorageAccount)
	}

	// Check default provider
	if conf.DefaultProvider != "aws-sandbox" {
		t.Errorf("expected DefaultProvider 'aws-sandbox', got %q", conf.DefaultProvider)
	}
}

// TestNewConfig_MissingKey verifies that newConfig returns an empty Config struct
// when the required "minio" key is missing from the YAML configuration.
func TestNewConfig_MissingKey(t *testing.T) {
	// YAML content without the "minio" key.
	const yamlContent = `
notminio:
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

	// Since the "minio" key is missing, StorageProviders should be empty.
	if len(conf.StorageProviders) != 0 {
		t.Errorf("expected empty StorageProviders when key is missing, got %+v", conf.StorageProviders)
	}
	if conf.DefaultProvider != "" {
		t.Errorf("expected empty DefaultProvider when key is missing, got %q", conf.DefaultProvider)
	}
}
