package handler

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlRTClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// APIHandlerBuilder builds API handlers using the builder pattern.
// Inspired by Kubernetes REST client builder and Flyte's configuration patterns.
type APIHandlerBuilder struct {
	// Core dependencies
	k8sClient       ctrlRTClient.Client
	metadataStorage storage.MetadataStorage
	blobStorage     storage.BlobStorage
	
	// Configuration
	config *Config
	
	// Observability
	logger  logr.Logger
	metrics tally.Scope
	
	// Flags for enabling/disabling features
	enableMetadata bool
	enableBlob     bool
	enableMetrics  bool
}

// NewAPIHandlerBuilder creates a new API handler builder with sensible defaults.
func NewAPIHandlerBuilder() *APIHandlerBuilder {
	return &APIHandlerBuilder{
		config: &Config{
			EnableMetadataStorage: false,
			EnableBlobStorage:     false,
			ConcurrentOperations:  true,
			MaxConcurrency:        10,
		},
		logger:        zapr.NewLogger(zap.NewNop()),
		metrics:       tally.NoopScope,
		enableMetrics: false,
	}
}

// WithK8sClient sets the Kubernetes client (required).
func (b *APIHandlerBuilder) WithK8sClient(client ctrlRTClient.Client) *APIHandlerBuilder {
	b.k8sClient = client
	return b
}

// WithMetadataStorage enables and configures metadata storage.
func (b *APIHandlerBuilder) WithMetadataStorage(storage storage.MetadataStorage, config storage.MetadataStorageConfig) *APIHandlerBuilder {
	b.metadataStorage = storage
	b.config.EnableMetadataStorage = true
	b.config.MetadataStorageConfig = config
	b.enableMetadata = true
	return b
}

// WithBlobStorage enables and configures blob storage.
func (b *APIHandlerBuilder) WithBlobStorage(storage storage.BlobStorage) *APIHandlerBuilder {
	b.blobStorage = storage
	b.config.EnableBlobStorage = true
	b.enableBlob = true
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
	b.enableMetrics = true
	return b
}

// WithConcurrency configures concurrent operations.
func (b *APIHandlerBuilder) WithConcurrency(enabled bool, maxConcurrency int) *APIHandlerBuilder {
	b.config.ConcurrentOperations = enabled
	if maxConcurrency > 0 {
		b.config.MaxConcurrency = maxConcurrency
	}
	return b
}

// WithConfig sets the entire configuration (advanced usage).
func (b *APIHandlerBuilder) WithConfig(config *Config) *APIHandlerBuilder {
	if config != nil {
		b.config = config
	}
	return b
}

// Build creates the API handler with the configured options.
// Returns an error if required dependencies are missing.
func (b *APIHandlerBuilder) Build() (api.Handler, error) {
	// Validate required dependencies
	if err := b.validate(); err != nil {
		return nil, err
	}

	// Create focused handlers
	k8sHandler := NewK8sHandler(b.k8sClient, b.logger)
	
	var metadataHandler MetadataHandler
	if b.enableMetadata && b.metadataStorage != nil {
		metadataHandler = NewMetadataHandler(b.metadataStorage, b.logger)
	} else {
		metadataHandler = NewNullMetadataHandler(b.logger)
	}

	var blobHandler BlobHandler
	if b.enableBlob && b.blobStorage != nil {
		blobHandler = NewBlobHandler(b.blobStorage, b.logger)
	} else {
		blobHandler = NewNullBlobHandler(b.logger)
	}

	validationHandler := NewValidationHandler(b.logger)

	var metricsHandler MetricsHandler
	if b.enableMetrics {
		metricsHandler = NewMetricsHandler(b.metrics, b.logger)
	} else {
		metricsHandler = NewNullMetricsHandler(b.logger)
	}

	// Create composite handler
	return NewCompositeAPIHandler(
		k8sHandler,
		metadataHandler,
		blobHandler,
		validationHandler,
		metricsHandler,
		b.config,
		b.logger,
	), nil
}

// BuildForTesting creates a test-friendly API handler with minimal dependencies.
func (b *APIHandlerBuilder) BuildForTesting() (api.Handler, error) {
	// Ensure we have at least a K8s client for testing
	if b.k8sClient == nil {
		return nil, fmt.Errorf("k8s client is required even for testing")
	}

	// Disable features that might not be available in tests
	b.config.EnableMetadataStorage = false
	b.config.EnableBlobStorage = false
	b.enableMetadata = false
	b.enableBlob = false
	b.enableMetrics = false

	return b.Build()
}

// validate checks that all required dependencies are provided.
func (b *APIHandlerBuilder) validate() error {
	if b.k8sClient == nil {
		return fmt.Errorf("k8s client is required")
	}

	if b.config.EnableMetadataStorage && b.metadataStorage == nil {
		return fmt.Errorf("metadata storage is required when EnableMetadataStorage is true")
	}

	if b.config.EnableBlobStorage && b.blobStorage == nil {
		return fmt.Errorf("blob storage is required when EnableBlobStorage is true")
	}

	if b.config.MaxConcurrency <= 0 {
		b.config.MaxConcurrency = 10 // set default
	}

	return nil
}

// NullMetadataHandler is a no-op implementation for when metadata storage is disabled.
type NullMetadataHandler struct {
	logger logr.Logger
}

// NewNullMetadataHandler creates a no-op metadata handler.
func NewNullMetadataHandler(logger logr.Logger) MetadataHandler {
	return &NullMetadataHandler{
		logger: logger.WithName("null-metadata-handler"),
	}
}

// CreateInMetadata is a no-op for null handler.
func (n *NullMetadataHandler) CreateInMetadata(ctx context.Context, obj ctrlRTClient.Object) error {
	n.logger.V(2).Info("Metadata storage disabled, skipping create",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)
	return nil
}

// GetFromMetadata is a no-op for null handler.
func (n *NullMetadataHandler) GetFromMetadata(ctx context.Context, namespace, name string, obj ctrlRTClient.Object) error {
	n.logger.V(2).Info("Metadata storage disabled, skipping get",
		"namespace", namespace,
		"name", name,
	)
	return fmt.Errorf("metadata storage not enabled")
}

// UpdateInMetadata is a no-op for null handler.
func (n *NullMetadataHandler) UpdateInMetadata(ctx context.Context, obj ctrlRTClient.Object) error {
	n.logger.V(2).Info("Metadata storage disabled, skipping update",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)
	return nil
}

// DeleteFromMetadata is a no-op for null handler.
func (n *NullMetadataHandler) DeleteFromMetadata(ctx context.Context, obj ctrlRTClient.Object) error {
	n.logger.V(2).Info("Metadata storage disabled, skipping delete",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)
	return nil
}

// ListFromMetadata is a no-op for null handler.
func (n *NullMetadataHandler) ListFromMetadata(ctx context.Context, namespace string, opts *metav1.ListOptions, list ctrlRTClient.ObjectList) error {
	n.logger.V(2).Info("Metadata storage disabled, skipping list",
		"namespace", namespace,
	)
	return fmt.Errorf("metadata storage not enabled")
}

// CheckExistsInMetadata is a no-op for null handler.
func (n *NullMetadataHandler) CheckExistsInMetadata(ctx context.Context, namespace, name string, obj ctrlRTClient.Object) (bool, error) {
	n.logger.V(2).Info("Metadata storage disabled, skipping existence check",
		"namespace", namespace,
		"name", name,
	)
	return false, nil
}


// BuildFromParams creates an API handler from legacy parameters (for backward compatibility).
func BuildFromParams(params Params) (api.Handler, error) {
	builder := NewAPIHandlerBuilder()

	// Set logger if provided
	if params.Logger != nil {
		builder = builder.WithZapLogger(params.Logger)
	}

	// Set metrics if provided
	if params.Metrics != nil {
		builder = builder.WithMetrics(params.Metrics)
	}

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

// Legacy factory function replacements

// NewAPIServerHandler replaces the legacy newAPIServerHandler function.
func NewAPIServerHandler(params Params) (api.Handler, error) {
	// Create K8s client
	k8sClient, err := ctrlRTClient.New(params.K8sRestConfig, ctrlRTClient.Options{Scheme: params.Scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %w", err)
	}

	builder := NewAPIHandlerBuilder().
		WithK8sClient(k8sClient)

	// Set logger if provided
	if params.Logger != nil {
		builder = builder.WithZapLogger(params.Logger)
	}

	// Set metrics if provided  
	if params.Metrics != nil {
		builder = builder.WithMetrics(params.Metrics)
	}

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

// NewCtrlManagerHandler replaces the legacy newCtrlManagerHandler function.
func NewCtrlManagerHandler(params Params) (api.Handler, error) {
	if params.Manager == nil {
		return nil, fmt.Errorf("manager is required for controller manager handler")
	}

	builder := NewAPIHandlerBuilder().
		WithK8sClient(params.Manager.GetClient())

	// Set logger if provided
	if params.Logger != nil {
		builder = builder.WithZapLogger(params.Logger)
	}

	// Set metrics if provided
	if params.Metrics != nil {
		builder = builder.WithMetrics(params.Metrics)
	}

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

// NewK8sOnlyHandler replaces the legacy newK8sOnlyFactory function.
func NewK8sOnlyHandler(params Params) (api.Handler, error) {
	// Create K8s client
	k8sClient, err := ctrlRTClient.New(params.K8sRestConfig, ctrlRTClient.Options{Scheme: params.Scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %w", err)
	}

	builder := NewAPIHandlerBuilder().
		WithK8sClient(k8sClient)

	// Set logger if provided
	if params.Logger != nil {
		builder = builder.WithZapLogger(params.Logger)
	}

	// Set metrics if provided
	if params.Metrics != nil {
		builder = builder.WithMetrics(params.Metrics)
	}

	// Explicitly disable metadata and blob storage
	builder.config.EnableMetadataStorage = false
	builder.config.EnableBlobStorage = false

	return builder.Build()
}