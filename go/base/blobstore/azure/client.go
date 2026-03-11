package azure

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
)

var _ blobstore.BlobStoreClient = (*azureBlobClient)(nil)

// azureBlobClient is a client for Azure Blob Storage with SAS token authentication.
// It provides methods to interact with Azure Blob Storage using container-level SAS tokens.
type azureBlobClient struct {
	storageAccount string
	sasToken       string
	endpoint       string
	httpClient     *http.Client
	scheme         string
}

// newAzureBlobClient creates a new Azure Blob Storage client with SAS token authentication.
func newAzureBlobClient(storageAccount, sasToken, endpoint string) *azureBlobClient {
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://%s.blob.core.windows.net", storageAccount)
	}

	return &azureBlobClient{
		storageAccount: storageAccount,
		sasToken:       sasToken,
		endpoint:       endpoint,
		httpClient:     &http.Client{},
		scheme:         "abfss", // Support ABFSS scheme for Azure Data Lake Storage Gen2
	}
}

// Get retrieves an object from Azure Blob Storage using SAS token authentication.
// The blobURI is expected to be in the format "abfss://container@storageaccount.blob.core.windows.net/path".
func (c *azureBlobClient) Get(ctx context.Context, blobURI string) ([]byte, error) {
	parsedURL, err := url.Parse(blobURI)
	if err != nil {
		return nil, fmt.Errorf("failed to parse url: %w", err)
	}

	if parsedURL.Scheme != c.scheme {
		return nil, fmt.Errorf("scheme %s is not supported by azure blob client", parsedURL.Scheme)
	}

	container, blobPath := c.parseABFSSURL(parsedURL)
	blobURL := c.buildBlobURL(container, blobPath)

	return c.downloadBlob(ctx, blobURI, blobURL)
}

// parseABFSSURL extracts container and blob path from an ABFSS URL.
// ABFSS URLs use the format: abfss://container@storageaccount.blob.core.windows.net/path
// The @ symbol is treated as userinfo by url.Parse, so container is in parsedURL.User.Username()
func (c *azureBlobClient) parseABFSSURL(parsedURL *url.URL) (container, blobPath string) {
	if parsedURL.User != nil && parsedURL.User.Username() != "" {
		// Standard ABFSS format: container@storageaccount.blob.core.windows.net
		container = parsedURL.User.Username()
		blobPath = strings.TrimPrefix(parsedURL.Path, "/")
	} else if strings.Contains(parsedURL.Host, "@") {
		// Alternative parsing if URL parsing doesn't handle userinfo properly
		parts := strings.Split(parsedURL.Host, "@")
		container = parts[0]
		blobPath = strings.TrimPrefix(parsedURL.Path, "/")
	} else {
		// Fallback to simple format: abfss://container/path
		container = parsedURL.Host
		blobPath = strings.TrimPrefix(parsedURL.Path, "/")
	}
	return container, blobPath
}

// buildBlobURL constructs the Azure Blob Storage REST API URL with SAS token.
// Format: https://{storageaccount}.blob.core.windows.net/{container}/{blob}?{sas_token}
func (c *azureBlobClient) buildBlobURL(container, blobPath string) string {
	return fmt.Sprintf("%s/%s/%s?%s", c.endpoint, container, blobPath, c.sasToken)
}

// downloadBlob performs the HTTP request to download the blob data.
func (c *azureBlobClient) downloadBlob(ctx context.Context, originalURI, blobURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", blobURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for %s: %w", originalURI, err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get object from %s: %w", originalURI, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get object from %s: HTTP %d - URL: %s", originalURI, resp.StatusCode, blobURL)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read object from %s: %w", originalURI, err)
	}

	return data, nil
}

// Scheme returns the scheme identifier used by this client.
func (c *azureBlobClient) Scheme() string {
	return c.scheme
}

// Put uploads data to Azure Blob Storage at the given URI.
// The blobURI is expected to be in the format "abfss://container@storageaccount.blob.core.windows.net/path".
func (c *azureBlobClient) Put(ctx context.Context, blobURI string, data []byte) error {
	parsedURL, err := url.Parse(blobURI)
	if err != nil {
		return fmt.Errorf("failed to parse url: %w", err)
	}

	if parsedURL.Scheme != c.scheme {
		return fmt.Errorf("scheme %s is not supported by azure blob client", parsedURL.Scheme)
	}

	container, blobPath := c.parseABFSSURL(parsedURL)
	blobURL := c.buildBlobURL(container, blobPath)

	req, err := http.NewRequestWithContext(ctx, "PUT", blobURL, strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("failed to create request for %s: %w", blobURI, err)
	}
	req.Header.Set("x-ms-blob-type", "BlockBlob")
	req.ContentLength = int64(len(data))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to put object to %s: %w", blobURI, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to put object to %s: HTTP %d", blobURI, resp.StatusCode)
	}

	return nil
}

// Delete removes a blob from Azure Blob Storage at the given URI.
// The blobURI is expected to be in the format "abfss://container@storageaccount.blob.core.windows.net/path".
func (c *azureBlobClient) Delete(ctx context.Context, blobURI string) error {
	parsedURL, err := url.Parse(blobURI)
	if err != nil {
		return fmt.Errorf("failed to parse url: %w", err)
	}

	if parsedURL.Scheme != c.scheme {
		return fmt.Errorf("scheme %s is not supported by azure blob client", parsedURL.Scheme)
	}

	container, blobPath := c.parseABFSSURL(parsedURL)
	blobURL := c.buildBlobURL(container, blobPath)

	req, err := http.NewRequestWithContext(ctx, "DELETE", blobURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request for %s: %w", blobURI, err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete object from %s: %w", blobURI, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to delete object from %s: HTTP %d", blobURI, resp.StatusCode)
	}

	return nil
}
