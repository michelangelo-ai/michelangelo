package apihook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/config"
)

const releasesDir = "testdata/releases"

func provider(t *testing.T, yaml string) config.Provider {
	t.Helper()
	p, err := config.NewYAML(config.Source(strings.NewReader(yaml)))
	require.NoError(t, err)
	return p
}

func TestNewConfig(t *testing.T) {
	t.Run("key absent", func(t *testing.T) {
		cfg, err := NewConfig(provider(t, "apiserver: {}\n"))
		require.NoError(t, err)
		assert.Equal(t, Config{}, cfg)
	})
	t.Run("camelCase keys", func(t *testing.T) {
		cfg, err := NewConfig(provider(t, `
apiserver:
  declarative:
    enabled: true
    defaultInterpreter: core-v1
    releasesDir: /etc/releases
`))
		require.NoError(t, err)
		assert.Equal(t, Config{Enabled: true, DefaultInterpreter: "core-v1", ReleasesDir: "/etc/releases"}, cfg)
	})
}

func TestNewDeclarative(t *testing.T) {
	t.Run("disabled loads nothing", func(t *testing.T) {
		d, err := NewDeclarative(Config{ReleasesDir: "/does/not/exist"})
		require.NoError(t, err)
		assert.False(t, d.enabled)
		assert.Empty(t, d.releases)
	})
	t.Run("enabled loads releases and finds the default", func(t *testing.T) {
		d, err := NewDeclarative(Config{Enabled: true, DefaultInterpreter: "core-v1", ReleasesDir: releasesDir})
		require.NoError(t, err)
		assert.True(t, d.enabled)
		assert.ElementsMatch(t, []string{"core-v1", "core-v2"}, d.Versions())
		assert.Equal(t, "core-v1", d.defaultRelease.Version)
	})
	t.Run("default has no release file", func(t *testing.T) {
		_, err := NewDeclarative(Config{Enabled: true, DefaultInterpreter: "x", ReleasesDir: releasesDir})
		require.EqualError(t, err, `default interpreter "x" has no release file in testdata/releases`)
	})
	t.Run("LoadReleases fails", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("version: other\n"), 0o600))
		_, err := NewDeclarative(Config{Enabled: true, DefaultInterpreter: "bad", ReleasesDir: dir})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `release file "bad.yaml"`)
	})
}

func TestDeclarativeAccessors(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		d, err := NewDeclarative(Config{})
		require.NoError(t, err)
		assert.False(t, d.Enabled())
		r, ok := d.Release("core-v1")
		assert.False(t, ok)
		assert.Nil(t, r)
	})
	t.Run("enabled", func(t *testing.T) {
		d, err := NewDeclarative(Config{Enabled: true, DefaultInterpreter: "core-v1", ReleasesDir: releasesDir})
		require.NoError(t, err)
		assert.True(t, d.Enabled())
		r, ok := d.Release("core-v2")
		require.True(t, ok)
		assert.Equal(t, "core-v2", r.Version)
		assert.Equal(t, "s3://ma-artifacts/interpreters/core-v2.tar.gz", r.Artifact)
		_, ok = d.Release("core-v9")
		assert.False(t, ok)
	})
}
