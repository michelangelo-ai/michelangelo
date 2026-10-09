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
	if !conf.configured {
		t.Error("expected configured to be true when gcs section is present")
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
	if conf.configured {
		t.Error("expected configured to be false when gcs section is absent")
	}
}

func TestNewConfig_EmptySection(t *testing.T) {
	// A bare "gcs:" key with no fields still declares intent to use GCS
	// (with Application Default Credentials), so it must count as
	// configured.
	yamlContent := `
gcs:
`
	provider, err := config.NewYAMLProviderFromBytes([]byte(yamlContent))
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	conf, err := newConfig(provider)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if conf.CredentialsFile != "" || conf.Endpoint != "" || conf.Anonymous {
		t.Errorf("expected zero-value fields for empty section, got %+v", conf)
	}
	if !conf.configured {
		t.Error("expected configured to be true for an empty gcs section")
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

func TestConfiguredSectionFailsFastEndToEnd(t *testing.T) {
	// With a gcs section declared, a broken configuration must surface as
	// a constructor error (which fails fx application startup) instead of
	// waiting for the first gs:// read.
	yamlContent := `
gcs:
  credentialsFile: /nonexistent/credentials.json
`
	provider, err := config.NewYAMLProviderFromBytes([]byte(yamlContent))
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	conf, err := newConfig(provider)
	if err != nil {
		t.Fatalf("expected no config error, got %v", err)
	}
	if _, err := newClient(conf); err == nil {
		t.Fatal("expected eager construction error for broken gcs config, got nil")
	}
}

func TestAbsentSectionStaysLazyEndToEnd(t *testing.T) {
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
		t.Fatalf("expected no config error, got %v", err)
	}
	out, err := newClient(conf)
	if err != nil {
		t.Fatalf("expected lazy construction to succeed, got %v", err)
	}
	if out.BlobStoreClient.(*gcsBlobClient).client != nil {
		t.Error("expected storage client to remain unconstructed without a gcs section")
	}
}
