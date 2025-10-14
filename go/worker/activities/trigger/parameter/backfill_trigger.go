package parameter

import (
	"fmt"
	"sort"
	"time"

	pbtypes "github.com/gogo/protobuf/types"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/robfig/cron"
)

type BackfillParameterHandler struct{}

// BackfillParam is a struct to store backfill execution metadata
// for backfill trigger, it is initialized with execution timestamp and parameter id when batch created
// and will be updated with pipeline run name and created timestamp when pipeline run is created
type BackfillParam struct {
	ExecutionTimestamp *time.Time
	ParamID            string
	PipelineRunName    string
	CreatedAt          *time.Time
}

// GenerateBatchParams generates batch parameters for backfill trigger
func (h *BackfillParameterHandler) GenerateBatchParams(triggerRun *v2pb.TriggerRun) ([][]Params, error) {
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
	h.SortParams(keys)
	for i := 0; i < len(keys); i = i + batchSize {
		if i+batchSize <= len(keys) {
			batchedTriggeredRuns[i/batchSize] = keys[i : i+batchSize]
		} else {
			batchedTriggeredRuns[i/batchSize] = keys[i:]
		}
	}
	return batchedTriggeredRuns, nil
}

// GenerateConcurrentParams generates concurrent parameters for backfill trigger
func (h *BackfillParameterHandler) GenerateConcurrentParams(triggerRun *v2pb.TriggerRun) ([]Params, error) {
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
	h.SortParams(params)
	return params, nil
}

// SortParams sorts the parameters alphabetically and chronologically
func (h *BackfillParameterHandler) SortParams(params []Params) {
	sort.Slice(params, func(i, j int) bool {
		if (*params[i].Backfill.ExecutionTimestamp).Equal(*params[j].Backfill.ExecutionTimestamp) {
			return params[i].Backfill.ParamID < params[j].Backfill.ParamID
		}
		return (*params[i].Backfill.ExecutionTimestamp).Before(*params[j].Backfill.ExecutionTimestamp)
	})
}

// GetParameterID returns the parameter ID for backfill trigger
func (h *BackfillParameterHandler) GetParameterID(param Params) string {
	return param.Backfill.ParamID
}

// GetExecutionTimestamp returns the execution timestamp for backfill trigger
func (h *BackfillParameterHandler) GetExecutionTimestamp(param Params, logicalTs time.Time) time.Time {
	return *param.Backfill.ExecutionTimestamp
}

// UpdateTriggerContext updates the trigger context for backfill trigger
func (h *BackfillParameterHandler) UpdateTriggerContext(triggerContext Object, param Params, pipelineRunName string, createdTimestamp time.Time) {
	backfillParam := BackfillParam{
		ParamID:            param.Backfill.ParamID,
		PipelineRunName:    pipelineRunName,
		ExecutionTimestamp: param.Backfill.ExecutionTimestamp,
		CreatedAt:          &createdTimestamp,
	}
	triggerContext["TriggeredRuns"] = append(triggerContext["TriggeredRuns"].([]BackfillParam), backfillParam)
}

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
