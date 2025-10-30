package blobstore

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

func TestContextAwareBlobStore_Get_WithProvider(t *testing.T) {
	// Create mock clients
	client1 := &mockBlobStoreClient{
		scheme:      "s3",
		providerKey: "aws-prod",
		readFn: func(ctx context.Context, blobURI string) ([]byte, error) {
			return []byte(`{"data": "aws-prod"}`), nil
		},
	}

	client2 := &mockBlobStoreClient{
		scheme:      "s3",
		providerKey: "aws-dev",
		readFn: func(ctx context.Context, blobURI string) ([]byte, error) {
			return []byte(`{"data": "aws-dev"}`), nil
		},
	}

	// Setup blobstore
	blobStore := &BlobStore{}
	blobStore.RegisterClient(client1)
	blobStore.RegisterClient(client2)

	// Create context-aware wrapper
	logger := zap.NewNop()
	contextAware := NewContextAwareBlobStore(blobStore, logger)

	// Test with aws-prod provider in context
	ctx := WithStorageProvider(context.Background(), "aws-prod")
	data, err := contextAware.Get(ctx, "s3://bucket/file")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	expected := `{"data": "aws-prod"}`
	if string(data) != expected {
		t.Errorf("Expected %s, got %s", expected, string(data))
	}

	// Test with aws-dev provider in context
	ctx = WithStorageProvider(context.Background(), "aws-dev")
	data, err = contextAware.Get(ctx, "s3://bucket/file")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	expected = `{"data": "aws-dev"}`
	if string(data) != expected {
		t.Errorf("Expected %s, got %s", expected, string(data))
	}
}

func TestContextAwareBlobStore_Get_WithoutProvider(t *testing.T) {
	// Create mock client (only scheme-based)
	client := &mockLegacyClient{
		scheme: "s3",
		readFn: func(ctx context.Context, blobURI string) ([]byte, error) {
			return []byte(`{"data": "fallback"}`), nil
		},
	}

	// Setup blobstore
	blobStore := &BlobStore{}
	blobStore.RegisterClient(client)

	// Create context-aware wrapper
	logger := zap.NewNop()
	contextAware := NewContextAwareBlobStore(blobStore, logger)

	// Test without provider in context (should fallback to scheme-based)
	ctx := context.Background()
	data, err := contextAware.Get(ctx, "s3://bucket/file")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	expected := `{"data": "fallback"}`
	if string(data) != expected {
		t.Errorf("Expected %s, got %s", expected, string(data))
	}
}

func TestContextProviderHelpers(t *testing.T) {
	// Test WithStorageProvider and GetStorageProvider
	ctx := context.Background()

	// Test empty context
	provider := GetStorageProvider(ctx)
	if provider != "" {
		t.Errorf("Expected empty provider, got %s", provider)
	}

	// Test with provider
	ctx = WithStorageProvider(ctx, "aws-company1")
	provider = GetStorageProvider(ctx)
	if provider != "aws-company1" {
		t.Errorf("Expected aws-company1, got %s", provider)
	}

	// Test overriding provider
	ctx = WithStorageProvider(ctx, "azure-sharezone")
	provider = GetStorageProvider(ctx)
	if provider != "azure-sharezone" {
		t.Errorf("Expected azure-sharezone, got %s", provider)
	}
}