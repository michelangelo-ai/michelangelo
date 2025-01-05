package config

import (
	"io/ioutil"

	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"os"
	"path/filepath"
	"testing"
)

func TestGetYAMLFiles(t *testing.T) {
	tests := []struct {
		desc  string
		env   env.Context
		files []FileInfo
	}{
		{
			desc: "development",
			env:  env.Context{RuntimeEnvironment: "development"},
			files: []FileInfo{
				{Name: "config/base.yaml", Interpolate: true},
				{Name: "config/development.yaml", Interpolate: true},
				{Name: "config/secrets.yaml", Interpolate: false},
			},
		},
		{
			desc: "staging",
			env:  env.Context{RuntimeEnvironment: "staging"},
			files: []FileInfo{
				{Name: "config/base.yaml", Interpolate: true},
				{Name: "config/staging.yaml", Interpolate: true},
				{Name: "config/secrets.yaml", Interpolate: false},
			},
		},
		{
			desc: "production",
			env:  env.Context{RuntimeEnvironment: "production"},
			files: []FileInfo{
				{Name: "config/base.yaml", Interpolate: true},
				{Name: "config/production.yaml", Interpolate: true},
				{Name: "config/secrets.yaml", Interpolate: false},
			},
		},
		{
			desc: "production with ConfigPath",
			env: env.Context{
				ConfigPath:         "config/one:config/two",
				RuntimeEnvironment: "production",
			},
			files: []FileInfo{
				{Name: "config/one/base.yaml", Interpolate: true},
				{Name: "config/two/base.yaml", Interpolate: true},
				{Name: "config/one/production.yaml", Interpolate: true},
				{Name: "config/two/production.yaml", Interpolate: true},
				{Name: "config/one/secrets.yaml", Interpolate: false},
				{Name: "config/two/secrets.yaml", Interpolate: false},
			},
		},
	}

	for _, test := range tests {
		files := getYAMLFiles(test.env)
		assert.Equal(t, test.files, files)
	}
}

func TestNewYAML(t *testing.T) {
	dir := t.TempDir()
	content := `
redis:
  host: "${HOST:127.0.0.1}"
  port: 10766
`
	type redisConfig struct {
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
	}
	secretContent := `password: ${SECRET:1111}`
	overwriteFile(t, dir, "production.yaml", content)
	overwriteFile(t, dir, "secrets.yaml", secretContent)

	env := env.New().Environment
	env.RuntimeEnvironment = "production"
	env.ConfigPath = dir

	t.Run("load without env variable override", func(t *testing.T) {
		provider, err := newYAML(env, os.LookupEnv)
		assert.NoError(t, err)
		cfg := &redisConfig{}
		provider.Get("redis").Populate(cfg)
		assert.Equal(t, "127.0.0.1", cfg.Host)
		assert.Equal(t, 10766, cfg.Port)
		assert.Equal(t, "${SECRET:1111}", provider.Get("password").String())
	})

	t.Run("load with env variable override", func(t *testing.T) {
		//set env and test variable expanding in yaml
		defer setEnv("HOST", "127.0.0.2")()
		provider, err := newYAML(env, os.LookupEnv)
		assert.NoError(t, err)
		cfg := &redisConfig{}
		provider.Get("redis").Populate(cfg)
		assert.Equal(t, "127.0.0.2", cfg.Host)
	})
}

func overwriteFile(t *testing.T, dir, name, contents string) func() {
	path := filepath.Join(dir, name)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		t.Fatalf("failed to remove file %q: %v", path, err)
	}
	require.NoError(t, ioutil.WriteFile(path, []byte(contents), os.ModePerm))
	return func() {
		require.NoError(t, os.Remove(path), "failed to clean up file %q", name)
	}
}
