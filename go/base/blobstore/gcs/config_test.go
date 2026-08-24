package gcs

import (
	"testing"

	"go.uber.org/config"
)

func TestNewConfig_Success(t *testing.T) {
	yamlContent := `
gcs:
  credentialsFile: /etc/gcs/credentials.json
  endpoint: https://storage.googleapis.com
  anonymous: false
`
	provider, err := config.NewYAMLProviderFromBytes([]byte(yamlContent))
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	conf, err := newConfig(provider)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if conf.CredentialsFile != "/etc/gcs/credentials.json" {
		t.Errorf("expected credentialsFile to be /etc/gcs/credentials.json, got %s", conf.CredentialsFile)
	}
	if conf.Endpoint != "https://storage.googleapis.com" {
		t.Errorf("expected endpoint to be https://storage.googleapis.com, got %s", conf.Endpoint)
	}
	if conf.Anonymous {
		t.Errorf("expected anonymous to be false, got true")
	}
}

func TestNewConfig_MissingKey(t *testing.T) {
	yamlContent := `
otherKey:
  value: something
`
	provider, err := config.NewYAMLProviderFromBytes([]byte(yamlContent))
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	conf, err := newConfig(provider)
	if err != nil {
		t.Fatalf("expected no error for missing key, got %v", err)
	}
	if conf.CredentialsFile != "" || conf.Endpoint != "" || conf.Anonymous {
		t.Errorf("expected zero-value config for missing key, got %+v", conf)
	}
}

func TestNewConfig_PartialConfig(t *testing.T) {
	yamlContent := `
gcs:
  credentialsFile: /path/to/key.json
`
	provider, err := config.NewYAMLProviderFromBytes([]byte(yamlContent))
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	conf, err := newConfig(provider)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if conf.CredentialsFile != "/path/to/key.json" {
		t.Errorf("expected credentialsFile to be /path/to/key.json, got %s", conf.CredentialsFile)
	}
	if conf.Endpoint != "" {
		t.Errorf("expected endpoint to be empty, got %s", conf.Endpoint)
	}
	if conf.Anonymous {
		t.Errorf("expected anonymous to be false, got true")
	}
}
