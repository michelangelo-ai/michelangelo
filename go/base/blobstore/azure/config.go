// Package azure provides Azure Blob Storage client implementation for the Michelangelo blobstore system.
// This package supports multiple Azure storage provider configurations with different authentication methods.
//
// Usage Example:
//
//	# YAML configuration for multiple Azure providers
//	azure:
//	  storageProviders:
//	    azure-prod:
//	      type: "azure"
//	      azureStorageAccount: "prodstorageacct"
//	      azureSASToken: "sv=2022-11-02&ss=b&srt=sco&sp=rwdlacupx&se=2024-12-31T23:59:59Z..."
//	    azure-dev:
//	      type: "azure"
//	      azureStorageAccount: "devstorageacct"
//	      azureSASToken: "sv=..."
//	      azureEndpoint: "https://custom.endpoint.net"  # Optional custom endpoint
//	  defaultProvider: "azure-prod"
//
// URL Format Supported:
//   - Standard ABFSS: "abfss://container@storageaccount.blob.core.windows.net/path/file.json"
//   - Simplified ABFSS: "abfss://container/path/file.json"
//
// Security Notes:
//   - Never commit SAS tokens to version control
//   - Use environment variables or secret management for tokens
//   - Rotate SAS tokens regularly with appropriate expiration times
package azure

import (
	"fmt"

	"go.uber.org/config"
)

const (
	// configKey is the root YAML key for Azure storage provider configuration.
	// The configuration system looks for this key in the YAML config files.
	configKey = "azure"
)

type (
	// StorageProvider defines configuration for a single Azure Blob Storage provider.
	// Each provider represents a specific Azure Storage Account with its authentication method.
	//
	// Example configuration:
	//   azure-prod:
	//     type: "azure"                              # Must be "azure" for Azure providers
	//     azureStorageAccount: "mlprodstorageacct"   # Azure Storage Account name
	//     azureSASToken: "sv=2022-11-02&ss=b..."     # SAS token for authentication
	//     azureEndpoint: ""                          # Optional custom endpoint
	StorageProvider struct {
		// Type specifies the storage provider type. Must be "azure" for Azure Blob Storage.
		// This field is used by the factory to determine which client implementation to create.
		Type string `yaml:"type"`

		// AzureStorageAccount is the name of the Azure Storage Account to connect to.
		// This is used to construct the default endpoint URL and identify the storage account.
		// Example: "mlprodstorageacct", "devstorageacct"
		//
		// Required: Yes
		// Validation: Must be a valid Azure Storage Account name (3-24 characters, lowercase letters and numbers only)
		AzureStorageAccount string `yaml:"azureStorageAccount"`

		// AzureSASToken is the Shared Access Signature token for authentication.
		// SAS tokens provide delegated access to Azure Storage resources with fine-grained permissions.
		//
		// Required: Yes
		// Security: Never commit real SAS tokens to version control
		// Example: "sv=2022-11-02&ss=b&srt=sco&sp=rwdlacupx&se=2024-12-31T23:59:59Z&st=2023-01-01T00:00:00Z&spr=https&sig=..."
		//
		// SAS Token Components:
		// - sv: Storage service version
		// - ss: Storage service (b=blob, f=file, q=queue, t=table)
		// - srt: Resource types (s=service, c=container, o=object)
		// - sp: Permissions (r=read, w=write, d=delete, l=list, a=add, c=create, u=update, p=process, x=execute)
		// - se: Expiry time (ISO 8601 UTC datetime)
		// - st: Start time (ISO 8601 UTC datetime)
		// - spr: Protocol (https, http,https)
		// - sig: Signature hash
		AzureSASToken string `yaml:"azureSASToken"`

		// AzureEndpoint is an optional custom endpoint URL for the Azure Blob Storage service.
		// If not specified, defaults to "https://{storageAccount}.blob.core.windows.net"
		//
		// Required: No (defaults to standard Azure endpoint)
		// Use cases:
		// - Azure Stack deployments: "https://mystackaccount.blob.local.azurestack.external"
		// - Azure Government Cloud: "https://mystorageaccount.blob.core.usgovcloudapi.net"
		// - Private endpoints: "https://mystorageaccount.privatelink.blob.core.windows.net"
		//
		// Example: "https://custom.blob.storage.endpoint.net"
		AzureEndpoint string `yaml:"azureEndpoint,omitempty"`
	}

	// Config defines the top-level configuration structure for Azure storage providers.
	// This allows configuration of multiple Azure storage providers with different accounts,
	// regions, or authentication methods that can be selected per-project.
	//
	// Example configuration:
	//   azure:
	//     storageProviders:
	//       azure-prod:
	//         type: "azure"
	//         azureStorageAccount: "prodstorageacct"
	//         azureSASToken: "sv=..."
	//       azure-dev:
	//         type: "azure"
	//         azureStorageAccount: "devstorageacct"
	//         azureSASToken: "sv=..."
	//     defaultProvider: "azure-prod"
	Config struct {
		// StorageProviders is a map of Azure storage provider configurations.
		// The key is used as the provider identifier (e.g., "azure-prod", "azure-dev")
		// and will be referenced in project configurations to specify which storage to use.
		//
		// Provider Key Naming Conventions:
		// - Use descriptive names that indicate environment: "azure-prod", "azure-dev", "azure-staging"
		// - Include team or project identifiers: "azure-ml-team", "azure-analytics"
		// - Be consistent across your organization
		//
		// Example:
		//   "azure-prod": Production Azure storage with high availability SLA
		//   "azure-dev": Development Azure storage for testing
		//   "azure-eu": European region storage for GDPR compliance
		StorageProviders map[string]StorageProvider `yaml:"storageProviders"`

		// DefaultProvider specifies which provider key to use when no specific provider is requested.
		// This should typically be your main production storage provider.
		//
		// Required: No (but recommended for fallback behavior)
		// Example: "azure-prod"
		//
		// Usage: When a request doesn't specify a provider key, the system will use this default.
		// This ensures backward compatibility and provides sensible fallback behavior.
		DefaultProvider string `yaml:"defaultProvider,omitempty"`
	}
)

// newConfig creates a new Azure storage configuration from the provided config provider.
// It reads the configuration from the YAML key specified by configKey ("azure").
//
// Parameters:
//   - provider: The config.Provider that contains the YAML configuration data
//
// Returns:
//   - Config: The parsed Azure storage configuration with all provider definitions
//   - error: Any error that occurred during configuration parsing
//
// Error Handling:
//   - Returns wrapped error with operation context for debugging
//   - Preserves original parsing errors for upstream handling
//   - Does not log errors (follows domain layer pattern - logging handled at service boundaries)
//
// Example usage:
//
//	provider, err := config.NewYAMLProviderFromFile("config.yaml")
//	if err != nil {
//	    return fmt.Errorf("create config provider: %w", err)
//	}
//
//	azureConfig, err := newConfig(provider)
//	if err != nil {
//	    return fmt.Errorf("parse azure config: %w", err)
//	}
//
// Configuration Validation:
//   - Empty configuration is valid (will result in no Azure providers)
//   - Invalid YAML structure will return parsing errors
//   - Missing required fields in StorageProvider will be caught at runtime
func newConfig(provider config.Provider) (Config, error) {
	conf := Config{}
	err := provider.Get(configKey).Populate(&conf)
	if err != nil {
		// Follow error handling guidelines: return wrapped error with operation context
		// Domain layer: no logging, just return enriched errors
		return conf, fmt.Errorf("populate azure config from key %q: %w", configKey, err)
	}
	return conf, nil
}
