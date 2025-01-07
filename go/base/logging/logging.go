package logging

import (
	"context"
	"fmt"

	"github.com/go-logr/zapr"

	"github.com/go-logr/logr"
	envfx "github.com/michelangelo-ai/michelangelo/go/base/env"
	"go.uber.org/config"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Params defines the dependencies of the logging fx module.
type Params struct {
	fx.In

	Environment    envfx.Context
	ConfigProvider config.Provider
	Lifecycle      fx.Lifecycle
}

// Result defines the objects that the logging fx module provides.
type Result struct {
	fx.Out

	Logger logr.Logger
}

// Module provides a logr Logger based on the config and environment context.
// More information about logr can be found at: https://pkg.go.dev/github.com/go-logr/logr
//
// Usage:
// logger.Info("setting target", "value", targetValue)
// logger.Error(err, "failed to open the pod bay door", "user", user)
// logger.V(4).Info("you should only see this when verbosity level is high")
// Verbosity:
// Logger has a V() method to set verbosity. The higher the V-level of a log line, the less critical it is considered.
// Level V(0) is the default, and logger.V(0).Info() has the same meaning as logger.Info()
// For Zapr implementation, V(-2) is ErrorLevel, V(-1) is WarnLevel, V(0) is InfoLevel, V(1) is DebugLevel ...
var Module = fx.Module("logging",
	fx.Provide(New),
)

// New exports functionality similar to Module, but allows the caller to wrap
// or modify Result. Most users should use Module instead.
func New(p Params) (Result, error) {
	cfg, err := newConfig(p.ConfigProvider, p.Environment)
	if err != nil {
		return Result{}, err
	}

	logger, err := cfg.build()
	if err != nil {
		return Result{}, err
	}
	p.Lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			logger.Info("starting up with environment", "environment", p.Environment)
			return nil
		},
		// flush buffer before the application exit
		OnStop: func(context.Context) error {
			if zapLogger, ok := logger.GetSink().(zapr.Underlier); ok {
				_ = zapLogger.GetUnderlying().Core().Sync()
				return nil
			}
			return fmt.Errorf("failed to flush log buffer before exit")
		},
	})
	return Result{
		Logger: logger,
	}, nil
}

// newConfig generates a Config by merging env based default with specs provided in yaml config file
func newConfig(p config.Provider, env envfx.Context) (Config, error) {
	var cfg Config
	switch env.RuntimeEnvironment {
	case envfx.EnvProduction, envfx.EnvStaging:
		cfg = defaultProdConfig()
	default:
		cfg = defaultDevConfig()
	}
	// merge yaml config with default config based on env
	if err := p.Get(_configKey).Populate(&cfg); err != nil {
		return Config{}, fmt.Errorf("failed to load logging config: %v", err)
	}
	if cfg.Development {
		return cfg, nil
	}
	// add initial fields for production env
	cfg.defaultField("runtime_env", env.RuntimeEnvironment)
	cfg.defaultField("hostname", env.Hostname)
	return cfg, nil
}

func defaultDevConfig() Config {
	return Config{
		VerbosityLevel: 1, // Debug Level
		Development:    true,
		Encoding:       "console",
		OutputPaths:    []string{"stdout"},
	}
}

func defaultProdConfig() Config {
	return Config{
		VerbosityLevel: 0, // Info Level
		Encoding:       "json",
		Sampling: &zap.SamplingConfig{
			Initial:    100,
			Thereafter: 100,
		},
		OutputPaths: []string{"stdout"},
	}
}
