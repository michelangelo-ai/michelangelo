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
		StorageProviders: map[string]StorageProvider{
			"aws-prod": {
				Type:               "s3",
				AwsRegion:          "us-west-2",
				AwsAccessKeyId:     "testkey",
				AwsSecretAccessKey: "testsecret",
				AwsEndpointUrl:     "s3.amazonaws.com",
			},
			"azure-dev": {
				Type:                "azure",
				AzureStorageAccount: "testaccount",
				AzureSASToken:       "testtoken",
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
	if !providerKeys["azure-dev"] {
		t.Error("expected azure-dev provider key")
	}

	// Should have both schemes
	if !schemes["s3"] {
		t.Error("expected s3 scheme")
	}
	if !schemes["abfss"] {
		t.Error("expected abfss scheme")
	}
}

func TestNewS3ClientWithKey_UseEnvAws(t *testing.T) {
	config := StorageProvider{
		Type:      "s3",
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
		Type:               "s3",
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

func TestNewAzureClientWithKey_Success(t *testing.T) {
	config := StorageProvider{
		Type:                "azure",
		AzureStorageAccount: "testaccount",
		AzureSASToken:       "testtoken",
	}

	client, err := newAzureClientWithKey("test-azure", config)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	blobClient := client.BlobStoreClient
	if blobClient.Scheme() != "abfss" {
		t.Errorf("expected scheme 'abfss', got %q", blobClient.Scheme())
	}

	if providerClient, ok := blobClient.(interface{ ProviderKey() string }); ok {
		if providerClient.ProviderKey() != "test-azure" {
			t.Errorf("expected provider key 'test-azure', got %q", providerClient.ProviderKey())
		}
	} else {
		t.Error("expected client to implement ProviderKey method")
	}
}

func TestNewAzureClientWithKey_MissingStorageAccount(t *testing.T) {
	config := StorageProvider{
		Type:          "azure",
		AzureSASToken: "testtoken",
		// Missing AzureStorageAccount
	}

	_, err := newAzureClientWithKey("test-azure", config)
	if err == nil {
		t.Fatal("expected error for missing storage account")
	}

	expectedError := "azure storage account is required for provider test-azure"
	if err.Error() != expectedError {
		t.Errorf("expected error %q, got %q", expectedError, err.Error())
	}
}

func TestNewAzureClientWithKey_MissingSASToken(t *testing.T) {
	config := StorageProvider{
		Type:                "azure",
		AzureStorageAccount: "testaccount",
		// Missing AzureSASToken
	}

	_, err := newAzureClientWithKey("test-azure", config)
	if err == nil {
		t.Fatal("expected error for missing SAS token")
	}

	expectedError := "azure SAS token is required for provider test-azure"
	if err.Error() != expectedError {
		t.Errorf("expected error %q, got %q", expectedError, err.Error())
	}
}

func TestNewClient_UnsupportedProviderType(t *testing.T) {
	config := Config{
		StorageProviders: map[string]StorageProvider{
			"gcp-prod": {
				Type: "gcp", // Unsupported type
			},
		},
	}

	_, err := newClient(config)
	if err == nil {
		t.Fatal("expected error for unsupported provider type")
	}

	expectedError := "unsupported storage provider type: gcp for provider gcp-prod"
	if err.Error() != expectedError {
		t.Errorf("expected error %q, got %q", expectedError, err.Error())
	}
}