package logging

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultField(t *testing.T) {
	cfg := Config{}
	set := cfg.defaultField("foo", "bar")
	assert.Equal(t, true, set)
	assert.Equal(t, "bar", cfg.InitialFields["foo"])

	set = cfg.defaultField("foo", "baz")
	assert.Equal(t, false, set)
	assert.Equal(t, "bar", cfg.InitialFields["foo"])
}

func TestBuildDefault(t *testing.T) {
	tests := []struct {
		cfg Config
	}{
		{
			cfg: defaultProdConfig(),
		},
		{
			cfg: defaultDevConfig(),
		},
	}
	for _, test := range tests {
		logger, err := test.cfg.build()
		assert.NoError(t, err)
		assert.NotNil(t, logger)
		assert.NotNil(t, logger.GetSink())
	}
}
