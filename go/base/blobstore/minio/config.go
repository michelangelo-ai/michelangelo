// Package minio provides MinIO/S3 client implementation for the Michelangelo blobstore system.
// This package supports multiple S3-compatible storage provider configurations with different authentication methods.
//
// Usage Example:
//
//	# YAML configuration for multiple MinIO/S3 providers (simplified array structure)
//	minio:
//	  - name: "aws-sandbox"
//	    awsRegion: "us-west-2"
//	    useEnvAws: true                               # Use AWS credentials from environment
//	    default: true                                 # Use as default provider
//	  - name: "aws-prod"
//	    awsRegion: "us-east-1"
//	    useIam: true                                  # Use IAM role-based authentication
//	  - name: "minio-local"
//	    awsRegion: "us-east-1"
//	    awsAccessKeyId: "minioadmin"                  # Static credentials for local MinIO
//	    awsSecretAccessKey: "minioadmin"
//	    awsEndpointUrl: "localhost:9000"              # Custom MinIO endpoint
//
// URL Format Supported:
//   - Standard S3: "s3://bucket-name/path/to/file.json"
//   - Compatible with AWS S3, MinIO, and other S3-compatible storage services
//
// Authentication Methods:
//   - Environment variables (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_SESSION_TOKEN)
//   - IAM roles (recommended for EC2/EKS deployments)
//   - Static credentials (for development and CI/CD)
//
// Security Notes:
//   - Never commit static credentials to version control
//   - Use environment variables or secret management for credentials
//   - Prefer IAM roles over static credentials in production
package minio

import (
	"fmt"

	"go.uber.org/config"
)

const (
	// configKey is the root YAML key for MinIO/S3 storage provider configuration.
	// The configuration system looks for this key in the YAML config files.
	configKey = "minio"
)

type (
	// StorageProvider defines configuration for a single S3-compatible storage provider.
	// Each provider represents a specific S3 service (AWS S3, MinIO, etc.) with its authentication method.
	// Since this is in the MinIO module, all providers are implicitly S3-compatible.
	//
	// Example configuration:
	//   - name: "aws-prod"
	//     awsRegion: "us-east-1"                  # AWS region for the S3 service
	//     useIam: true                            # Use IAM role authentication
	//     awsEndpointUrl: "s3.amazonaws.com"      # Optional custom endpoint
	StorageProvider struct {

		// AwsRegion specifies the AWS region for S3 operations.
		// Used for both AWS S3 and S3-compatible services that support regions.
		//
		// Required: Yes (for AWS S3), Optional (for MinIO/custom endpoints)
		// Examples: "us-west-2", "eu-west-1", "ap-southeast-1"
		//
		// For MinIO: Can be any value since MinIO doesn't enforce AWS regions
		// For AWS S3: Must be a valid AWS region where your bucket exists
		AwsRegion string `yaml:"awsRegion,omitempty"`

		// AwsAccessKeyId is the AWS access key ID for static credential authentication.
		// Used when useEnvAws and useIam are both false.
		//
		// Required: Only when using static credentials
		// Security: Never commit real access keys to version control
		// Example: "AKIA..." (AWS access key format)
		//
		// Usage scenarios:
		// - Local development with MinIO
		// - CI/CD pipelines with service account keys
		// - Legacy systems that can't use IAM roles
		AwsAccessKeyId string `yaml:"awsAccessKeyId,omitempty"`

		// AwsSecretAccessKey is the AWS secret access key for static credential authentication.
		// Must be provided together with AwsAccessKeyId for static credentials.
		//
		// Required: Only when using static credentials
		// Security: Never commit real secret keys to version control
		// Format: Base64-encoded string (40 characters for AWS)
		//
		// Best Practices:
		// - Use environment variables or secret management systems
		// - Rotate credentials regularly
		// - Use minimal required permissions (principle of least privilege)
		AwsSecretAccessKey string `yaml:"awsSecretAccessKey,omitempty"`

		// AwsEndpointUrl specifies a custom endpoint URL for S3-compatible services.
		// If not specified, defaults to "s3.amazonaws.com" for AWS S3.
		//
		// Required: No (defaults to AWS S3)
		// Use cases:
		// - MinIO deployments: "localhost:9000", "minio.company.com"
		// - AWS VPC endpoints: "vpce-12345-abcdef.s3.us-west-2.vpce.amazonaws.com"
		// - S3-compatible services: "storage.digitalocean.com", "s3.wasabisys.com"
		// - LocalStack: "localhost:4566"
		//
		// Examples:
		// - MinIO local: "localhost:9000"
		// - DigitalOcean Spaces: "nyc3.digitaloceanspaces.com"
		// - Wasabi: "s3.wasabisys.com"
		AwsEndpointUrl string `yaml:"awsEndpointUrl,omitempty"`

		// UseEnvAws enables authentication using AWS credentials from environment variables.
		// When true, the client will read AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, and AWS_SESSION_TOKEN.
		//
		// Required: No (defaults to false)
		// Environment variables used:
		// - AWS_ACCESS_KEY_ID: Access key ID
		// - AWS_SECRET_ACCESS_KEY: Secret access key
		// - AWS_SESSION_TOKEN: Session token (for temporary credentials)
		// - AWS_REGION: Default region (if not specified in config)
		//
		// Recommended for:
		// - Local development with AWS CLI configured
		// - Docker containers with environment-based secrets
		// - CI/CD pipelines with injected credentials
		//
		// Security: Environment variables are safer than hardcoded credentials
		UseEnvAws bool `yaml:"useEnvAws,omitempty"`

		// UseIAM enables authentication using AWS IAM roles (EC2 instance profiles, EKS service accounts).
		// When true, the client will use the AWS SDK's default credential chain for IAM authentication.
		//
		// Required: No (defaults to false)
		// Recommended: Yes for production deployments on AWS infrastructure
		//
		// Supported IAM authentication methods:
		// - EC2 instance profiles (for EC2 instances)
		// - EKS service account annotations (for Kubernetes pods)
		// - ECS task roles (for ECS containers)
		// - Lambda execution roles (for Lambda functions)
		//
		// Benefits:
		// - No static credentials to manage or rotate
		// - Automatic credential renewal
		// - Integrated with AWS security model
		// - Fine-grained permission control via IAM policies
		//
		// Setup for EKS:
		//   apiVersion: v1
		//   kind: ServiceAccount
		//   metadata:
		//     annotations:
		//       eks.amazonaws.com/role-arn: arn:aws:iam::ACCOUNT:role/S3AccessRole
		UseIAM bool `yaml:"useIam,omitempty"`
	}

	// Config represents the direct array of S3-compatible storage provider configurations.
	// The "minio" YAML key maps directly to this array, eliminating intermediate nesting.
	//
	// Simplified structure example:
	//   minio:
	//     - name: "aws-prod"
	//       awsRegion: "us-east-1"
	//       useIam: true
	//     - name: "aws-sandbox"
	//       awsRegion: "us-west-2"
	//       useEnvAws: true
	//       default: true
	//     - name: "minio-local"
	//       awsAccessKeyId: "minioadmin"
	//       awsSecretAccessKey: "minioadmin"
	//       awsEndpointUrl: "localhost:9000"
	Config []ProviderConfig

	// ProviderConfig combines the provider name with the storage configuration.
	// This structure enables clean configuration where each provider has both
	// a name and its specific configuration details.
	ProviderConfig struct {
		// Name is the unique identifier for this provider (e.g., "aws-prod", "minio-local").
		// This name will be referenced in project configurations to specify which storage to use.
		Name string `yaml:"name"`

		// Default indicates if this provider should be used as the fallback when no specific
		// provider is requested. Only one provider in the array should have default: true.
		Default bool `yaml:"default,omitempty"`

		// StorageProvider embeds all the storage configuration details.
		// This includes authentication, endpoints, regions, etc.
		StorageProvider `yaml:",inline"`
	}
)

// newConfig creates a new MinIO/S3 storage configuration from the provided config provider.
// It reads the configuration from the YAML key specified by configKey ("minio").
//
// Parameters:
//   - provider: The config.Provider that contains the YAML configuration data
//
// Returns:
//   - Config: The parsed MinIO/S3 storage configuration with all provider definitions
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
//	minioConfig, err := newConfig(provider)
//	if err != nil {
//	    return fmt.Errorf("parse minio config: %w", err)
//	}
//
// Configuration Validation:
//   - Empty configuration is valid (will create default AWS S3 client)
//   - Invalid YAML structure will return parsing errors
//   - Missing required fields in StorageProvider will be caught at runtime
//   - Authentication method validation (useEnvAws, useIam, or static credentials)
//
// Default Behavior:
//   - If no providers are configured, a default AWS S3 client is created
//   - Default client uses environment-based credentials
//   - Default provider key is "aws-sandbox"
func newConfig(provider config.Provider) (Config, error) {
	conf := Config{}
	err := provider.Get(configKey).Populate(&conf)
	if err != nil {
		// Follow error handling guidelines: return wrapped error with operation context
		// Domain layer: no logging, just return enriched errors
		return conf, fmt.Errorf("populate minio config from key %q: %w", configKey, err)
	}
	return conf, nil
}
