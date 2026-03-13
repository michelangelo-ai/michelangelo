package main

import (
	"fmt"

	baseconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/components/ingester"
	"github.com/michelangelo-ai/michelangelo/go/storage"
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

func mysqlConfigEnabled(config baseconfig.MySQLConfig) bool {
	if config.Enabled {
		return true
	}

	return config.Host != "" || config.Database != "" || config.User != ""
}
