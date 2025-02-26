package minio

import (
	"testing"

	"go.uber.org/config"
)

// TestNewConfig_Success verifies that newConfig correctly populates the Config struct
// when valid YAML configuration data is provided.
func TestNewConfig_Success(t *testing.T) {
	// YAML content with the "minio" key and its configuration.
	const yamlContent = `
minio:
  awsRegion: "us-west-2"
  awsAccessKeyId: "testAccessKey"
  awsSecretAccessKey: "testSecretKey"
  awsEndpointUrl: "http://localhost:9000"
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
	if conf.AwsRegion != "us-west-2" {
		t.Errorf("expected AwsRegion 'us-west-2', got %q", conf.AwsRegion)
	}
	if conf.AwsAccessKeyId != "testAccessKey" {
		t.Errorf("expected AwsAccessKeyId 'testAccessKey', got %q", conf.AwsAccessKeyId)
	}
	if conf.AwsSecretAccessKey != "testSecretKey" {
		t.Errorf("expected AwsSecretAccessKey 'testSecretKey', got %q", conf.AwsSecretAccessKey)
	}
	if conf.AwsEndpointUrl != "http://localhost:9000" {
		t.Errorf("expected AwsEndpointUrl 'http://localhost:9000', got %q", conf.AwsEndpointUrl)
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

	// Since the "minio" key is missing, all fields should be empty.
	if conf.AwsRegion != "" || conf.AwsAccessKeyId != "" || conf.AwsSecretAccessKey != "" || conf.AwsEndpointUrl != "" {
		t.Errorf("expected empty Config when key is missing, got %+v", conf)
	}
}
