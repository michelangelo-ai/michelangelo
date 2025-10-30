package minio

import (
	"testing"
)

func TestNewClient_EmptyConfig_CreatesDefaultClient(t *testing.T) {
	config := Config{}

	clients, err := newClient(config)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(clients) != 1 {
		t.Fatalf("expected 1 default client, got %d", len(clients))
	}

	// Check that the client has the correct provider key
	client := clients[0].BlobStoreClient
	if providerClient, ok := client.(interface{ ProviderKey() string }); ok {
		if providerClient.ProviderKey() != "aws-sandbox" {
			t.Errorf("expected provider key 'aws-sandbox', got %q", providerClient.ProviderKey())
		}
	} else {
		t.Errorf("expected client to implement ProviderKey method")
	}

	if client.Scheme() != "s3" {
		t.Errorf("expected scheme 's3', got %q", client.Scheme())
	}
}

func TestNewClient_MultipleProviders_CreatesMultipleClients(t *testing.T) {
	config := Config{
		{
			Name: "aws-prod",
			StorageProvider: StorageProvider{
				AwsRegion:          "us-west-2",
				AwsAccessKeyId:     "testkey",
				AwsSecretAccessKey: "testsecret",
				AwsEndpointUrl:     "s3.amazonaws.com",
			},
		},
		{
			Name: "aws-dev",
			StorageProvider: StorageProvider{
				AwsRegion:          "us-east-1",
				AwsAccessKeyId:     "devkey",
				AwsSecretAccessKey: "devsecret",
				AwsEndpointUrl:     "localhost:9000",
			},
		},
	}

	clients, err := newClient(config)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(clients) != 2 {
		t.Fatalf("expected 2 clients, got %d", len(clients))
	}

	// Check provider keys
	providerKeys := make(map[string]bool)
	schemes := make(map[string]bool)

	for _, clientOut := range clients {
		client := clientOut.BlobStoreClient
		schemes[client.Scheme()] = true

		if providerClient, ok := client.(interface{ ProviderKey() string }); ok {
			providerKeys[providerClient.ProviderKey()] = true
		}
	}

	// Should have both provider keys
	if !providerKeys["aws-prod"] {
		t.Error("expected aws-prod provider key")
	}
	if !providerKeys["aws-dev"] {
		t.Error("expected aws-dev provider key")
	}

	// Should have s3 scheme
	if !schemes["s3"] {
		t.Error("expected s3 scheme")
	}
	if len(schemes) != 1 {
		t.Errorf("expected only 1 scheme, got %d: %v", len(schemes), schemes)
	}
}

func TestNewS3ClientWithKey_UseEnvAws(t *testing.T) {
	config := StorageProvider{
		UseEnvAws: true,
	}

	client, err := newS3ClientWithKey("test-provider", config)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	blobClient := client.BlobStoreClient
	if blobClient.Scheme() != "s3" {
		t.Errorf("expected scheme 's3', got %q", blobClient.Scheme())
	}

	if providerClient, ok := blobClient.(interface{ ProviderKey() string }); ok {
		if providerClient.ProviderKey() != "test-provider" {
			t.Errorf("expected provider key 'test-provider', got %q", providerClient.ProviderKey())
		}
	} else {
		t.Error("expected client to implement ProviderKey method")
	}
}

func TestNewS3ClientWithKey_StaticCredentials(t *testing.T) {
	config := StorageProvider{
		AwsAccessKeyId:     "testkey",
		AwsSecretAccessKey: "testsecret",
		AwsEndpointUrl:     "localhost:9000",
	}

	client, err := newS3ClientWithKey("test-provider", config)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	blobClient := client.BlobStoreClient
	if blobClient.Scheme() != "s3" {
		t.Errorf("expected scheme 's3', got %q", blobClient.Scheme())
	}
}

func TestNewClient_SingleProvider_CreatesOneClient(t *testing.T) {
	config := Config{
		{
			Name: "aws-dev",
			StorageProvider: StorageProvider{
				AwsAccessKeyId:     "devkey",
				AwsSecretAccessKey: "devsecret",
			},
		},
	}

	clients, err := newClient(config)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Should create 1 S3 client
	if len(clients) != 1 {
		t.Fatalf("expected 1 S3 client, got %d", len(clients))
	}

	client := clients[0].BlobStoreClient
	if client.Scheme() != "s3" {
		t.Errorf("expected scheme 's3', got %q", client.Scheme())
	}
}
