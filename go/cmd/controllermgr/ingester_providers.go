package main

import (
	"fmt"

	baseconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	"github.com/michelangelo-ai/michelangelo/go/components/ingester"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	"github.com/michelangelo-ai/michelangelo/go/storage/blobstorage"
	mysqlstorage "github.com/michelangelo-ai/michelangelo/go/storage/mysql"
	"k8s.io/apimachinery/pkg/runtime"
)

func provideMetadataStorage(
	storageConfig storage.MetadataStorageConfig,
	mysqlConfig baseconfig.MySQLConfig,
	scheme *runtime.Scheme,
) (storage.MetadataStorage, error) {
	if !storage.EnableMetadataStorage(&storageConfig) {
		return nil, nil
	}

	if !mysqlConfigEnabled(mysqlConfig) {
		return nil, fmt.Errorf("metadata storage is enabled but mysql config is empty")
	}

	return mysqlstorage.NewMetadataStorage(mysqlConfig.ToMySQLConfig(), scheme)
}

func provideIngesterConfig(config baseconfig.IngesterConfig) ingester.Config {
	return ingester.Config{
		ConcurrentReconciles:    config.ConcurrentReconciles,
		RequeuePeriod:           config.RequeuePeriod,
		ConcurrentReconcilesMap: config.ConcurrentReconcilesMap,
		RequeuePeriodMap:        config.RequeuePeriodMap,
	}
}

// provideBlobStorage returns a BlobStorage implementation backed by the given BlobStore.
// Returns nil when blob storage is disabled in config, making it optional for the ingester.
func provideBlobStorage(store *blobstore.BlobStore, config baseconfig.BlobStorageConfig) storage.BlobStorage {
	if !config.Enabled {
		return nil
	}
	return blobstorage.New(store, config.ToBlobStorageConfig())
}

func mysqlConfigEnabled(config baseconfig.MySQLConfig) bool {
	if config.Enabled {
		return true
	}

	return config.Host != "" || config.Database != "" || config.User != ""
}

