//go:generate mamockgen Factory
package handler

import (
	"fmt"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
	ctrlRTClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// APIHandlerBuilder builds API handlers using the builder pattern.
//
// This builder provides a fluent interface for configuring API handlers with
// optional dependencies, replacing factory pattern anti-patterns with clean
// composition. The design is inspired by Kubernetes REST client builders and
// Flyte's configuration patterns.
//
// Example usage:
//
//	handler, err := NewAPIHandlerBuilder().
//		WithK8sClient(client).
//		WithMetadataStorage(storage, config).
//		WithZapLogger(logger).
//		Build()
//	if err != nil {
//		return fmt.Errorf("failed to build handler: %w", err)
//	}
type APIHandlerBuilder struct {
	// Core dependencies
	k8sClient       ctrlRTClient.Client
	metadataStorage storage.MetadataStorage
	blobStorage     storage.BlobStorage

	// Configuration
	storageConfig storage.MetadataStorageConfig

	// Observability
	logger  logr.Logger
	metrics tally.Scope
}

// NewAPIHandlerBuilder creates a new API handler builder with sensible defaults.
// The builder is configured with no-op logger and metrics scope by default,
// requiring only a Kubernetes client to produce a functional handler.
func NewAPIHandlerBuilder() *APIHandlerBuilder {
	return &APIHandlerBuilder{
		logger:  zapr.NewLogger(zap.NewNop()),
		metrics: tally.NoopScope,
	}
}

// WithK8sClient sets the Kubernetes client (required).
// The client must be properly configured with the appropriate scheme and credentials.
func (b *APIHandlerBuilder) WithK8sClient(client ctrlRTClient.Client) *APIHandlerBuilder {
	b.k8sClient = client
	return b
}

// WithMetadataStorage enables and configures metadata storage.
// Both storage implementation and config must be provided when metadata storage is enabled.
func (b *APIHandlerBuilder) WithMetadataStorage(storage storage.MetadataStorage, config storage.MetadataStorageConfig) *APIHandlerBuilder {
	b.metadataStorage = storage
	b.storageConfig = config
	return b
}

// WithBlobStorage enables and configures blob storage.
func (b *APIHandlerBuilder) WithBlobStorage(storage storage.BlobStorage) *APIHandlerBuilder {
	b.blobStorage = storage
	return b
}

// WithLogger sets the logger for structured logging.
func (b *APIHandlerBuilder) WithLogger(logger logr.Logger) *APIHandlerBuilder {
	b.logger = logger
	return b
}

// WithZapLogger sets a zap logger (convenience method).
func (b *APIHandlerBuilder) WithZapLogger(logger *zap.Logger) *APIHandlerBuilder {
	b.logger = zapr.NewLogger(logger)
	return b
}

// WithMetrics enables metrics collection with the provided scope.
func (b *APIHandlerBuilder) WithMetrics(scope tally.Scope) *APIHandlerBuilder {
	b.metrics = scope
	return b
}

// Build creates the API handler with focused, composed handlers.
// Returns an error if required dependencies are missing or invalid.
// The returned handler is ready for use and thread-safe.
func (b *APIHandlerBuilder) Build() (api.Handler, error) {
	// Validate required dependencies
	if err := b.validate(); err != nil {
		return nil, err
	}

	// Create focused handlers using existing implementations
	k8sHandler := NewK8sHandler(b.k8sClient)
	metadataHandler := NewMetadataHandler(b.metadataStorage, b.blobStorage, b.logger)
	blobHandler := NewBlobHandler(b.blobStorage)
	validationHandler := NewValidationHandler()

	// Create the main handler using existing apiHandler but with focused dependencies
	handler := &apiHandler{
		conf:    b.storageConfig,
		logger:  b.logger,
		metrics: b.metrics,
		// Inject focused handlers to reduce coupling
		k8sHandler:        k8sHandler,
		metadataHandler:   metadataHandler,
		blobHandler:       blobHandler,
		validationHandler: validationHandler,
	}

	return handler, nil
}

// validate checks that all required dependencies are provided.
func (b *APIHandlerBuilder) validate() error {
	if b.k8sClient == nil {
		return fmt.Errorf("k8s client is required")
	}

	if b.storageConfig.EnableMetadataStorage && b.metadataStorage == nil {
		return fmt.Errorf("metadata storage is required when EnableMetadataStorage is true")
	}

	return nil
}

// Factory function replacements using builder pattern

// NewAPIServerHandler creates an API handler for the API server component.
// This replaces the legacy newAPIServerHandler function with builder-based construction.
func NewAPIServerHandler(params Params) (api.Handler, error) {
	// Create K8s client
	k8sClient, err := ctrlRTClient.New(params.K8sRestConfig, ctrlRTClient.Options{Scheme: params.Scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %w", err)
	}

	builder := NewAPIHandlerBuilder().
		WithK8sClient(k8sClient).
		WithZapLogger(params.Logger).
		WithMetrics(params.Metrics)

	// Configure metadata storage if enabled
	if params.StorageConfig.EnableMetadataStorage && params.MetadataStorage != nil {
		builder = builder.WithMetadataStorage(params.MetadataStorage, params.StorageConfig)
	}

	// Configure blob storage if provided
	if params.BlobStorage != nil {
		builder = builder.WithBlobStorage(params.BlobStorage)
	}

	return builder.Build()
}

// NewCtrlManagerHandler creates an API handler for the controller manager component.
// This replaces the legacy newCtrlManagerHandler function with builder-based construction.
func NewCtrlManagerHandler(params Params) (api.Handler, error) {
	if params.Manager == nil {
		return nil, fmt.Errorf("manager is required for controller manager handler")
	}

	builder := NewAPIHandlerBuilder().
		WithK8sClient(params.Manager.GetClient()).
		WithZapLogger(params.Logger).
		WithMetrics(params.Metrics)

	// Configure metadata storage if enabled
	if params.StorageConfig.EnableMetadataStorage && params.MetadataStorage != nil {
		builder = builder.WithMetadataStorage(params.MetadataStorage, params.StorageConfig)
	}

	// Configure blob storage if provided
	if params.BlobStorage != nil {
		builder = builder.WithBlobStorage(params.BlobStorage)
	}

	return builder.Build()
}

// newK8sAndMetadataStorageFactory creates an API handler with both K8s and metadata storage enabled.
// This function maintains compatibility with the old factory pattern while using the builder internally.
func newK8sAndMetadataStorageFactory(params Params) (api.Handler, error) {
	// Create K8s client
	k8sClient, err := ctrlRTClient.New(params.K8sRestConfig, ctrlRTClient.Options{Scheme: params.Scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %w", err)
	}

	builder := NewAPIHandlerBuilder().
		WithK8sClient(k8sClient).
		WithZapLogger(params.Logger).
		WithMetrics(params.Metrics)

	// Configure metadata storage if enabled
	if params.StorageConfig.EnableMetadataStorage && params.MetadataStorage != nil {
		builder = builder.WithMetadataStorage(params.MetadataStorage, params.StorageConfig)
	}

	// Configure blob storage if provided
	if params.BlobStorage != nil {
		builder = builder.WithBlobStorage(params.BlobStorage)
	}

	return builder.Build()
}

// Factory interface that controllers can use to create handlers with different K8s clients
type Factory interface {
	GetAPIHandler(client ctrlRTClient.Client) (api.Handler, error)
}

// apiHandlerFactory implements Factory using the builder pattern internally
type apiHandlerFactory struct {
	logger          logr.Logger
	metrics         tally.Scope
	metadataStorage storage.MetadataStorage
	blobStorage     storage.BlobStorage
	storageConfig   storage.MetadataStorageConfig
}

// GetAPIHandler creates an API handler using the provided K8s client
func (f *apiHandlerFactory) GetAPIHandler(client ctrlRTClient.Client) (api.Handler, error) {
	builder := NewAPIHandlerBuilder().
		WithK8sClient(client).
		WithLogger(f.logger).
		WithMetrics(f.metrics)

	// Configure metadata storage if enabled
	if f.storageConfig.EnableMetadataStorage {
		builder = builder.WithMetadataStorage(f.metadataStorage, f.storageConfig)
	}

	// Configure blob storage if provided
	if f.blobStorage != nil {
		builder = builder.WithBlobStorage(f.blobStorage)
	}

	return builder.Build()
}

// NewAPIHandlerFactory creates a factory that controllers can use to build handlers
// This provides the same interface as the old factory pattern but uses the builder internally
func NewAPIHandlerFactory(params Params) Factory {
	return &apiHandlerFactory{
		logger:          zapr.NewLogger(params.Logger),
		metrics:         params.Metrics,
		metadataStorage: params.MetadataStorage,
		blobStorage:     params.BlobStorage,
		storageConfig:   params.StorageConfig,
	}
}
