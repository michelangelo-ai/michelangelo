package utils

import (
	"testing"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestScaleKnownResource(t *testing.T) {
	input := make(corev1.ResourceList)
	input[corev1.ResourceCPU] = *resource.NewQuantity(
		6, resource.DecimalSI)
	input[corev1.ResourceMemory] = *resource.NewScaledQuantity(
		4, 9)
	input[constants.ResourceNvidiaGPU] = *resource.NewQuantity(
		2, resource.DecimalSI)
	input[corev1.ResourceEphemeralStorage] = *resource.NewQuantity(
		1, resource.DecimalSI)

	scaleFactor := int64(5)

	expected := make(corev1.ResourceList)
	expected[corev1.ResourceCPU] = *resource.NewQuantity(
		30, resource.DecimalSI)
	expected[corev1.ResourceMemory] = *resource.NewQuantity(
		20000000000, resource.DecimalSI)
	expected[constants.ResourceNvidiaGPU] = *resource.NewQuantity(
		10, resource.DecimalSI)
	expected[corev1.ResourceEphemeralStorage] = *resource.NewQuantity(
		5, resource.DecimalSI)

	output, err := ScaleKnownResources(input, scaleFactor)
	require.NoError(t, err)

	require.EqualValues(t, expected, output)
}
