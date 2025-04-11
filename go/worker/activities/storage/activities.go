// Package storage provides functionality for reading data using different storage protocols.
package storage

import (
	"context"
	"fmt"

	"github.com/cadence-workflow/starlark-worker/workflow"
	"go.uber.org/yarpc/yarpcerrors" // YARPC errors for standardized error codes.
	"go.uber.org/zap"               // Logger for structured logging.
	"sigs.k8s.io/controller-runtime/pkg/log"

	intf "github.com/michelangelo-ai/michelangelo/go/worker/activities/storage/interface"
)

// Activities is a package-level variable that holds the activities implementation.
var Activities = (*activities)(nil)

// activities holds implementations for different storage protocols.
// The map keys represent protocol names, and the values are Storage implementations.
type activities struct {
	impls map[string]intf.Storage
}

// Read attempts to read data from the specified path using the given protocol.
// It logs the start of the activity, checks for a valid protocol implementation,
// and wraps any errors using Cadence's CustomError for consistent error handling.
func (a *activities) Read(ctx context.Context, protocol string, path string) (any, error) {
	// Retrieve logger from context and log the start of the read activity.
	logger := log.FromContext(ctx)
	logger.Info("activity-start", zap.Any("path", path))

	// Check if there is an implementation available for the requested protocol.
	if impl, ok := a.impls[protocol]; ok {
		// Attempt to read from the storage using the protocol's implementation.
		result, err := impl.Read(ctx, path)
		if err != nil {
			// Wrap the error in a Cadence CustomError using YARPC error codes.
			return nil, workflow.NewCustomError(
				ctx,
				yarpcerrors.FromError(err).Code().String(),
				err.Error(),
			)
		}
		// Return the successful result.
		return result, nil
	}
	// Return an error if the protocol is not supported.
	return nil, workflow.NewCustomError(
		ctx,
		fmt.Sprintf("protocol %s is not supported", protocol),
		"",
	)
}
