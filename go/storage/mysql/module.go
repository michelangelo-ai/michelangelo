package mysql

import (
	"github.com/michelangelo-ai/michelangelo/go/storage/object"
	"go.uber.org/config"
	"go.uber.org/fx"
)

// Module provides the fx module of MySQL MetadataStorage
var Module = fx.Options(
	fx.Provide(
		newConfig,
		GetMetadataStorage,
		object.NewtManager,
	),
)

func newConfig(provider config.Provider) (Config, error) {
	conf := Config{}
	err := provider.Get(configKey).Populate(&conf)
	return conf, err
}
