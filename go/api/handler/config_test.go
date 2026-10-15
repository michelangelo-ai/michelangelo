package handler

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/config"
)

func TestNewConfig_RestrictedBranches(t *testing.T) {
	t.Run("populated with multiple environment keys", func(t *testing.T) {
		yaml := `
apiserver:
  pipelineRunDefaults:
    environment: "production"
    restrictedBranches:
      production:
        - main
        - master
      staging:
        - release
`
		provider, err := config.NewYAML(config.Source(strings.NewReader(yaml)))
		require.NoError(t, err)

		conf, err := NewConfig(provider)
		require.NoError(t, err)

		assert.Equal(t, "production", conf.PipelineRunDefaultEnvironment)
		assert.Equal(t, map[string][]string{
			"production": {"main", "master"},
			"staging":    {"release"},
		}, conf.RestrictedBranches)
	})

	t.Run("single environment key with single branch", func(t *testing.T) {
		yaml := `
apiserver:
  pipelineRunDefaults:
    restrictedBranches:
      prod:
        - main
`
		provider, err := config.NewYAML(config.Source(strings.NewReader(yaml)))
		require.NoError(t, err)

		conf, err := NewConfig(provider)
		require.NoError(t, err)

		assert.Equal(t, map[string][]string{"prod": {"main"}}, conf.RestrictedBranches)
	})

	t.Run("absent restrictedBranches key yields nil map", func(t *testing.T) {
		yaml := `
apiserver:
  pipelineRunDefaults:
    environment: "development"
`
		provider, err := config.NewYAML(config.Source(strings.NewReader(yaml)))
		require.NoError(t, err)

		conf, err := NewConfig(provider)
		require.NoError(t, err)

		assert.Equal(t, "development", conf.PipelineRunDefaultEnvironment)
		// go.uber.org/config's Populate leaves an absent map-typed field as
		// its Go zero value (nil), not an empty non-nil map. Both satisfy
		// api.IsBranchAllowedForEnvironment's !ok check, but we assert the
		// actual behavior so this test documents the real contract.
		assert.Nil(t, conf.RestrictedBranches)
	})

	t.Run("entirely absent pipelineRunDefaults block yields zero-value config", func(t *testing.T) {
		yaml := `
someOtherKey: value
`
		provider, err := config.NewYAML(config.Source(strings.NewReader(yaml)))
		require.NoError(t, err)

		conf, err := NewConfig(provider)
		require.NoError(t, err)

		assert.Equal(t, "", conf.PipelineRunDefaultEnvironment)
		assert.Nil(t, conf.RestrictedBranches)
	})

	t.Run("existing PipelineRunDefaultEnvironment field still works", func(t *testing.T) {
		yaml := `
apiserver:
  pipelineRunDefaults:
    environment: "staging"
`
		provider, err := config.NewYAML(config.Source(strings.NewReader(yaml)))
		require.NoError(t, err)

		conf, err := NewConfig(provider)
		require.NoError(t, err)

		assert.Equal(t, "staging", conf.PipelineRunDefaultEnvironment)
		assert.Nil(t, conf.RestrictedBranches)
	})
}
