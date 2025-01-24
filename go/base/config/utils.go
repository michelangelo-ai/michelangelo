package config

import "go.uber.org/config"

// ProvideConfig is a helper function to provide a config value from a config.Provider.
func ProvideConfig[T any](key string) func(config.Provider) (T, error) {
	return func(p config.Provider) (T, error) {
		c := new(T)
		err := p.Get(key).Populate(&c)
		return *c, err
	}
}
