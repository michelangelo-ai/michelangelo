// Package azure provides Azure Blob Storage client implementation for the Michelangelo blobstore system.
// This package supports multiple Azure storage provider configurations with different authentication methods.
//
// Usage Example:
//
//	# YAML configuration for multiple Azure providers (simplified array structure)
//	azure:
//	  - name: "azure-prod"
//	    azureStorageAccount: "prodstorageacct"
//	    azureSASToken: "sv=2022-11-02&ss=b&srt=sco&sp=rwdlacupx&se=2024-12-31T23:59:59Z..."
//	    default: true
//	  - name: "azure-dev"
//	    azureStorageAccount: "devstorageacct"
//	    azureSASToken: "sv=..."
//	    azureEndpoint: "https://custom.endpoint.net"  # Optional custom endpoint
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
	// Since this is in the Azure module, all providers are implicitly Azure Blob Storage.
	//
	// Example configuration:
	//   - name: "azure-prod"
	//     azureStorageAccount: "mlprodstorageacct"   # Azure Storage Account name
	//     azureSASToken: "sv=2022-11-02&ss=b..."     # SAS token for authentication
	//     azureEndpoint: ""                          # Optional custom endpoint
	StorageProvider struct {

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

	// Config represents the direct array of Azure Blob Storage provider configurations.
	// The "azure" YAML key maps directly to this array, eliminating intermediate nesting.
	//
	// Simplified structure example:
	//   azure:
	//     - name: "azure-prod"
	//       azureStorageAccount: "prodstorageacct"
	//       azureSASToken: "sv=..."
	//       default: true
	//     - name: "azure-dev"
	//       azureStorageAccount: "devstorageacct"
	//       azureSASToken: "sv=..."
	Config []ProviderConfig

	// ProviderConfig combines the provider name with the Azure storage configuration.
	// This structure enables clean configuration where each provider has both
	// a name and its specific configuration details.
	ProviderConfig struct {
		// Name is the unique identifier for this provider (e.g., "azure-prod", "azure-dev").
		// This name will be referenced in project configurations to specify which storage to use.
		Name string `yaml:"name"`

		// Default indicates if this provider should be used as the fallback when no specific
		// provider is requested. Only one provider in the array should have default: true.
		Default bool `yaml:"default,omitempty"`

		// StorageProvider embeds all the Azure storage configuration details.
		// This includes authentication, endpoints, storage accounts, etc.
		StorageProvider `yaml:",inline"`
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
