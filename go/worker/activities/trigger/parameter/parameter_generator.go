package parameter

import (
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// ParameterGenerator defines the interface for generating trigger parameters
type ParameterGenerator interface {
	// GenerateBatchParams generates batch parameters for the specific trigger type
	GenerateBatchParams(triggerRun *v2pb.TriggerRun) ([][]Params, error)

	// GenerateConcurrentParams generates concurrent parameters for the specific trigger type
	GenerateConcurrentParams(triggerRun *v2pb.TriggerRun) ([]Params, error)

	// SortParams sorts the parameters alphabetically and chronologically
	SortParams(params []Params)
}
