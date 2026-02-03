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
	ConcurrentReconciles int           `yaml:"concurrentReconciles"`
	RequeuePeriod        time.Duration `yaml:"requeuePeriod"`
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
func (c IngesterConfig) ToIngesterConfig() ingester.Config {
	return ingester.Config{
		ConcurrentReconciles: c.ConcurrentReconciles,
		RequeuePeriod:        c.RequeuePeriod,
	}
}
