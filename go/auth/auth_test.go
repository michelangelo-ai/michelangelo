package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

func TestDummyAuthModule(t *testing.T) {
	var executed = false
	app := fxtest.New(t, DummyAuthModule, fx.Invoke(func(auth Auth) {
		authenticate, err := auth.UserAuthenticated(context.Background())
		assert.NoError(t, err)
		assert.True(t, authenticate)
		authorize, err := auth.UserAuthorized(context.Background(), "namespace", Create, "Project")
		assert.NoError(t, err)
		assert.True(t, authorize)
		executed = true
	}))
	app.RequireStart()
	app.RequireStop()
	assert.True(t, executed)
}
