package conditions

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
)

// Result represents the result of running a conditions plugin
type Result struct {
	Result     ctrl.Result
	IsTerminal bool
}

// RequestContext provides logging and event recording capabilities
type RequestContext struct {
	Logger   logr.Logger
	Recorder record.EventRecorder
}

// NewRequestContext creates a new request context
func NewRequestContext(logger logr.Logger, recorder record.EventRecorder) RequestContext {
	return RequestContext{
		Logger:   logger,
		Recorder: recorder,
	}
}

// Plugin represents a conditions plugin that can be executed
type Plugin[T any] interface {
	// Execute runs the plugin logic
	Execute(ctx context.Context, runtimeContext RequestContext, resource T) (Result, error)
}

// Engine manages the execution of conditions plugins
type Engine[T any] interface {
	// Run executes a plugin and returns the result
	Run(ctx context.Context, runtimeContext RequestContext, plugin Plugin[T], resource T) (Result, error)
}

// NoOpEngine is a simple engine that always returns success
type NoOpEngine[T any] struct{}

// NewNoOpEngine creates a new no-op engine
func NewNoOpEngine[T any]() Engine[T] {
	return &NoOpEngine[T]{}
}

// Run always returns success for the no-op engine
func (e *NoOpEngine[T]) Run(ctx context.Context, runtimeContext RequestContext, plugin Plugin[T], resource T) (Result, error) {
	if plugin == nil {
		return Result{
			Result:     ctrl.Result{},
			IsTerminal: false,
		}, nil
	}

	// Execute the plugin
	return plugin.Execute(ctx, runtimeContext, resource)
}

// NoOpPlugin is a plugin that always succeeds
type NoOpPlugin[T any] struct{}

// NewNoOpPlugin creates a new no-op plugin
func NewNoOpPlugin[T any]() Plugin[T] {
	return &NoOpPlugin[T]{}
}

// Execute always returns success for the no-op plugin
func (p *NoOpPlugin[T]) Execute(ctx context.Context, runtimeContext RequestContext, resource T) (Result, error) {
	return Result{
		Result:     ctrl.Result{},
		IsTerminal: false,
	}, nil
}