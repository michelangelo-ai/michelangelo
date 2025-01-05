package config

import (
	envfx "github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/stretchr/testify/assert"

	"os"
	"testing"
)

func TestNew(t *testing.T) {
	dir := t.TempDir()
	defer overwriteFile(t, dir, "base.yaml", "foo: ${ENV_FOO:123}")()
	defer overwriteFile(t, dir, "secrets.yaml", "password: ${PASSWORD}")()

	newEnv := func() envfx.Context {
		e := envfx.New().Environment
		e.ConfigPath = dir
		return e
	}

	t.Run("base only without env", func(t *testing.T) {
		cfg, err := New(Params{
			Environment: newEnv(),
		})
		assert.NoError(t, err)
		assert.Equal(t, "123", cfg.Provider.Get("foo").String())
		assert.Equal(t, "${PASSWORD}", cfg.Provider.Get("password").String())
	})

	t.Run("base with env override", func(t *testing.T) {
		defer setEnv("ENV_FOO", "666")()
		defer setEnv("PASSWORD", "fake_password")()
		cfg, err := New(Params{
			Environment: newEnv(),
		})
		assert.NoError(t, err)
		assert.Equal(t, "666", cfg.Provider.Get("foo").String())
		// secrets file is not expanded
		assert.Equal(t, "${PASSWORD}", cfg.Provider.Get("password").String())
	})

	t.Run("base with production override", func(t *testing.T) {
		defer overwriteFile(t, dir, "production.yaml", "foo: bar")()
		defer setEnv("RUNTIME_ENVIRONMENT", "production")()
		cfg, err := New(Params{Environment: newEnv()})
		assert.NoError(t, err)
		assert.Equal(t, "bar", cfg.Provider.Get("foo").String())
		assert.Equal(t, "${PASSWORD}", cfg.Provider.Get("password").String())
	})
}

func TestGetConfigDirs(t *testing.T) {
	t.Run("env not set", func(t *testing.T) {
		env := envfx.New().Environment
		dirs := getConfigDirs(env)
		assert.Equal(t, []string{"config"}, dirs)
	})

	t.Run("env with CONFIG_DIR", func(t *testing.T) {
		defer setEnv("CONFIG_DIR", "config/one:config/two")()
		env := envfx.New().Environment
		dirs := getConfigDirs(env)
		assert.Equal(t, []string{"config/one", "config/two"}, dirs)
	})
}

// setEnv sets the environment variable with provided key-value pair and returns a function to revert the change.
// often used in a deferred function call
func setEnv(key, value string) func() {
	res := func() { os.Unsetenv(key) }
	if oldVal, present := os.LookupEnv(key); present {
		res = func() { os.Setenv(key, oldVal) }
	}

	os.Setenv(key, value)
	return res
}
