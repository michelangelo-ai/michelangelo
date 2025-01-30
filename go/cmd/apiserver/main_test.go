package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
)

func TestDependenciesAreSatisfied(t *testing.T) {
	assert.NoError(t, fx.ValidateApp(opts()))
}

func TestGetDummyAuth(t *testing.T) {
	auth := getDummyAuth()
	assert.NotNil(t, auth)
}

func TestGetTallyScope(t *testing.T) {
	scope, err := getTallyScope()
	assert.NoError(t, err)
	assert.NotNil(t, scope)
}

func TestGetDummyAuditLog(t *testing.T) {
	auditLog := getDummyAuditLog()
	assert.NotNil(t, auditLog)
	auditLog.Emit(nil, nil)
}

func TestGetScheme(t *testing.T) {
	scheme := getScheme()
	assert.NotNil(t, scheme)
}
