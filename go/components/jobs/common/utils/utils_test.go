package utils

import (
	"testing"

	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
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
			// KubeRay reports this transiently after a RayCluster is created but
			// before its head pod is scheduled; it must not be treated as
			// terminal or the controller races KubeRay and tears the cluster
			// down before the head pod ever exists.
			name: "HeadPodNotFound is not terminal (transient provisioning window)",
			errors: []*v2pb.PodErrors{
				{Name: "HeadPodReady", Reason: "HeadPodNotFound", Message: "Head Pod not found"},
			},
			expected: false,
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

func TestProjectNameForJob(t *testing.T) {
	tests := []struct {
		name      string
		labels    map[string]string
		namespace string
		expected  string
	}{
		{
			name:      "label wins over namespace",
			labels:    map[string]string{"ma/project-name": "proj1"},
			namespace: "other-ns",
			expected:  "proj1",
		},
		{
			name:      "no label falls back to namespace",
			labels:    map[string]string{"unrelated": "x"},
			namespace: "proj-ns",
			expected:  "proj-ns",
		},
		{
			name:      "empty label value falls back to namespace",
			labels:    map[string]string{"ma/project-name": ""},
			namespace: "proj-ns",
			expected:  "proj-ns",
		},
		{
			name:      "nil labels fall back to namespace",
			labels:    nil,
			namespace: "proj-ns",
			expected:  "proj-ns",
		},
		{
			name:      "no identity at all",
			labels:    nil,
			namespace: "",
			expected:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ProjectNameForJob(tt.labels, tt.namespace))
		})
	}
}

func TestResolveLocalQueueName(t *testing.T) {
	tests := []struct {
		name    string
		cfg     maconfig.KueueConfig
		project string
		want    string
	}{
		{
			name:    "default template",
			cfg:     maconfig.KueueConfig{},
			project: "proj1",
			want:    "ma-proj1",
		},
		{
			name:    "custom template",
			cfg:     maconfig.KueueConfig{LocalQueueTemplate: "queue-{project}-batch"},
			project: "proj1",
			want:    "queue-proj1-batch",
		},
		{
			name: "override wins over template",
			cfg: maconfig.KueueConfig{
				LocalQueueTemplate:  "ma-{project}",
				LocalQueueOverrides: map[string]string{"proj1": "custom-queue"},
			},
			project: "proj1",
			want:    "custom-queue",
		},
		{
			name: "empty override value falls back to template",
			cfg: maconfig.KueueConfig{
				LocalQueueOverrides: map[string]string{"proj1": ""},
			},
			project: "proj1",
			want:    "ma-proj1",
		},
		{
			name: "override for another project does not apply",
			cfg: maconfig.KueueConfig{
				LocalQueueOverrides: map[string]string{"other": "custom-queue"},
			},
			project: "proj1",
			want:    "ma-proj1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ResolveLocalQueueName(tt.cfg, tt.project))
		})
	}
}
