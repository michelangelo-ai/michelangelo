package config

import (
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"go.uber.org/config"

	"os"
	"strings"

	"go.uber.org/fx"
)

const (
	_configKeySeparator = ":"
	_defaultConfigDir   = "config"
)

// Params defines the dependencies of the config fx module.
type Params struct {
	fx.In

	Environment env.Context
}

// Result defines the objects that the config fx module provides.
type Result struct {
	fx.Out

	Provider config.Provider
}

// Module load config.Provider based on the environment context.
var Module = fx.Module("config",
	fx.Provide(New),
)

// New exports functionality similar to Module, but allows the caller to wrap
// or modify Result. Most users should use Module instead.
func New(p Params) (Result, error) {
	// use os.LookupEnv to look up environment variables
	lookupFun := os.LookupEnv
	cfg, err := newYAML(p.Environment, lookupFun)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Provider: cfg,
	}, nil
}

// getConfigDirs extract config dirs from env if ConfigPath was set as environment variable,
// otherwise use default config dir
func getConfigDirs(env env.Context) []string {
	// Allow overriding the directory where config is loaded from
	if env.ConfigPath != "" {
		return strings.Split(env.ConfigPath, _configKeySeparator)
	}
	return []string{_defaultConfigDir}
}
