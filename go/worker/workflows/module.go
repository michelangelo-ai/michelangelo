package workflows

import (
	"github.com/michelangelo-ai/michelangelo/go/worker/workflows/ray"
	"github.com/michelangelo-ai/michelangelo/go/worker/workflows/trigger"
	"go.uber.org/fx"
)

// Module provides the ray and trigger workflow registrations for the shared
// worker binary. Notification workflows are registered separately by the
// notification-worker binary (go/cmd/notification-worker).
var Module = fx.Options(
	ray.Module,
	trigger.Module,
)
