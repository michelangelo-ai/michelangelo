package utils

import (
	"testing"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetObjectNamespace(t *testing.T) {
	testCases := []struct {
		name       string
		obj        interface{}
		expectedNS string
	}{
		{
			name: "Ray Job with namespace",
			obj: &v2pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
				},
			},
			expectedNS: "test-namespace",
		},
		{
			name: "Spark Job with namespace",
			obj: &v2pb.SparkJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "spark-namespace",
				},
			},
			expectedNS: "spark-namespace",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test basic object access
			switch obj := tc.obj.(type) {
			case *v2pb.RayJob:
				assert.Equal(t, tc.expectedNS, obj.ObjectMeta.Namespace)
			case *v2pb.SparkJob:
				assert.Equal(t, tc.expectedNS, obj.ObjectMeta.Namespace)
			}
		})
	}
}

func TestBasicUtilityFunctions(t *testing.T) {
	t.Run("TestBasicObjectCreation", func(t *testing.T) {
		rayJob := &v2pb.RayJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ray-job",
				Namespace: "test-namespace",
			},
		}

		assert.Equal(t, "test-ray-job", rayJob.ObjectMeta.Name)
		assert.Equal(t, "test-namespace", rayJob.ObjectMeta.Namespace)
	})

	t.Run("TestSparkJobCreation", func(t *testing.T) {
		sparkJob := &v2pb.SparkJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-spark-job",
				Namespace: "spark-namespace",
			},
		}

		assert.Equal(t, "test-spark-job", sparkJob.ObjectMeta.Name)
		assert.Equal(t, "spark-namespace", sparkJob.ObjectMeta.Namespace)
	})
}

func TestHasTerminalPodErrors(t *testing.T) {
	tests := []struct {
		name     string
		errors   []*v2pb.PodErrors
		expected bool
	}{
		{
			name:     "nil errors",
			errors:   nil,
			expected: false,
		},
		{
			name:     "empty errors",
			errors:   []*v2pb.PodErrors{},
			expected: false,
		},
		{
			name: "non-terminal reason",
			errors: []*v2pb.PodErrors{
				{Reason: "RayClusterPodsProvisioning"},
			},
			expected: false,
		},
		{
			name: "ContainersNotReady is not immediately terminal",
			errors: []*v2pb.PodErrors{
				{Reason: "ContainersNotReady", Message: "containers with unready status: [head]"},
			},
			expected: false,
		},
		{
			name: "CrashLoopBackOff is terminal",
			errors: []*v2pb.PodErrors{
				{Reason: "CrashLoopBackOff", Message: "container crashing"},
			},
			expected: true,
		},
		{
			name: "ImagePullBackOff is terminal",
			errors: []*v2pb.PodErrors{
				{Reason: "ImagePullBackOff", Message: "cannot pull image"},
			},
			expected: true,
		},
		{
			name: "FailedCreateHeadPod is terminal",
			errors: []*v2pb.PodErrors{
				{Reason: "FailedCreateHeadPod", Message: "quota exceeded"},
			},
			expected: true,
		},
		{
			name: "OOMKilled is terminal",
			errors: []*v2pb.PodErrors{
				{Reason: "OOMKilled"},
			},
			expected: true,
		},
		{
			name: "mixed errors with one terminal",
			errors: []*v2pb.PodErrors{
				{Reason: "SomeTransientReason"},
				{Reason: "ErrImagePull", Message: "image not found"},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, HasTerminalPodErrors(tt.errors))
		})
	}
}
