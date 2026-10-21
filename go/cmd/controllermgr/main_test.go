package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
)

// TestDependenciesAreSatisfied validates the controller manager's fx graph
// without running any constructor. The graph is the same under both
// scheduler backends: jobs.scheduler.backend is read from config inside the
// scheduler's constructors, so options() selects nothing.
func TestDependenciesAreSatisfied(t *testing.T) {
	assert.NoError(t, fx.ValidateApp(options()))
}
