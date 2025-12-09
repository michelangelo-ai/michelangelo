package azure

import (
	"testing"

	"go.uber.org/config"
)

// TestNewConfig_Success verifies that newConfig correctly populates the Config struct
// when valid YAML configuration data is provided.
func TestNewConfig_Success(t *testing.T) {
	// YAML content with the "azure" key and its configuration.
	const yamlContent = `
azure:
  storageAccount: "teststorageaccount"
  sasToken: "sv=2022-11-02&ss=b&srt=sco&sp=rwdlacupx&se=2024-12-31T23:59:59Z&st=2023-01-01T00:00:00Z&spr=https&sig=testSignature"
  endpoint: "https://teststorageaccount.blob.core.windows.net"
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
	if conf.StorageAccount != "teststorageaccount" {
		t.Errorf("expected StorageAccount 'teststorageaccount', got %q", conf.StorageAccount)
	}
	expectedSASToken := "sv=2022-11-02&ss=b&srt=sco&sp=rwdlacupx&se=2024-12-31T23:59:59Z&st=2023-01-01T00:00:00Z&spr=https&sig=testSignature"
	if conf.SASToken != expectedSASToken {
		t.Errorf("expected SASToken '%s', got %q", expectedSASToken, conf.SASToken)
	}
	if conf.Endpoint != "https://teststorageaccount.blob.core.windows.net" {
		t.Errorf("expected Endpoint 'https://teststorageaccount.blob.core.windows.net', got %q", conf.Endpoint)
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

	// Since the "azure" key is missing, all fields should be empty.
	if conf.StorageAccount != "" || conf.SASToken != "" || conf.Endpoint != "" {
		t.Errorf("expected empty Config when key is missing, got %+v", conf)
	}
}

// TestNewConfig_PartialConfig verifies that newConfig correctly handles partial configuration
// where only some fields are provided.
func TestNewConfig_PartialConfig(t *testing.T) {
	// YAML content with only storageAccount and sasToken (endpoint is optional).
	const yamlContent = `
azure:
  storageAccount: "teststorageaccount"
  sasToken: "sv=2022-11-02&ss=b&srt=sco&sp=rwdlacupx&se=2024-12-31T23:59:59Z&st=2023-01-01T00:00:00Z&spr=https&sig=testSignature"
`

	provider, err := config.NewYAMLProviderFromBytes([]byte(yamlContent))
	if err != nil {
		t.Fatalf("failed to create YAML provider: %v", err)
	}

	conf, err := newConfig(provider)
	if err != nil {
		t.Fatalf("newConfig returned error: %v", err)
	}

	// Validate that the provided fields are populated and endpoint is empty (as expected).
	if conf.StorageAccount != "teststorageaccount" {
		t.Errorf("expected StorageAccount 'teststorageaccount', got %q", conf.StorageAccount)
	}
	expectedSASToken := "sv=2022-11-02&ss=b&srt=sco&sp=rwdlacupx&se=2024-12-31T23:59:59Z&st=2023-01-01T00:00:00Z&spr=https&sig=testSignature"
	if conf.SASToken != expectedSASToken {
		t.Errorf("expected SASToken '%s', got %q", expectedSASToken, conf.SASToken)
	}
	if conf.Endpoint != "" {
		t.Errorf("expected empty Endpoint when not provided, got %q", conf.Endpoint)
	}
}