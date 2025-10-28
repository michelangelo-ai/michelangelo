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
    aws-prod:
      type: "s3"
      awsRegion: "us-east-1"
      awsAccessKeyId: "prodAccessKey"
      awsSecretAccessKey: "prodSecretKey"
      awsEndpointUrl: "s3.amazonaws.com"
      useIam: true
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

	// Check AWS sandbox provider
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

	// Check AWS prod provider
	awsProdProvider, exists := conf.StorageProviders["aws-prod"]
	if !exists {
		t.Errorf("expected aws-prod provider to exist")
	}
	if awsProdProvider.Type != "s3" {
		t.Errorf("expected Type 's3', got %q", awsProdProvider.Type)
	}
	if awsProdProvider.AwsRegion != "us-east-1" {
		t.Errorf("expected AwsRegion 'us-east-1', got %q", awsProdProvider.AwsRegion)
	}
	if awsProdProvider.UseIAM != true {
		t.Errorf("expected UseIAM 'true', got %v", awsProdProvider.UseIAM)
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
