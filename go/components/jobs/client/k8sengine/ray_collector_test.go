package k8sengine

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestInjectCollectorSidecar(t *testing.T) {
	config := LogPersistenceConfig{
		Enabled:           true,
		StorageEndpoint:   "minio:9091",
		Bucket:            "ray-history",
		PathPrefix:        "clusters/",
		Region:            "us-east-1",
		CredentialsSecret: "aws-credentials",
		CollectorImage:    "kuberay-collector:local",
	}

	tests := []struct {
		name             string
		role             string
		clusterName      string
		clusterNamespace string
		podTemplate      corev1.PodTemplateSpec
	}{
		{
			name:             "head pod gets collector sidecar",
			role:             "Head",
			clusterName:      "test-cluster",
			clusterNamespace: "default",
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "ray-head", Image: "rayproject/ray:2.10.0"},
					},
				},
			},
		},
		{
			name:             "worker pod gets collector sidecar",
			role:             "Worker",
			clusterName:      "test-cluster",
			clusterNamespace: "default",
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "ray-worker", Image: "rayproject/ray:2.10.0"},
					},
				},
			},
		},
		{
			name:             "pod with multiple containers",
			role:             "Head",
			clusterName:      "multi-container-cluster",
			clusterNamespace: "test-ns",
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "ray-head", Image: "rayproject/ray:2.10.0"},
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

			injectCollectorSidecar(pt, config, tt.clusterName, tt.clusterNamespace, tt.role)

			// Verify ray-logs emptyDir volume added (only 1 volume)
			require.Len(t, pt.Spec.Volumes, 1, "should have ray-logs volume only")
			assert.Equal(t, "ray-logs", pt.Spec.Volumes[0].Name)
			require.NotNil(t, pt.Spec.Volumes[0].VolumeSource.EmptyDir)

			// Verify all original containers have volume mount, env vars, and lifecycle hook
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
				envMap := make(map[string]string)
				for _, e := range c.Env {
					envMap[e.Name] = e.Value
				}
				assert.Equal(t, "true", envMap["RAY_enable_ray_event"])
				assert.Equal(t, "true", envMap["RAY_enable_core_worker_ray_event_to_aggregator"])
				assert.Equal(t, fmt.Sprintf("http://localhost:%d/v1/events", collectorPort), envMap["RAY_DASHBOARD_AGGREGATOR_AGENT_EVENTS_EXPORT_ADDR"])
				assert.NotEmpty(t, envMap["RAY_DASHBOARD_AGGREGATOR_AGENT_EXPOSABLE_EVENT_TYPES"])

				// PostStart lifecycle hook for raylet node ID extraction
				require.NotNil(t, c.Lifecycle, "container %s should have lifecycle", c.Name)
				require.NotNil(t, c.Lifecycle.PostStart, "container %s should have PostStart hook", c.Name)
				require.NotNil(t, c.Lifecycle.PostStart.Exec)
				assert.Contains(t, c.Lifecycle.PostStart.Exec.Command[3], "raylet_node_id")
			}

			// Verify collector sidecar container added
			require.Len(t, pt.Spec.Containers, originalContainerCount+1)
			collector := pt.Spec.Containers[originalContainerCount]
			assert.Equal(t, "collector", collector.Name)
			assert.Equal(t, config.CollectorImage, collector.Image)

			// Verify collector command (not args) matches kuberay historyserver pattern
			expectedCommand := []string{
				"collector",
				"--role=" + tt.role,
				"--runtime-class-name=s3",
				"--ray-cluster-name=" + tt.clusterName,
				"--ray-root-dir=log",
				fmt.Sprintf("--events-port=%d", collectorPort),
			}
			assert.Equal(t, expectedCommand, collector.Command)

			// Verify S3 env vars on collector (env var pattern, not ConfigMap)
			collectorEnvMap := make(map[string]string)
			collectorSecretEnvs := make(map[string]string) // name -> secret name
			for _, e := range collector.Env {
				if e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
					collectorSecretEnvs[e.Name] = e.ValueFrom.SecretKeyRef.Name
				} else {
					collectorEnvMap[e.Name] = e.Value
				}
			}

			// AWS credentials from secret
			assert.Equal(t, "aws-credentials", collectorSecretEnvs["AWS_ACCESS_KEY_ID"])
			assert.Equal(t, "aws-credentials", collectorSecretEnvs["AWS_SECRET_ACCESS_KEY"])

			// S3 config env vars
			assert.Equal(t, "", collectorEnvMap["AWS_SESSION_TOKEN"])
			assert.Equal(t, "ray-history", collectorEnvMap["S3_BUCKET"])
			assert.Equal(t, "minio:9091", collectorEnvMap["S3_ENDPOINT"])
			assert.Equal(t, "us-east-1", collectorEnvMap["S3_REGION"])
			assert.Equal(t, "true", collectorEnvMap["S3FORCE_PATH_STYLE"])
			assert.Equal(t, "true", collectorEnvMap["S3DISABLE_SSL"])

			// Head-specific env vars
			if tt.role == "Head" {
				assert.Equal(t, "http://localhost:8265", collectorEnvMap["RAY_DASHBOARD_ADDRESS"])
				assert.NotEmpty(t, collectorEnvMap["RAY_COLLECTOR_ADDITIONAL_ENDPOINTS"])
				assert.Equal(t, "30s", collectorEnvMap["RAY_COLLECTOR_POLL_INTERVAL"])
			} else {
				_, hasDashboard := collectorEnvMap["RAY_DASHBOARD_ADDRESS"]
				assert.False(t, hasDashboard, "worker collector should not have RAY_DASHBOARD_ADDRESS")
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

func TestInjectCollectorSidecar_PreservesExistingLifecycle(t *testing.T) {
	config := LogPersistenceConfig{
		Enabled:           true,
		StorageEndpoint:   "minio:9091",
		Bucket:            "ray-history",
		Region:            "us-east-1",
		CredentialsSecret: "aws-credentials",
		CollectorImage:    "kuberay-collector:local",
	}

	pt := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "ray-head",
					Image: "rayproject/ray:2.10.0",
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

	injectCollectorSidecar(pt, config, "test-cluster", "default", "Head")

	// PostStart should be set
	require.NotNil(t, pt.Spec.Containers[0].Lifecycle.PostStart)
	// PreStop should be preserved
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
		Region:            "us-east-1",
		CredentialsSecret: "aws-credentials",
		CollectorImage:    "kuberay-collector:local",
	}

	pt := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "ray-head", Image: "rayproject/ray:2.10.0"},
			},
		},
	}

	injectCollectorSidecar(pt, config, "test-cluster", "default", "Head")

	// Function always injects when called — the enabled check is the caller's responsibility
	require.Len(t, pt.Spec.Containers, 2)
	assert.Equal(t, "collector", pt.Spec.Containers[1].Name)
}
