package logging

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

func TestDummyAuditLogModule(t *testing.T) {
	var executed = false
	app := fxtest.New(t, DummyAuditLogModule, fx.Invoke(func(auditLog AuditLog) {
		auditLog.Emit(nil, nil)
		executed = true
	}))
	app.RequireStart()
	app.RequireStop()
	assert.True(t, executed)
}
