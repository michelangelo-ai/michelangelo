package logging

import (
	"os"
	"testing"

	"github.com/go-logr/logr"

	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	"go.uber.org/zap"

	"github.com/stretchr/testify/assert"

	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"go.uber.org/config"
)

func TestNewConfig(t *testing.T) {
	tests := []struct {
		desc     string
		p        config.Provider
		e        env.Context
		expected Config
	}{
		{
			desc: "prod with no yaml config provided",
			p:    newStaticProvider(t, map[string]interface{}{}),
			e: env.Context{
				Hostname:           "host123",
				RuntimeEnvironment: env.EnvProduction,
			},
			expected: Config{
				VerbosityLevel: 0, // Info Level
				Encoding:       "json",
				Sampling: &zap.SamplingConfig{
					Initial:    100,
					Thereafter: 100,
				},
				OutputPaths: []string{"stdout"},
				InitialFields: map[string]interface{}{
					"runtime_env": env.EnvProduction,
					"hostname":    "host123",
				},
			},
		},
		{
			desc: "dev with no yaml config provided",
			p:    newStaticProvider(t, map[string]interface{}{}),
			e: env.Context{
				Hostname:           "host123",
				RuntimeEnvironment: env.EnvDevelopment,
			},
			expected: Config{
				VerbosityLevel: 1, // Debug Level
				Development:    true,
				Encoding:       "console",
				OutputPaths:    []string{"stdout"},
			},
		},
		{
			desc: "prod with yaml config provided",
			p: newStaticProvider(t, map[string]interface{}{
				"logging": map[string]interface{}{
					"verbosityLevel": 4,
					"outputPaths":    []string{"stdout", "/var/logs/temp.log"},
					"initialFields": map[string]string{
						"language": "go",
					},
					"disableStacktrace": true,
				},
			}),
			e: env.Context{
				Hostname:           "host123",
				RuntimeEnvironment: env.EnvProduction,
			},
			expected: Config{
				VerbosityLevel: 4,
				Development:    false,
				Encoding:       "json",
				Sampling: &zap.SamplingConfig{
					Initial:    100,
					Thereafter: 100,
				},
				OutputPaths: []string{"stdout", "/var/logs/temp.log"},
				InitialFields: map[string]interface{}{
					"runtime_env": env.EnvProduction,
					"hostname":    "host123",
					"language":    "go",
				},
				DisableStacktrace: true,
			},
		},
	}

	for _, test := range tests {
		cfg, err := newConfig(test.p, test.e)
		assert.NoError(t, err)
		assert.Equal(t, test.expected, cfg)
	}
}

func TestEndToEnd(t *testing.T) {
	tests := []struct {
		RuntimeEnvironment string
		VerboseMsg         string
		DebugMsg           string
		InfoMsg            string
		SeeDebugMsg        bool
	}{
		{
			RuntimeEnvironment: env.EnvProduction,
			VerboseMsg:         "verbose info log",
			DebugMsg:           "debug msg in prod env",
			InfoMsg:            "info msg in prod env",
			SeeDebugMsg:        false,
		},
		{
			RuntimeEnvironment: env.EnvStaging,
			VerboseMsg:         "verbose info log",
			DebugMsg:           "debug msg in staging env",
			InfoMsg:            "info msg in staging env",
			SeeDebugMsg:        false,
		},
		{
			RuntimeEnvironment: env.EnvDevelopment,
			VerboseMsg:         "verbose info log",
			DebugMsg:           "debug msg in development env",
			InfoMsg:            "info msg in development env",
			SeeDebugMsg:        true,
		},
		{
			RuntimeEnvironment: env.EnvTest,
			VerboseMsg:         "verbose info log",
			DebugMsg:           "debug msg in test env",
			InfoMsg:            "info msg in test env",
			SeeDebugMsg:        true,
		},
	}

	for _, test := range tests {
		t.Run(test.RuntimeEnvironment, func(t *testing.T) {
			tempFile := t.TempDir() + "/logr.log"
			app := fxtest.New(t,
				fx.Provide(
					New,
					func() env.Context {
						return env.Context{RuntimeEnvironment: test.RuntimeEnvironment}
					},
					func() config.Provider {
						return newStaticProvider(t, map[string]interface{}{
							"logging": map[string]interface{}{
								"outputPaths": []string{tempFile},
							},
						})
					},
				),
				fx.Invoke(func(logger logr.Logger) {
					logger.Info(test.InfoMsg)
					logger.V(1).Info(test.DebugMsg)
					logger.V(4).Info(test.VerboseMsg)
				}),
			)
			app.RequireStart().RequireStop()
			output, err := os.ReadFile(tempFile)
			assert.NoError(t, err, "Failed to read log file")
			assert.Contains(t, string(output), test.InfoMsg, "Should contain info msg")
			if test.SeeDebugMsg {
				assert.Contains(t, string(output), test.DebugMsg, "Should contain debug msg")
			}
			assert.NotContainsf(t, string(output), test.VerboseMsg, "Should not log verbose msg")
		})
	}
}

func newStaticProvider(t testing.TB, data map[string]interface{}) config.Provider {
	p, err := config.NewYAML(config.Static(data))
	if err != nil {
		t.Fatalf("Failed to create static provider: %v", err)
	}
	return p
}
