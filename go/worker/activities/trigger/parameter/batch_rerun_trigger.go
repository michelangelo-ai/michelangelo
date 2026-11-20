package parameter

import (
	"fmt"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// BatchRerunParameterGenerator generates parameters for batch rerun triggers.
//
// A batch rerun executes one resumed PipelineRun per entry in
// TriggerRun.Spec.Trigger.BatchRerun.PipelineRuns. Unlike cron/backfill, there is no
// parameter map or timestamp range to expand: the list of pipeline_runs to resume from
// is already the full unit of work, and it is already ordered (a proto repeated field),
// so no additional sorting is needed here.
type BatchRerunParameterGenerator struct{}

// GenerateBatchParams splits the pipeline_runs to rerun into batches, honoring
// BatchPolicy.BatchSize the same way CronParameterGenerator does.
func (g *BatchRerunParameterGenerator) GenerateBatchParams(triggerRun *v2pb.TriggerRun) ([][]Params, error) {
	params, err := toBatchRerunParams(triggerRun)
	if err != nil {
		return nil, err
	}
	batchSize := _defaultBatchSize
	if triggerRun.Spec.Trigger.BatchPolicy != nil && triggerRun.Spec.Trigger.BatchPolicy.BatchSize != 0 {
		batchSize = int(triggerRun.Spec.Trigger.BatchPolicy.BatchSize)
	}
	numOfBatches := len(params) / batchSize
	if len(params)%batchSize != 0 {
		numOfBatches++
	}
	batches := make([][]Params, numOfBatches)
	for i := 0; i < len(params); i += batchSize {
		end := i + batchSize
		if end > len(params) {
			end = len(params)
		}
		batches[i/batchSize] = params[i:end]
	}
	return batches, nil
}

// GenerateConcurrentParams returns one Params per pipeline_run to rerun, for execution
// with controlled concurrency (TriggerRun.Spec.Trigger.MaxConcurrency).
func (g *BatchRerunParameterGenerator) GenerateConcurrentParams(triggerRun *v2pb.TriggerRun) ([]Params, error) {
	return toBatchRerunParams(triggerRun)
}

func toBatchRerunParams(triggerRun *v2pb.TriggerRun) ([]Params, error) {
	refs := triggerRun.Spec.Trigger.GetBatchRerun().GetPipelineRuns()
	if len(refs) == 0 {
		return nil, fmt.Errorf("batch rerun trigger %s/%s has no pipeline_runs to rerun",
			triggerRun.Namespace, triggerRun.Name)
	}
	params := make([]Params, len(refs))
	for i, ref := range refs {
		params[i] = Params{BatchRerun: BatchRerunParam{PipelineRun: ref}}
	}
	return params, nil
}
