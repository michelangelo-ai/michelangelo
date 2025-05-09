package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/config"
)

func TestGetYARPCConfig(t *testing.T) {
	yamlConfStr := `
apiserver:
  yarpc:
    host: "127.0.0.1"
    port: 8080
`
	provider, err := config.NewYAML(config.Source(strings.NewReader(yamlConfStr)))
	assert.NoError(t, err)

	conf, err := getYARPCConfig(provider)
	assert.NoError(t, err)
	assert.Equal(t, "127.0.0.1", conf.Host)
	assert.Equal(t, 8080, conf.Port)
}
