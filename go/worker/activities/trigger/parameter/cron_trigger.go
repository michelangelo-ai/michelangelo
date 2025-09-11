package parameter

import (
	"sort"
	"time"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var _defaultBatchSize = 10

type CronParameterGenerator struct{}

// Params is for batch/concurrent run to store parameters for different triggers
type Params struct {
	ParamID string
	// TriggeredRun is a struct to store triggered run information
	// for backfill trigger, it is initialized with execution timestamp and parameter id when patch created
	// and will be updated with pipeline run name and created timestamp when pipeline run is created
	TriggeredRun struct {
		ExecutionTimestamp *time.Time
		ParameterID        string
		PipelineRunName    string
		CreatedAt          *time.Time
	}
}

// GenerateBatchParams generates batch parameters for cron/interval triggers
func (g *CronParameterGenerator) GenerateBatchParams(triggerRun *v2pb.TriggerRun) ([][]Params, error) {
	batchSize := _defaultBatchSize
	if triggerRun.Spec.Trigger.BatchPolicy != nil && triggerRun.Spec.Trigger.BatchPolicy.BatchSize != 0 {
		batchSize = int(triggerRun.Spec.Trigger.BatchPolicy.BatchSize)
	}
	paramsMap := triggerRun.Spec.Trigger.ParametersMap
	numOfBatches := 1
	if len(paramsMap) > 0 {
		if len(paramsMap)%batchSize == 0 {
			numOfBatches = len(paramsMap) / batchSize
		} else {
			numOfBatches = len(paramsMap)/batchSize + 1
		}
	}

	batchedParams := make([][]Params, numOfBatches)
	// no parameters are defined for this trigger
	if len(paramsMap) == 0 {
		batchedParams[0] = []Params{{ParamID: ""}}
		return batchedParams, nil
	}
	keys := make([]Params, 0, len(paramsMap))
	for k := range paramsMap {
		cur := Params{
			ParamID: k,
		}
		keys = append(keys, cur)
	}
	g.SortParams(keys)
	for i := 0; i < len(keys); i = i + batchSize {
		if i+batchSize <= len(keys) {
			batchedParams[i/batchSize] = keys[i : i+batchSize]
		} else {
			batchedParams[i/batchSize] = keys[i:]
		}
	}
	return batchedParams, nil
}

// GenerateConcurrentParams generates concurrent parameters for cron/interval triggers
func (g *CronParameterGenerator) GenerateConcurrentParams(triggerRun *v2pb.TriggerRun) ([]Params, error) {
	params := make([]Params, len(triggerRun.Spec.Trigger.ParametersMap))
	i := 0
	for paramID := range triggerRun.Spec.Trigger.ParametersMap {
		params[i] = Params{
			ParamID: paramID,
		}
		i++
	}
	g.SortParams(params)
	return params, nil
}

// SortParams sorts the parameters alphabetically and chronologically
func (g *CronParameterGenerator) SortParams(params []Params) {
	sort.Slice(params, func(i, j int) bool {
		return params[i].ParamID < params[j].ParamID
	})
}
