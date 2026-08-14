package k8sengine

import "go.uber.org/fx"

// Module wires the k8sengine implementation of types.Mapper and its supporting
// LogPersistenceConfig provider. Compose this at the application root alongside
// client.Module so the consumer/impl boundary stays explicit.
var Module = fx.Options(
	fx.Provide(NewLogPersistenceConfig),
	fx.Provide(NewMapperConfig),
	fx.Provide(NewMapper),
)
