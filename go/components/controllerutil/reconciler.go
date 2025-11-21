package controllerutil

import (
	"context"
)

// LeaderOnlyRunnable is a Runnable that should only run on the leader node.
// It's very important that the given function block
// until it's done running.
type LeaderOnlyRunnable func(context.Context) error

// Start implements Runnable.
func (r LeaderOnlyRunnable) Start(ctx context.Context) error {
	return r(ctx)
}

// NeedLeaderElection returns true to indicate that the Runnable should only run on the leader node.
func (r LeaderOnlyRunnable) NeedLeaderElection() bool {
	return true
}

// NonLeaderRunnable is a Runnable that should run on all nodes.
// It's very important that the given function block
// until it's done running.
type NonLeaderRunnable func(context.Context) error

// Start implements Runnable.
func (r NonLeaderRunnable) Start(ctx context.Context) error {
	return r(ctx)
}

// NeedLeaderElection returns false to indicate that the Runnable should run on all nodes.
func (r NonLeaderRunnable) NeedLeaderElection() bool {
	return false
}
