package k8sengine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// gpuToleration is the toleration the mapper is expected to add for the
// nvidia.com/gpu=present:NoSchedule taint.
var gpuToleration = corev1.Toleration{
	Key:      "nvidia.com/gpu",
	Operator: corev1.TolerationOpExists,
	Effect:   corev1.TaintEffectNoSchedule,
}

func gpuPodTemplate(quantity string) *corev1.PodTemplateSpec {
	return &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "ray-head",
					Image: "rayproject/ray:2.10.0",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:                    resource.MustParse("4"),
							corev1.ResourceName("nvidia.com/gpu"): resource.MustParse(quantity),
						},
					},
				},
			},
		},
	}
}

func TestTolerateNVIDIAGPUTaint(t *testing.T) {
	tests := []struct {
		name        string
		podTemplate corev1.PodTemplateSpec
		expected    []corev1.Toleration
	}{
		{
			name:        "gpu request gets the toleration",
			podTemplate: *gpuPodTemplate("1"),
			expected:    []corev1.Toleration{gpuToleration},
		},
		{
			name: "gpu limit without an explicit request gets the toleration",
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "ray-worker",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("2"),
							},
						},
					}},
				},
			},
			expected: []corev1.Toleration{gpuToleration},
		},
		{
			name: "gpu requested by an init container gets the toleration",
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{
						Name: "warmup",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("1"),
							},
						},
					}},
					Containers: []corev1.Container{{Name: "ray-worker"}},
				},
			},
			expected: []corev1.Toleration{gpuToleration},
		},
		{
			name: "gpu request is appended after existing tolerations",
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Tolerations: []corev1.Toleration{{
						Key:      "dedicated",
						Operator: corev1.TolerationOpEqual,
						Value:    "ml",
						Effect:   corev1.TaintEffectNoSchedule,
					}},
					Containers: gpuPodTemplate("1").Spec.Containers,
				},
			},
			expected: []corev1.Toleration{
				{
					Key:      "dedicated",
					Operator: corev1.TolerationOpEqual,
					Value:    "ml",
					Effect:   corev1.TaintEffectNoSchedule,
				},
				gpuToleration,
			},
		},
		{
			name: "no gpu request leaves the pod untolerated",
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "ray-head",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("4")},
						},
					}},
				},
			},
			expected: nil,
		},
		{
			name:        "zero gpu request does not count as a gpu pod",
			podTemplate: *gpuPodTemplate("0"),
			expected:    nil,
		},
		{
			name: "caller's own gpu toleration is not duplicated",
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Tolerations: []corev1.Toleration{{
						Key:      "nvidia.com/gpu",
						Operator: corev1.TolerationOpEqual,
						Value:    "present",
						Effect:   corev1.TaintEffectNoSchedule,
					}},
					Containers: gpuPodTemplate("1").Spec.Containers,
				},
			},
			expected: []corev1.Toleration{{
				Key:      "nvidia.com/gpu",
				Operator: corev1.TolerationOpEqual,
				Value:    "present",
				Effect:   corev1.TaintEffectNoSchedule,
			}},
		},
		{
			name: "wildcard toleration already covers the taint",
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Tolerations: []corev1.Toleration{{Operator: corev1.TolerationOpExists}},
					Containers:  gpuPodTemplate("1").Spec.Containers,
				},
			},
			expected: []corev1.Toleration{{Operator: corev1.TolerationOpExists}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			podTemplate := tt.podTemplate
			tolerateNVIDIAGPUTaint(&podTemplate)
			assert.Equal(t, tt.expected, podTemplate.Spec.Tolerations)
		})
	}
}

// The mapper receives pod templates by value but their slices still alias the
// caller's v2pb object, which the RayCluster controller keeps using after the
// mapping. Adding a toleration must not write through that alias.
func TestTolerateNVIDIAGPUTaint_DoesNotMutateSource(t *testing.T) {
	source := gpuPodTemplate("1")
	source.Spec.Tolerations = []corev1.Toleration{
		{Key: "dedicated", Operator: corev1.TolerationOpExists},
	}

	mapped := *source
	tolerateNVIDIAGPUTaint(&mapped)

	require.Len(t, mapped.Spec.Tolerations, 2)
	assert.Equal(t, []corev1.Toleration{
		{Key: "dedicated", Operator: corev1.TolerationOpExists},
	}, source.Spec.Tolerations)
}

func TestGetHeadGroupSpec_NVIDIAGPUToleration(t *testing.T) {
	t.Run("gpu head tolerates the taint", func(t *testing.T) {
		got := getHeadGroupSpec(&v2pb.RayHeadSpec{
			ServiceType: string(corev1.ServiceTypeClusterIP),
			Pod:         gpuPodTemplate("1"),
		})
		assert.Equal(t, []corev1.Toleration{gpuToleration}, got.Template.Spec.Tolerations)
	})

	t.Run("cpu-only head gets no toleration", func(t *testing.T) {
		got := getHeadGroupSpec(&v2pb.RayHeadSpec{
			ServiceType: string(corev1.ServiceTypeClusterIP),
			Pod: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "ray-head"}}},
			},
		})
		assert.Empty(t, got.Template.Spec.Tolerations)
	})
}

// A cluster can mix a CPU head with GPU workers, so each group is evaluated on
// its own pod template.
func TestGetWorkerGroupSpecs_NVIDIAGPUToleration(t *testing.T) {
	specs := getWorkerGroupSpecs("test-cluster", []*v2pb.RayWorkerSpec{
		{MinInstances: 1, MaxInstances: 2, Pod: gpuPodTemplate("1")},
		{MinInstances: 1, MaxInstances: 2, Pod: &corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "ray-worker"}}},
		}},
	})

	require.Len(t, specs, 2)
	assert.Equal(t, []corev1.Toleration{gpuToleration}, specs[0].Template.Spec.Tolerations)
	assert.Empty(t, specs[1].Template.Spec.Tolerations)
}

// The submitter pod is built without the head's GPU or scheduling constraints,
// so it must not pick up the GPU toleration either.
func TestBuildSubmitterPodTemplate_NoNVIDIAGPUToleration(t *testing.T) {
	submitter := buildSubmitterPodTemplate(*gpuPodTemplate("1"))
	assert.Empty(t, submitter.Spec.Tolerations)
}
