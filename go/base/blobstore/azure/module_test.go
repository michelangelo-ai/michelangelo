package azure

import (
	"testing"
)

func TestNewClient_EmptyConfig_ReturnsEmptyList(t *testing.T) {
	config := Config{}

	clients, err := newClient(config)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(clients) != 0 {
		t.Fatalf("expected 0 clients for empty config, got %d", len(clients))
	}
}

func TestNewClient_MultipleAzureProviders_CreatesMultipleClients(t *testing.T) {
	config := Config{
		StorageProviders: map[string]StorageProvider{
			"azure-dev": {
				Type:                "azure",
				AzureStorageAccount: "devaccount",
				AzureSASToken:       "devtoken",
			},
			"azure-prod": {
				Type:                "azure",
				AzureStorageAccount: "prodaccount",
				AzureSASToken:       "prodtoken",
				AzureEndpoint:       "https://custom.endpoint.net",
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

	// Check provider keys and schemes
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
	if !providerKeys["azure-dev"] {
		t.Error("expected azure-dev provider key")
	}
	if !providerKeys["azure-prod"] {
		t.Error("expected azure-prod provider key")
	}

	// All should have abfss scheme
	if !schemes["abfss"] {
		t.Error("expected abfss scheme")
	}
	if len(schemes) != 1 {
		t.Errorf("expected only 1 scheme, got %d: %v", len(schemes), schemes)
	}
}

func TestNewClient_NonAzureProviders_SkipsNonAzure(t *testing.T) {
	config := Config{
		StorageProviders: map[string]StorageProvider{
			"azure-dev": {
				Type:                "azure",
				AzureStorageAccount: "devaccount",
				AzureSASToken:       "devtoken",
			},
			"s3-prod": {
				Type: "s3", // Non-Azure provider should be skipped
			},
		},
	}

	clients, err := newClient(config)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Should only create 1 Azure client, skipping the S3 provider
	if len(clients) != 1 {
		t.Fatalf("expected 1 Azure client, got %d", len(clients))
	}

	client := clients[0].BlobStoreClient
	if client.Scheme() != "abfss" {
		t.Errorf("expected scheme 'abfss', got %q", client.Scheme())
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