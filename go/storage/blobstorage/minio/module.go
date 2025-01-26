package minio

import (
	"go.uber.org/config"
	"go.uber.org/fx"
)


const (
	configKey = "minio"
)

// Config defines configuration
type Config struct {
	BucketName  string `yaml:"bucketName"`
	Endpoint              string          `yaml:"endpoint"`
	AccessKey              string `yaml:"accessKey"`
	SecretKey string          `yaml:"secretKey"`
	UseSSL bool          `yaml:"useSSL"`
}

func newConfig(provider config.Provider) (Config, error) {
	conf := Config{}
	err := provider.Get(configKey).Populate(&conf)
	return conf, err
}
// Module provides the MinioBlobStorage client into an Fx application.
var Module = fx.Options(
	fx.Provide(newConfig),
	fx.Provide(NewMinioBlobStorageClient),
	fx.Provide(NewMinioBlobStorage),
)
