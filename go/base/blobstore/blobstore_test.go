package blobstore

import (
	"context"
	"errors"
	"testing"
)

type mockBlobStoreClient struct {
	scheme      string
	providerKey string
	readFn      func(ctx context.Context, path string) ([]byte, error)
}

func (m *mockBlobStoreClient) Get(ctx context.Context, path string) ([]byte, error) {
	return m.readFn(ctx, path)
}

func (m *mockBlobStoreClient) Scheme() string {
	return m.scheme
}

func (m *mockBlobStoreClient) ProviderKey() string {
	return m.providerKey
}

func TestBlobStore_Get(t *testing.T) {
	bs := BlobStore{}
	bs.RegisterClient(&mockBlobStoreClient{scheme: "test", readFn: func(ctx context.Context, blobURI string) ([]byte, error) {
		return []byte("test"), nil
	}})
	result, err := bs.Get(context.Background(), "test://test")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if string(result) != "test" {
		t.Fatalf("expected test, got %v", result)
	}
}

func TestBlobStore_Get_Error(t *testing.T) {
	bs := BlobStore{}
	bs.RegisterClient(&mockBlobStoreClient{scheme: "test", readFn: func(ctx context.Context, blobURI string) ([]byte, error) {
		return nil, errors.New("test error")
	}})
	result, err := bs.Get(context.Background(), "test://test")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}
}

func TestBlobStore_Get_UnsupportedScheme(t *testing.T) {
	bs := BlobStore{}
	result, err := bs.Get(context.Background(), "unsupported://test")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}
}

// Test provider-based functionality
func TestBlobStore_RegisterClient_WithProvider(t *testing.T) {
	bs := BlobStore{}
	client := &mockBlobStoreClient{
		scheme:      "s3",
		providerKey: "aws-prod",
		readFn: func(ctx context.Context, blobURI string) ([]byte, error) {
			return []byte("aws-prod-data"), nil
		},
	}

	bs.RegisterClient(client)

	// Test that it's registered by scheme
	if len(bs.Clients) != 1 {
		t.Fatalf("expected 1 client by scheme, got %d", len(bs.Clients))
	}

	// Test that it's registered by provider key
	if len(bs.ProviderClients) != 1 {
		t.Fatalf("expected 1 client by provider key, got %d", len(bs.ProviderClients))
	}

	// Test retrieval by provider key
	retrievedClient, err := bs.GetClientByProvider("aws-prod")
	if err != nil {
		t.Fatalf("expected no error getting client by provider, got %v", err)
	}
	if retrievedClient != client {
		t.Fatalf("expected same client instance")
	}
}

func TestBlobStore_GetWithProvider(t *testing.T) {
	bs := BlobStore{}

	// Register multiple providers
	awsProd := &mockBlobStoreClient{
		scheme:      "s3",
		providerKey: "aws-prod",
		readFn: func(ctx context.Context, blobURI string) ([]byte, error) {
			return []byte("aws-prod-data"), nil
		},
	}

	awsDev := &mockBlobStoreClient{
		scheme:      "s3",
		providerKey: "aws-dev",
		readFn: func(ctx context.Context, blobURI string) ([]byte, error) {
			return []byte("aws-dev-data"), nil
		},
	}

	azureDev := &mockBlobStoreClient{
		scheme:      "abfss",
		providerKey: "azure-dev",
		readFn: func(ctx context.Context, blobURI string) ([]byte, error) {
			return []byte("azure-dev-data"), nil
		},
	}

	bs.RegisterClient(awsProd)
	bs.RegisterClient(awsDev)
	bs.RegisterClient(azureDev)

	// Test specific provider access
	result, err := bs.GetWithProvider(context.Background(), "s3://bucket/file", "aws-prod")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if string(result) != "aws-prod-data" {
		t.Fatalf("expected aws-prod-data, got %s", string(result))
	}

	// Test different provider
	result, err = bs.GetWithProvider(context.Background(), "s3://bucket/file", "aws-dev")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if string(result) != "aws-dev-data" {
		t.Fatalf("expected aws-dev-data, got %s", string(result))
	}

	// Test Azure provider
	result, err = bs.GetWithProvider(context.Background(), "abfss://container@account.blob.core.windows.net/file", "azure-dev")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if string(result) != "azure-dev-data" {
		t.Fatalf("expected azure-dev-data, got %s", string(result))
	}
}

func TestBlobStore_GetWithProvider_FallbackToScheme(t *testing.T) {
	bs := BlobStore{}
	client := &mockBlobStoreClient{
		scheme:      "s3",
		providerKey: "aws-prod",
		readFn: func(ctx context.Context, blobURI string) ([]byte, error) {
			return []byte("fallback-data"), nil
		},
	}

	bs.RegisterClient(client)

	// Test fallback to scheme-based routing when empty provider key
	result, err := bs.GetWithProvider(context.Background(), "s3://bucket/file", "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if string(result) != "fallback-data" {
		t.Fatalf("expected fallback-data, got %s", string(result))
	}
}

func TestBlobStore_GetClientByProvider_NotFound(t *testing.T) {
	bs := BlobStore{}

	_, err := bs.GetClientByProvider("nonexistent-provider")
	if err == nil {
		t.Fatalf("expected error for nonexistent provider, got nil")
	}

	expectedError := "provider nonexistent-provider is not configured"
	if err.Error() != expectedError {
		t.Fatalf("expected error %q, got %q", expectedError, err.Error())
	}
}

func TestBlobStore_GetWithProvider_InvalidProvider(t *testing.T) {
	bs := BlobStore{}

	_, err := bs.GetWithProvider(context.Background(), "s3://bucket/file", "invalid-provider")
	if err == nil {
		t.Fatalf("expected error for invalid provider, got nil")
	}
}

func TestBlobStore_RegisterClient_LegacyClient(t *testing.T) {
	bs := BlobStore{}

	// Create a client that doesn't implement ProviderClient interface
	legacyClient := &mockLegacyClient{
		scheme: "legacy",
		readFn: func(ctx context.Context, blobURI string) ([]byte, error) {
			return []byte("legacy-data"), nil
		},
	}

	bs.RegisterClient(legacyClient)

	// Should be registered by scheme but not by provider
	if len(bs.Clients) != 1 {
		t.Fatalf("expected 1 client by scheme, got %d", len(bs.Clients))
	}
	if len(bs.ProviderClients) != 0 {
		t.Fatalf("expected 0 clients by provider key, got %d", len(bs.ProviderClients))
	}
}

// Legacy client that doesn't implement ProviderClient interface
type mockLegacyClient struct {
	scheme string
	readFn func(ctx context.Context, path string) ([]byte, error)
}

func (m *mockLegacyClient) Get(ctx context.Context, path string) ([]byte, error) {
	return m.readFn(ctx, path)
}

func (m *mockLegacyClient) Scheme() string {
	return m.scheme
}
