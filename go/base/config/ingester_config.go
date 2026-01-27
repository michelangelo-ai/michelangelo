package config

import (
	"time"

	"github.com/michelangelo-ai/michelangelo/go/components/ingester"
	"github.com/michelangelo-ai/michelangelo/go/storage/minio"
	"github.com/michelangelo-ai/michelangelo/go/storage/mysql"
)

// Config holds the configuration for sandbox setup
type Config struct {
	MySQL   MySQLConfig   `yaml:"mysql"`
	MinIO   MinIOConfig   `yaml:"minio"`
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

// MinIOConfig holds MinIO configuration
type MinIOConfig struct {
	Enabled         bool   `yaml:"enabled"`
	Endpoint        string `yaml:"endpoint"`
	AccessKeyID     string `yaml:"accessKeyID"`
	SecretAccessKey string `yaml:"secretAccessKey"`
	UseSSL          bool   `yaml:"useSSL"`
	BucketName      string `yaml:"bucketName"`
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

// ToMinIOConfig converts to minio.Config
func (c MinIOConfig) ToMinIOConfig() minio.Config {
	return minio.Config{
		Endpoint:        c.Endpoint,
		AccessKeyID:     c.AccessKeyID,
		SecretAccessKey: c.SecretAccessKey,
		UseSSL:          c.UseSSL,
		BucketName:      c.BucketName,
	}
}

// ToIngesterConfig converts to ingester.Config
func (c IngesterConfig) ToIngesterConfig() ingester.Config {
	return ingester.Config{
		ConcurrentReconciles: c.ConcurrentReconciles,
		RequeuePeriod:        c.RequeuePeriod,
	}
}
