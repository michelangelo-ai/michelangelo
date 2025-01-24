// Package zapfx configures and provides a zap logger for structured logging.
// The configuration for the module is specified in YAML. See Config for reference.
package zapfx

import (
	"github.com/michelangelo-ai/michelangelo/go/base/config"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Module provides a zap logger for structured logging.
// See Config for the configuration reference.
var Module = fx.Options(
	fx.Provide(config.ProvideConfig[Config](configKey)),
	fx.Provide(provide),
)

type In struct {
	fx.In
	Config Config
}

type Out struct {
	fx.Out
	Config zap.Config
	Level  zap.AtomicLevel
	Logger *zap.Logger
}

func provide(in In) (Out, error) {
	out := Out{}

	level, err := zap.ParseAtomicLevel(in.Config.Level)
	if err != nil {
		return out, err
	}
	out.Level = level

	development := in.Config.Development
	encoding := in.Config.Encoding
	if encoding == "" {
		if development {
			encoding = "console"
		} else {
			encoding = "json"
		}
	}

	var encoderConfig zapcore.EncoderConfig
	if development {
		encoderConfig = zap.NewDevelopmentEncoderConfig()
	} else {
		encoderConfig = zap.NewProductionEncoderConfig()
	}

	out.Config = zap.Config{
		Level:            level,
		Development:      development,
		Encoding:         encoding,
		EncoderConfig:    encoderConfig,
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stdout"},
	}

	if out.Logger, err = out.Config.Build(); err != nil {
		return out, err
	}

	return out, nil
}
