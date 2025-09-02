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
// This replaces the factory pattern anti-pattern with a clean builder approach
// inspired by Kubernetes REST client builder and Flyte's configuration patterns.
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
func NewAPIHandlerBuilder() *APIHandlerBuilder {
	return &APIHandlerBuilder{
		logger:  zapr.NewLogger(zap.NewNop()),
		metrics: tally.NoopScope,
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
// Returns an error if required dependencies are missing.
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
		k8sClient:       b.k8sClient,
		metadataStorage: b.metadataStorage,
		conf:            b.storageConfig,
		blobStorage:     b.blobStorage,
		logger:          b.logger,
		metrics:         b.metrics,
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

// Builder-based factory function replacements

// NewAPIServerHandler replaces the legacy newAPIServerHandler function.
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

// NewCtrlManagerHandler replaces the legacy newCtrlManagerHandler function.
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
