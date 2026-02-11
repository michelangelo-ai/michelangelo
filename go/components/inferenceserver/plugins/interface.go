package plugins

import (
	"context"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Plugin defines the interface for backend-specific lifecycle management.
// Implementations provide condition plugins for creation, deletion, and state management.
type Plugin interface {
	// GetCreationPlugin returns the plugin for infrastructure creation.
	GetCreationPlugin() conditionInterfaces.Plugin[*v2pb.InferenceServer]

	// GetDeletionPlugin returns the plugin for infrastructure cleanup.
	GetDeletionPlugin(resource *v2pb.InferenceServer) conditionInterfaces.Plugin[*v2pb.InferenceServer]

	// ParseState provides the state based on the set of conditions for a given inference server.
	ParseState(resource *v2pb.InferenceServer) v2pb.InferenceServerState

	// UpdateDetails will get the status for an inference server, and fill annotations, labels, and the status message
	// with plugin specific details.
	UpdateDetails(ctx context.Context, resource *v2pb.InferenceServer) error

	// UpdateConditions updates the current set of conditions for a given inference server.
	UpdateConditions(resource *v2pb.InferenceServer, conditionPlugin conditionInterfaces.Plugin[*v2pb.InferenceServer])
}
