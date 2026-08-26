package handler

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/config"
)

func TestNewConfig_PopulatesFromProvider(t *testing.T) {
	yamlConfig := []byte(`
apiserver:
  pipelineRunDefaults:
    environment: staging
`)
	provider, err := config.NewYAML(config.Source(bytes.NewReader(yamlConfig)))
	assert.NoError(t, err)

	conf, err := NewConfig(provider)
	assert.NoError(t, err)
	assert.Equal(t, "staging", conf.PipelineRunDefaultEnvironment)
}

func TestNewConfig_UnconfiguredDefaultsToEmpty(t *testing.T) {
	provider, err := config.NewYAML(config.Source(bytes.NewReader([]byte(`{}`))))
	assert.NoError(t, err)

	conf, err := NewConfig(provider)
	assert.NoError(t, err)
	assert.Equal(t, "", conf.PipelineRunDefaultEnvironment)
}
