package k8sengine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// envIndex returns name -> position in the container's env list. Position
// matters: Kubernetes expands $(VAR) only against vars declared earlier.
func envIndex(envs []corev1.EnvVar) map[string]int {
	idx := make(map[string]int, len(envs))
	for i, e := range envs {
		idx[e.Name] = i
	}
	return idx
}

// splitEnv separates literal-valued env vars from secret-backed and
// downward-API ones, keyed by env var name.
func splitEnv(envs []corev1.EnvVar) (values map[string]string, secretRefs map[string]corev1.SecretKeySelector, fieldRefs map[string]string) {
	values = make(map[string]string)
	secretRefs = make(map[string]corev1.SecretKeySelector)
	fieldRefs = make(map[string]string)
	for _, e := range envs {
		switch {
		case e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil:
			secretRefs[e.Name] = *e.ValueFrom.SecretKeyRef
		case e.ValueFrom != nil && e.ValueFrom.FieldRef != nil:
			fieldRefs[e.Name] = e.ValueFrom.FieldRef.FieldPath
		default:
			values[e.Name] = e.Value
		}
	}
	return values, secretRefs, fieldRefs
}

func TestInjectCollectorSidecar(t *testing.T) {
	config := LogPersistenceConfig{
		Enabled:           true,
		StorageEndpoint:   "minio:9091",
		Bucket:            "ray-history",
		PathPrefix:        "log",
		Region:            "us-east-1",
		CredentialsSecret: "minio-credentials",
		CollectorImage:    "quay.io/kuberay/collector:v1.7.1",
		S3DisableSSL:      true,
	}

	tests := []struct {
		name        string
		role        string
		podTemplate corev1.PodTemplateSpec
	}{
		{
			name: "head pod gets collector sidecar",
			role: "Head",
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "ray-head", Image: "rayproject/ray:2.56.0"},
					},
				},
			},
		},
		{
			name: "worker pod gets collector sidecar",
			role: "Worker",
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "ray-worker", Image: "rayproject/ray:2.56.0"},
					},
				},
			},
		},
		{
			name: "pod with multiple containers",
			role: "Head",
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "ray-head", Image: "rayproject/ray:2.56.0"},
						{Name: "sidecar", Image: "some-sidecar:latest"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pt := tt.podTemplate.DeepCopy()
			originalContainerCount := len(pt.Spec.Containers)

			injectCollectorSidecar(pt, config, tt.role)

			// Verify ray-logs emptyDir volume added (only 1 volume)
			require.Len(t, pt.Spec.Volumes, 1, "should have ray-logs volume only")
			assert.Equal(t, "ray-logs", pt.Spec.Volumes[0].Name)
			require.NotNil(t, pt.Spec.Volumes[0].VolumeSource.EmptyDir)

			// Verify all original containers have volume mount and env vars,
			// and that no lifecycle hook was injected.
			for i := 0; i < originalContainerCount; i++ {
				c := pt.Spec.Containers[i]

				// Volume mount
				hasMount := false
				for _, vm := range c.VolumeMounts {
					if vm.Name == "ray-logs" && vm.MountPath == "/tmp/ray" {
						hasMount = true
						break
					}
				}
				assert.True(t, hasMount, "container %s should have ray-logs volume mount", c.Name)

				// Env vars for Ray event export
				envMap, _, _ := splitEnv(c.Env)
				assert.Equal(t, "/tmp/ray", envMap["RAY_TMP_ROOT"],
					"container %s needs RAY_TMP_ROOT pinned to the shared volume", c.Name)
				assert.Equal(t, "true", envMap["RAY_enable_ray_event"])
				assert.Equal(t, "true", envMap["RAY_enable_core_worker_ray_event_to_aggregator"])
				assert.Equal(t, fmt.Sprintf("http://localhost:%d/v1/events", collectorPort), envMap["RAY_DASHBOARD_AGGREGATOR_AGENT_EVENTS_EXPORT_ADDR"])
				assert.Equal(t, "ALL", envMap["RAY_DASHBOARD_AGGREGATOR_AGENT_EXPOSABLE_EVENT_TYPES"],
					"empty config must default to ALL")

				// v1.7 identifies nodes from POD_IP/FQ_RAY_IP — the v1.6
				// node-ID PostStart hook must be gone.
				assert.Nil(t, c.Lifecycle, "container %s should have no injected lifecycle hook", c.Name)
			}

			// Verify collector sidecar container added
			require.Len(t, pt.Spec.Containers, originalContainerCount+1)
			collector := pt.Spec.Containers[originalContainerCount]
			assert.Equal(t, "collector", collector.Name)
			assert.Equal(t, config.CollectorImage, collector.Image)

			// v1.7 collector is configured entirely by env — the image
			// entrypoint runs, so neither Command nor Args may be set.
			assert.Nil(t, collector.Command, "v1.7 collector must not override the entrypoint")
			assert.Nil(t, collector.Args, "v1.7 collector takes no args")
			assert.Nil(t, collector.Lifecycle)

			collectorEnvMap, collectorSecretEnvs, collectorFieldRefs := splitEnv(collector.Env)

			// Downward API: cluster identity and pod IP
			assert.Equal(t, "metadata.labels['ray.io/cluster']", collectorFieldRefs["RAY_CLUSTER_NAME"])
			assert.Equal(t, "metadata.namespace", collectorFieldRefs["RAY_CLUSTER_NAMESPACE"])
			assert.Equal(t, "status.podIP", collectorFieldRefs["POD_IP"])

			// Kubernetes only expands $(VAR) against vars declared earlier in
			// the same list, so the two referenced vars must come first.
			idx := envIndex(collector.Env)
			assert.Equal(t, "$(RAY_CLUSTER_NAME)-head-svc.$(RAY_CLUSTER_NAMESPACE).svc.cluster.local", collectorEnvMap["FQ_RAY_IP"])
			assert.Less(t, idx["RAY_CLUSTER_NAME"], idx["FQ_RAY_IP"],
				"RAY_CLUSTER_NAME must precede FQ_RAY_IP for $(VAR) expansion")
			assert.Less(t, idx["RAY_CLUSTER_NAMESPACE"], idx["FQ_RAY_IP"],
				"RAY_CLUSTER_NAMESPACE must precede FQ_RAY_IP for $(VAR) expansion")

			// Core v1.7 contract
			assert.Equal(t, "/tmp/ray", collectorEnvMap["RAY_TMP_ROOT"])
			assert.Equal(t, tt.role, collectorEnvMap["RAY_ROLE"])
			assert.Equal(t, "s3", collectorEnvMap["STORAGE_BACKEND"])
			assert.Equal(t, "log", collectorEnvMap["STORAGE_ROOT_DIR"])
			assert.Equal(t, fmt.Sprintf("%d", collectorPort), collectorEnvMap["EVENTS_PORT"])

			// S3 config env vars
			assert.Equal(t, "ray-history", collectorEnvMap["S3_BUCKET"])
			assert.Equal(t, "minio:9091", collectorEnvMap["S3_ENDPOINT"])
			assert.Equal(t, "us-east-1", collectorEnvMap["S3_REGION"])
			assert.Equal(t, "true", collectorEnvMap["S3FORCE_PATH_STYLE"])
			assert.Equal(t, "true", collectorEnvMap["S3DISABLE_SSL"])

			// Credentials: standard AWS names, from the configured secret
			require.Contains(t, collectorSecretEnvs, "AWS_ACCESS_KEY_ID")
			assert.Equal(t, "minio-credentials", collectorSecretEnvs["AWS_ACCESS_KEY_ID"].Name)
			assert.Equal(t, "AWS_ACCESS_KEY_ID", collectorSecretEnvs["AWS_ACCESS_KEY_ID"].Key)
			require.Contains(t, collectorSecretEnvs, "AWS_SECRET_ACCESS_KEY")
			assert.Equal(t, "minio-credentials", collectorSecretEnvs["AWS_SECRET_ACCESS_KEY"].Name)
			assert.Equal(t, "AWS_SECRET_ACCESS_KEY", collectorSecretEnvs["AWS_SECRET_ACCESS_KEY"].Key)
			assert.Equal(t, "", collectorEnvMap["AWS_SESSION_TOKEN"])

			// The v1.6 fork's custom credential aliases — the AWS_S3* family —
			// must be gone. Leaving them set is harmless to the collector but
			// masks a half-finished migration. Scanning collector.Env directly
			// covers literal and secret-backed vars in one pass.
			for _, e := range collector.Env {
				assert.False(t, strings.HasPrefix(e.Name, "AWS_S3"),
					"collector env %s is a v1.6 credential alias and must be gone", e.Name)
			}
			// Region moved off the generic AWS variable onto S3_REGION.
			assert.NotContains(t, collectorEnvMap, "AWS_REGION")
			assert.NotContains(t, collectorSecretEnvs, "AWS_REGION")

			// Head-specific env vars
			if tt.role == "Head" {
				assert.Equal(t, "http://localhost:8265", collectorEnvMap["RAY_DASHBOARD_ADDRESS"])
				assert.NotEmpty(t, collectorEnvMap["RAY_COLLECTOR_ADDITIONAL_ENDPOINTS"])
				assert.Equal(t, "30s", collectorEnvMap["RAY_COLLECTOR_POLL_INTERVAL"])
			} else {
				assert.NotContains(t, collectorEnvMap, "RAY_DASHBOARD_ADDRESS",
					"worker collector should not have RAY_DASHBOARD_ADDRESS")
			}

			// Verify collector port
			require.Len(t, collector.Ports, 1)
			assert.Equal(t, "events", collector.Ports[0].Name)
			assert.Equal(t, int32(collectorPort), collector.Ports[0].ContainerPort)

			// Verify collector resources
			assert.Equal(t, resource.MustParse("100m"), collector.Resources.Requests[corev1.ResourceCPU])
			assert.Equal(t, resource.MustParse("128Mi"), collector.Resources.Requests[corev1.ResourceMemory])

			// Verify collector volume mounts (ray-logs only, no ConfigMap)
			require.Len(t, collector.VolumeMounts, 1)
			assert.Equal(t, "ray-logs", collector.VolumeMounts[0].Name)
			assert.Equal(t, "/tmp/ray", collector.VolumeMounts[0].MountPath)
		})
	}
}

// TestInjectCollectorSidecar_LegacyExposableEventTypes covers Ray images older
// than 2.54, which reject "ALL". Michelangelo does not own the Ray image — it
// comes from the user's task spec — so the explicit list must reach the Ray
// containers verbatim.
func TestInjectCollectorSidecar_LegacyExposableEventTypes(t *testing.T) {
	config := LogPersistenceConfig{
		Enabled:             true,
		StorageEndpoint:     "minio:9091",
		Bucket:              "ray-history",
		CredentialsSecret:   "minio-credentials",
		CollectorImage:      "quay.io/kuberay/collector:v1.7.1",
		ExposableEventTypes: legacyExposableEventTypes,
	}

	pt := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "ray-head", Image: "rayproject/ray:2.10.0"},
			},
		},
	}

	injectCollectorSidecar(pt, config, "Head")

	envMap, _, _ := splitEnv(pt.Spec.Containers[0].Env)
	assert.Equal(t, legacyExposableEventTypes, envMap["RAY_DASHBOARD_AGGREGATOR_AGENT_EXPOSABLE_EVENT_TYPES"])
	assert.NotEqual(t, defaultExposableEventTypes, envMap["RAY_DASHBOARD_AGGREGATOR_AGENT_EXPOSABLE_EVENT_TYPES"])
}

func TestInjectCollectorSidecar_PreservesExistingLifecycle(t *testing.T) {
	config := LogPersistenceConfig{
		Enabled:           true,
		StorageEndpoint:   "minio:9091",
		Bucket:            "ray-history",
		CredentialsSecret: "minio-credentials",
		CollectorImage:    "quay.io/kuberay/collector:v1.7.1",
	}

	pt := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "ray-head",
					Image: "rayproject/ray:2.56.0",
					Lifecycle: &corev1.Lifecycle{
						PreStop: &corev1.LifecycleHandler{
							Exec: &corev1.ExecAction{
								Command: []string{"/bin/sh", "-c", "ray stop"},
							},
						},
					},
				},
			},
		},
	}

	injectCollectorSidecar(pt, config, "Head")

	// v1.7 injects no PostStart hook of its own
	require.NotNil(t, pt.Spec.Containers[0].Lifecycle)
	assert.Nil(t, pt.Spec.Containers[0].Lifecycle.PostStart)
	// The user's own PreStop must survive untouched
	require.NotNil(t, pt.Spec.Containers[0].Lifecycle.PreStop)
	assert.Equal(t, []string{"/bin/sh", "-c", "ray stop"}, pt.Spec.Containers[0].Lifecycle.PreStop.Exec.Command)
}

func TestInjectCollectorSidecar_DisabledConfig(t *testing.T) {
	// When config.Enabled is false, the caller (mapRayCluster) should not call injectCollectorSidecar.
	// This test verifies injectCollectorSidecar still works correctly if called — it always injects.
	config := LogPersistenceConfig{
		Enabled:           false,
		StorageEndpoint:   "minio:9091",
		Bucket:            "ray-history",
		CredentialsSecret: "minio-credentials",
		CollectorImage:    "quay.io/kuberay/collector:v1.7.1",
	}

	pt := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "ray-head", Image: "rayproject/ray:2.10.0"},
			},
		},
	}

	injectCollectorSidecar(pt, config, "Head")

	// Function always injects when called — the enabled check is the caller's responsibility
	require.Len(t, pt.Spec.Containers, 2)
	assert.Equal(t, "collector", pt.Spec.Containers[1].Name)
}
