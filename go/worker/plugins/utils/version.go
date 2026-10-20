package utils

import (
	"fmt"

	"github.com/cadence-workflow/starlark-worker/workflow"
	tmpworkflow "go.temporal.io/sdk/workflow"
	cadworkflow "go.uber.org/cadence/workflow"
)

// DefaultVersion is the version GetVersion returns to a workflow execution that reached the change point
// before the change was deployed. It equals workflow.DefaultVersion of both the Cadence and the Temporal SDK.
const DefaultVersion = -1

// GetVersion is the engine-native workflow.GetVersion, which starlark-worker does not expose. A workflow
// execution that reached changeID before the change was deployed replays it as DefaultVersion; any other
// execution records maxSupported. The first result for changeID is fixed for the rest of the execution.
//
// Params:
// - ctx: The workflow context of a Cadence or a Temporal workflow.
// - changeID: Identifies the change.
// - minSupported, maxSupported: The oldest and the newest version the calling code supports.
//
// Returns:
// - int: The version to execute.
// - error: An error if ctx is not a Cadence or a Temporal workflow context.
func GetVersion(ctx workflow.Context, changeID string, minSupported, maxSupported int) (int, error) {
	switch c := ctx.(type) {
	case cadworkflow.Context:
		return int(cadworkflow.GetVersion(c, changeID, cadworkflow.Version(minSupported), cadworkflow.Version(maxSupported))), nil
	case tmpworkflow.Context:
		return int(tmpworkflow.GetVersion(c, changeID, tmpworkflow.Version(minSupported), tmpworkflow.Version(maxSupported))), nil
	default:
		return 0, fmt.Errorf("get version %q: unsupported workflow context %T", changeID, ctx)
	}
}
