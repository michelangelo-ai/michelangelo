package k8sengine

import (
	"fmt"
	"strconv"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sptr "k8s.io/utils/ptr"
)

// LogPersistenceConfig holds platform-level configuration for log persistence.
// Loaded from YAML under jobs.k8sengine.mapper.logPersistence.
// See: https://github.com/ray-project/kuberay/tree/master/historyserver/config
type LogPersistenceConfig struct {
	Enabled           bool   `yaml:"enabled"`
	StorageEndpoint   string `yaml:"storageEndpoint"`   // S3-compatible endpoint (e.g. "minio:9091")
	Bucket            string `yaml:"bucket"`            // S3 bucket name (e.g. "ray-history")
	PathPrefix        string `yaml:"pathPrefix"`        // Key prefix under the bucket (e.g. "log")
	Region            string `yaml:"region"`            // S3 region — required by AWS SDK for SigV4 signing even with custom endpoints (e.g. "us-east-1", "us-ashburn-1")
	CredentialsSecret string `yaml:"credentialsSecret"` // K8s Secret with AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY
	CollectorImage    string `yaml:"collectorImage"`    // KubeRay collector sidecar image
	S3DisableSSL      bool   `yaml:"s3DisableSSL"`      // Set S3DISABLE_SSL on the collector (true for in-cluster MinIO; false for OCI/S3)

	// ExposableEventTypes sets RAY_DASHBOARD_AGGREGATOR_AGENT_EXPOSABLE_EVENT_TYPES
	// on every Ray container. Empty means "ALL", which requires Ray >= 2.54; for
	// older Ray images set the explicit comma-separated event-type list instead
	// (legacyExposableEventTypes). Michelangelo does not own the Ray runtime
	// image — it comes from the user's task spec — so this must stay overridable.
	ExposableEventTypes string `yaml:"exposableEventTypes"`

	// LogURLFormat is a Go text/template applied during local→global cluster
	// status translation to produce the human-browsable log URL surfaced on
	// v2 RayClusterStatus. Available template variables: Bucket, PathPrefix,
	// ClusterName, RayLocalNamespace. Empty string disables log_url emission.
	LogURLFormat string `yaml:"logURLFormat"`
}

func (m Mapper) mapRay(rayJob *v2pb.RayJob, jobClusterObject runtime.Object, cluster *v2pb.Cluster) (runtime.Object, error) {
	if jobClusterObject == nil {
		return nil, fmt.Errorf("ray job requires associated RayCluster object")
	}
	rayCluster, ok := jobClusterObject.(*v2pb.RayCluster)
	if !ok {
		return nil, fmt.Errorf("expected *v2pb.RayCluster, got %T", jobClusterObject)
	}
	head := k8sptr.Deref(rayCluster.GetSpec().Head.GetPod(), corev1.PodTemplateSpec{})
	kubeRayJob := &rayv1.RayJob{
		TypeMeta: metav1.TypeMeta{
			Kind:       RayJobKind,
			APIVersion: RayAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      rayJob.Name,
			Namespace: RayLocalNamespace,
			// RayJobs target an existing RayCluster via ClusterSelector, so
			// they are never Kueue-queued themselves (Kueue admits the
			// cluster); no queue label is resolved here.
			Labels: mapLabels(rayJob.GetLabels(), ""),
		},
		Spec: rayv1.RayJobSpec{
			ClusterSelector: map[string]string{
				"ray.io/cluster":      rayCluster.Name,
				"rayClusterNamespace": RayLocalNamespace,
			},
			Entrypoint:               rayJob.Spec.Entrypoint,
			TTLSecondsAfterFinished:  int32(300),
			ShutdownAfterJobFinishes: true,
			// Right-size the submitter pod rather than cloning the head. The submitter
			// only runs `ray job submit` against the existing cluster, so it gets modest
			// resources and none of the head's GPU or scheduling constraints — but it
			// still reuses the head image and pull/auth settings so it can start in the
			// same environment. See buildSubmitterPodTemplate.
			SubmitterPodTemplate: buildSubmitterPodTemplate(head),
		},
	}

	return kubeRayJob, nil
}

// submitterCPURequest, submitterMemRequest, submitterCPULimit and submitterMemLimit
// mirror KubeRay's built-in default submitter sizing (GetDefaultSubmitterContainer):
// modest and GPU-free, since the pod only runs `ray job submit`.
const (
	submitterCPURequest = "500m"
	submitterMemRequest = "200Mi"
	submitterCPULimit   = "1"
	submitterMemLimit   = "1Gi"
)

// buildSubmitterPodTemplate builds a right-sized submitter pod template for the RayJob.
//
// KubeRay can default this itself when SubmitterPodTemplate is nil, but its default
// drops pod-level settings the submitter needs to run in the same environment as the
// cluster it targets — most importantly the container image pull policy (KubeRay's
// default leaves it unset, so an image tagged :latest falls back to Always and cannot
// use a preloaded/local-only image), plus image pull secrets and the service account.
//
// So instead of cloning the head pod (which would reserve head-sized, possibly GPU,
// compute) or leaving the template nil, we construct a minimal submitter that reuses
// the head image and its pull/auth settings while keeping modest resources and none of
// the head's GPU or scheduling constraints.
func buildSubmitterPodTemplate(head corev1.PodTemplateSpec) *corev1.PodTemplateSpec {
	var image string
	var pullPolicy corev1.PullPolicy
	if len(head.Spec.Containers) > 0 {
		image = head.Spec.Containers[0].Image
		pullPolicy = head.Spec.Containers[0].ImagePullPolicy
	}
	return &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			// Kubernetes Jobs require restartPolicy to be either "OnFailure" or "Never".
			RestartPolicy:      corev1.RestartPolicyNever,
			ImagePullSecrets:   head.Spec.ImagePullSecrets,
			ServiceAccountName: head.Spec.ServiceAccountName,
			Containers: []corev1.Container{
				{
					Name:            "ray-job-submitter",
					Image:           image,
					ImagePullPolicy: pullPolicy,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse(submitterCPURequest),
							corev1.ResourceMemory: resource.MustParse(submitterMemRequest),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse(submitterCPULimit),
							corev1.ResourceMemory: resource.MustParse(submitterMemLimit),
						},
					},
				},
			},
		},
	}
}

func (m Mapper) mapRayCluster(rayCluster *v2pb.RayCluster, cluster *v2pb.Cluster) (runtime.Object, error) {
	workerGroupSpecs := getWorkerGroupSpecs(rayCluster.GetName(), rayCluster.GetSpec().Workers)
	headGroupSpec := getHeadGroupSpec(rayCluster.GetSpec().Head)

	if m.LogPersistence.Enabled {
		injectCollectorSidecar(&headGroupSpec.Template, m.LogPersistence, "Head")
		for i := range workerGroupSpecs {
			injectCollectorSidecar(&workerGroupSpecs[i].Template, m.LogPersistence, "Worker")
		}
	}

	queueName, err := m.kueueQueueName(rayCluster.GetLabels(), rayCluster.GetNamespace(), cluster)
	if err != nil {
		return nil, err
	}

	rayV1Cluster := &rayv1.RayCluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       RayClusterKind,
			APIVersion: RayAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      rayCluster.Name,
			Namespace: RayLocalNamespace,
			Labels:    mapLabels(rayCluster.GetLabels(), queueName),
		},
		Spec: rayv1.RayClusterSpec{
			HeadGroupSpec:    headGroupSpec,
			RayVersion:       rayCluster.GetSpec().RayVersion,
			WorkerGroupSpecs: workerGroupSpecs,
		},
	}
	return rayV1Cluster, nil
}

// nonNilRayStartParams returns params unchanged when non-nil, otherwise an
// empty (non-nil) map. KubeRay's HeadGroupSpec/WorkerGroupSpec.RayStartParams
// field has no `omitempty`, so a nil map serializes to JSON `null` — and the
// RayCluster CRD rejects `null` ("rayStartParams in body must be of type
// object"). Michelangelo's v2 API omits rayStartParams by default (and proto3
// drops empty maps on the wire, so callers cannot force `{}` from the request),
// which leaves this field nil here. The bug stays latent until an admission
// webhook re-serializes the RayCluster on create — notably Kueue's mutating
// webhook, which decodes the object and marshals it whole, turning the omitted
// map into an explicit `null` that the CRD then rejects. Emitting `{}` instead
// is always valid and is the equivalent of "no extra ray start params".
func nonNilRayStartParams(params map[string]string) map[string]string {
	if params == nil {
		return map[string]string{}
	}
	return params
}

func getHeadGroupSpec(head *v2pb.RayHeadSpec) rayv1.HeadGroupSpec {
	spec := rayv1.HeadGroupSpec{
		ServiceType:    corev1.ServiceType(head.GetServiceType()),
		RayStartParams: nonNilRayStartParams(head.GetRayStartParams()),
		Template:       k8sptr.Deref(head.GetPod(), corev1.PodTemplateSpec{}),
	}
	tolerateNVIDIAGPUTaint(&spec.Template)
	return spec
}

func getWorkerGroupSpecs(clusterName string, workers []*v2pb.RayWorkerSpec) []rayv1.WorkerGroupSpec {
	workerGroupSpecsJSON := make([]rayv1.WorkerGroupSpec, len(workers))
	for i, workerGroup := range workers {
		wg := rayv1.WorkerGroupSpec{
			GroupName:      RayWorkerNodePrefix + clusterName,
			Replicas:       &workerGroup.MinInstances,
			MinReplicas:    &workerGroup.MinInstances,
			MaxReplicas:    &workerGroup.MaxInstances,
			RayStartParams: nonNilRayStartParams(workerGroup.GetRayStartParams()),
			Template:       k8sptr.Deref(workerGroup.Pod, corev1.PodTemplateSpec{}),
		}
		tolerateNVIDIAGPUTaint(&wg.Template)
		workerGroupSpecsJSON[i] = wg
	}
	return workerGroupSpecsJSON
}

// nvidiaGPUTaintKey is the key of the taint NVIDIA's GPU operator puts on GPU
// nodes (nvidia.com/gpu=present:NoSchedule) to keep GPU-free workloads off
// scarce accelerator capacity. NVIDIA reuses the extended-resource name as the
// taint key, so both are derived from one constant here.
const nvidiaGPUTaintKey = string(constants.ResourceNvidiaGPU)

// nvidiaGPUToleration lets a pod schedule onto a node carrying that taint. It
// matches on key and effect rather than on value: "present" is the GPU
// operator's convention, but other driver installers and node pools use other
// values (e.g. "true"), and a pod that asked for a GPU belongs on a GPU node
// under any of them.
var nvidiaGPUToleration = corev1.Toleration{
	Key:      nvidiaGPUTaintKey,
	Operator: corev1.TolerationOpExists,
	Effect:   corev1.TaintEffectNoSchedule,
}

// tolerateNVIDIAGPUTaint adds the NVIDIA GPU toleration to podTemplate when it
// asks for nvidia.com/gpu. Michelangelo's v2 RayCluster API exposes no
// toleration field, so without this a GPU cluster would sit Pending forever
// against a tainted GPU node pool. Pods that want no GPU are left alone, so the
// taint keeps doing its job of reserving those nodes.
func tolerateNVIDIAGPUTaint(podTemplate *corev1.PodTemplateSpec) {
	if !requestsNVIDIAGPU(&podTemplate.Spec) || toleratesNVIDIAGPUTaint(podTemplate.Spec.Tolerations) {
		return
	}
	// Build a new slice rather than appending in place: podTemplate is a shallow
	// copy of the caller's v2pb pod template, so its slice header still points at
	// the caller's backing array.
	tolerations := make([]corev1.Toleration, 0, len(podTemplate.Spec.Tolerations)+1)
	tolerations = append(tolerations, podTemplate.Spec.Tolerations...)
	podTemplate.Spec.Tolerations = append(tolerations, nvidiaGPUToleration)
}

// requestsNVIDIAGPU reports whether any container in the pod asks for at least
// one nvidia.com/gpu. Limits count alongside requests because Kubernetes
// defaults an extended resource's request from its limit, and init containers
// count because their requests also feed the pod's effective request.
func requestsNVIDIAGPU(podSpec *corev1.PodSpec) bool {
	for _, container := range podSpec.InitContainers {
		if containerRequestsNVIDIAGPU(container) {
			return true
		}
	}
	for _, container := range podSpec.Containers {
		if containerRequestsNVIDIAGPU(container) {
			return true
		}
	}
	return false
}

func containerRequestsNVIDIAGPU(container corev1.Container) bool {
	if qty, ok := container.Resources.Requests[constants.ResourceNvidiaGPU]; ok && !qty.IsZero() {
		return true
	}
	qty, ok := container.Resources.Limits[constants.ResourceNvidiaGPU]
	return ok && !qty.IsZero()
}

// toleratesNVIDIAGPUTaint reports whether the pod already covers the GPU taint,
// either naming it explicitly or tolerating everything through the wildcard
// (empty key + Exists) toleration. A caller who expressed their own intent for
// this taint keeps it; we do not stack a second, redundant entry on top.
func toleratesNVIDIAGPUTaint(tolerations []corev1.Toleration) bool {
	for _, toleration := range tolerations {
		if toleration.Key == nvidiaGPUTaintKey {
			return true
		}
		if toleration.Key == "" && toleration.Operator == corev1.TolerationOpExists {
			return true
		}
	}
	return false
}

const (
	rayLogsVolumeName = "ray-logs"
	rayLogsPath       = "/tmp/ray"
	collectorPort     = 8084

	// defaultExposableEventTypes forwards every Ray event type to the collector.
	// This is what KubeRay's v1.7 reference config uses; it requires Ray >= 2.54.
	defaultExposableEventTypes = "ALL"

	// legacyExposableEventTypes is the explicit list that "ALL" replaced, kept as
	// the documented fallback for Ray images older than 2.54, which reject "ALL".
	// Select it through LogPersistenceConfig.ExposableEventTypes.
	legacyExposableEventTypes = "TASK_DEFINITION_EVENT,TASK_LIFECYCLE_EVENT,ACTOR_TASK_DEFINITION_EVENT," +
		"TASK_PROFILE_EVENT,DRIVER_JOB_DEFINITION_EVENT,DRIVER_JOB_LIFECYCLE_EVENT," +
		"ACTOR_DEFINITION_EVENT,ACTOR_LIFECYCLE_EVENT,NODE_DEFINITION_EVENT,NODE_LIFECYCLE_EVENT"
)

// exposableEventTypes resolves the event-type filter to put on the Ray
// containers, defaulting to "ALL" when the operator has not pinned one.
func (c LogPersistenceConfig) exposableEventTypes() string {
	if c.ExposableEventTypes == "" {
		return defaultExposableEventTypes
	}
	return c.ExposableEventTypes
}

// injectCollectorSidecar injects a KubeRay History Server collector sidecar container
// into the pod template. Follows the official KubeRay v1.7 config pattern:
// https://github.com/ray-project/kuberay/blob/v1.7.1/historyserver/config/raycluster.yaml
//
// It adds:
// - Shared emptyDir volume for /tmp/ray
// - Ray event export env vars (plus RAY_TMP_ROOT) on all existing containers
// - Collector sidecar, configured entirely through env
//
// The v1.7 collector is env-driven: it runs its own image entrypoint (no Command
// override), and it identifies its node from the downward-API POD_IP/FQ_RAY_IP
// rather than the v1.6 PostStart hook that scraped --node_id out of `ps -ef`.
// Cluster identity likewise comes from the downward API, not from the caller.
//
// OWNER_KIND/OWNER_NAME are deliberately left unset: Michelangelo never creates
// RayJob-owned clusters, so every cluster lands under
// cluster-history/raycluster/{namespace}/{cluster}/, which is what
// LogPersistenceConfig.LogURLFormat renders.
func injectCollectorSidecar(podTemplate *corev1.PodTemplateSpec, config LogPersistenceConfig, role string) {
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

	// Ray event export env vars — tells Ray to forward events to the collector's
	// HTTP endpoint. RAY_TMP_ROOT pins Ray's temp root to the shared volume the
	// collector reads from; v1.7 requires it on both sides.
	eventExportEnvVars := []corev1.EnvVar{
		{
			Name:  "RAY_TMP_ROOT",
			Value: rayLogsPath,
		},
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
			Value: config.exposableEventTypes(),
		},
	}

	// 2. Update all existing containers: add volume mount and env vars.
	// Any Lifecycle the user set is left untouched — v1.7 needs no hook of its own.
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
	}

	// 3. Build the collector env. The v1.7 collector takes no flags: everything
	// below is read by historyserver/cmd/collector/main.go at startup.
	//
	// ORDER MATTERS. Kubernetes expands $(VAR) references only against env vars
	// declared EARLIER in the same container's list, so RAY_CLUSTER_NAME and
	// RAY_CLUSTER_NAMESPACE must precede FQ_RAY_IP or it resolves to the literal
	// "$(RAY_CLUSTER_NAME)-head-svc...." and the collector never finds the head.
	//
	// Credentials use the standard AWS names only; v1.7 dropped the custom
	// credential aliases the v1.6 fork read, and region moved from the generic
	// AWS region variable to S3_REGION.
	collectorEnv := []corev1.EnvVar{
		{
			Name: "RAY_CLUSTER_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.labels['ray.io/cluster']",
				},
			},
		},
		{
			Name: "RAY_CLUSTER_NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		},
		{
			Name: "POD_IP",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "status.podIP",
				},
			},
		},
		{Name: "FQ_RAY_IP", Value: "$(RAY_CLUSTER_NAME)-head-svc.$(RAY_CLUSTER_NAMESPACE).svc.cluster.local"},
		{Name: "RAY_TMP_ROOT", Value: rayLogsPath},
		{Name: "RAY_ROLE", Value: role},
		{Name: "STORAGE_BACKEND", Value: "s3"},
		{Name: "STORAGE_ROOT_DIR", Value: config.PathPrefix},
		{Name: "EVENTS_PORT", Value: strconv.Itoa(collectorPort)},
		{Name: "S3_BUCKET", Value: config.Bucket},
		{Name: "S3_ENDPOINT", Value: config.StorageEndpoint},
		{Name: "S3_REGION", Value: config.Region},
		{Name: "S3FORCE_PATH_STYLE", Value: "true"},
		{Name: "S3DISABLE_SSL", Value: strconv.FormatBool(config.S3DisableSSL)},
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
		// Static credentials only — no STS session token. Set empty rather than
		// omitted to match upstream's reference config.
		{Name: "AWS_SESSION_TOKEN", Value: ""},
	}

	// Head collector gets additional env vars for dashboard polling
	if role == "Head" {
		collectorEnv = append(collectorEnv,
			corev1.EnvVar{Name: "RAY_DASHBOARD_ADDRESS", Value: "http://localhost:8265"},
			corev1.EnvVar{Name: "RAY_COLLECTOR_ADDITIONAL_ENDPOINTS", Value: "/api/v0/placement_groups?detail=1&limit=10000"},
			corev1.EnvVar{Name: "RAY_COLLECTOR_POLL_INTERVAL", Value: "30s"},
		)
	}

	// 4. Build the collector sidecar. No Command/Args override: the v1.7 image
	// entrypoint reads the env above.
	collectorContainer := corev1.Container{
		Name:            "collector",
		Image:           config.CollectorImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Env:             collectorEnv,
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
		VolumeMounts:    []corev1.VolumeMount{rayLogsVolumeMount},
		SecurityContext: collectorSecurityContext(&podTemplate.Spec),
	}

	podTemplate.Spec.Containers = append(podTemplate.Spec.Containers, collectorContainer)
}

// collectorSecurityContext picks the identity the collector sidecar runs as.
//
// The v1.7 collector does not start collecting until it can dial the raylet
// unix socket under RAY_TMP_ROOT (historyserver utils.GetSessionDir ->
// IsSessionDirActive). Connecting to a unix socket needs write permission on
// the socket file, which Ray creates 0755 owned by whatever uid runs
// `ray start`. The collector image runs as 1000:1000, so against a Ray image
// that runs as root (michelangelo-examples and most custom task images) the
// dial fails with EACCES, the collector gives up after 60s and exits 1,
// kubelet restarts it into CrashLoopBackOff, utils.terminalPodErrorReasons
// then kills the whole cluster, and nothing is ever uploaded.
//
// The image USER is not visible from the pod spec, so match the Ray container
// (Containers[0], KubeRay's convention) instead of assuming uid 1000:
//   - explicit container-level runAsUser on the Ray container: mirror it;
//   - pod-level runAsUser: return nil so the sidecar inherits that identity;
//   - otherwise: root, which can open any uid's socket and read any uid's logs.
func collectorSecurityContext(podSpec *corev1.PodSpec) *corev1.SecurityContext {
	if len(podSpec.Containers) > 0 {
		if sc := podSpec.Containers[0].SecurityContext; sc != nil && sc.RunAsUser != nil {
			uid := *sc.RunAsUser
			out := &corev1.SecurityContext{RunAsUser: &uid}
			if sc.RunAsGroup != nil {
				gid := *sc.RunAsGroup
				out.RunAsGroup = &gid
			}
			return out
		}
	}
	if podSpec.SecurityContext != nil && podSpec.SecurityContext.RunAsUser != nil {
		return nil
	}
	root := int64(0)
	rootGroup := int64(0)
	return &corev1.SecurityContext{RunAsUser: &root, RunAsGroup: &rootGroup}
}

// getRayClusterStateFromStatus maps KubeRay v1 cluster state to our internal v2pb.RayClusterState
func getRayClusterStateFromKubeRayState(kubeRayState rayv1.ClusterState) v2pb.RayClusterState {
	switch kubeRayState {
	case rayv1.Ready:
		return v2pb.RAY_CLUSTER_STATE_READY
	case rayv1.Failed:
		return v2pb.RAY_CLUSTER_STATE_FAILED
	case "unhealthy":
		return v2pb.RAY_CLUSTER_STATE_UNHEALTHY
	case rayv1.Suspended:
		return v2pb.RAY_CLUSTER_STATE_SUSPENDED
	case "": // Empty state means unknown
		return v2pb.RAY_CLUSTER_STATE_UNKNOWN
	default:
		// For any future states we don't recognize, default to unknown
		return v2pb.RAY_CLUSTER_STATE_UNKNOWN
	}
}

// KubeRay writes these condition types while suspending/resuming a cluster
// (RayCluster.spec.suspend). The vendored ray-operator API (v1.2.2) predates
// their constants, so they are string-matched here.
const (
	rayClusterSuspendingConditionType = "RayClusterSuspending"
	rayClusterSuspendedConditionType  = "RayClusterSuspended"
)

// isSuspended reports whether the cluster is suspended or being suspended
// (spec.suspend=true, e.g. set by a queueing/admission controller such as
// Kueue). Spec intent, reported state, and conditions are all checked because
// status.state lags spec.suspend when a cluster is suspended at creation.
func isSuspended(rc *rayv1.RayCluster) bool {
	if rc.Spec.Suspend != nil && *rc.Spec.Suspend {
		return true
	}
	if rc.Status.State == rayv1.Suspended {
		return true
	}
	for _, cond := range rc.Status.Conditions {
		if cond.Status != metav1.ConditionTrue {
			continue
		}
		if cond.Type == rayClusterSuspendingConditionType || cond.Type == rayClusterSuspendedConditionType {
			return true
		}
	}
	return false
}

func isFailureCondition(cond metav1.Condition) bool {
	switch rayv1.RayClusterConditionType(cond.Type) {
	case rayv1.HeadPodReady:
		return cond.Status == metav1.ConditionFalse &&
			cond.Reason != "" &&
			cond.Reason != rayv1.RayClusterPodsProvisioning
	case rayv1.RayClusterReplicaFailure:
		return cond.Status == metav1.ConditionTrue
	default:
		return false
	}
}

func extractPodErrorsFromConditions(rc *rayv1.RayCluster) []*v2pb.PodErrors {
	// Pods are intentionally absent while a cluster is suspended, so pod-level
	// failure conditions (HeadPodReady=False, ReplicaFailure) are not errors in
	// that window. Once unsuspended they count again.
	if isSuspended(rc) {
		return nil
	}
	var podErrors []*v2pb.PodErrors
	for _, cond := range rc.Status.Conditions {
		if !isFailureCondition(cond) {
			continue
		}
		podErrors = append(podErrors, &v2pb.PodErrors{
			Name:    cond.Type,
			Reason:  cond.Reason,
			Message: cond.Message,
		})
	}
	return podErrors
}

func deriveReasonFromConditions(rc *rayv1.RayCluster) string {
	if isSuspended(rc) {
		for _, cond := range rc.Status.Conditions {
			if cond.Type == rayClusterSuspendingConditionType && cond.Status == metav1.ConditionTrue {
				return "ClusterSuspending"
			}
		}
		return "ClusterSuspended"
	}
	for _, cond := range rc.Status.Conditions {
		if rayv1.RayClusterConditionType(cond.Type) == rayv1.RayClusterReplicaFailure &&
			cond.Status == metav1.ConditionTrue && cond.Reason != "" {
			return cond.Reason
		}
	}
	for _, cond := range rc.Status.Conditions {
		if rayv1.RayClusterConditionType(cond.Type) == rayv1.HeadPodReady &&
			cond.Status == metav1.ConditionFalse && cond.Reason != "" &&
			cond.Reason != rayv1.RayClusterPodsProvisioning {
			return cond.Reason
		}
	}
	return ""
}

// convertRayV1ClusterStatusToV2 converts a KubeRay v1 RayCluster status to our internal v2pb.RayClusterStatus
func convertRayV1ClusterStatusToV2(rayV1Cluster *rayv1.RayCluster) *v2pb.RayClusterStatus {
	status := &v2pb.RayClusterStatus{}

	// Map state using the conversion function
	status.State = getRayClusterStateFromKubeRayState(rayV1Cluster.Status.State)

	// Suspension is visible in spec/conditions before status.state catches up
	// (e.g. a cluster suspended by its admission webhook at creation still has
	// state ""); report SUSPENDED for the whole window.
	if isSuspended(rayV1Cluster) {
		status.State = v2pb.RAY_CLUSTER_STATE_SUSPENDED
	}

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

	if len(rayV1Cluster.Status.Conditions) > 0 {
		status.PodErrors = extractPodErrorsFromConditions(rayV1Cluster)
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
	case rayv1.JobDeploymentStatusInitializing, rayv1.JobDeploymentStatusWaiting:
		return v2pb.RAY_JOB_STATE_INITIALIZING
	}

	return v2pb.RAY_JOB_STATE_INVALID
}
