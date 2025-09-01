package handler

import (
	"github.com/michelangelo-ai/michelangelo/go/storage"
)

// Config represents the configuration for API handlers.
// Inspired by Flyte's modular configuration approach and Kubernetes controller patterns.
type Config struct {
	// Storage configuration
	EnableMetadataStorage bool
	EnableBlobStorage     bool
	MetadataStorageConfig storage.MetadataStorageConfig
	
	// Concurrency configuration following Flyte's patterns
	ConcurrentOperations bool
	MaxConcurrency       int
	
	// Feature flags
	EnableValidation bool
	EnableMetrics    bool
	
	// Timeouts and limits
	OperationTimeoutSeconds int
	RetryAttempts           int
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		EnableMetadataStorage:   false,
		EnableBlobStorage:       false,
		ConcurrentOperations:    true,
		MaxConcurrency:          10,
		EnableValidation:        true,
		EnableMetrics:           false,
		OperationTimeoutSeconds: 30,
		RetryAttempts:           3,
	}
}

// WithMetadataStorage enables metadata storage with the provided config.
func (c *Config) WithMetadataStorage(config storage.MetadataStorageConfig) *Config {
	c.EnableMetadataStorage = true
	c.MetadataStorageConfig = config
	return c
}

// WithBlobStorage enables blob storage.
func (c *Config) WithBlobStorage() *Config {
	c.EnableBlobStorage = true
	return c
}

// WithConcurrency configures concurrent operations.
func (c *Config) WithConcurrency(enabled bool, maxConcurrency int) *Config {
	c.ConcurrentOperations = enabled
	if maxConcurrency > 0 {
		c.MaxConcurrency = maxConcurrency
	}
	return c
}

// WithMetrics enables metrics collection.
func (c *Config) WithMetrics() *Config {
	c.EnableMetrics = true
	return c
}

// WithValidation configures validation.
func (c *Config) WithValidation(enabled bool) *Config {
	c.EnableValidation = enabled
	return c
}