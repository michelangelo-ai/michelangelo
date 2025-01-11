package storage

import "time"

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
