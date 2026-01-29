package parameter

import (
	"sort"
	"time"

	"github.com/michelangelo-ai/michelangelo/go/components/triggerrun"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Object alias for map[string]interface{}
type Object = map[string]interface{}

// ParameterGenerator defines the interface for generating trigger parameters
type ParameterGenerator interface {
	// GenerateBatchParams generates batch parameters for the specific trigger type
	GenerateBatchParams(triggerRun *v2pb.TriggerRun) ([][]Params, error)

	// GenerateConcurrentParams generates concurrent parameters for the specific trigger type
	GenerateConcurrentParams(triggerRun *v2pb.TriggerRun) ([]Params, error)
}

// Params is for batch/concurrent run to store parameters for different triggers
type Params struct {
	// ParamID is the parameter ID for cron/interval triggers
	ParamID string
	// Backfill stores execution metadata for backfill triggers (empty for cron/interval triggers)
	Backfill BackfillParam
}

// GetParameterID returns the parameter ID (works for both cron and backfill)
func (p *Params) GetParameterID() string {
	// For backfill trigger, return the parameter ID
	if p.Backfill.ParamID != "" {
		return p.Backfill.ParamID
	}
	// Otherwise it's a cron/interval trigger
	return p.ParamID
}

// GetExecutionTimestamp returns the execution timestamp (different behavior for each type)
func (p *Params) GetExecutionTimestamp(logicalTs time.Time) time.Time {
	// For a backfill trigger, return the execution timestamp
	if p.Backfill.ExecutionTimestamp != nil {
		return *p.Backfill.ExecutionTimestamp
	}
	// Otherwise it's a cron/interval trigger - use logical timestamp
	return logicalTs
}

// TriggeredRun contains information about a triggered pipeline run
type TriggeredRun struct {
	ParamID            string
	PipelineRunName    string
	ExecutionTimestamp time.Time
	CreatedAt          time.Time
	TriggerType        string // "cron", "interval", "backfill", etc.
}

// GetTriggeredRun returns structured information about the triggered run
// This data can be used by the caller to update trigger context in whatever format they need
func (p *Params) GetTriggeredRun(pipelineRunName string, executionTimestamp, createdTimestamp time.Time) TriggeredRun {
	triggerType := triggerrun.TriggerTypeCron
	if p.Backfill.ExecutionTimestamp != nil {
		triggerType = triggerrun.TriggerTypeBackfill
	}

	return TriggeredRun{
		ParamID:            p.GetParameterID(),
		PipelineRunName:    pipelineRunName,
		ExecutionTimestamp: executionTimestamp,
		CreatedAt:          createdTimestamp,
		TriggerType:        triggerType,
	}
}

// sortParams sorts parameters to make cron, backfill and batch rerun triggers deterministic
// For backfill: sorts chronologically by execution timestamp, then alphabetically by param ID
// For cron/interval: sorts alphabetically by param ID
func sortParams(params []Params) {
	sort.Slice(params, func(i, j int) bool {
		// If both have execution timestamps (backfill), sort by timestamp first
		if params[i].Backfill.ExecutionTimestamp != nil && params[j].Backfill.ExecutionTimestamp != nil {
			if (*params[i].Backfill.ExecutionTimestamp).Equal(*params[j].Backfill.ExecutionTimestamp) {
				// If timestamps are equal, sort by param ID
				return params[i].Backfill.ParamID < params[j].Backfill.ParamID
			}
			// Sort by timestamp
			return (*params[i].Backfill.ExecutionTimestamp).Before(*params[j].Backfill.ExecutionTimestamp)
		}
		// For cron/interval (or default), sort by ParamID
		return params[i].ParamID < params[j].ParamID
	})
}

// GetParameterGenerator returns the appropriate generator based on the parameter type
func GetParameterGenerator(triggerType string) ParameterGenerator {
	switch triggerType {
	case triggerrun.TriggerTypeCron, triggerrun.TriggerTypeInterval:
		return &CronParameterGenerator{}
	case triggerrun.TriggerTypeBackfill:
		return &BackfillParameterGenerator{}
	default:
		return &CronParameterGenerator{}
	}
}
