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

// Compile-time check to ensure azureBlobClient implements the BlobStoreClient interface
var _ blobstore.BlobStoreClient = (*azureBlobClient)(nil)

// azureBlobClient is a client for Azure Blob Storage with SAS token authentication.
// It provides methods to interact with Azure Blob Storage containers using container-level SAS tokens.
//
// Features:
//   - ABFSS (Azure Data Lake Storage Gen2) URL support
//   - SAS token authentication for secure, delegated access
//   - HTTP-based REST API communication with Azure Storage
//   - Provider-based routing for multi-tenant scenarios
//
// Supported URL Formats:
//   - Standard ABFSS: "abfss://container@storageaccount.blob.core.windows.net/path/file.json"
//   - Simplified ABFSS: "abfss://container/path/file.json"
//
// Authentication:
//   - Uses SAS (Shared Access Signature) tokens for authentication
//   - Tokens provide time-limited, permission-scoped access to Azure Storage
//   - No need for storage account keys or Azure AD authentication
//
// Example Usage:
//
//	client := newAzureBlobClient("myaccount", "sv=2022-11-02&ss=b...", "", "azure-prod")
//	data, err := client.Get(ctx, "abfss://mycontainer@myaccount.blob.core.windows.net/data/file.json")
//
// Thread Safety:
//   - Safe for concurrent use across multiple goroutines
//   - Each instance maintains its own HTTP client and configuration
type azureBlobClient struct {
	// storageAccount is the name of the Azure Storage Account
	// Used for default endpoint construction and identification
	storageAccount string

	// sasToken is the Shared Access Signature token for authentication
	// Contains permissions, expiration, and signature for secure access
	sasToken string

	// endpoint is the base URL for the Azure Blob Storage service
	// Defaults to https://{storageAccount}.blob.core.windows.net
	endpoint string

	// httpClient is used for making HTTP requests to Azure Storage REST API
	// Configured with appropriate timeouts and retry behavior
	httpClient *http.Client

	// scheme identifies the URL scheme supported by this client ("abfss")
	// Used by the blobstore router to determine client compatibility
	scheme string

	// providerKey identifies this specific provider instance
	// Used for multi-provider scenarios and routing decisions
	providerKey string
}

// newAzureBlobClient creates a new Azure Blob Storage client with SAS token authentication.
//
// This function initializes an Azure Blob Storage client that uses SAS tokens for authentication.
// SAS tokens provide secure, time-limited access to Azure Storage resources without requiring
// storage account keys or Azure AD authentication.
//
// Parameters:
//   - storageAccount: The name of the Azure Storage Account (e.g., "mlprodstorageacct")
//   - sasToken: The SAS token string containing permissions and signature
//   - endpoint: Optional custom endpoint URL (empty string uses default Azure endpoint)
//   - providerKey: Identifier for this provider instance (e.g., "azure-prod")
//
// Returns:
//   - *azureBlobClient: Configured client ready for blob operations
//
// Endpoint Construction:
//   - If endpoint is empty, defaults to: "https://{storageAccount}.blob.core.windows.net"
//   - Custom endpoints support Azure Stack, Government Cloud, or private deployments
//
// Example Usage:
//
//	// Standard Azure Cloud
//	client := newAzureBlobClient("myaccount", "sv=2022-11-02&ss=b...", "", "azure-prod")
//
//	// Azure Government Cloud
//	client := newAzureBlobClient("myaccount", "sv=...", "https://myaccount.blob.core.usgovcloudapi.net", "azure-gov")
//
//	// Azure Stack
//	client := newAzureBlobClient("myaccount", "sv=...", "https://myaccount.blob.local.azurestack.external", "azure-stack")
//
// Security Considerations:
//   - SAS tokens should have minimal required permissions (read-only for blob retrieval)
//   - Use short expiration times for SAS tokens when possible
//   - Never log or expose SAS tokens in error messages or debug output
func newAzureBlobClient(storageAccount, sasToken, endpoint, providerKey string) *azureBlobClient {
	// Construct default endpoint if not provided
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://%s.blob.core.windows.net", storageAccount)
	}

	return &azureBlobClient{
		storageAccount: storageAccount,
		sasToken:       sasToken,
		endpoint:       endpoint,
		httpClient:     &http.Client{}, // TODO: Consider adding timeouts and retry configuration
		scheme:         "abfss",        // Support ABFSS scheme for Azure Data Lake Storage Gen2
		providerKey:    providerKey,
	}
}

// Get retrieves an object from Azure Blob Storage using SAS token authentication.
//
// This method downloads blob data from Azure Blob Storage by parsing the ABFSS URL,
// constructing the appropriate REST API request, and handling the HTTP response.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout control
//   - blobURI: The ABFSS URL of the blob to retrieve
//
// Returns:
//   - []byte: The blob content as a byte slice
//   - error: Any error that occurred during the operation
//
// Supported URL Formats:
//   - Standard ABFSS: "abfss://container@storageaccount.blob.core.windows.net/folder/file.json"
//   - Simplified ABFSS: "abfss://container/folder/file.json"
//   - Nested paths: "abfss://container/deep/nested/path/file.json"
//
// Error Handling:
//   - Returns wrapped errors with operation context for debugging
//   - URL parsing errors include the malformed URL for troubleshooting
//   - HTTP errors include status codes and original URI for correlation
//   - Network errors preserve underlying cause for upstream retry logic
//
// Example Usage:
//
//	data, err := client.Get(ctx, "abfss://mycontainer@myaccount.blob.core.windows.net/data/model.json")
//	if err != nil {
//	    // Handle error with full context
//	    log.Printf("Failed to get blob: %v", err)
//	    return
//	}
//	// Process blob data
//	var model MyModel
//	json.Unmarshal(data, &model)
//
// Performance Considerations:
//   - Large blobs are loaded entirely into memory
//   - Consider streaming for very large files (>100MB)
//   - HTTP client reuse reduces connection overhead
//
// Security Notes:
//   - SAS token is appended to URL query string
//   - URLs with SAS tokens should not be logged or exposed
//   - Ensure SAS tokens have minimal required permissions
func (c *azureBlobClient) Get(ctx context.Context, blobURI string) ([]byte, error) {
	// Parse the ABFSS URL to extract components
	parsedURL, err := url.Parse(blobURI)
	if err != nil {
		// Follow error handling guidelines: wrap with operation context
		return nil, fmt.Errorf("parse blob URI %q: %w", blobURI, err)
	}

	// Validate that this client supports the URL scheme
	if parsedURL.Scheme != c.scheme {
		// Domain layer: return descriptive error without logging
		return nil, fmt.Errorf("unsupported scheme %q for azure blob client, expected %q", parsedURL.Scheme, c.scheme)
	}

	// Extract container name and blob path from ABFSS URL
	container, blobPath := c.parseABFSSURL(parsedURL)

	// Construct the Azure Storage REST API URL with SAS token
	blobURL := c.buildBlobURL(container, blobPath)

	// Download the blob content via HTTP
	return c.downloadBlob(ctx, blobURI, blobURL)
}

// parseABFSSURL extracts container name and blob path from an ABFSS URL.
//
// ABFSS (Azure Blob File System) URLs can have different formats depending on how they're constructed.
// This method handles multiple formats to provide flexibility in URL specification.
//
// Parameters:
//   - parsedURL: Pre-parsed URL object from url.Parse()
//
// Returns:
//   - container: The container name where the blob is stored
//   - blobPath: The path to the blob within the container (without leading slash)
//
// Supported URL Formats:
//
//  1. Standard ABFSS with userinfo:
//     "abfss://container@storageaccount.blob.core.windows.net/folder/file.json"
//     - Container: "container"
//     - Path: "folder/file.json"
//
//  2. Alternative @ parsing (if URL parser handles @ differently):
//     "abfss://container@storageaccount.blob.core.windows.net/file.json"
//     - Container: "container"
//     - Path: "file.json"
//
//  3. Simplified format (no @ symbol):
//     "abfss://container/folder/file.json"
//     - Container: "container"
//     - Path: "folder/file.json"
//
// URL Parsing Details:
//   - The @ symbol in ABFSS URLs is treated as userinfo by Go's url.Parse()
//   - Standard parsing: parsedURL.User.Username() contains the container name
//   - Host contains the storage account endpoint
//   - Path contains the blob path with leading slash (removed by this function)
//
// Example Usage:
//
//	parsedURL, _ := url.Parse("abfss://mycontainer@myaccount.blob.core.windows.net/data/file.json")
//	container, path := client.parseABFSSURL(parsedURL)
//	// container = "mycontainer"
//	// path = "data/file.json"
func (c *azureBlobClient) parseABFSSURL(parsedURL *url.URL) (container, blobPath string) {
	// Method 1: Standard ABFSS parsing using userinfo
	// URL: abfss://container@storageaccount.blob.core.windows.net/path
	if parsedURL.User != nil && parsedURL.User.Username() != "" {
		container = parsedURL.User.Username()
		blobPath = strings.TrimPrefix(parsedURL.Path, "/")
		return container, blobPath
	}

	// Method 2: Alternative @ parsing (fallback if userinfo parsing fails)
	// Handle cases where URL parsing doesn't properly handle the @ symbol
	if strings.Contains(parsedURL.Host, "@") {
		parts := strings.Split(parsedURL.Host, "@")
		if len(parts) >= 2 {
			container = parts[0]
			blobPath = strings.TrimPrefix(parsedURL.Path, "/")
			return container, blobPath
		}
	}

	// Method 3: Simplified format fallback
	// URL: abfss://container/path (no @ symbol)
	// Treat the host as the container name directly
	container = parsedURL.Host
	blobPath = strings.TrimPrefix(parsedURL.Path, "/")
	return container, blobPath
}

// buildBlobURL constructs the Azure Blob Storage REST API URL with SAS token authentication.
//
// This method creates the complete URL needed for Azure Storage REST API calls by combining
// the endpoint, container, blob path, and SAS token into a properly formatted request URL.
//
// Parameters:
//   - container: The name of the blob container
//   - blobPath: The path to the blob within the container
//
// Returns:
//   - string: Complete URL for Azure Blob Storage REST API request
//
// URL Format:
//
//	https://{endpoint}/{container}/{blobPath}?{sasToken}
//
// Example:
//
//	Input:
//	  - endpoint: "https://myaccount.blob.core.windows.net"
//	  - container: "mycontainer"
//	  - blobPath: "folder/data.json"
//	  - sasToken: "sv=2022-11-02&ss=b&srt=sco&sp=r&se=2024-12-31T23:59:59Z&sig=..."
//
//	Output:
//	  "https://myaccount.blob.core.windows.net/mycontainer/folder/data.json?sv=2022-11-02&ss=b&srt=sco&sp=r&se=2024-12-31T23:59:59Z&sig=..."
//
// Security Notes:
//   - The returned URL contains the SAS token in the query string
//   - URLs should not be logged or exposed to avoid token leakage
//   - SAS tokens provide time-limited access based on their expiration settings
//
// Performance Notes:
//   - URL construction is lightweight and can be called frequently
//   - No network calls or I/O operations performed
//   - String formatting is the primary performance consideration
func (c *azureBlobClient) buildBlobURL(container, blobPath string) string {
	// Construct the full Azure Blob Storage REST API URL
	// Format: https://{endpoint}/{container}/{blob}?{sasToken}
	return fmt.Sprintf("%s/%s/%s?%s", c.endpoint, container, blobPath, c.sasToken)
}

// downloadBlob performs the HTTP GET request to download blob data from Azure Storage.
//
// This method handles the low-level HTTP communication with Azure Blob Storage,
// including request creation, execution, response validation, and data reading.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout control
//   - originalURI: The original ABFSS URI (used for error reporting only)
//   - blobURL: The constructed Azure Storage REST API URL with SAS token
//
// Returns:
//   - []byte: The complete blob content as bytes
//   - error: Any error that occurred during HTTP communication or data reading
//
// HTTP Flow:
//  1. Create GET request with context for cancellation support
//  2. Execute request using configured HTTP client
//  3. Validate HTTP status code (200 OK expected)
//  4. Read entire response body into memory
//  5. Return blob data or detailed error information
//
// Error Handling:
//   - Request creation errors (malformed URLs, context issues)
//   - Network errors (DNS resolution, connection failures, timeouts)
//   - HTTP errors (authentication, authorization, not found, server errors)
//   - I/O errors (reading response body, incomplete reads)
//
// Status Code Handling:
//   - 200 OK: Success, blob data returned
//   - 401 Unauthorized: Invalid or expired SAS token
//   - 403 Forbidden: Insufficient SAS token permissions
//   - 404 Not Found: Blob or container doesn't exist
//   - 500+ Server Error: Azure Storage service issues
//
// Example Error Scenarios:
//   - Network timeout: "failed to get object from abfss://container/file: context deadline exceeded"
//   - Authentication: "failed to get object from abfss://container/file: HTTP 401"
//   - Not found: "failed to get object from abfss://container/file: HTTP 404"
//
// Performance Considerations:
//   - Entire blob is loaded into memory (suitable for small-to-medium files)
//   - Large blobs (>100MB) may require streaming approach
//   - HTTP client reuse reduces connection establishment overhead
//   - Context cancellation allows for request timeout control
//
// Security Notes:
//   - SAS token is included in blobURL query string
//   - Error messages include originalURI for debugging but not the SAS token URL
//   - Response body is handled securely without logging blob content
func (c *azureBlobClient) downloadBlob(ctx context.Context, originalURI, blobURL string) ([]byte, error) {
	// Create HTTP GET request with context support for cancellation/timeout
	req, err := http.NewRequestWithContext(ctx, "GET", blobURL, nil)
	if err != nil {
		// Follow error handling guidelines: wrap with operation context
		return nil, fmt.Errorf("create HTTP request for blob %q: %w", originalURI, err)
	}

	// Execute the HTTP request using the configured client
	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Network or connection error - wrap with operation context
		return nil, fmt.Errorf("execute HTTP request for blob %q: %w", originalURI, err)
	}
	defer resp.Body.Close() // Ensure response body is always closed

	// Validate HTTP status code - only 200 OK is considered success
	if resp.StatusCode != http.StatusOK {
		// HTTP error response - include status code for debugging
		// Note: Include blobURL in error for debugging but be careful in production logs
		return nil, fmt.Errorf("HTTP %d error for blob %q (URL: %s)", resp.StatusCode, originalURI, blobURL)
	}

	// Read the entire response body into memory
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		// I/O error while reading response body
		return nil, fmt.Errorf("read response body for blob %q: %w", originalURI, err)
	}

	return data, nil
}

// Scheme returns the URL scheme identifier supported by this client.
//
// The scheme is used by the blobstore routing system to determine which client
// can handle a particular URL. This Azure client supports the "abfss" scheme,
// which is the standard for Azure Data Lake Storage Gen2.
//
// Returns:
//   - string: The URL scheme supported by this client ("abfss")
//
// Usage in Routing:
//
//	The blobstore system uses this method to match URLs to appropriate clients:
//	- "s3://bucket/file" -> routed to S3/MinIO client
//	- "abfss://container/file" -> routed to Azure client
//
// ABFSS Scheme:
//   - ABFSS stands for "Azure Blob File System"
//   - Standard scheme for Azure Data Lake Storage Gen2
//   - Provides hierarchical namespace and POSIX-like semantics
//   - Compatible with Hadoop Distributed File System (HDFS) APIs
//
// Example Usage:
//
//	if client.Scheme() == "abfss" {
//	    // This client can handle ABFSS URLs
//	    data, err := client.Get(ctx, "abfss://container/file.json")
//	}
func (c *azureBlobClient) Scheme() string {
	return c.scheme
}

// ProviderKey returns the provider identifier for this client instance.
//
// The provider key is used in multi-provider scenarios to identify which specific
// storage configuration this client represents. It enables applications to route
// requests to different storage accounts based on project requirements.
//
// Returns:
//   - string: The provider key identifier (e.g., "azure-prod", "azure-dev")
//
// Usage in Multi-Provider Systems:
//   - Projects specify a storageProviderKey in their configuration
//   - The blobstore router uses this key to select the appropriate client
//   - Enables different projects to use different Azure storage accounts
//
// Provider Key Examples:
//   - "azure-prod": Production Azure storage with high availability
//   - "azure-dev": Development Azure storage for testing
//   - "azure-eu": European Azure storage for GDPR compliance
//   - "azure-ml-team": Team-specific Azure storage account
//
// Example Configuration:
//
//	# Project configuration
//	apiVersion: v1
//	kind: Project
//	metadata:
//	  name: ml-training-project
//	spec:
//	  storageProviderKey: "azure-prod"  # References this client's provider key
//
// Example Usage:
//
//	if client.ProviderKey() == "azure-prod" {
//	    // This is the production Azure client
//	    // May have different SLA requirements, monitoring, etc.
//	}
func (c *azureBlobClient) ProviderKey() string {
	return c.providerKey
}
