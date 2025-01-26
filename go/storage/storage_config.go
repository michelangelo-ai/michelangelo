package storage

import (
	"go.uber.org/fx"
	"time"
)

// MetadataStorageConfig is the schema of the yaml configuration of MetadataStorage.
type MetadataStorageConfig struct {
	EnableMetadataStorage      bool          `yaml:"enableMetadataStorage"`
	DeletionDelay              time.Duration `yaml:"deletionDelay"`
	EnableResourceVersionCache bool          `yaml:"enableResourceVersionCache"`
}

// EnableMetadataStorage determines if we enable storage based on the runtime setting and namespace.
func EnableMetadataStorage(conf *MetadataStorageConfig) bool {
	return conf.EnableMetadataStorage
}

// newConfig constructs and provides a MetadataStorageConfig.
func newConfig() MetadataStorageConfig {
	return MetadataStorageConfig{
		EnableMetadataStorage:      false,          // Set default values or load from a config file
		DeletionDelay:              10 * time.Minute,
		EnableResourceVersionCache: false,
	}
}

// ConfigModule provides the configuration for storage library
var ConfigModule = fx.Options(
	fx.Provide(newConfig))
