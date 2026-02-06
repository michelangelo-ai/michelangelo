package pluginmanager

import (
	"fmt"

	"github.com/go-logr/logr"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Registrar manages plugin registration and retrieval
type Registrar[T any] interface {
	RegisterPlugin(targetType, subType string, plugin T) error
	GetPlugin(targetType, subType string, deployment *v2pb.Deployment) (T, error)
}

// SimpleRegistrar is a simple implementation of Registrar
type SimpleRegistrar[T any] struct {
	plugins map[string]T
	logger  logr.Logger
}

// NewSimpleRegistrar creates a new simple registrar
func NewSimpleRegistrar[T any](logger logr.Logger) Registrar[T] {
	return &SimpleRegistrar[T]{
		plugins: make(map[string]T),
		logger:  logger,
	}
}

// RegisterPlugin registers a plugin for a target type and subtype
func (r *SimpleRegistrar[T]) RegisterPlugin(targetType, subType string, plugin T) error {
	key := fmt.Sprintf("%s:%s", targetType, subType)
	r.plugins[key] = plugin
	r.logger.Info("Registered plugin", "targetType", targetType, "subType", subType)
	return nil
}

// GetPlugin retrieves a plugin for a target type and subtype
func (r *SimpleRegistrar[T]) GetPlugin(targetType, subType string, deployment *v2pb.Deployment) (T, error) {
	key := fmt.Sprintf("%s:%s", targetType, subType)
	plugin, exists := r.plugins[key]
	if !exists {
		var zero T
		return zero, fmt.Errorf("no plugin found for targetType=%s, subType=%s", targetType, subType)
	}
	return plugin, nil
}
