package compute

import (
	"testing"

	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

func TestDependenciesAreSatisfied(t *testing.T) {
	fxtest.New(t, fx.Options()).RequireStart().RequireStop()
}
