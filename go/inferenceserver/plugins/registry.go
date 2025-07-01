package plugins

import (
	"fmt"
	"sync"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// PluginRegistryImpl implements the PluginRegistry interface
type PluginRegistryImpl struct {
	plugins map[v2pb.BackendType]InferenceServerPlugin
	mutex   sync.RWMutex
}

// NewPluginRegistry creates a new plugin registry
func NewPluginRegistry() PluginRegistry {
	return &PluginRegistryImpl{
		plugins: make(map[v2pb.BackendType]InferenceServerPlugin),
	}
}

// RegisterPlugin registers a plugin for a specific backend type
func (r *PluginRegistryImpl) RegisterPlugin(backendType v2pb.BackendType, plugin InferenceServerPlugin) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	r.plugins[backendType] = plugin
}

// GetPlugin returns the plugin for a given backend type
func (r *PluginRegistryImpl) GetPlugin(backendType v2pb.BackendType) (InferenceServerPlugin, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	plugin, exists := r.plugins[backendType]
	if !exists {
		return nil, fmt.Errorf("no plugin registered for backend type: %v", backendType)
	}
	
	return plugin, nil
}