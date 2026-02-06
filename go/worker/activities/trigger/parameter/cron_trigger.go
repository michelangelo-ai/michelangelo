package parameter

import (
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _defaultBatchSize = 10

type CronParameterGenerator struct{}

// GenerateBatchParams generates batch parameters for cron/interval triggers.
//
// Cron/interval triggers execute pipeline runs on a recurring schedule (e.g., daily at 8 AM, every 6 hours).
// Unlike backfill, cron triggers execute for a SINGLE timestamp (the current trigger time).
//
// This method creates batches from the trigger's ParametersMap, where each parameter represents
// a different configuration or input dataset for the same execution time.
//
// Example with 5 parameters (p1, p2, p3, p4, p5) and BatchSize=2:
//
//	Total runs: 5 runs at the same execution timestamp
//	Returns: [[p1, p2], [p3, p4], [p5]]
//
// Execution flow (with BatchPolicy.WaitSeconds=60):
//
//	Batch 1: Run p1 and p2 sequentially → wait 60 seconds
//	Batch 2: Run p3 and p4 sequentially → wait 60 seconds
//	Batch 3: Run p5
//
// Use case: Batch execution is useful when you want to process parameter sets sequentially
// with controlled delays between batches.
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
		keys = append(keys, Params{ParamID: k})
	}
	sortParams(keys)
	for i := 0; i < len(keys); i = i + batchSize {
		if i+batchSize <= len(keys) {
			batchedParams[i/batchSize] = keys[i : i+batchSize]
		} else {
			batchedParams[i/batchSize] = keys[i:]
		}
	}
	return batchedParams, nil
}

// GenerateConcurrentParams generates concurrent parameters for cron/interval triggers.
//
// Similar to GenerateBatchParams, this returns all parameters from ParametersMap,
// but they are executed with controlled concurrency using a worker pool pattern.
//
// Example with 5 parameters (p1, p2, p3, p4, p5) and MaxConcurrency=2:
//
//	Total runs: 5 runs at the same execution timestamp
//	Returns: [p1, p2, p3, p4, p5]
//
// Execution flow (with MaxConcurrency=2):
//   - Start: p1 and p2 running (queue: [p3, p4, p5])
//   - p1 finishes → immediately start p3 (p2 and p3 now running, queue: [p4, p5])
//   - p2 finishes → immediately start p4 (p3 and p4 now running, queue: [p5])
//   - p3 finishes → immediately start p5 (p4 and p5 now running, queue: [])
//   - p4 finishes (only p5 running)
//   - p5 finishes (done)
//
// This maintains exactly MaxConcurrency runs executing at any time,
// maximizing throughput while controlling resource usage.
func (g *CronParameterGenerator) GenerateConcurrentParams(triggerRun *v2pb.TriggerRun) ([]Params, error) {
	params := make([]Params, 0, len(triggerRun.Spec.Trigger.ParametersMap))
	for paramID := range triggerRun.Spec.Trigger.ParametersMap {
		params = append(params, Params{ParamID: paramID})
	}
	sortParams(params)
	return params, nil
}
