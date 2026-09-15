package apihook

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestBeforeCreateStampsOwnSourcePipelineTypeLabel(t *testing.T) {
	hook := apiHook{logger: zaptest.NewLogger(t)}

	pipeline := &v2.Pipeline{Spec: v2.PipelineSpec{Type: v2.PIPELINE_TYPE_TRAIN}}
	require.NoError(t, hook.BeforeCreate(context.Background(), &v2.CreatePipelineRequest{Pipeline: pipeline}))

	require.Equal(t, "PIPELINE_TYPE_TRAIN", pipeline.GetLabels()["michelangelo/SourcePipelineType"])
}

func TestBeforeCreateNoopWhenTypeUnset(t *testing.T) {
	hook := apiHook{logger: zaptest.NewLogger(t)}

	pipeline := &v2.Pipeline{}
	require.NoError(t, hook.BeforeCreate(context.Background(), &v2.CreatePipelineRequest{Pipeline: pipeline}))

	require.Empty(t, pipeline.GetLabels())
}

func TestBeforeUpdateStampsOwnSourcePipelineTypeLabel(t *testing.T) {
	hook := apiHook{logger: zaptest.NewLogger(t)}

	pipeline := &v2.Pipeline{Spec: v2.PipelineSpec{Type: v2.PIPELINE_TYPE_EVAL}}
	require.NoError(t, hook.BeforeUpdate(context.Background(), &v2.UpdatePipelineRequest{Pipeline: pipeline}))

	require.Equal(t, "PIPELINE_TYPE_EVAL", pipeline.GetLabels()["michelangelo/SourcePipelineType"])
}
