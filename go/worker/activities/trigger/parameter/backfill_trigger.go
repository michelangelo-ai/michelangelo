package parameter

import (
	"fmt"
	"time"

	pbtypes "github.com/gogo/protobuf/types"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/robfig/cron"
)

type BackfillParameterGenerator struct{}

// BackfillParam is a struct to store backfill execution metadata
// for backfill trigger, it is initialized with execution timestamp and parameter id when batch created
// and will be updated with pipeline run name and created timestamp when pipeline run is created
type BackfillParam struct {
	ExecutionTimestamp *time.Time
	ParamID            string
	PipelineRunName    string
	CreatedAt          *time.Time
}

// GenerateBatchParams generates batch parameters for backfill trigger.
//
// Backfill triggers execute pipeline runs for past time periods based on a cron/interval schedule.
// For example, if you have a daily pipeline that should have run from 2024-01-01 to 2024-01-31,
// backfill will generate timestamps for each day and execute the pipeline for each timestamp.
//
// This method creates a Cartesian product of:
//   - Execution timestamps (calculated from startTimestamp to endTimestamp based on cron/interval)
//   - Parameter sets (from trigger.ParametersMap)
//
// Example:
//
//	3 timestamps (t1, t2, t3) × 2 params (p1, p2)
//	= 6 total runs: [(t1,p1), (t1,p2), (t2,p1), (t2,p2), (t3,p1), (t3,p2)]
//
// These 6 runs are then split into batches based on BatchPolicy.BatchSize.
// If BatchSize=2, returns: [[(t1,p1), (t1,p2)], [(t2,p1), (t2,p2)], [(t3,p1), (t3,p2)]]
//
// The batches are executed sequentially with a wait period between them (BatchPolicy.WaitSeconds).
func (g *BackfillParameterGenerator) GenerateBatchParams(triggerRun *v2pb.TriggerRun) ([][]Params, error) {
	backfillRunTimestamps, err := calculateBackfillRunTimestamps(triggerRun)
	if err != nil {
		return nil, err
	}
	batchSize := _defaultBatchSize
	if triggerRun.Spec.Trigger.BatchPolicy != nil && triggerRun.Spec.Trigger.BatchPolicy.BatchSize != 0 {
		batchSize = int(triggerRun.Spec.Trigger.BatchPolicy.BatchSize)
	}
	paramsMap := triggerRun.Spec.Trigger.ParametersMap
	backfillParamsMapLen := len(paramsMap) * len(backfillRunTimestamps)
	numOfBatches := 1
	if backfillParamsMapLen > 0 {
		if backfillParamsMapLen%batchSize == 0 {
			numOfBatches = backfillParamsMapLen / batchSize
		} else {
			numOfBatches = backfillParamsMapLen/batchSize + 1
		}
	}

	batchedTriggeredRuns := make([][]Params, numOfBatches)
	// no parameters are defined for this trigger
	if backfillParamsMapLen == 0 {
		batchedTriggeredRuns[0] = []Params{{Backfill: BackfillParam{}}}
		return batchedTriggeredRuns, nil
	}
	keys := make([]Params, 0, backfillParamsMapLen)
	for executionTimestamp := range backfillRunTimestamps {
		for parameterID := range paramsMap {
			keys = append(keys, Params{
				Backfill: BackfillParam{
					ExecutionTimestamp: &backfillRunTimestamps[executionTimestamp],
					ParamID:            parameterID,
				},
			})
		}
	}
	sortParams(keys)
	for i := 0; i < len(keys); i = i + batchSize {
		if i+batchSize <= len(keys) {
			batchedTriggeredRuns[i/batchSize] = keys[i : i+batchSize]
		} else {
			batchedTriggeredRuns[i/batchSize] = keys[i:]
		}
	}
	return batchedTriggeredRuns, nil
}

// GenerateConcurrentParams generates concurrent parameters for backfill trigger.
//
// Similar to GenerateBatchParams, this creates a Cartesian product of timestamps × parameters,
// but returns them as a flat list to be executed with controlled concurrency using a worker pool pattern.
//
// Example with 6 runs and MaxConcurrency=2:
//
//	6 runs: [(t1,p1), (t1,p2), (t2,p1), (t2,p2), (t3,p1), (t3,p2)]
//
// Execution flow (with MaxConcurrency=2):
//   - Start: (t1,p1) and (t1,p2) running (queue: [(t2,p1), (t2,p2), (t3,p1), (t3,p2)])
//   - (t1,p1) finishes → immediately start (t2,p1)
//   - (t1,p2) finishes → immediately start (t2,p2)
//   - (t2,p1) finishes → immediately start (t3,p1)
//   - ... and so on
//
// This maintains exactly MaxConcurrency runs executing at any time,
// useful for rate limiting or resource management.
func (g *BackfillParameterGenerator) GenerateConcurrentParams(triggerRun *v2pb.TriggerRun) ([]Params, error) {
	backfillRunTimestamps, err := calculateBackfillRunTimestamps(triggerRun)
	if err != nil {
		return nil, err
	}
	params := make([]Params, len(triggerRun.Spec.Trigger.ParametersMap)*len(backfillRunTimestamps))
	i := 0
	for executionTimestamp := range backfillRunTimestamps {
		for paramID := range triggerRun.Spec.Trigger.ParametersMap {
			params[i] = Params{
				Backfill: BackfillParam{
					ExecutionTimestamp: &backfillRunTimestamps[executionTimestamp],
					ParamID:            paramID,
				},
			}
			i++
		}
	}
	sortParams(params)
	return params, nil
}

// calculateBackfillRunTimestamps generates execution timestamps for backfill based on the cron/interval schedule.
//
// Given a time range [startTimestamp, endTimestamp] and a schedule (cron or interval),
// this function calculates all the timestamps when the pipeline should have executed.
//
// Example with daily cron "0 8 * * *" (8 AM daily):
//
//	Start: 2024-01-01 00:00:00
//	End:   2024-01-03 23:59:59
//	Returns: [2024-01-01 08:00:00, 2024-01-02 08:00:00, 2024-01-03 08:00:00]
//
// Example with interval schedule (every 6 hours):
//
//	Start: 2024-01-01 00:00:00
//	End:   2024-01-01 18:00:00
//	Returns: [2024-01-01 00:00:00, 2024-01-01 06:00:00, 2024-01-01 12:00:00, 2024-01-01 18:00:00]
func calculateBackfillRunTimestamps(triggerRun *v2pb.TriggerRun) ([]time.Time, error) {
	startTimestamp, err := pbtypes.TimestampFromProto(triggerRun.Spec.StartTimestamp)
	if err != nil {
		return nil, err
	}
	endTimestamp, err := pbtypes.TimestampFromProto(triggerRun.Spec.EndTimestamp)
	if err != nil {
		return nil, err
	}
	var (
		cronExp               string
		backfillRunTimestamps []time.Time
	)
	if triggerRun.Spec.Trigger.GetCronSchedule() != nil {
		cronExp = triggerRun.Spec.Trigger.GetCronSchedule().Cron
	} else if triggerRun.Spec.Trigger.GetIntervalSchedule() != nil {
		interval := triggerRun.Spec.Trigger.GetIntervalSchedule().Interval.GetSeconds()
		cronExp = fmt.Sprintf("@every %ds", interval)
	} else {
		return nil, fmt.Errorf("cron and interval schedule cannot be empty")
	}
	cronSchedule, err := cron.ParseStandard(cronExp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cron schedule in original trigger run: %v", err)
	}
	nextRunTimestamp := cronSchedule.Next(startTimestamp)
	previousOneRunTimestamp := nextRunTimestamp.Add(-cronSchedule.Next(nextRunTimestamp).Sub(nextRunTimestamp))
	if previousOneRunTimestamp.Equal(startTimestamp) {
		backfillRunTimestamps = append(backfillRunTimestamps, startTimestamp)
	}
	for nextRunTimestamp.Compare(endTimestamp) <= 0 {
		backfillRunTimestamps = append(backfillRunTimestamps, nextRunTimestamp)
		nextRunTimestamp = cronSchedule.Next(nextRunTimestamp)
	}
	return backfillRunTimestamps, nil
}
