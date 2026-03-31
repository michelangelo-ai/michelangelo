package minio

import (
	"context"
	"testing"

	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	"go.uber.org/config"
)

// TestNewClient_StaticCredentials tests the newClient function with static credentials
func TestNewClient_StaticCredentials(t *testing.T) {
	config := Config{
		AwsRegion:          "us-west-2",
		AwsAccessKeyId:     "testAccessKey",
		AwsSecretAccessKey: "testSecretKey",
		AwsEndpointUrl:     "localhost:9000",
		UseEnvAws:          false,
		UseIAM:             false,
	}

	out, err := newClient(config)
	if err != nil {
		t.Fatalf("newClient returned error: %v", err)
	}

	// Verify the client implements BlobStoreClient interface
	var _ blobstore.BlobStoreClient = out.BlobStoreClient

	// Verify the scheme is correct
	if out.BlobStoreClient.Scheme() != "s3" {
		t.Errorf("expected scheme 's3', got %q", out.BlobStoreClient.Scheme())
	}
}

// TestNewClient_EnvCredentials tests the newClient function with environment AWS credentials
func TestNewClient_EnvCredentials(t *testing.T) {
	config := Config{
		AwsEndpointUrl: "localhost:9000",
		UseEnvAws:      true,
		UseIAM:         false,
	}

	out, err := newClient(config)
	if err != nil {
		t.Fatalf("newClient returned error: %v", err)
	}

	// Verify the client implements BlobStoreClient interface
	var _ blobstore.BlobStoreClient = out.BlobStoreClient

	// Verify the scheme is correct
	if out.BlobStoreClient.Scheme() != "s3" {
		t.Errorf("expected scheme 's3', got %q", out.BlobStoreClient.Scheme())
	}
}

// TestNewClient_IAMCredentials tests the newClient function with IAM credentials
func TestNewClient_IAMCredentials(t *testing.T) {
	config := Config{
		AwsEndpointUrl: "localhost:9000",
		UseEnvAws:      false,
		UseIAM:         true,
	}

	out, err := newClient(config)
	if err != nil {
		t.Fatalf("newClient returned error: %v", err)
	}

	// Verify the client implements BlobStoreClient interface
	var _ blobstore.BlobStoreClient = out.BlobStoreClient

	// Verify the scheme is correct
	if out.BlobStoreClient.Scheme() != "s3" {
		t.Errorf("expected scheme 's3', got %q", out.BlobStoreClient.Scheme())
	}
}

// TestNewConfig_WithUseEnvAws tests config loading with UseEnvAws flag
func TestNewConfig_WithUseEnvAws(t *testing.T) {
	const yamlContent = `
minio:
  awsEndpointUrl: "http://localhost:9000"
  useEnvAws: true
  useIam: false
`

	provider, err := config.NewYAMLProviderFromBytes([]byte(yamlContent))
	if err != nil {
		t.Fatalf("failed to create YAML provider: %v", err)
	}

	conf, err := newConfig(provider)
	if err != nil {
		t.Fatalf("newConfig returned error: %v", err)
	}

	if !conf.UseEnvAws {
		t.Errorf("expected UseEnvAws to be true, got %v", conf.UseEnvAws)
	}
	if conf.UseIAM {
		t.Errorf("expected UseIAM to be false, got %v", conf.UseIAM)
	}
}

// TestNewConfig_WithUseIAM tests config loading with UseIAM flag
func TestNewConfig_WithUseIAM(t *testing.T) {
	const yamlContent = `
minio:
  awsEndpointUrl: "http://localhost:9000"
  useEnvAws: false
  useIam: true
`

	provider, err := config.NewYAMLProviderFromBytes([]byte(yamlContent))
	if err != nil {
		t.Fatalf("failed to create YAML provider: %v", err)
	}

	conf, err := newConfig(provider)
	if err != nil {
		t.Fatalf("newConfig returned error: %v", err)
	}

	if conf.UseEnvAws {
		t.Errorf("expected UseEnvAws to be false, got %v", conf.UseEnvAws)
	}
	if !conf.UseIAM {
		t.Errorf("expected UseIAM to be true, got %v", conf.UseIAM)
	}
}

// TestMinioClient_Scheme tests that the client returns the correct scheme
func TestMinioClient_Scheme(t *testing.T) {
	client := &minioClient{scheme: "s3"}
	if client.Scheme() != "s3" {
		t.Errorf("expected scheme 's3', got %q", client.Scheme())
	}
}

// TestMinioClient_Get_InvalidURL tests error handling for invalid URLs
func TestMinioClient_Get_InvalidURL(t *testing.T) {
	client := &minioClient{scheme: "s3"}
	ctx := context.Background()

	// Test with invalid URL
	_, err := client.Get(ctx, "://invalid-url")
	if err == nil {
		t.Error("expected error for invalid URL, got nil")
	}
}

// TestMinioClient_Get_WrongScheme tests error handling for wrong scheme
func TestMinioClient_Get_WrongScheme(t *testing.T) {
	client := &minioClient{scheme: "s3"}
	ctx := context.Background()

	// Test with wrong scheme
	_, err := client.Get(ctx, "gs://bucket/path")
	if err == nil {
		t.Error("expected error for wrong scheme, got nil")
	}
	expectedMsg := "scheme gs is not supported by minio client"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message %q, got %q", expectedMsg, err.Error())
	}
}

// TestCredentialSelection tests that the correct credential type is chosen based on config
func TestCredentialSelection(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		expectErr bool
	}{
		{
			name: "static credentials",
			config: Config{
				AwsAccessKeyId:     "key",
				AwsSecretAccessKey: "secret",
				AwsEndpointUrl:     "localhost:9000",
				UseEnvAws:          false,
				UseIAM:             false,
			},
			expectErr: false,
		},
		{
			name: "env aws credentials",
			config: Config{
				AwsEndpointUrl: "localhost:9000",
				UseEnvAws:      true,
				UseIAM:         false,
			},
			expectErr: false,
		},
		{
			name: "iam credentials",
			config: Config{
				AwsEndpointUrl: "localhost:9000",
				UseEnvAws:      false,
				UseIAM:         true,
			},
			expectErr: false,
		},
		{
			name: "precedence: env over static",
			config: Config{
				AwsAccessKeyId:     "key",
				AwsSecretAccessKey: "secret",
				AwsEndpointUrl:     "localhost:9000",
				UseEnvAws:          true,
				UseIAM:             false,
			},
			expectErr: false,
		},
		{
			name: "precedence: env over iam",
			config: Config{
				AwsEndpointUrl: "localhost:9000",
				UseEnvAws:      true,
				UseIAM:         true,
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := newClient(tt.config)
			if (err != nil) != tt.expectErr {
				t.Errorf("newClient() error = %v, expectErr %v", err, tt.expectErr)
				return
			}
			if !tt.expectErr && out.BlobStoreClient == nil {
				t.Error("expected BlobStoreClient to be set, got nil")
			}
		})
	}
}
