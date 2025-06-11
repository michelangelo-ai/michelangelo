package uke

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"code.uber.internal/base/hash"
	"code.uber.internal/base/ptr"
	computeconstants "code.uber.internal/infra/compute/compute-common/constants"
	sandboxlib "code.uber.internal/infra/compute/k8s-sandbox/lib"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/secrets"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/utils"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/ray/kuberay"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	v2beta1pb "michelangelo/api/v2beta1"
)

const (
	_headReplicas                 = 1
	_rayContainerIdentifierEnvKey = "RAY"
	// This string contains "mesos" in the path even though we're running all Ray jobs
	// on Kubernetes. This is mainly for backwards compatibility since we have this path
	// hardcoded in several places in the Canvas code base.
	// https://sg.uberinternal.com/search?q=context:global+repo:%5Ecode%5C.uber%5C.internal/uber-code/data-ml-code%24+%22/mnt/mesos/sandbox%22&patternType=keyword&sm=0
	_rayContainerSandboxMountPath = "/mnt/mesos/sandbox"
)

// object spill related config https://docs.ray.io/en/latest/ray-core/objects/object-spilling.html
const (
	_objectSpillVolumeMount = "ray-obj-spill-vol"
	_objectSpillMountPoint  = "/ray/obj_store_spill"
)

const _gpuNodeSelectorKey = "compute.uber.com/gpu-node-label-model"

var (
	// We need to reserve the disk size large enough to accommodate all the mounted volumes.
	// In addition to that, we add a buffer space for any writes within the container not going
	// to any of the mounts.
	_bufferDiskSize = resource.MustParse("50Gi")
)

var (
	_diskSpillFliprName      = "rayJobsDiskSpillSize"
	_diskSpillGpuSkuKeyName  = "gpu_sku"
	_badFliprValueMetricName = "invalid_flipr_disk_spill_value"
	_diskSpillMetricKeyName  = "ray_disk_spill_value"
)

func (m Mapper) getDiskSpillVolumeSize(gpuSku string) (resource.Quantity, error) {
	constraintsMap := make(map[string]interface{})
	constraintsMap[_diskSpillGpuSkuKeyName] = strings.ToLower(gpuSku)
	fliprConstraints := m.fliprConstraintsBuilder.GetFliprConstraints(constraintsMap)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	diskSpillSize, err := m.fliprClient.GetStringValue(ctx, _diskSpillFliprName, fliprConstraints, "")
	if err != nil {
		return resource.Quantity{}, fmt.Errorf("flipr %v could not be queried, err: %v", _diskSpillFliprName, err)
	}

	spillQuantity, err := resource.ParseQuantity(diskSpillSize)
	if err != nil {
		// We emit a metric when we find an invalid value. We alert based on this value.
		// https://umonitor.uberinternal.com/alerts/5ff42e26-00cd-4fdb-96e8-f9a37424025a
		m.metrics.MetricsScope.Tagged(map[string]string{
			_diskSpillMetricKeyName: diskSpillSize,
		}).Counter(_badFliprValueMetricName).Inc(1)
		return resource.Quantity{}, fmt.Errorf("flipr %v returned invalid value for gpu sku: %v, err: %v", _diskSpillFliprName, gpuSku, err)
	}
	return spillQuantity, nil
}

type gangInfo struct {
	labelValue      string
	annotationValue string
}

func (m Mapper) getGangInfo(job *v2beta1pb.RayJob) gangInfo {
	var info gangInfo

	// we hash the namespace and name into a uint to comply with label value guidelines.
	// See https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#syntax-and-character-set
	info.labelValue = strconv.FormatUint(uint64(hash.Fnv32a([]byte(job.Namespace+job.Name))), 10)

	totalMembers := 1 // head node is assumed to always be present
	if utils.IsRayWorkersFieldSpecified(job) {
		for _, worker := range job.Spec.Workers {
			totalMembers += int(worker.MinInstances)
		}
	} else {
		totalMembers += int(job.Spec.Worker.MinInstances)
	}

	info.annotationValue = strconv.Itoa(totalMembers)
	return info
}

func (m Mapper) mapRay(job *v2beta1pb.RayJob, cluster *v2beta1pb.Cluster) (runtime.Object, error) {
	enableMTLS, err := m.mTLSHandler.EnableMTLS(job.Namespace)
	if err != nil {
		// If there is an error in Flipr or MA API calls, disable MTLS labels
		// TODO: Add a metric to track this
		enableMTLS = false
	}
	enableMTLSRuntimeClass, err := m.mTLSHandler.EnableMTLSRuntimeClass(job.Namespace)
	if err != nil {
		// If there is an error in Flipr or MA API calls, disable MTLS runtime class
		// TODO: Add a metric to track this
		enableMTLSRuntimeClass = false
	}

	info := m.getGangInfo(job)

	headGroupSpec, err := m.getHeadGroupSpec(job, cluster, info, enableMTLS, enableMTLSRuntimeClass)
	if err != nil {
		return nil, err
	}

	workerGroupSpec, err := m.getWorkerGroupSpec(job, cluster, info, enableMTLS, enableMTLSRuntimeClass)
	if err != nil {
		return nil, err
	}

	kr := &kuberay.RayCluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "RayCluster",
			APIVersion: kuberay.SchemeGroupVersion.String(),
		},
		ObjectMeta: job.ObjectMeta,
		Spec: kuberay.RayClusterSpec{
			WorkerGroupSpecs: workerGroupSpec,
			HeadGroupSpec:    headGroupSpec,
		},
	}

	// Add a label to preserve the original namespace of the job
	if kr.Labels == nil {
		kr.Labels = make(map[string]string)
	}
	kr.Labels[constants.ProjectNameLabelKey] = job.Namespace
	kr.Labels[constants.JobControlPlaneEnvKey] = m.env.RuntimeEnvironment

	if enableMTLS {
		kr.Labels[constants.SecureServiceMeshKey] = constants.SecureServiceMeshMTLSValue
	}

	if kr.Annotations == nil {
		kr.Annotations = make(map[string]string)
	}

	kr.Annotations[constants.UserUIDAnnotationKey] = m.spiffeProvider.GetUserID(job.Spec.GetUser().GetName())
	kr.Annotations[constants.SpiffeAnnotationKey] = m.spiffeProvider.GetSpiffeID(job.Spec.GetUser().GetName())

	// Set the local namespace. We don't set individual head/worker namespaces
	// because they are copied from the RayCluster namespace by the operator.
	kr.Namespace = RayLocalNamespace

	m.preprocessRayRequest(kr)
	return kr, nil
}

// Node label to apply to a head node when a heterogeneous cluster is provisioned
var _heterogeneousHeadNodeLabel = `{\"HEAD_NODE_0\":1}`

func (m Mapper) getHeadGroupSpec(job *v2beta1pb.RayJob, cluster *v2beta1pb.Cluster, gangInfo gangInfo, enableMTLS bool, enableMTLSRuntimeClass bool) (kuberay.HeadGroupSpec, error) {
	headEnvVars := m.getHeadEnvVars(job)
	labels, annotations := m.getHeadLabelsAndAnnotations(job, gangInfo, enableMTLS)

	headPodResources := job.Spec.GetHead().GetPod().GetResource()
	memory := resource.MustParse(headPodResources.GetMemory())

	// It is possible that we may want to let users supply this label in the future
	// for further flexibility
	var nodeLabel string

	// For now, only heterogeneous clusters use the node label
	if utils.IsHeterogeneousRayJob(job) {
		nodeLabel = _heterogeneousHeadNodeLabel
	}

	headCommand := job.GetSpec().Head.GetPod().GetCommand()
	if len(headCommand) == 0 {
		// use the default command
		headCommand = utils.GetDefaultHeadCommand(utils.HeadCommandArguments{
			CPU:                  headPodResources.GetCpu(),
			GPU:                  headPodResources.GetGpu(),
			Memory:               memory.Value(),
			NodeLabel:            nodeLabel,
			LogPath:              m.getRayLogPath(),
			ObjectStoreSpillPath: _objectSpillMountPoint,
		})
	}
	headEntrypoint := utils.CreateTiniWrapper(headCommand)

	jobResource := job.Spec.GetHead().GetPod().GetResource()
	runtimeClass := m.getRuntimeClass(enableMTLSRuntimeClass, jobResource)
	nodeSelector, err := m.getNodeSelectorFromResource(jobResource, cluster)

	resourceRequirements, err := CreateResourceRequirements(headPodResources)
	if err != nil {
		return kuberay.HeadGroupSpec{}, err
	}

	spillVolumeSize, err := m.getDiskSpillVolumeSize(job.Spec.GetHead().GetPod().GetResource().GetGpuSku())
	if err != nil {
		return kuberay.HeadGroupSpec{}, err
	}

	objSpillVolume := corev1.Volume{
		Name: _objectSpillVolumeMount,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{
				Medium:    corev1.StorageMediumDefault,
				SizeLimit: &spillVolumeSize,
			},
		},
	}

	headGroupSpec := kuberay.HeadGroupSpec{
		Replicas:       ptr.Int32(_headReplicas),
		RayStartParams: job.Spec.GetRayConf(),
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Name:        RayHeadNodePrefix + job.Name,
				Annotations: annotations,
				Labels:      labels,
			},
			Spec: corev1.PodSpec{
				NodeSelector: nodeSelector,
				Containers: []corev1.Container{
					{
						Name:            constants.HeadContainerName,
						Image:           utils.FormatDockerImage("127.0.0.1:5055", job.Spec.GetHead().GetPod().GetImage()),
						ImagePullPolicy: corev1.PullIfNotPresent,
						Env:             headEnvVars,
						Command:         headEntrypoint,
						Resources:       resourceRequirements,
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      constants.VolumePrefix + secrets.GetKubeSecretName(job.GetName()),
								MountPath: constants.SecretHadoopMountPath,
							},
							{
								Name:      _objectSpillVolumeMount,
								MountPath: _objectSpillMountPoint,
							},
						},
						SecurityContext: getSecurityContext(job),
					},
				},
				// EnableServiceLinks is disabled because K8 engine defaults this value to true.
				// This flag populates all running services envs in the pod which is not desired.
				EnableServiceLinks: ptr.Of(false),
				HostNetwork:        true,
				RestartPolicy:      corev1.RestartPolicyNever,
				RuntimeClassName:   runtimeClass,
				SchedulerName:      computeconstants.K8sResourceManagerSchedulerName,
				Volumes: []corev1.Volume{
					{
						Name: constants.VolumePrefix + secrets.GetKubeSecretName(job.GetName()),
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: secrets.GetKubeSecretName(job.GetName()),
							},
						},
					},
					objSpillVolume,
				},
			},
		},
	}
	headGroupSpec.Template = m.setupSandboxSideCar(headGroupSpec.Template)

	sandboxVolume, err := m.getVolume(headGroupSpec.Template, computeconstants.SandboxSharedVolumeName)
	if err != nil {
		return kuberay.HeadGroupSpec{}, err
	}

	headGroupSpec.Template = m.setupIdleDetectionSideCar(headGroupSpec.Template, job)

	headGroupSpec.Template = m.adjustRequirementsForVolumes(headGroupSpec.Template, objSpillVolume, sandboxVolume)
	return headGroupSpec, nil
}

func (m Mapper) getRayLogPath() string {
	return path.Join(_rayContainerSandboxMountPath, "ray_log")
}

func (m Mapper) getNodeSelector(gpuAlias string, cluster *v2beta1pb.Cluster) (map[string]string, error) {
	selector := make(map[string]string)
	skuName, err := m.skuCache.GetSkuName(gpuAlias, cluster.Name)
	if err != nil {
		return nil, err
	}

	selector[_gpuNodeSelectorKey] = skuName
	return selector, nil
}

// This function will add sidecar container, add env var RAY=true and add Mount Volume.
func (m Mapper) setupSandboxSideCar(template corev1.PodTemplateSpec) corev1.PodTemplateSpec {
	config := sandboxlib.GetDefaultSandboxSupportConfig()
	config.SharedVolumeMountPath = _rayContainerSandboxMountPath
	m.addEnvToRayContainer(&template)
	template.Spec.Containers = sandboxlib.AddSandboxSupportSidecarContainer(config, template.Spec.Containers)
	templateSpec := sandboxlib.AddSandboxSupportVolumeAndMounts(config, template)
	return sandboxlib.UpdateSandboxSupportTerminationGracePeriodSeconds(config, templateSpec)
}

// This method will add ray=true in worker or head node. This will be used by compute to identify primary container
// for ray jobs.
func (m Mapper) addEnvToRayContainer(template *corev1.PodTemplateSpec) {
	containers := template.Spec.Containers
	for i, c := range containers {
		if c.Name == constants.HeadContainerName || c.Name == constants.WorkerContainerName {
			template.Spec.Containers[i].Env = append(template.Spec.Containers[i].Env, corev1.EnvVar{Name: _rayContainerIdentifierEnvKey, Value: "true"})
			break
		}
	}
}

func (m Mapper) getHeadLabelsAndAnnotations(job *v2beta1pb.RayJob, gangInfo gangInfo, enableMTLS bool) (map[string]string, map[string]string) {
	labels := map[string]string{
		constants.UserLabelKey:                 job.Spec.GetUser().GetName(),
		constants.ProjectNameLabelKey:          job.GetNamespace(),
		constants.JobNameLabelKey:              job.Name,
		constants.UOwnLabelKey:                 job.GetLabels()[constants.UOwnLabelKey],
		constants.OwnerServiceLabelKey:         job.GetLabels()[constants.OwnerServiceLabelKey],
		constants.JobControlPlaneEnvKey:        m.env.RuntimeEnvironment,
		constants.GenericSpireIdentityLabelKey: constants.GenericSpireIdentityLabelValue,
	}

	if enableMTLS {
		labels[constants.SecureServiceMeshKey] = constants.SecureServiceMeshMTLSValue
	}

	// Add GPU SKU node selection label
	if job.Spec.GetHead().GetPod().GetResource().GetGpuSku() != "" {
		labels[computeconstants.GPUNodeLabelMajor] = job.Spec.GetHead().GetPod().GetResource().GetGpuSku()
	}

	// Add annotations for dynamic assignments for all the ports
	headNodeAnnotations := make(map[string]string)
	for _, port := range constants.RayPorts {
		aKey := constants.DynamicPortAnnotationKeyPrefix + port
		headNodeAnnotations[aKey] = constants.DynamicPortAnnotationValue
	}

	headNodeAnnotations[constants.UserUIDAnnotationKey] = m.spiffeProvider.GetUserID(job.Spec.GetUser().GetName())
	headNodeAnnotations[constants.SpiffeAnnotationKey] = m.spiffeProvider.GetSpiffeID(job.Spec.GetUser().GetName())

	m.setComputeAnnotations(job, headNodeAnnotations)
	m.setGangLabelsAndAnnotations(gangInfo, labels, headNodeAnnotations)
	return labels, headNodeAnnotations
}

func (m Mapper) getHeadEnvVars(job *v2beta1pb.RayJob) []corev1.EnvVar {
	// Job identity env vars
	identityEnvVars := []corev1.EnvVar{
		{
			Name:  "JOB_NAME",
			Value: job.Name,
		},
		{
			Name:  "JOB_NAMESPACE",
			Value: job.Namespace,
		},
		{
			Name:  "JOB_CONTROL_PLANE_ENV",
			Value: m.env.RuntimeEnvironment,
		},
		{
			Name:  "USER",
			Value: job.Spec.GetUser().GetName(),
		},
	}

	headEnvVars := make(
		[]corev1.EnvVar,
		len(job.Spec.GetHead().GetPod().GetEnv()),
		len(job.Spec.GetHead().GetPod().GetEnv())+len(constants.RayPorts)+len(identityEnvVars))

	headEnvVars = append(headEnvVars, identityEnvVars...)

	// Add the env from the global spec
	portsInEnv := make(map[string]bool) // list of ports found in supplied env
	for i, e := range job.Spec.GetHead().GetPod().GetEnv() {
		if _, ok := constants.PortsMap[e.Name]; ok {
			portsInEnv[e.Name] = true
		}

		headEnvVars[i] = corev1.EnvVar{
			Name:  e.Name,
			Value: e.Value,
		}
	}

	// Add environment variable for the ports
	for _, port := range constants.RayPorts {
		if _, ok := portsInEnv[port]; ok {
			continue
		}

		// Add dynamic port variables only if no values are
		// supplied in the CRD. This is useful for testing
		// and cases when dynamic port allocation doesn't work
		// as expected.
		aKey := constants.DynamicPortAnnotationKeyPrefix + port
		fp := fmt.Sprintf("metadata.annotations['%s']", aKey)
		headEnvVars = append(headEnvVars, corev1.EnvVar{
			Name: port,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: fp,
				},
			},
		})
	}

	// Add pod ip to the env so that that ray runtime can
	// refer to it
	podIPEnv := corev1.EnvVar{
		Name: constants.PodIP,
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: "status.podIP",
			},
		},
	}
	headEnvVars = append(headEnvVars, podIPEnv)

	return headEnvVars
}

// CreateResourceRequirements creates the resource requirements according to the job's ResourceSpec
// TODO make this method private after uke refactor will remove the requirement of this being available outside
func CreateResourceRequirements(resources *v2beta1pb.ResourceSpec) (corev1.ResourceRequirements, error) {
	requestRequirements, err := utils.ConvertToResourceList(resources)
	if err != nil {
		return corev1.ResourceRequirements{}, err
	}
	return corev1.ResourceRequirements{
		// Limit == Requests
		// Set limits otherwise ray has access to all cpu resources on the host
		// ref: https://t3.uberinternal.com/browse/MA-22656
		Requests: requestRequirements,
		Limits:   requestRequirements,
	}, nil
}

func (m Mapper) getWorkerGroupSpec(
	job *v2beta1pb.RayJob, cluster *v2beta1pb.Cluster, gangInfo gangInfo, enableMTLS bool, enableMTLSRuntimeClass bool) ([]kuberay.WorkerGroupSpec, error) {
	workers := []*v2beta1pb.WorkerSpec{job.Spec.GetWorker()}
	if utils.IsRayWorkersFieldSpecified(job) {
		workers = job.Spec.GetWorkers()
	}

	var kuberayWorkers []kuberay.WorkerGroupSpec

	for _, workerSpec := range workers {
		ports := m.getWorkerPorts(workerSpec)
		env := m.getWorkerEnvVars(workerSpec, ports, job)
		labels, annotations := m.getWorkerLabelsAndAnnotations(job, ports, gangInfo, enableMTLS)
		resources := workerSpec.GetPod().GetResource()

		workerCommand := workerSpec.GetPod().GetCommand()
		if len(workerCommand) == 0 {
			var err error
			workerCommand, err = utils.GetDefaultWorkerCommand(utils.WorkerCommandArguments{
				CPU:                    resources.GetCpu(),
				Memory:                 resources.GetMemory(),
				GPU:                    resources.GetGpu(),
				JobName:                job.Name,
				NodeLabel:              workerSpec.NodeType,
				ObjectStoreMemoryRatio: workerSpec.ObjectStoreMemoryRatio,
			})
			if err != nil {
				return nil, err
			}
		}
		workerEntrypoint := utils.CreateTiniWrapper(workerCommand)

		initSpec, initVolumes, err := m.getInitContainerSpec(job, cluster)
		if err != nil {
			return nil, err
		}

		spillVolumeSize, err := m.getDiskSpillVolumeSize(workerSpec.GetPod().GetResource().GetGpuSku())
		if err != nil {
			return nil, err
		}

		objSpillVolume := corev1.Volume{
			Name: _objectSpillVolumeMount,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					Medium:    corev1.StorageMediumDefault,
					SizeLimit: &spillVolumeSize,
				},
			},
		}

		volumes := append([]corev1.Volume{
			{
				Name: constants.VolumePrefix + secrets.GetKubeSecretName(job.Name),
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: secrets.GetKubeSecretName(job.Name),
					},
				},
			},
			objSpillVolume,
		}, initVolumes...)

		const _serviceAccountName = "sa-michelangelo-ray-init"

		jobResource := workerSpec.GetPod().GetResource()
		runtimeClass := m.getRuntimeClass(enableMTLSRuntimeClass, jobResource)
		nodeSelector, err := m.getNodeSelectorFromResource(jobResource, cluster)

		groupNameSuffix := job.Name
		// Use node type as labels when they exist. The operator distinguishes a worker pod
		// by its group name. Therefore, we make sure that the labels are different between different
		// types of workers.
		// We use he hyphenated strings below to make sure that they're valid field names.
		if strings.EqualFold(workerSpec.NodeType, constants.RayDataNodeLabel) {
			groupNameSuffix = "data-node"
		} else if strings.EqualFold(workerSpec.NodeType, constants.RayTrainerNodeLabel) {
			groupNameSuffix = "trainer-node"
		}

		resourceRequirements, err := CreateResourceRequirements(resources)
		if err != nil {
			return nil, err
		}

		workerGroupSpec := kuberay.WorkerGroupSpec{
			GroupName:      RayWorkerNodePrefix + groupNameSuffix,
			MaxReplicas:    ptr.Int32(workerSpec.GetMaxInstances()),
			MinReplicas:    ptr.Int32(workerSpec.GetMinInstances()),
			Replicas:       ptr.Int32(workerSpec.GetMaxInstances()),
			RayStartParams: job.Spec.GetRayConf(),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: annotations,
					Labels:      labels,
				},
				Spec: corev1.PodSpec{
					NodeSelector: nodeSelector,
					Containers: []corev1.Container{
						{
							Name:            constants.WorkerContainerName,
							Image:           utils.FormatDockerImage("127.0.0.1:5055", workerSpec.GetPod().GetImage()),
							ImagePullPolicy: corev1.PullIfNotPresent,
							Env:             env,
							Resources:       resourceRequirements,
							Command:         workerEntrypoint,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      constants.VolumePrefix + secrets.GetKubeSecretName(job.Name),
									MountPath: constants.SecretHadoopMountPath,
								},
								{
									Name:      _objectSpillVolumeMount,
									MountPath: _objectSpillMountPoint,
								},
								{
									Name:      "workdir",
									MountPath: "/data",
								},
							},
							SecurityContext: getSecurityContext(job),
						},
					},
					// EnableServiceLinks is disabled because K8 engine defaults this value to true.
					// This flag populates all running services envs in the pod which is not desired.
					EnableServiceLinks: ptr.Of(false),
					HostNetwork:        true,
					InitContainers: []corev1.Container{
						// Init container is used in UKE for facilitating head node discovery by worker nodes.
						initSpec,
					},
					RestartPolicy:      corev1.RestartPolicyNever,
					RuntimeClassName:   runtimeClass,
					SchedulerName:      computeconstants.K8sResourceManagerSchedulerName,
					ServiceAccountName: _serviceAccountName,
					Volumes:            volumes,
				},
			},
		}
		workerGroupSpec.Template = m.setupSandboxSideCar(workerGroupSpec.Template)

		sandboxVolume, err := m.getVolume(workerGroupSpec.Template, computeconstants.SandboxSharedVolumeName)
		if err != nil {
			return nil, err
		}

		workerGroupSpec.Template = m.adjustRequirementsForVolumes(workerGroupSpec.Template, objSpillVolume, sandboxVolume)

		kuberayWorkers = append(kuberayWorkers, workerGroupSpec)
	}

	return kuberayWorkers, nil
}

func (m Mapper) getRuntimeClass(enableMTLS bool, resources *v2beta1pb.ResourceSpec) *string {
	if resources.GetGpu() != 0 {
		// GPU is requested
		if enableMTLS {
			return ptr.String(constants.MTLSGPURuntimeClassName)
		}
		return ptr.String(constants.GPURuntimeClassName)
	}

	// No GPU
	if enableMTLS {
		return ptr.String(constants.MTLSRuntimeClassName)
	}

	return nil
}

func (m Mapper) getNodeSelectorFromResource(resources *v2beta1pb.ResourceSpec, cluster *v2beta1pb.Cluster) (map[string]string, error) {
	if gpuSku := resources.GetGpuSku(); gpuSku != "" {
		return m.getNodeSelector(gpuSku, cluster)
	}
	return nil, nil
}

func (m Mapper) getVolume(spec corev1.PodTemplateSpec, name string) (corev1.Volume, error) {
	for _, v := range spec.Spec.Volumes {
		if v.Name == name {
			return v, nil
		}
	}

	return corev1.Volume{}, fmt.Errorf("cannot find volume in the pod template spec: %s", name)
}

// The disk request for the Ray container should at the minimum cover the size of all the mounted volumes plus a buffer size. This is to make
// sure that the Kubernetes scheduler accounts for that storage requirement when scheduling the pod. See
// https://kubernetes.io/blog/2022/09/19/local-storage-capacity-isolation-ga/#how-to-use-local-storage-capacity-isolation
func (m Mapper) adjustRequirementsForVolumes(spec corev1.PodTemplateSpec, volumes ...corev1.Volume) corev1.PodTemplateSpec {
	minRequiredStorage := resource.Quantity{}
	for _, v := range volumes {
		if v.VolumeSource.EmptyDir != nil && v.VolumeSource.EmptyDir.SizeLimit != nil {
			minRequiredStorage.Add(*v.VolumeSource.EmptyDir.SizeLimit)
		}
	}
	minRequiredStorage.Add(_bufferDiskSize)

	// Since the volume mounts are shared across all the containers in the pod, we only need to adjust the requirements
	// only for the main Ray container to avoid double counting the storage requirements.
	for i, c := range spec.Spec.Containers {
		if !m.isRayContainer(c) {
			continue
		}

		req := c.Resources.Requests[corev1.ResourceEphemeralStorage]
		if req.Cmp(minRequiredStorage) < 0 {
			spec.Spec.Containers[i].Resources.Requests[corev1.ResourceEphemeralStorage] = minRequiredStorage
			spec.Spec.Containers[i].Resources.Limits[corev1.ResourceEphemeralStorage] = minRequiredStorage
		}
	}

	return spec
}

func (m Mapper) isRayContainer(c corev1.Container) bool {
	return c.Name == constants.HeadContainerName || c.Name == constants.WorkerContainerName
}

func (m Mapper) getWorkerPorts(workerSpec *v2beta1pb.WorkerSpec) []string {
	// Worker needs the following ports
	// 1. OBJECT_MANAGER_PORT : Raylet port for object manager
	// 2. W_*                 : Ports for the worker process in the worker node.
	// 3. RAY_PORT            : Raylet port for node manager
	// 4. METRICS_EXPORT_PORT : Port to expose metrics endpoint
	workerPorts := []string{
		constants.ObjectManagerPort,
		constants.RayPort,
		constants.MetricsExportPort,
	}
	// Append worker ports based on the CPU limit
	// This is based on recommendation from Anyscale.
	numCPUs := int(workerSpec.GetPod().GetResource().GetCpu())
	numPorts := numCPUs * 5
	for i := 0; i < numPorts; i++ {
		workerPorts = append(workerPorts, fmt.Sprintf("W_%d", i))
	}
	return workerPorts
}

func (m Mapper) getWorkerEnvVars(workerSpec *v2beta1pb.WorkerSpec, workerPorts []string, job *v2beta1pb.RayJob) []corev1.EnvVar {
	// list of ports found in supplied env
	portsInEnv := make(map[string]bool)
	workerEnvVars := make(
		[]corev1.EnvVar,
		len(workerSpec.GetPod().GetEnv()),
		len(workerSpec.GetPod().GetEnv())+len(workerPorts)+1)

	for i, e := range workerSpec.GetPod().GetEnv() {
		if _, ok := constants.PortsMap[e.Name]; ok {
			portsInEnv[e.Name] = true
		}

		workerEnvVars[i] = corev1.EnvVar{
			Name:  e.Name,
			Value: e.Value,
		}
	}

	for _, port := range workerPorts {
		if _, ok := portsInEnv[port]; ok {
			continue
		}

		// Add dynamic port variables only if no values are
		// supplied in the CRD. This is useful for testing
		// and cases when dynamic port allocation doesn't work
		// as expected.
		aKey := constants.DynamicPortAnnotationKeyPrefix + port
		fp := fmt.Sprintf("metadata.annotations['%s']", aKey)
		workerEnvVars = append(workerEnvVars, corev1.EnvVar{
			Name: port,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: fp,
				},
			},
		})
	}

	// add the USER env variable
	workerEnvVars = append(workerEnvVars, corev1.EnvVar{
		Name:  "USER",
		Value: job.Spec.GetUser().GetName(),
	})
	return workerEnvVars
}

func (m Mapper) getWorkerLabelsAndAnnotations(job *v2beta1pb.RayJob, ports []string, gangInfo gangInfo, enableMTLS bool) (map[string]string, map[string]string) {
	labels := map[string]string{
		constants.UserLabelKey:                 job.Spec.GetUser().GetName(),
		constants.ProjectNameLabelKey:          job.Namespace,
		constants.JobNameLabelKey:              job.Name,
		constants.UOwnLabelKey:                 job.GetLabels()[constants.UOwnLabelKey],
		constants.OwnerServiceLabelKey:         job.GetLabels()[constants.OwnerServiceLabelKey],
		constants.JobControlPlaneEnvKey:        m.env.RuntimeEnvironment,
		constants.GenericSpireIdentityLabelKey: constants.GenericSpireIdentityLabelValue,
	}

	if enableMTLS {
		labels[constants.SecureServiceMeshKey] = constants.SecureServiceMeshMTLSValue
	}

	// Add GPU SKU node selection label
	gpuSku := ""

	// in case of heterogeneous clusters, pick the GPU sku from the first available GPU worker type
	if utils.IsRayWorkersFieldSpecified(job) {
		for _, w := range job.Spec.GetWorkers() {
			if w.GetPod().GetResource().GetGpuSku() != "" {
				gpuSku = w.GetPod().GetResource().GetGpuSku()
				break
			}
		}
	} else {
		gpuSku = job.Spec.GetWorker().GetPod().GetResource().GetGpuSku()
	}

	if gpuSku != "" {
		labels[computeconstants.GPUNodeLabelMajor] = gpuSku
	}

	// 1. Annotations required for the init container fx app to startup and access the secrets
	annotations := map[string]string{
		"com.uber.secrets_volume_name":                 "usecret",
		"com.uber.scp.service.id":                      "michelangelo-ray-init",
		"org.apache.aurora.metadata.usecrets.enable":   "true",
		"org.apache.aurora.metadata.usecrets.regional": "true",
		constants.UserUIDAnnotationKey:                 m.spiffeProvider.GetUserID(job.Spec.GetUser().GetName()),
		constants.SpiffeAnnotationKey:                  m.spiffeProvider.GetSpiffeID(job.Spec.GetUser().GetName()),
	}

	// Add dynamic assignments for all the ports
	// UKE assigns dynamic ports based on the annotation in the pod spec.
	for _, port := range ports {
		aKey := constants.DynamicPortAnnotationKeyPrefix + port
		annotations[aKey] = constants.DynamicPortAnnotationValue
	}
	m.setComputeAnnotations(job, annotations)
	m.setGangLabelsAndAnnotations(gangInfo, labels, annotations)

	return labels, annotations
}

func (m Mapper) setGangLabelsAndAnnotations(gangInfo gangInfo, labels, annotations map[string]string) {
	// TODO: Disabling until Compute can fix issues with gang scheduling https://t3.uberinternal.com/browse/COMPUTE-6467
	//labels[computeconstants.GangMemberLabelKey] = gangInfo.labelValue
	//annotations[computeconstants.GangMemberNumberAnnotationKey] = gangInfo.annotationValue
}

func (m Mapper) setComputeAnnotations(job *v2beta1pb.RayJob, annotations map[string]string) {
	//Jobs will be marked as preemptible by default.
	//We will allow the customers to optionally mark their workloads as non-preemptible
	annotations[computeconstants.PreemptibleAnnotationKey] = "true"
	if job.Spec.Scheduling != nil {
		annotations[computeconstants.PreemptibleAnnotationKey] = strconv.FormatBool(job.Spec.GetScheduling().GetPreemptible())
	}
	// annotation for resource pool admission
	annotations[computeconstants.ResourcePoolAnnotationKey] = job.Status.GetAssignment().GetResourcePool()
}

// GetInitContainerDefaultResources gets the init container's default resources
// Required for resource quota since it won't place the pod without all containers having limits.cpu and limits.memory
// TODO make this method private after ray controller test refactor will remove the requirement of this being available outside
func GetInitContainerDefaultResources() *v2beta1pb.ResourceSpec {
	return &v2beta1pb.ResourceSpec{
		Cpu:    1,
		Memory: "2048Mi",
	}
}

const (
	_rayInitContainerName          = "ray-init"
	_rayInitImage                  = constants.InitContainerImage
	_rayIdleDetectionContainerName = "ray-idle-detection-sidecar-container"
	_rayIdleDetectionImage         = constants.IdleDetectionImage
)

const (
	_appIDEnv       = "UDEPLOY_APP_ID"
	_clusterHostEnv = "KUBERNETES_SERVICE_HOST"
	_clusterPortEnv = "KUBERNETES_SERVICE_PORT"
	_secretsPathEnv = "SECRETS_PATH"
)

func (m Mapper) getInitContainerSpec(job *v2beta1pb.RayJob, cluster *v2beta1pb.Cluster) (
	corev1.Container, []corev1.Volume, error) {

	hostURL, err := url.Parse(cluster.Spec.GetKubernetes().Rest.Host)
	if err != nil {
		return corev1.Container{}, nil, err
	}

	resourceRequirements, err := CreateResourceRequirements(GetInitContainerDefaultResources())
	if err != nil {
		return corev1.Container{}, nil, err
	}

	spec := corev1.Container{
		Command:   []string{},
		Name:      _rayInitContainerName,
		Image:     _rayInitImage,
		Resources: resourceRequirements,
		Env: []corev1.EnvVar{
			{
				Name:  _secretsPathEnv,
				Value: "/usecret/current/michelangelo-ray-init/",
			},
			{
				Name:  _appIDEnv,
				Value: "michelangelo-ray-init",
			},
			{
				Name:  _clusterHostEnv,
				Value: hostURL.Host,
			},
			{
				Name:  _clusterPortEnv,
				Value: cluster.Spec.GetKubernetes().Rest.Port,
			},
			{
				Name:  RayHeadNodeEnv,
				Value: RayHeadNodePrefix + job.Name,
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			// workdir is where the head info is written and shared b/w the init and the application container.
			{
				Name:      "workdir",
				MountPath: "/data",
			},
			// These are required for secrets
			{
				Name:      "dns",
				MountPath: "/etc/resolv.conf",
				ReadOnly:  true,
			},
			{
				Name:      "ncsd",
				MountPath: "/var/run/ncsd",
				ReadOnly:  true,
			},
			{
				Name:      "usec",
				MountPath: "/var/run/usec",
				ReadOnly:  true,
			},
			{
				Name:      "usecret",
				MountPath: "/usecret",
			},
			{
				Name:      "sslcert",
				MountPath: "/etc/ssl/certs",
				ReadOnly:  true,
			},
		},
	}

	volumes := []corev1.Volume{
		{
			Name: "workdir",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "log-volume",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "dns",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/resolv.conf",
				},
			},
		},
		{
			Name: "ncsd",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/run/nscd",
				},
			},
		},
		{
			Name: "sslcert",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/ssl/certs",
				},
			},
		},
		{
			Name: "usec",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/run/usec",
				},
			},
		},
		{
			Name: "usecret",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "k8s-batch-secret",
				},
			},
		},
	}

	return spec, volumes, nil
}

func (m Mapper) setupIdleDetectionSideCar(template corev1.PodTemplateSpec, job *v2beta1pb.RayJob) corev1.PodTemplateSpec {
	idleDetectionSidecar := corev1.Container{
		Name:    _rayIdleDetectionContainerName,
		Image:   _rayIdleDetectionImage,
		Command: []string{},
		Env: []corev1.EnvVar{
			{
				Name:  _appIDEnv,
				Value: "michelangelo-ray-idle-detection",
			},
			{
				Name:  "JOB_NAME",
				Value: job.Name,
			},
			{
				Name:  "JOB_NAMESPACE",
				Value: job.Namespace,
			},
			{
				Name:  "USER_NAME",
				Value: job.Spec.GetUser().GetName(),
			},
		},

		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("0"),
				corev1.ResourceMemory: resource.MustParse("0"),
				// Must request 5MiB to satisfy admission webhook.
				corev1.ResourceEphemeralStorage: resource.MustParse("5Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("1"),
				// Setting to 110MiB based on these findings: https://docs.google.com/document/d/1jStGfeQZr_NK2wOQFidB0WCcXkjcQMs-Y2iotUMqkx4/edit?tab=t.0
				corev1.ResourceMemory:           resource.MustParse("110Mi"),
				corev1.ResourceEphemeralStorage: resource.MustParse("5Mi"),
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "flipr",
				MountPath: "/var/cache/flipr-config",
				ReadOnly:  true,
			},
		},
	}

	volumes := []corev1.Volume{
		// Mount flipr-config from host to enable feature flag access to read configuration values.
		{
			Name: "flipr",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/opt/uber/shared/flipr-config",
				},
			},
		},
	}

	template.Spec.Volumes = append(template.Spec.Volumes, volumes...)

	template.Spec.Containers = append(template.Spec.Containers, idleDetectionSidecar)

	return template
}

func (m Mapper) preprocessRayRequest(job *kuberay.RayCluster) {
	// shed the resource version before sending the request
	job.ResourceVersion = ""

	// remove any finalizers
	job.ObjectMeta.Finalizers = nil
}

// getSecurityContext builds and returns the security context for the given RayJob.
// May return nil if no security context is required.
func getSecurityContext(job *v2beta1pb.RayJob) *corev1.SecurityContext {
	var securityContext *corev1.SecurityContext
	if v, found := job.Annotations[PtraceEnabledAnnotation]; found && strings.EqualFold(v, "true") {
		securityContext = &corev1.SecurityContext{
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{
					"SYS_PTRACE",
				},
			},
		}
	}
	return securityContext
}
