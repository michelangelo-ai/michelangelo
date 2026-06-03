package revision

import (
	"context"

	"go.uber.org/yarpc"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type noopManager struct{}

// NewNoOpManager returns a Manager that does nothing.
// Used as the default when revision storage is not configured.
func NewNoOpManager() Manager {
	return &noopManager{}
}

func (m *noopManager) UpsertRevision(_ context.Context, _ client.Object, _ UpsertOpts, _ ...yarpc.CallOption) (bool, error) {
	return false, nil
}
