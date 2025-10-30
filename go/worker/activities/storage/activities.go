// Package storage provides functionality for reading data using different storage protocols.
package storage

import (
	"context"
	"fmt"

	"github.com/cadence-workflow/starlark-worker/activity"
	"github.com/cadence-workflow/starlark-worker/workflow"
	jsoniter "github.com/json-iterator/go"
	"go.uber.org/yarpc/yarpcerrors" // YARPC errors for standardized error codes.
	"go.uber.org/zap"               // Logger for structured logging.

	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
)

// Activities is a package-level variable that holds the activities implementation.
var Activities = (*activities)(nil)

// activities holds implementations for different storage protocols.
// Uses context-aware blob store for transparent multi-tenant routing.
type activities struct {
	blobStore *blobstore.ContextAwareBlobStore
}

// Read attempts to read data from the specified path using the given protocol.
// It logs the start of the activity, checks for a valid protocol implementation,
// and wraps any errors using Cadence's CustomError for consistent error handling.
func (a *activities) Read(ctx context.Context, url string) (any, error) {
	// Retrieve logger from context and log the start of the read activity.
	logger := activity.GetLogger(ctx)
	logger.Info("activity-start", zap.Any("url", url))

	// Check if there is an implementation available for the requested protocol.
	data, err := a.blobStore.Get(ctx, url)
	if err != nil {
		// Wrap the error in a Cadence CustomError using YARPC error codes.
		return nil, workflow.NewCustomError(
			ctx,
			fmt.Sprintf("%s: %s", yarpcerrors.FromError(err).Code().String(), err.Error()),
		)
	}

	// Unmarshal the JSON data into a generic interface.
	var result any
	err = jsoniter.Unmarshal(data, &result)
	if err != nil {
		// Wrap the error in a Cadence CustomError using YARPC error codes.
		return nil, workflow.NewCustomError(
			ctx,
			fmt.Sprintf("%s: %s", yarpcerrors.FromError(err).Code().String(), err.Error()),
		)
	}
	// Return the successful result.
	return result, nil
}

// Note: ReadWithProvider method removed - provider routing is now transparent via context
