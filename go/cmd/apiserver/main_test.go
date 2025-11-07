package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/uber-go/tally"
	"go.uber.org/fx"
)

func TestDependenciesAreSatisfied(t *testing.T) {
	assert.NoError(t, fx.ValidateApp(opts()))
}

func TestGetTallyScope(t *testing.T) {
	var scope tally.Scope
	app := fx.New(
		fx.Provide(getTallyScope),
		fx.Populate(&scope),
	)
	assert.NoError(t, app.Err())
	assert.NotNil(t, scope)
}

func TestGetScheme(t *testing.T) {
	scheme, err := getScheme()
	assert.NoError(t, err)
	assert.NotNil(t, scheme)
}
