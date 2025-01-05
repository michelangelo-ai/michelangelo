package logging

import (
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	_configKey = "logging"
)

type Config struct {
	// Controls the verbosity level of the logger, default is 0
	// For Zapr implementation, -2 is ErrorLevel, -1 is WarnLevel, 0 is InfoLevel, 1 is DebugLevel
	VerbosityLevel int32 `yaml:"verbosityLevel"`

	// Development enables a handful of behaviors useful during development and
	// testing. First, it alters the behavior of the logger's DPanic ("panic in
	// development") method to panic instead of just writing an error. Second,
	// it reduces console noise by omitting some fields
	Development bool `yaml:"development"`

	// Sampling sets a sampling policy. A nil SamplingConfig disables sampling.
	Sampling *zap.SamplingConfig `yaml:"sampling,omitempty"`

	// OutputPaths is a list of URLs or file paths to write logging output to.
	OutputPaths []string `yaml:"outputPaths"`

	// Encoding specifies the output encoding. Valid options are "console" and "json".
	Encoding string `yaml:"encoding"`

	// InitialFields adds predefined context to every log message.
	// e.g. your service's name, the hostname
	InitialFields map[string]interface{} `yaml:"initialFields"`

	// DisableCaller instructs the logger not to log annotations identifying
	// the calling function.
	DisableCaller bool `yaml:"disableCaller"`

	// DisableStacktrace disables zap's automatic stacktrace collection. By
	// default, stacktraces are included in error-and-above logs in production
	// and warn-and-above logs in development.
	DisableStacktrace bool `yaml:"disableStacktrace"`
}

// defaultField sets a kv in InitialFields if it's not already present.
// defaultField returns true if it set the field and false if the field was
// already present.
func (c *Config) defaultField(key string, val interface{}) bool {
	// create fields map if missing
	if c.InitialFields == nil {
		c.InitialFields = make(map[string]interface{})
	}

	_, ok := c.InitialFields[key]
	if !ok {
		c.InitialFields[key] = val
		return true
	}
	return false
}

// builds a loger Logger with provided configuration
func (c *Config) build() (logr.Logger, error) {
	lvl := zapcore.Level(-1 * c.VerbosityLevel)
	atomicLvl := zap.NewAtomicLevelAt(lvl)
	z := zap.Config{
		Level:             atomicLvl,
		Development:       c.Development,
		DisableCaller:     c.DisableCaller,
		DisableStacktrace: c.DisableStacktrace,
		Encoding:          c.Encoding,
		Sampling:          c.Sampling,
		OutputPaths:       c.OutputPaths,
		ErrorOutputPaths:  []string{"stderr"},
		InitialFields:     c.InitialFields,
	}

	if z.Development {
		z.EncoderConfig = zap.NewDevelopmentEncoderConfig()
	} else {
		z.EncoderConfig = zap.NewProductionEncoderConfig()
		z.EncoderConfig.MessageKey = "message"
		z.EncoderConfig.StacktraceKey = "stack"
	}

	zLogger, err := z.Build()
	if err != nil {
		return zapr.NewLogger(zap.NewNop()), err
	}
	logger := zapr.NewLogger(zLogger)
	return logger, nil
}
