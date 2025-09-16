package parameter

import (
	"testing"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCronParameterGenerator_GenerateBatchParams(t *testing.T) {
	generator := &CronParameterGenerator{}

	t.Run("success - empty parameters map", func(t *testing.T) {
		triggerRun := &v2pb.TriggerRun{
			Spec: v2pb.TriggerRunSpec{
				Trigger: &v2pb.Trigger{
					ParametersMap: map[string]*v2pb.PipelineExecutionParameters{},
				},
			},
		}

		result, err := generator.GenerateBatchParams(triggerRun)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Len(t, result, 1) // Should return one empty batch
		assert.Len(t, result[0], 1) // Should have one parameter with empty ID
		assert.Equal(t, "", result[0][0].ParamID)
	})
}

func TestCronParameterGenerator_GenerateConcurrentParams(t *testing.T) {
	generator := &CronParameterGenerator{}

	t.Run("success - empty parameters map", func(t *testing.T) {
		triggerRun := &v2pb.TriggerRun{
			Spec: v2pb.TriggerRunSpec{
				Trigger: &v2pb.Trigger{
					ParametersMap: map[string]*v2pb.PipelineExecutionParameters{},
				},
			},
		}

		result, err := generator.GenerateConcurrentParams(triggerRun)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Len(t, result, 0) // Should return no parameters
	})
}

func TestParams_Structure(t *testing.T) {
	param := Params{
		ParamID: "test-param",
	}

	assert.Equal(t, "test-param", param.ParamID)
}