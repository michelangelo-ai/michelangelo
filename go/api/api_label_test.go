package api

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestStampSourcePipelineTypeLabelOnCreateHappyPath(t *testing.T) {
	child := &unstructured.Unstructured{}
	child.SetName("child")

	StampSourcePipelineTypeLabelOnCreate(child, "PIPELINE_TYPE_TRAIN")

	require.Equal(t, "PIPELINE_TYPE_TRAIN", child.GetLabels()[SourcePipelineTypeLabelName])
}

func TestStampSourcePipelineTypeLabelOnCreatePreservesExistingLabels(t *testing.T) {
	child := &unstructured.Unstructured{}
	child.SetName("child")
	child.SetLabels(map[string]string{"other-label": "keep-me"})

	StampSourcePipelineTypeLabelOnCreate(child, "PIPELINE_TYPE_TRAIN")

	labels := child.GetLabels()
	require.Equal(t, "PIPELINE_TYPE_TRAIN", labels[SourcePipelineTypeLabelName])
	require.Equal(t, "keep-me", labels["other-label"])
}

func TestStampSourcePipelineTypeLabelOnCreateEmptyTypeIsNoop(t *testing.T) {
	child := &unstructured.Unstructured{}
	child.SetName("child")

	StampSourcePipelineTypeLabelOnCreate(child, "")

	require.Nil(t, child.GetLabels())
}
