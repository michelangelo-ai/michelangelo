package minio

import (
	"testing"

	"go.uber.org/config"
)

// TestNewConfig_Success verifies that NewConfig correctly populates the Config struct
// when valid YAML configuration data is provided.
func TestNewConfig_Success(t *testing.T) {
	// YAML content with the "minio" key and its array-based configuration.
	const yamlContent = `
minio:
  - name: "aws-sandbox"
    awsRegion: "us-west-2"
    awsAccessKeyId: "testAccessKey"
    awsSecretAccessKey: "testSecretKey"
    awsEndpointUrl: "http://localhost:9000"
    default: true
  - name: "aws-prod"
    awsRegion: "us-east-1"
    awsAccessKeyId: "prodAccessKey"
    awsSecretAccessKey: "prodSecretKey"
    awsEndpointUrl: "s3.amazonaws.com"
    useIam: true
`

	// Create a new YAML provider using the YAML configuration.
	provider, err := config.NewYAMLProviderFromBytes([]byte(yamlContent))
	if err != nil {
		t.Fatalf("failed to create YAML provider: %v", err)
	}

	// Call NewConfig with the provider.
	conf, err := NewConfig(provider)
	if err != nil {
		t.Fatalf("NewConfig returned error: %v", err)
	}

	// Validate that the configuration values are correctly populated.
	if len(conf) != 2 {
		t.Errorf("expected 2 storage providers, got %d", len(conf))
	}

	// Check AWS sandbox provider (first in array)
	awsProvider := conf[0]
	if awsProvider.Name != "aws-sandbox" {
		t.Errorf("expected first provider name to be 'aws-sandbox', got '%s'", awsProvider.Name)
	}
	if !awsProvider.Default {
		t.Errorf("expected aws-sandbox to be default provider, got %v", awsProvider.Default)
	}
	if awsProvider.AwsRegion != "us-west-2" {
		t.Errorf("expected AwsRegion 'us-west-2', got %q", awsProvider.AwsRegion)
	}
	if awsProvider.AwsAccessKeyId != "testAccessKey" {
		t.Errorf("expected AwsAccessKeyId 'testAccessKey', got %q", awsProvider.AwsAccessKeyId)
	}

	// Check AWS prod provider (second in array)
	awsProdProvider := conf[1]
	if awsProdProvider.Name != "aws-prod" {
		t.Errorf("expected second provider name to be 'aws-prod', got '%s'", awsProdProvider.Name)
	}
	if awsProdProvider.Default {
		t.Errorf("expected aws-prod to not be default provider, got %v", awsProdProvider.Default)
	}
	if awsProdProvider.AwsRegion != "us-east-1" {
		t.Errorf("expected AwsRegion 'us-east-1', got %q", awsProdProvider.AwsRegion)
	}
	if awsProdProvider.UseIAM != true {
		t.Errorf("expected UseIAM 'true', got %v", awsProdProvider.UseIAM)
	}
}

// TestNewConfig_MissingKey verifies that NewConfig returns an empty Config struct
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

	conf, err := NewConfig(provider)
	if err != nil {
		t.Fatalf("NewConfig returned error: %v", err)
	}

	// Since the "minio" key is missing, the array should be empty.
	if len(conf) != 0 {
		t.Errorf("expected empty array when key is missing, got %+v", conf)
	}
}
