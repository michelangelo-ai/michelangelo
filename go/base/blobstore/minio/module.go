package minio

import (
	"fmt"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/fx"

	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
)

// BlobStoreClientOut represents the dependency injection output structure for blobstore clients.
// This struct is used by the fx dependency injection system to group multiple storage clients
// and make them available to other components that need blobstore functionality.
//
// The fx.Out embedding enables dependency injection output grouping, allowing multiple
// storage providers to be collected into a single group ("blobstore_clients") that can
// be injected as a slice into dependent components.
//
// Example usage in consuming component:
//
//	type BlobStoreClientIn struct {
//	    fx.In
//	    Clients []blobstore.BlobStoreClient `group:"blobstore_clients"`
//	}
//
// This enables the blobstore routing system to have access to all configured storage clients.
type BlobStoreClientOut struct {
	fx.Out
	// BlobStoreClient is the storage client implementation that will be added to the group.
	// Tagged with group:"blobstore_clients" to enable collection of multiple providers.
	BlobStoreClient blobstore.BlobStoreClient `group:"blobstore_clients"`
}

// Module sets up dependency injection for MinIO/S3 storage clients.
//
// This fx.Module configuration registers the necessary providers for:
// 1. Configuration parsing (newConfig) - reads YAML configuration and creates Config struct
// 2. Client creation (newClient) - creates multiple S3 clients based on configuration
//
// The module follows the dependency injection pattern where:
// - newConfig reads configuration from YAML and provides a Config struct
// - newClient consumes the Config and produces multiple BlobStoreClientOut instances
// - Each BlobStoreClientOut gets collected into the "blobstore_clients" group
//
// Example integration:
//
//	app := fx.New(
//	    minio.Module,           // Register MinIO providers
//	    azure.Module,           // Register Azure providers (if using both)
//	    fx.Invoke(startServer), // Start application with all storage clients available
//	)
var Module = fx.Options(
	fx.Provide(newConfig),
	fx.Provide(newClient),
)

// newClient initializes S3/MinIO storage clients using the provided configuration.
//
// This function creates multiple S3-compatible storage clients based on the configuration map.
// It supports various S3-compatible services including AWS S3, MinIO, DigitalOcean Spaces,
// Wasabi, and other S3-compatible storage solutions.
//
// Parameters:
//   - config: The parsed MinIO configuration containing storage provider definitions
//
// Returns:
//   - []BlobStoreClientOut: Slice of storage clients ready for dependency injection
//   - error: Any error that occurred during client creation
//
// Configuration Behavior:
//   - Empty configuration: Creates a default AWS S3 client with environment credentials
//   - Non-S3 providers: Skipped (allows mixed provider configurations)
//   - Multiple S3 providers: Creates one client per provider configuration
//
// Default Client Behavior:
//   - Uses environment-based AWS credentials (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY)
//   - Connects to "s3.amazonaws.com" endpoint
//   - Assigned provider key "aws-sandbox" for identification
//   - Enables backward compatibility for existing deployments without explicit configuration
//
// Error Scenarios:
//   - Invalid provider configuration (missing required fields)
//   - Network connectivity issues during client initialization
//   - Authentication failures (invalid credentials, expired tokens)
//   - Malformed endpoint URLs or unsupported authentication methods
//
// Example configurations handled:
//  1. Default (empty config): Creates aws-sandbox client with env credentials
//  2. Multiple AWS regions: Creates separate clients for different regions
//  3. Mixed environments: Creates clients for prod, dev, staging with different auth
//  4. Local MinIO: Creates clients pointing to local MinIO instances
//
// Error Handling:
//   - Follows domain layer pattern: returns wrapped errors without logging
//   - Preserves original errors for upstream handling and retry logic
//   - Includes provider context in error messages for debugging
//   - Service boundary logging will be handled by consuming components
func newClient(config Config) ([]BlobStoreClientOut, error) {
	var clients []BlobStoreClientOut

	// Handle empty configuration by creating default AWS S3 client
	// This ensures backward compatibility and provides sensible defaults
	if len(config.StorageProviders) == 0 {
		defaultClient, err := newDefaultS3Client()
		if err != nil {
			// Follow error handling guidelines: wrap with operation context
			return nil, fmt.Errorf("create default S3 client: %w", err)
		}
		return []BlobStoreClientOut{defaultClient}, nil
	}

	// Create clients for each configured S3 storage provider
	// Skip non-S3 providers to allow mixed storage configurations (S3 + Azure)
	for providerKey, providerConfig := range config.StorageProviders {
		if providerConfig.Type != "s3" {
			continue // Skip non-S3 providers (e.g., Azure providers in mixed config)
		}

		client, err := newS3ClientWithKey(providerKey, providerConfig)
		if err != nil {
			// Include provider key in error for debugging multi-provider scenarios
			return nil, fmt.Errorf("create S3 client for provider %q: %w", providerKey, err)
		}
		clients = append(clients, client)
	}

	return clients, nil
}

// newDefaultS3Client creates a default S3 client when no providers are explicitly configured.
//
// This function provides backward compatibility and sensible defaults for deployments that
// haven't explicitly configured storage providers. It creates a standard AWS S3 client
// using environment-based credentials.
//
// Returns:
//   - BlobStoreClientOut: Default S3 client ready for dependency injection
//   - error: Any error that occurred during client creation
//
// Default Configuration:
//   - Endpoint: "s3.amazonaws.com" (AWS S3 service)
//   - Credentials: Environment variables (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_SESSION_TOKEN)
//   - Provider Key: "aws-sandbox" (indicates default/development usage)
//   - Security: TLS enabled (Secure: true)
//   - Region: Handled by AWS SDK default region detection
//
// Authentication Requirements:
//
//	The following environment variables must be set for authentication:
//	- AWS_ACCESS_KEY_ID: AWS access key identifier
//	- AWS_SECRET_ACCESS_KEY: AWS secret access key
//	- AWS_SESSION_TOKEN: (Optional) Session token for temporary credentials
//	- AWS_REGION: (Optional) Default region for operations
//
// Use Cases:
//   - Legacy deployments migrating to multi-provider configuration
//   - Simple single-provider deployments that don't need multiple storage accounts
//   - Development environments using developer AWS credentials
//   - CI/CD pipelines with injected AWS credentials
//
// Error Handling:
//   - MinIO SDK errors (invalid endpoints, credential issues)
//   - Network connectivity problems during client initialization
//   - Missing or invalid environment variables
//   - Returns unwrapped errors since context is obvious from function name
func newDefaultS3Client() (BlobStoreClientOut, error) {
	// Use environment-based credentials for default client
	// This reads AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_SESSION_TOKEN
	creds := credentials.NewEnvAWS()

	// Create MinIO client configured for AWS S3
	s3Client, err := minio.New("s3.amazonaws.com", &minio.Options{
		Creds:  creds,
		Secure: true, // Always use HTTPS for security
	})
	if err != nil {
		// Return direct error since context is clear (default S3 client creation)
		return BlobStoreClientOut{}, err
	}

	return BlobStoreClientOut{
		BlobStoreClient: &minioClient{
			s3Client:    s3Client,
			scheme:      "s3",
			providerKey: "aws-sandbox", // Default provider key for identification
		},
	}, nil
}

// newS3ClientWithKey creates a new S3/MinIO client for a specific provider configuration.
//
// This function handles the creation of S3-compatible clients with various authentication methods
// and endpoint configurations. It supports AWS S3, MinIO, and other S3-compatible storage services.
//
// Parameters:
//   - providerKey: Unique identifier for this provider (e.g., "aws-prod", "minio-local")
//   - config: StorageProvider configuration containing authentication and endpoint details
//
// Returns:
//   - BlobStoreClientOut: Configured S3 client ready for dependency injection
//   - error: Any error that occurred during client creation
//
// Authentication Methods Supported:
//
//  1. Environment Variables (config.UseEnvAws = true):
//     - Reads AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_SESSION_TOKEN
//     - Recommended for development and containerized environments
//
//  2. IAM Roles (config.UseIAM = true):
//     - Uses AWS SDK credential chain (EC2 instance profiles, EKS service accounts)
//     - Recommended for production deployments on AWS infrastructure
//
//  3. Static Credentials (neither UseEnvAws nor UseIAM):
//     - Uses config.AwsAccessKeyId and config.AwsSecretAccessKey
//     - Only recommended for local development and legacy systems
//
// Endpoint Configuration:
//   - AWS S3: Leave AwsEndpointUrl empty or set to "s3.amazonaws.com"
//   - MinIO: Set to MinIO server address (e.g., "localhost:9000", "minio.company.com")
//   - Other S3-compatible: Set to service endpoint (e.g., "s3.wasabisys.com")
//
// Example Provider Configurations:
//
//	Production AWS S3 with IAM:
//	  providerKey: "aws-prod"
//	  config: {Type: "s3", AwsRegion: "us-east-1", UseIAM: true}
//
//	Development AWS S3 with env credentials:
//	  providerKey: "aws-dev"
//	  config: {Type: "s3", AwsRegion: "us-west-2", UseEnvAws: true}
//
//	Local MinIO with static credentials:
//	  providerKey: "minio-local"
//	  config: {
//	      Type: "s3",
//	      AwsAccessKeyId: "minioadmin",
//	      AwsSecretAccessKey: "minioadmin",
//	      AwsEndpointUrl: "localhost:9000"
//	  }
//
// Security Considerations:
//   - Always uses TLS/HTTPS (Secure: true)
//   - Static credentials should only be used for development
//   - IAM roles provide automatic credential rotation and better security
//   - Environment variables are safer than hardcoded credentials
//
// Error Scenarios:
//   - Invalid authentication credentials
//   - Network connectivity issues to specified endpoint
//   - Malformed endpoint URLs
//   - Missing required configuration fields
//
// Error Handling:
//   - Returns unwrapped MinIO SDK errors since context will be added by caller
//   - Error context (provider key) is added by the calling function
func newS3ClientWithKey(providerKey string, config StorageProvider) (BlobStoreClientOut, error) {
	// Determine authentication method based on configuration
	var creds *credentials.Credentials
	if config.UseEnvAws {
		// Environment-based authentication: AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_SESSION_TOKEN
		creds = credentials.NewEnvAWS()
	} else if config.UseIAM {
		// IAM role-based authentication: EC2 instance profiles, EKS service accounts, etc.
		creds = credentials.NewIAM("")
	} else {
		// Static credential authentication: explicit access key and secret
		creds = credentials.NewStaticV4(config.AwsAccessKeyId, config.AwsSecretAccessKey, "")
	}

	// Determine endpoint URL, defaulting to AWS S3 if not specified
	endpoint := config.AwsEndpointUrl
	if endpoint == "" {
		endpoint = "s3.amazonaws.com"
	}

	// Create MinIO client with configured credentials and endpoint
	s3Client, err := minio.New(endpoint, &minio.Options{
		Creds:  creds,
		Secure: true, // Always use HTTPS for security
	})
	if err != nil {
		// Return direct error - caller will add provider context
		return BlobStoreClientOut{}, err
	}

	return BlobStoreClientOut{
		BlobStoreClient: &minioClient{
			s3Client:    s3Client,
			scheme:      "s3",
			providerKey: providerKey, // Provider key for multi-provider routing
		},
	}, nil
}
