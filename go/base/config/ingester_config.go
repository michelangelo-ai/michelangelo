package config

import (
	"time"

	"github.com/michelangelo-ai/michelangelo/go/components/ingester"
	"github.com/michelangelo-ai/michelangelo/go/storage/mysql"
)

// Config holds the configuration for ingester setup
type Config struct {
	MySQL    MySQLConfig    `yaml:"mysql"`
	Ingester IngesterConfig `yaml:"ingester"`
}

// MySQLConfig holds MySQL configuration
type MySQLConfig struct {
	Enabled         bool          `yaml:"enabled"`
	Host            string        `yaml:"host"`
	Port            int           `yaml:"port"`
	User            string        `yaml:"user"`
	Password        string        `yaml:"password"`
	Database        string        `yaml:"database"`
	MaxOpenConns    int           `yaml:"maxOpenConns"`
	MaxIdleConns    int           `yaml:"maxIdleConns"`
	ConnMaxLifetime time.Duration `yaml:"connMaxLifetime"`
}

// IngesterConfig holds ingester controller configuration
type IngesterConfig struct {
	// Deprecated: Use ConcurrentReconcilesMap instead
	ConcurrentReconciles int `yaml:"concurrentReconciles"`
	// Deprecated: Use RequeuePeriodMap instead
	RequeuePeriod time.Duration `yaml:"requeuePeriod"`

	// ConcurrentReconcilesMap allows per-controller concurrency configuration
	ConcurrentReconcilesMap map[string]int           `yaml:"concurrentReconcilesMap"`
	RequeuePeriodMap        map[string]time.Duration `yaml:"requeuePeriodMap"`
}

// GetControllerConfig returns the config for a specific CRD kind
// Falls back to legacy single values if map is not configured
func (c IngesterConfig) GetControllerConfig(crdKind string) ingester.Config {
	concurrency := c.ConcurrentReconciles // Default from legacy field
	requeuePeriod := c.RequeuePeriod      // Default from legacy field

	// Override with map values if present
	if c.ConcurrentReconcilesMap != nil {
		if val, ok := c.ConcurrentReconcilesMap[crdKind]; ok {
			concurrency = val
		}
	}

	if c.RequeuePeriodMap != nil {
		if val, ok := c.RequeuePeriodMap[crdKind]; ok {
			requeuePeriod = val
		}
	}

	return ingester.Config{
		ConcurrentReconciles: concurrency,
		RequeuePeriod:        requeuePeriod,
	}
}

// ToMySQLConfig converts to mysql.Config
func (c MySQLConfig) ToMySQLConfig() mysql.Config {
	return mysql.Config{
		Host:            c.Host,
		Port:            c.Port,
		User:            c.User,
		Password:        c.Password,
		Database:        c.Database,
		MaxOpenConns:    c.MaxOpenConns,
		MaxIdleConns:    c.MaxIdleConns,
		ConnMaxLifetime: c.ConnMaxLifetime,
	}
}

// ToIngesterConfig converts to ingester.Config
// Deprecated: Use GetControllerConfig for per-controller configuration
func (c IngesterConfig) ToIngesterConfig() ingester.Config {
	return ingester.Config{
		ConcurrentReconciles: c.ConcurrentReconciles,
		RequeuePeriod:        c.RequeuePeriod,
	}
}
