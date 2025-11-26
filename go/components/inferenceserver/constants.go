package inferenceserver

import (
	"time"
)

// Controller configuration constants
const (
	// ControllerName is the name of the inference server controller
	ControllerName = "inference-server-controller"

	// Finalizer to ensure proper cleanup
	FinalizerName = "inferenceservers.michelangelo.api/finalizer"

	// Timeout for reconciliation operations
	ReconcilerTimeout = 1 * time.Minute

	// Requeue period for active reconciliation
	ActiveRequeueAfter = 1 * time.Minute

	// Requeue period for steady state
	SteadyStateRequeueAfter = 10 * time.Minute
)
