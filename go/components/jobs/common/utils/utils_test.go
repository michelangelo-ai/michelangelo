package utils

import (
	"testing"

	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
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

// rayContainers is the filter the ray cluster watcher uses. Ray container
// names come from the submitted pod spec, so the filter can only exclude the
// one container michelangelo names itself: the log-collector sidecar.
func rayContainers(containerStatus corev1.ContainerStatus) bool {
	return containerStatus.Name != "collector"
}

func TestGetContainerErrorFromPodStatus(t *testing.T) {
	tests := []struct {
		name            string
		pod             *corev1.Pod
		expectedReason  string
		expectedMessage string
		expectedExit    int32
		expectNil       bool
	}{
		{
			name: "terminated container is reported",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-head-abc12"},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "head",
							State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
								ExitCode: 137, Reason: "OOMKilled", Message: "out of memory",
							}},
						},
					},
				},
			},
			expectedReason:  "OOMKilled",
			expectedMessage: "out of memory",
			expectedExit:    137,
		},
		{
			// The case the pod watcher exists for: the container never starts,
			// so it never terminates and the pod is never deleted. Without the
			// waiting pass the failure would never reach the CR.
			name: "waiting on an unpullable image is reported",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-head-abc12"},
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "head",
							State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
								Reason: "ImagePullBackOff", Message: `Back-off pulling image "nope:latest"`,
							}},
						},
					},
				},
			},
			expectedReason:  "ImagePullBackOff",
			expectedMessage: `Back-off pulling image "nope:latest"`,
		},
		{
			name: "waiting on a crash loop is reported",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-worker-abc12"},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "worker",
							State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
								Reason: "CrashLoopBackOff", Message: "back-off restarting failed container",
							}},
						},
					},
				},
			},
			expectedReason:  "CrashLoopBackOff",
			expectedMessage: "back-off restarting failed container",
		},
		{
			// Every healthy pod passes through these on its way up.
			name: "ordinary startup is not an error",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-head-abc12"},
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					ContainerStatuses: []corev1.ContainerStatus{
						{Name: "head", State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{Reason: "ContainerCreating"},
						}},
					},
				},
			},
			expectNil: true,
		},
		{
			name: "pod initializing is not an error",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-worker-abc12"},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{Name: "worker", State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{Reason: "PodInitializing"},
						}},
					},
				},
			},
			expectNil: true,
		},
		{
			name: "a wedged init container is reported",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "worker-pod"},
				Status: corev1.PodStatus{
					InitContainerStatuses: []corev1.ContainerStatus{
						{Name: "wait-gcs-ready", State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff", Message: "back-off"},
						}},
					},
					ContainerStatuses: []corev1.ContainerStatus{
						{Name: "worker", State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{Reason: "PodInitializing"},
						}},
					},
				},
			},
			expectedReason:  "ImagePullBackOff",
			expectedMessage: "back-off",
		},
		{
			name: "a completed init container is not an error",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "worker-pod"},
				Status: corev1.PodStatus{
					InitContainerStatuses: []corev1.ContainerStatus{
						{Name: "wait-gcs-ready", State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{ExitCode: 0, Reason: "Completed"},
						}},
					},
					ContainerStatuses: []corev1.ContainerStatus{
						{Name: "worker", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
					},
				},
			},
			expectNil: true,
		},
		{
			name: "a failing sidecar is not the cluster's failure",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-head-abc12"},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{Name: "collector", State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"},
						}},
						{Name: "head", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
					},
				},
			},
			expectNil: true,
		},
		{
			name: "a clean exit is not an error",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-head-abc12"},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{Name: "head", State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{ExitCode: 0, Reason: "Completed"},
						}},
					},
				},
			},
			expectNil: true,
		},
		{
			// Ordering is deliberate: what a container actually died of beats
			// what a sibling is merely stuck waiting on.
			name: "a termination outranks a waiting sibling",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-head-abc12"},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{Name: "worker", State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"},
						}},
						{Name: "head", State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{ExitCode: 1, Reason: "Error", Message: "boom"},
						}},
					},
				},
			},
			expectedReason:  "Error",
			expectedMessage: "boom",
			expectedExit:    1,
		},
		{
			// The crux of the split from GetErrorFromPodStatus. A pod that is
			// still starting reports ContainersReady=False with reason
			// ContainersNotReady; treating that as an error on the live path
			// would stamp a bogus entry on every healthy cluster startup.
			name: "conditions are not consulted while the pod is alive",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-head-abc12"},
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					Conditions: []corev1.PodCondition{
						{
							Type:    corev1.ContainersReady,
							Status:  corev1.ConditionFalse,
							Reason:  "ContainersNotReady",
							Message: "containers with unready status: [head]",
						},
					},
					ContainerStatuses: []corev1.ContainerStatus{
						{Name: "head", State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{Reason: "ContainerCreating"},
						}},
					},
				},
			},
			expectNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			podError := GetContainerErrorFromPodStatus(tc.pod, rayContainers)
			if tc.expectNil {
				assert.Nil(t, podError)
				return
			}
			require.NotNil(t, podError)
			assert.Equal(t, tc.pod.Name, podError.GetName())
			assert.Equal(t, tc.expectedReason, podError.GetReason())
			assert.Equal(t, tc.expectedMessage, podError.GetMessage())
			assert.Equal(t, tc.expectedExit, podError.GetExitCode())
		})
	}
}

// TestGetErrorFromPodStatus_FallsBackToConditions pins the difference between
// the two extractors. Once the pod is gone ContainersReady=False is a final
// verdict rather than progress, so the delete path keeps the fallback the live
// path deliberately drops.
func TestGetErrorFromPodStatus_FallsBackToConditions(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-head-abc12"},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.ContainersReady,
					Status:  corev1.ConditionFalse,
					Reason:  "ContainersNotReady",
					Message: "containers with unready status: [head]",
				},
			},
		},
	}

	assert.Nil(t, GetContainerErrorFromPodStatus(pod, rayContainers))

	podError := GetErrorFromPodStatus(pod, rayContainers)
	require.NotNil(t, podError)
	assert.Equal(t, "ContainersNotReady", podError.GetReason())
	assert.Equal(t, "cluster-head-abc12", podError.GetName())
}

func TestGetSchedulingErrorFromPodStatus(t *testing.T) {
	tests := []struct {
		name            string
		conditions      []corev1.PodCondition
		expectNil       bool
		expectedReason  string
		expectedMessage string
	}{
		{
			// The case this exists for. An unschedulable pod reports no
			// container statuses at all, so the container extraction is blind
			// to it and the cluster would sit in UNKNOWN with nothing to show.
			name: "unschedulable is reported with the scheduler's explanation",
			conditions: []corev1.PodCondition{{
				Type:    corev1.PodScheduled,
				Status:  corev1.ConditionFalse,
				Reason:  corev1.PodReasonUnschedulable,
				Message: "0/1 nodes are available: 1 Insufficient cpu.",
			}},
			expectedReason:  "Unschedulable",
			expectedMessage: "0/1 nodes are available: 1 Insufficient cpu.",
		},
		{
			name: "scheduler errors are reported",
			conditions: []corev1.PodCondition{{
				Type:   corev1.PodScheduled,
				Status: corev1.ConditionFalse,
				Reason: corev1.PodReasonSchedulerError,
			}},
			expectedReason: "SchedulerError",
		},
		{
			// A gated pod is waiting on Kueue admission. That is the queue
			// working, not a failure, and reporting it would make every queued
			// cluster look broken.
			name: "scheduling gated is not an error",
			conditions: []corev1.PodCondition{{
				Type:   corev1.PodScheduled,
				Status: corev1.ConditionFalse,
				Reason: corev1.PodReasonSchedulingGated,
			}},
			expectNil: true,
		},
		{
			name: "a scheduled pod reports nothing",
			conditions: []corev1.PodCondition{
				{Type: corev1.PodScheduled, Status: corev1.ConditionTrue},
			},
			expectNil: true,
		},
		{
			// The counterweight to extractErrorFromPodConditions: a pod on its
			// way up says ContainersReady=False, and the live path must not
			// read that as a failure.
			name: "a starting pod reports nothing",
			conditions: []corev1.PodCondition{
				{Type: corev1.PodScheduled, Status: corev1.ConditionTrue},
				{Type: corev1.ContainersReady, Status: corev1.ConditionFalse, Reason: "ContainersNotReady"},
			},
			expectNil: true,
		},
		{
			name:      "no conditions reports nothing",
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-head-abc12"},
				Status:     corev1.PodStatus{Phase: corev1.PodPending, Conditions: tt.conditions},
			}

			podError := GetSchedulingErrorFromPodStatus(pod)

			if tt.expectNil {
				assert.Nil(t, podError)
				return
			}
			require.NotNil(t, podError)
			assert.Equal(t, "cluster-head-abc12", podError.GetName())
			assert.Equal(t, tt.expectedReason, podError.GetReason())
			assert.Equal(t, tt.expectedMessage, podError.GetMessage())
		})
	}
}

func TestIsPostMortemPodError(t *testing.T) {
	assert.True(t, IsPostMortemPodError(&v2pb.PodErrors{Reason: "ContainerStatusUnknown"}),
		"the kubelet's post-teardown reason carries no diagnosis")
	assert.False(t, IsPostMortemPodError(&v2pb.PodErrors{Reason: "ImagePullBackOff"}))
	assert.False(t, IsPostMortemPodError(&v2pb.PodErrors{}))
}
