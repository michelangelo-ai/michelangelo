package parameter

import (
	"testing"
	"time"

	api "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func batchRerunTriggerRun(refs []*api.ResourceIdentifier, batchSize int32) *v2pb.TriggerRun {
	tr := &v2pb.TriggerRun{
		Spec: v2pb.TriggerRunSpec{
			Trigger: &v2pb.Trigger{
				TriggerType: &v2pb.Trigger_BatchRerun{
					BatchRerun: &v2pb.BatchRerun{PipelineRuns: refs},
				},
			},
		},
	}
	if batchSize != 0 {
		tr.Spec.Trigger.BatchPolicy = &v2pb.BatchPolicy{BatchSize: batchSize}
	}
	return tr
}

func TestBatchRerunParameterGenerator_GenerateBatchParams(t *testing.T) {
	generator := &BatchRerunParameterGenerator{}
	refs := []*api.ResourceIdentifier{
		{Name: "run-1"}, {Name: "run-2"}, {Name: "run-3"},
	}

	tests := []struct {
		name           string
		triggerRun     *v2pb.TriggerRun
		expectedError  bool
		expectedParams [][]Params
	}{
		{
			name:          "success - single batch (default batch size)",
			triggerRun:    batchRerunTriggerRun(refs, 0),
			expectedError: false,
			expectedParams: [][]Params{
				{
					{BatchRerun: BatchRerunParam{PipelineRun: refs[0]}},
					{BatchRerun: BatchRerunParam{PipelineRun: refs[1]}},
					{BatchRerun: BatchRerunParam{PipelineRun: refs[2]}},
				},
			},
		},
		{
			name:          "success - multiple batches with batch policy",
			triggerRun:    batchRerunTriggerRun(refs, 2),
			expectedError: false,
			expectedParams: [][]Params{
				{
					{BatchRerun: BatchRerunParam{PipelineRun: refs[0]}},
					{BatchRerun: BatchRerunParam{PipelineRun: refs[1]}},
				},
				{
					{BatchRerun: BatchRerunParam{PipelineRun: refs[2]}},
				},
			},
		},
		{
			name:          "error - no pipeline_runs",
			triggerRun:    batchRerunTriggerRun(nil, 0),
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := generator.GenerateBatchParams(tt.triggerRun)
			if tt.expectedError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expectedParams, result)
		})
	}
}

func TestBatchRerunParameterGenerator_GenerateConcurrentParams(t *testing.T) {
	generator := &BatchRerunParameterGenerator{}
	refs := []*api.ResourceIdentifier{{Name: "run-1"}, {Name: "run-2"}}

	result, err := generator.GenerateConcurrentParams(batchRerunTriggerRun(refs, 0))
	require.NoError(t, err)
	assert.Equal(t, []Params{
		{BatchRerun: BatchRerunParam{PipelineRun: refs[0]}},
		{BatchRerun: BatchRerunParam{PipelineRun: refs[1]}},
	}, result)

	_, err = generator.GenerateConcurrentParams(batchRerunTriggerRun(nil, 0))
	require.Error(t, err)
}

func TestGetParameterGenerator_BatchRerun(t *testing.T) {
	generator := GetParameterGenerator("batch_rerun")
	_, ok := generator.(*BatchRerunParameterGenerator)
	assert.True(t, ok)
}

func TestParams_GetTriggeredRun_BatchRerun(t *testing.T) {
	ref := &api.ResourceIdentifier{Name: "run-1"}
	p := Params{BatchRerun: BatchRerunParam{PipelineRun: ref}}
	now := time.Now()

	info := p.GetTriggeredRun("pipeline-run-new", now, now)

	assert.Equal(t, "run-1", info.ParamID)
	assert.Equal(t, "batch_rerun", info.TriggerType)
}
