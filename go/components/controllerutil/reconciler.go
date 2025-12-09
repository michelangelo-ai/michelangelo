// Package controllerutil provides utilities for Kubernetes controller implementations,
// particularly for managing leader election and runnable execution patterns.
// It offers wrapper types that implement the Runnable interface with configurable
// leader election behavior.
package controllerutil

import (
	"context"
)

// LeaderOnlyRunnable is a function type that implements the Runnable interface
// and executes only on the leader node in a high-availability controller setup.
//
// This type is used to wrap functions that should be executed exclusively by the
// elected leader in a controller-runtime environment. Common use cases include:
//   - Reconciliation loops that modify cluster state
//   - Periodic cleanup operations
//   - Singleton background tasks
//
// The wrapped function must block until it completes or the context is cancelled.
// Returning from the function signals completion to the controller runtime.
//
// Example:
//
//	mgr.Add(controllerutil.LeaderOnlyRunnable(func(ctx context.Context) error {
//	    return runLeaderTask(ctx)
//	}))
type LeaderOnlyRunnable func(context.Context) error

// Start implements the Runnable interface by executing the wrapped function.
// It is called by the controller-runtime manager when starting the runnable.
//
// The function blocks until the wrapped function returns or the context is cancelled.
func (r LeaderOnlyRunnable) Start(ctx context.Context) error {
	return r(ctx)
}

// NeedLeaderElection implements the LeaderElectionRunnable interface.
// It returns true to indicate that this runnable should only execute on the
// elected leader node in a multi-instance deployment.
func (r LeaderOnlyRunnable) NeedLeaderElection() bool {
	return true
}

// NonLeaderRunnable is a function type that implements the Runnable interface
// and executes on all nodes in a high-availability controller setup.
//
// This type is used to wrap functions that should run on every instance of the
// controller, regardless of leader election status. Common use cases include:
//   - Health check endpoints
//   - Metrics collection
//   - Local cache warming
//   - Webhooks that need to run on all instances
//
// The wrapped function must block until it completes or the context is cancelled.
// Returning from the function signals completion to the controller runtime.
//
// Example:
//
//	mgr.Add(controllerutil.NonLeaderRunnable(func(ctx context.Context) error {
//	    return startHealthCheckServer(ctx)
//	}))
type NonLeaderRunnable func(context.Context) error

// Start implements the Runnable interface by executing the wrapped function.
// It is called by the controller-runtime manager when starting the runnable.
//
// The function blocks until the wrapped function returns or the context is cancelled.
func (r NonLeaderRunnable) Start(ctx context.Context) error {
	return r(ctx)
}

// NeedLeaderElection implements the LeaderElectionRunnable interface.
// It returns false to indicate that this runnable should execute on all nodes,
// not just the elected leader, in a multi-instance deployment.
func (r NonLeaderRunnable) NeedLeaderElection() bool {
	return false
}
