package parameter

import (
	"time"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// Object alias for map[string]interface{}
type Object = map[string]interface{}

// ParameterHandler defines the interface for handling trigger parameters
type ParameterHandler interface {
	// GenerateBatchParams generates batch parameters for the specific trigger type
	GenerateBatchParams(triggerRun *v2pb.TriggerRun) ([][]Params, error)

	// GenerateConcurrentParams generates concurrent parameters for the specific trigger type
	GenerateConcurrentParams(triggerRun *v2pb.TriggerRun) ([]Params, error)

	// SortParams sorts the parameters alphabetically and chronologically
	SortParams(params []Params)

	// GetParameterID returns the parameter ID from the params
	GetParameterID(param Params) string

	// GetExecutionTimestamp returns the execution timestamp for the params
	GetExecutionTimestamp(param Params, logicalTs time.Time) time.Time

	// UpdateTriggerContext updates the trigger workflow context with the parameters
	UpdateTriggerContext(triggerContext Object, param Params, pipelineRunName string, createdTimestamp time.Time)
}

// GetHandler returns the appropriate handler based on the parameter type
func (p *Params) GetHandler() ParameterHandler {
	// Check if this is a backfill trigger by checking if Backfill has ExecutionTimestamp
	if p.Backfill.ExecutionTimestamp != nil {
		return &BackfillParameterHandler{}
	}
	// Otherwise it's a cron trigger
	return &CronParameterHandler{}
}
