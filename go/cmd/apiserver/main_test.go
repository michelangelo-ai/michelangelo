package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
)

func TestDependenciesAreSatisfied(t *testing.T) {
	assert.NoError(t, fx.ValidateApp(opts()))
}

func TestGetTallyScope(t *testing.T) {
	scope, err := getTallyScope()
	assert.NoError(t, err)
	assert.NotNil(t, scope)
}

func TestGetScheme(t *testing.T) {
	scheme, err := getScheme()
	assert.NoError(t, err)
	assert.NotNil(t, scheme)
}
