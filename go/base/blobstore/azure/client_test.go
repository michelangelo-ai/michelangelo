package azure

import (
	"net/url"
	"testing"
)

func TestAzureBlobClient_ProviderKey(t *testing.T) {
	client := newAzureBlobClient("testaccount", "testtoken", "", "azure-provider")

	if client.ProviderKey() != "azure-provider" {
		t.Errorf("expected provider key 'azure-provider', got %q", client.ProviderKey())
	}
}

func TestAzureBlobClient_Scheme(t *testing.T) {
	client := newAzureBlobClient("testaccount", "testtoken", "", "azure-provider")

	if client.Scheme() != "abfss" {
		t.Errorf("expected scheme 'abfss', got %q", client.Scheme())
	}
}

func TestNewAzureBlobClient_DefaultEndpoint(t *testing.T) {
	client := newAzureBlobClient("testaccount", "testtoken", "", "azure-provider")

	expectedEndpoint := "https://testaccount.blob.core.windows.net"
	if client.endpoint != expectedEndpoint {
		t.Errorf("expected endpoint %q, got %q", expectedEndpoint, client.endpoint)
	}
}

func TestNewAzureBlobClient_CustomEndpoint(t *testing.T) {
	customEndpoint := "https://custom.endpoint.net"
	client := newAzureBlobClient("testaccount", "testtoken", customEndpoint, "azure-provider")

	if client.endpoint != customEndpoint {
		t.Errorf("expected endpoint %q, got %q", customEndpoint, client.endpoint)
	}
}

func TestAzureBlobClient_ParseABFSSURL(t *testing.T) {
	client := newAzureBlobClient("testaccount", "testtoken", "", "azure-provider")

	tests := []struct {
		name              string
		url               string
		expectedContainer string
		expectedBlobPath  string
	}{
		{
			name:              "standard ABFSS URL with userinfo",
			url:               "abfss://mycontainer@testaccount.blob.core.windows.net/folder/file.txt",
			expectedContainer: "mycontainer",
			expectedBlobPath:  "folder/file.txt",
		},
		{
			name:              "ABFSS URL with @ in host",
			url:               "abfss://mycontainer@testaccount.blob.core.windows.net/file.txt",
			expectedContainer: "mycontainer",
			expectedBlobPath:  "file.txt",
		},
		{
			name:              "simple format",
			url:               "abfss://mycontainer/file.txt",
			expectedContainer: "mycontainer",
			expectedBlobPath:  "file.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsedURL, err := url.Parse(tt.url)
			if err != nil {
				t.Fatalf("failed to parse URL: %v", err)
			}

			container, blobPath := client.parseABFSSURL(parsedURL)

			if container != tt.expectedContainer {
				t.Errorf("expected container %q, got %q", tt.expectedContainer, container)
			}

			if blobPath != tt.expectedBlobPath {
				t.Errorf("expected blob path %q, got %q", tt.expectedBlobPath, blobPath)
			}
		})
	}
}

func TestAzureBlobClient_BuildBlobURL(t *testing.T) {
	sasToken := "sv=2022-11-02&ss=b&srt=sco&sp=rwdlacupx"
	client := newAzureBlobClient("testaccount", sasToken, "", "azure-provider")

	container := "mycontainer"
	blobPath := "folder/file.txt"

	blobURL := client.buildBlobURL(container, blobPath)

	expectedURL := "https://testaccount.blob.core.windows.net/mycontainer/folder/file.txt?" + sasToken
	if blobURL != expectedURL {
		t.Errorf("expected URL %q, got %q", expectedURL, blobURL)
	}
}

func TestAzureBlobClient_BuildBlobURL_CustomEndpoint(t *testing.T) {
	sasToken := "sv=2022-11-02&ss=b&srt=sco&sp=rwdlacupx"
	customEndpoint := "https://custom.endpoint.net"
	client := newAzureBlobClient("testaccount", sasToken, customEndpoint, "azure-provider")

	container := "mycontainer"
	blobPath := "folder/file.txt"

	blobURL := client.buildBlobURL(container, blobPath)

	expectedURL := "https://custom.endpoint.net/mycontainer/folder/file.txt?" + sasToken
	if blobURL != expectedURL {
		t.Errorf("expected URL %q, got %q", expectedURL, blobURL)
	}
}
