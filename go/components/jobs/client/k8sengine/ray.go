package k8sengine

import (
	"fmt"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sptr "k8s.io/utils/ptr"
)

// LogPersistenceConfig holds platform-level configuration for log persistence.
// This is loaded from YAML config under controllers.rayCluster.logPersistence.
// See: https://github.com/ray-project/kuberay/tree/master/historyserver/config
type LogPersistenceConfig struct {
	Enabled           bool   `yaml:"enabled"`
	StorageEndpoint   string `yaml:"storageEndpoint"`   // S3-compatible endpoint (e.g. "minio:9091")
	Bucket            string `yaml:"bucket"`             // S3 bucket name (e.g. "ray-history")
	PathPrefix        string `yaml:"pathPrefix"`         // Key prefix under the bucket (e.g. "clusters/")
	Region            string `yaml:"region"`             // S3 region (e.g. "us-east-1")
	CredentialsSecret string `yaml:"credentialsSecret"`  // K8s Secret with AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY
	CollectorImage    string `yaml:"collectorImage"`     // KubeRay collector sidecar image
}

func (m Mapper) mapRay(rayJob *v2pb.RayJob, jobClusterObject runtime.Object, cluster *v2pb.Cluster) (runtime.Object, error) {
	if jobClusterObject == nil {
		return nil, fmt.Errorf("ray job requires associated RayCluster object")
	}
	rayCluster, ok := jobClusterObject.(*v2pb.RayCluster)
	if !ok {
		return nil, fmt.Errorf("expected *v2pb.RayCluster, got %T", jobClusterObject)
	}
	pod := rayCluster.GetSpec().Head.GetPod()
	submitterPod := k8sptr.Deref(pod, corev1.PodTemplateSpec{})
	// Kubernetes Jobs require restartPolicy to be either "OnFailure" or "Never"
	submitterPod.Spec.RestartPolicy = corev1.RestartPolicyNever

	kubeRayJob := &rayv1.RayJob{
		TypeMeta: metav1.TypeMeta{
			Kind:       RayJobKind,
			APIVersion: RayAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      rayJob.Name,
			Namespace: RayLocalNamespace,
		},
		Spec: rayv1.RayJobSpec{
			ClusterSelector: map[string]string{
				"ray.io/cluster":      rayCluster.Name,
				"rayClusterNamespace": RayLocalNamespace,
			},
			Entrypoint: rayJob.Spec.Entrypoint,
			// kuberay 1.0 only support SubmitterPodTemplate for configuration submitter pod
			// We need to allow user to configure the submitter pod template via ray task configuration
			// Note: Add support for v1.2.2 kuberay once we upgrade to newer version
			SubmitterPodTemplate: &submitterPod,
		},
	}

	return kubeRayJob, nil
}

func (m Mapper) mapRayCluster(rayCluster *v2pb.RayCluster) (runtime.Object, error) {
	workerGroupSpecs := getWorkerGroupSpecs(rayCluster.GetName(), rayCluster.GetSpec().Workers)
	headGroupSpec := getHeadGroupSpec(rayCluster.GetSpec().Head)

	if m.LogPersistence.Enabled {
		injectCollectorSidecar(&headGroupSpec.Template, m.LogPersistence, rayCluster.GetName(), RayLocalNamespace, "Head")
		for i := range workerGroupSpecs {
			injectCollectorSidecar(&workerGroupSpecs[i].Template, m.LogPersistence, rayCluster.GetName(), RayLocalNamespace, "Worker")
		}
	}

	rayV1Cluster := &rayv1.RayCluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       RayClusterKind,
			APIVersion: RayAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      rayCluster.Name,
			Namespace: RayLocalNamespace,
		},
		Spec: rayv1.RayClusterSpec{
			HeadGroupSpec:    headGroupSpec,
			RayVersion:       rayCluster.GetSpec().RayVersion,
			WorkerGroupSpecs: workerGroupSpecs,
		},
	}
	return rayV1Cluster, nil
}

func getHeadGroupSpec(head *v2pb.RayHeadSpec) rayv1.HeadGroupSpec {
	return rayv1.HeadGroupSpec{
		ServiceType:    corev1.ServiceType(head.GetServiceType()),
		RayStartParams: head.GetRayStartParams(),
		Template:       k8sptr.Deref(head.GetPod(), corev1.PodTemplateSpec{}),
	}
}

func getWorkerGroupSpecs(clusterName string, workers []*v2pb.RayWorkerSpec) []rayv1.WorkerGroupSpec {
	workerGroupSpecsJSON := make([]rayv1.WorkerGroupSpec, len(workers))
	for i, workerGroup := range workers {
		wg := rayv1.WorkerGroupSpec{
			GroupName:      RayWorkerNodePrefix + clusterName,
			Replicas:       &workerGroup.MinInstances,
			MinReplicas:    &workerGroup.MinInstances,
			MaxReplicas:    &workerGroup.MaxInstances,
			RayStartParams: workerGroup.RayStartParams,
			Template:       k8sptr.Deref(workerGroup.Pod, corev1.PodTemplateSpec{}),
		}
		workerGroupSpecsJSON[i] = wg
	}
	return workerGroupSpecsJSON
}

const (
	rayLogsVolumeName = "ray-logs"
	rayLogsPath       = "/tmp/ray"
	collectorPort     = 8084

	// nodeIDScript is the PostStart lifecycle hook that extracts the raylet node ID.
	// The collector watches /tmp/ray/raylet_node_id for node identification.
	// This script polls until the raylet process starts, then extracts --node_id from its args.
	// Copied from: https://github.com/ray-project/kuberay/blob/master/historyserver/config/raycluster.yaml
	nodeIDScript = `GetNodeId(){
  while true; do
    nodeid=$(ps -ef | grep raylet | grep node_id | grep -v grep | grep -oP '(?<=--node_id=)[^ ]*')
    if [ -n "$nodeid" ]; then
      echo "$(date) raylet started: ${nodeid}" >> /tmp/ray/init.log
      echo $nodeid > /tmp/ray/raylet_node_id
      break
    else
      echo "$(date) raylet not started" >> /tmp/ray/init.log
      sleep 1
    fi
  done
}
GetNodeId`

	// exposableEventTypes lists the Ray event types the collector should receive.
	// Required for Ray 2.52.0+. In 2.53.0+ the env var name changes to
	// RAY_DASHBOARD_AGGREGATOR_AGENT_PUBLISHER_HTTP_ENDPOINT_EXPOSABLE_EVENT_TYPES
	exposableEventTypes = "TASK_DEFINITION_EVENT,TASK_LIFECYCLE_EVENT,ACTOR_TASK_DEFINITION_EVENT," +
		"TASK_PROFILE_EVENT,DRIVER_JOB_DEFINITION_EVENT,DRIVER_JOB_LIFECYCLE_EVENT," +
		"ACTOR_DEFINITION_EVENT,ACTOR_LIFECYCLE_EVENT,NODE_DEFINITION_EVENT,NODE_LIFECYCLE_EVENT"
)

// injectCollectorSidecar injects a KubeRay History Server collector sidecar container
// into the pod template. Follows the official KubeRay config pattern:
// https://github.com/ray-project/kuberay/blob/master/historyserver/config/raycluster.yaml
//
// It adds:
// - Shared emptyDir volume for /tmp/ray
// - Ray event export env vars on all existing containers
// - PostStart lifecycle hook to extract raylet node ID
// - Collector sidecar with S3 env vars and event port
func injectCollectorSidecar(podTemplate *corev1.PodTemplateSpec, config LogPersistenceConfig, clusterName string, clusterNamespace string, role string) {
	// 1. Determine the volume name for /tmp/ray.
	// If a Ray container already mounts /tmp/ray, reuse that volume so
	// the collector shares the same data. Otherwise, create a new emptyDir.
	rayVolumeName := rayLogsVolumeName
	for _, c := range podTemplate.Spec.Containers {
		for _, vm := range c.VolumeMounts {
			if vm.MountPath == rayLogsPath {
				rayVolumeName = vm.Name
				break
			}
		}
		if rayVolumeName != rayLogsVolumeName {
			break
		}
	}

	// Only add a new volume if no existing volume is being reused
	if rayVolumeName == rayLogsVolumeName {
		hasVolume := false
		for _, v := range podTemplate.Spec.Volumes {
			if v.Name == rayLogsVolumeName {
				hasVolume = true
				break
			}
		}
		if !hasVolume {
			podTemplate.Spec.Volumes = append(podTemplate.Spec.Volumes, corev1.Volume{
				Name: rayLogsVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			})
		}
	}

	rayLogsVolumeMount := corev1.VolumeMount{
		Name:      rayVolumeName,
		MountPath: rayLogsPath,
	}

	// Ray event export env vars — tells Ray to forward events to the collector's HTTP endpoint
	eventExportEnvVars := []corev1.EnvVar{
		{
			Name:  "RAY_enable_ray_event",
			Value: "true",
		},
		{
			Name:  "RAY_enable_core_worker_ray_event_to_aggregator",
			Value: "true",
		},
		{
			Name:  "RAY_DASHBOARD_AGGREGATOR_AGENT_EVENTS_EXPORT_ADDR",
			Value: fmt.Sprintf("http://localhost:%d/v1/events", collectorPort),
		},
		{
			Name:  "RAY_DASHBOARD_AGGREGATOR_AGENT_EXPOSABLE_EVENT_TYPES",
			Value: exposableEventTypes,
		},
	}

	// 2. Update all existing containers: add volume mount, env vars, and lifecycle hook
	for i := range podTemplate.Spec.Containers {
		c := &podTemplate.Spec.Containers[i]
		// Only add volume mount if /tmp/ray is not already mounted
		hasRayLogsMount := false
		for _, vm := range c.VolumeMounts {
			if vm.MountPath == rayLogsPath {
				hasRayLogsMount = true
				break
			}
		}
		if !hasRayLogsMount {
			c.VolumeMounts = append(c.VolumeMounts, rayLogsVolumeMount)
		}
		c.Env = append(c.Env, eventExportEnvVars...)

		// Add PostStart lifecycle hook to extract raylet node ID.
		// Preserves any existing PreStop hook.
		if c.Lifecycle == nil {
			c.Lifecycle = &corev1.Lifecycle{}
		}
		c.Lifecycle.PostStart = &corev1.LifecycleHandler{
			Exec: &corev1.ExecAction{
				Command: []string{"/bin/sh", "-lc", "--", nodeIDScript},
			},
		}
	}

	// 3. Build S3 env vars for collector — matches official kuberay config pattern
	// (env vars, NOT --runtime-class-config-path)
	collectorS3Env := []corev1.EnvVar{
		{
			Name: "AWS_ACCESS_KEY_ID",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: config.CredentialsSecret,
					},
					Key: "AWS_ACCESS_KEY_ID",
				},
			},
		},
		{
			Name: "AWS_SECRET_ACCESS_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: config.CredentialsSecret,
					},
					Key: "AWS_SECRET_ACCESS_KEY",
				},
			},
		},
		{Name: "AWS_SESSION_TOKEN", Value: ""},
		{Name: "S3_BUCKET", Value: config.Bucket},
		{Name: "S3_ENDPOINT", Value: config.StorageEndpoint},
		{Name: "S3_REGION", Value: config.Region},
		{Name: "S3FORCE_PATH_STYLE", Value: "true"},
		{Name: "S3DISABLE_SSL", Value: "true"},
	}

	// Head collector gets additional env vars for dashboard polling
	if role == "Head" {
		collectorS3Env = append(collectorS3Env,
			corev1.EnvVar{Name: "RAY_DASHBOARD_ADDRESS", Value: "http://localhost:8265"},
			corev1.EnvVar{Name: "RAY_COLLECTOR_ADDITIONAL_ENDPOINTS", Value: "/api/v0/placement_groups?detail=1&limit=10000"},
			corev1.EnvVar{Name: "RAY_COLLECTOR_POLL_INTERVAL", Value: "30s"},
		)
	}

	// 4. Build collector sidecar container using command (not args) per official config
	collectorContainer := corev1.Container{
		Name:            "collector",
		Image:           config.CollectorImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command: []string{
			"collector",
			fmt.Sprintf("--role=%s", role),
			"--runtime-class-name=s3",
			fmt.Sprintf("--ray-cluster-name=%s", clusterName),
			fmt.Sprintf("--ray-root-dir=%s", "log"),
			fmt.Sprintf("--events-port=%d", collectorPort),
		},
		Env: collectorS3Env,
		Ports: []corev1.ContainerPort{
			{
				Name:          "events",
				ContainerPort: int32(collectorPort),
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
		},
		VolumeMounts: []corev1.VolumeMount{rayLogsVolumeMount},
	}

	podTemplate.Spec.Containers = append(podTemplate.Spec.Containers, collectorContainer)
}

// getRayClusterStateFromStatus maps KubeRay v1 cluster state to our internal v2pb.RayClusterState
func getRayClusterStateFromKubeRayState(kubeRayState rayv1.ClusterState) v2pb.RayClusterState {
	switch kubeRayState {
	case rayv1.Ready:
		return v2pb.RAY_CLUSTER_STATE_READY
	case rayv1.Failed:
		return v2pb.RAY_CLUSTER_STATE_FAILED
	case rayv1.Unhealthy:
		return v2pb.RAY_CLUSTER_STATE_UNHEALTHY
	case "": // Empty state means unknown
		return v2pb.RAY_CLUSTER_STATE_UNKNOWN
	default:
		// For any future states we don't recognize, default to unknown
		return v2pb.RAY_CLUSTER_STATE_UNKNOWN
	}
}

// convertRayV1ClusterStatusToV2 converts a KubeRay v1 RayCluster status to our internal v2pb.RayClusterStatus
func convertRayV1ClusterStatusToV2(rayV1Cluster *rayv1.RayCluster) *v2pb.RayClusterStatus {
	status := &v2pb.RayClusterStatus{}

	// Map state using the conversion function
	status.State = getRayClusterStateFromKubeRayState(rayV1Cluster.Status.State)

	// Map last update time
	if rayV1Cluster.Status.LastUpdateTime != nil && !rayV1Cluster.Status.LastUpdateTime.IsZero() {
		status.LastUpdateTime = rayV1Cluster.Status.LastUpdateTime
	}

	// Map head node info if available
	if rayV1Cluster.Status.Head.PodIP != "" {
		status.HeadNode = &v2pb.RayHeadNodeInfo{
			Ip: rayV1Cluster.Status.Head.PodIP,
		}
	}

	return status
}

// convertRayV1JobStatusToGlobal converts a KubeRay v1 RayJob status to our internal v2pb.RayJobStatus
func convertRayV1JobStatusToGlobal(rayV1Job *rayv1.RayJob) *v2pb.RayJobStatus {
	if rayV1Job == nil {
		return &v2pb.RayJobStatus{
			JobStatus: "UNKNOWN",
		}
	}

	globalJobStatus := &v2pb.RayJobStatus{}

	globalJobStatus.JobStatus = string(rayV1Job.Status.JobStatus)
	globalJobStatus.JobDeploymentStatus = string(rayV1Job.Status.JobDeploymentStatus)
	globalJobStatus.State = mapV1RayJobStatusToMAState(rayV1Job.Status.JobStatus, rayV1Job.Status.JobDeploymentStatus)
	globalJobStatus.Message = rayV1Job.Status.Message

	return globalJobStatus
}

func mapV1RayJobStatusToMAState(status rayv1.JobStatus, deploymentStatus rayv1.JobDeploymentStatus) v2pb.RayJobState {
	switch status {
	case rayv1.JobStatusSucceeded:
		return v2pb.RAY_JOB_STATE_SUCCEEDED
	case rayv1.JobStatusFailed:
		return v2pb.RAY_JOB_STATE_FAILED
	case rayv1.JobStatusStopped:
		return v2pb.RAY_JOB_STATE_KILLED
	case rayv1.JobStatusRunning:
		return v2pb.RAY_JOB_STATE_RUNNING
	}

	switch deploymentStatus {
	case rayv1.JobDeploymentStatusWaitForK8sJob, rayv1.JobDeploymentStatusWaitForDashboardReady, rayv1.JobDeploymentStatusWaitForDashboard, rayv1.JobDeploymentStatusInitializing:
		return v2pb.RAY_JOB_STATE_INITIALIZING
	}

	return v2pb.RAY_JOB_STATE_INVALID
}
