package utils

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const _defaultEphemeralStorage string = "50Gi"

// FormatDockerImage formats the docker image with the cluster's registry
func FormatDockerImage(registry, dockerImage string) string {
	if registry != "" && !strings.HasPrefix(dockerImage, registry) {
		return fmt.Sprintf("%s/%s", registry, dockerImage)
	}
	return dockerImage
}

// CreateTiniWrapper creates an entrypoint that uses tini as pid1 to launches the process tree.
// If tini is not present, then it continue with bash. We notice that Ray does not do a good job
// at propagating exit codes from subprocesses. Tini helps with that and graceful termination.
// https://github.com/krallin/tini and https://github.com/krallin/tini/issues/8
func CreateTiniWrapper(cmd []string) []string {
	return append([]string{
		"bash",
		"--norc",
		"-c",
		// exec with tini if available - we still use bash under it to find the PATH for the executables and for the
		// shell commands that the worker uses for its initialization
		// Summary of tini options used:
		// -v: Generate more verbose output. Repeat up to 3 times.
		// -w: Print a warning when processes are getting reaped.
		// -g: Send signals to the child's process group.
		`var_tini=$(command -v tini 2> /dev/null); if [ -x "$var_tini" ]; then exec $var_tini -gwvv -- bash --norc -c "$0" "$@"; else exec bash --norc -c "$0" "$@"; fi`,
	}, removeBashInEntrypoint(cmd)...)
}

var _bashCommands = map[string]any{
	"bash":   true,
	"--norc": true,
	"-c":     true,
}

// Remove bash command and its options from entrypoint. This is done to
// clean up the command and pass it in to construct a new entrypoint. Several
// LLM use cases pass in their own entrypoint including bash commands. We
// want to clean them up before creating a tini wrapper for them. Ideally we do not
// want customers to provide bash related wrappers and instead provide only the business
// logic. For example: run Ray with Jupyter.
// We will also work with customers to ensure this since we cannot validate and sanitize
// all possible inputs.
func removeBashInEntrypoint(cmd []string) []string {
	var ans []string
	for _, c := range cmd {
		if _, ok := _bashCommands[c]; !ok {
			ans = append(ans, c)
		}
	}
	return ans
}

// HeadCommandArguments is needed to construct the head command
type HeadCommandArguments struct {
	CPU                    int32
	GPU                    int32
	Memory                 int64
	NodeLabel              string
	LogPath                string
	ObjectStoreSpillPath   string
	ObjectStoreMemoryRatio float64
	IsRigidCluster         bool
}

// GetDefaultHeadCommand returns the default head command using the given resources
func GetDefaultHeadCommand(args HeadCommandArguments) []string {
	cmd := fmt.Sprintf("python -m data.michelangelo.ray_cluster.head_node_controller"+
		" ray start --head --temp-dir=%s"+
		" --num-cpus=%d --num-gpus=%d --memory=%d"+
		" --port=$RAY_PORT"+
		" --ray-client-server-port=$RAY_CLIENT_PORT"+
		" --node-manager-port=$NODE_MANAGER_PORT"+
		" --object-manager-port=$OBJECT_MANAGER_PORT"+
		" --dashboard-port=$DASHBOARD_PORT"+
		" --dashboard-agent-grpc-port=$DASHBOARD_AGENT_GRPC_PORT"+
		" --dashboard-agent-listen-port=$DASHBOARD_AGENT_LISTEN_PORT"+
		" --metrics-export-port=$METRICS_EXPORT_PORT"+
		// Note that `object_spilling_config` should be specified in json format.
		// We do not need to specify this for workers. https://docs.ray.io/en/latest/ray-core/objects/object-spilling.html#cluster-mode
		` --system-config="'{\"object_spilling_config\":\"{\\\"type\\\":\\\"filesystem\\\",\\\"params\\\":{\\\"directory_path\\\":\\\"%s\\\"}}\"}'"`+
		` --dashboard-host="0.0.0.0"`+
		" --block",
		args.LogPath, args.CPU, args.GPU, args.Memory, args.ObjectStoreSpillPath)

	// Add object store memory parameter for rigid clusters if ObjectStoreMemoryRatio is specified
	if args.IsRigidCluster && args.ObjectStoreMemoryRatio > 0 {
		objectStoreMemory := uint64(float64(args.Memory) * args.ObjectStoreMemoryRatio)
		cmd += fmt.Sprintf(" --object_store_memory %d", objectStoreMemory)
	}

	if args.IsRigidCluster {
		// For rigid Ray cluster, use the new label format
		cmd += fmt.Sprintf(" %s", getRigidHeadResourceLabel(args.GPU))
	} else if args.NodeLabel != "" {
		// For heterogeneous clusters, use the existing label format
		cmd += fmt.Sprintf(" %s", getHeadResourceLabel(args.NodeLabel))
	}

	return []string{cmd}
}

// This method helps generate the resource label in a format expected by ray start command
func getHeadResourceLabel(nodeLabel string) string {
	return fmt.Sprintf(`--resources="'%s'"`, nodeLabel)
}

// getRigidHeadResourceLabel generates the resource label for rigid Ray cluster head nodes
func getRigidHeadResourceLabel(numGPU int32) string {
	rigidLabel := fmt.Sprintf(`{\"DATA_NODE\":1,\"TRAINER_NODE\":%d, \"HEAD_NODE_0\": 1}`, numGPU)
	return fmt.Sprintf(`--resources="'%s'"`, rigidLabel)
}

// getRigidWorkerNodeLabel generates the node label for rigid Ray cluster worker nodes
func getRigidWorkerNodeLabel(numGPU int32) string {
	rigidLabel := fmt.Sprintf(`{\"DATA_NODE\":1,\"TRAINER_NODE\":%d}`, numGPU)
	return fmt.Sprintf(`--node_label='%s'`, rigidLabel)
}

// WorkerCommandArguments is needed to construct worker commans
type WorkerCommandArguments struct {
	CPU                    int32
	Memory                 string
	GPU                    int32
	JobName                string
	NodeLabel              string
	ObjectStoreMemoryRatio float64
	IsRigidCluster         bool
}

// These defaults are applied for heterogeneous Ray cluster. The values
// are taken from existing usage in ml-code repo.
const (
	_dataNodeObjectStoreMemoryRatio    = 0.6
	_trainerNodeObjectStoreMemoryRatio = 0.5
)

// GetDefaultWorkerCommand returns the default worker command using the given resources
func GetDefaultWorkerCommand(args WorkerCommandArguments) ([]string, error) {
	rayStartCmd := fmt.Sprintf(
		"python -m data.michelangelo.ray_cluster.worker_node_kubernetes --head_addr $RAY_IP --cpu_count %d"+
			" --gpu_count %d --kubernetes_job_id %s",
		args.CPU,
		args.GPU,
		args.JobName)

	memoryRatio := args.ObjectStoreMemoryRatio
	if args.IsRigidCluster {
		// For rigid Ray cluster, use the new label format
		rayStartCmd += fmt.Sprintf(" %s", getRigidWorkerNodeLabel(args.GPU))
	} else if isValidHeteroRayWorkerNodeLabel(args.NodeLabel) {
		// For heterogeneous clusters, use the existing label format
		rayStartCmd += fmt.Sprintf(" --node_label %s", args.NodeLabel)

		// If not specified, we choose a default value based on the node type
		if memoryRatio == 0 {
			if strings.EqualFold(args.NodeLabel, constants.RayDataNodeLabel) {
				memoryRatio = _dataNodeObjectStoreMemoryRatio
			} else if strings.EqualFold(args.NodeLabel, constants.RayTrainerNodeLabel) {
				memoryRatio = _trainerNodeObjectStoreMemoryRatio
			}
		}
	}

	if args.Memory != "" && memoryRatio > 0 {
		memQ, err := resource.ParseQuantity(args.Memory)
		if err != nil {
			return nil, err
		}
		objectStoreMemory := uint64(memQ.AsApproximateFloat64() * memoryRatio)
		rayStartCmd += fmt.Sprintf(" --object_store_memory %d", objectStoreMemory)
	}

	// head_info.env is created by the init-container and has the head node info in the form:
	// RAY_IP=172.4.5.100:3100
	exportHeadEnv := "export $(cat /data/head_info.env | tr -d ' ' | xargs -L 1)"

	echoRayStart := "echo ray start"

	workerCmd := strings.Join([]string{
		exportHeadEnv,
		echoRayStart,
		rayStartCmd,
	}, " && ")

	return []string{workerCmd}, nil
}

func isValidHeteroRayWorkerNodeLabel(label string) bool {
	return strings.EqualFold(label, constants.RayDataNodeLabel) || strings.EqualFold(label, constants.RayTrainerNodeLabel)
}

func newZeroResourceList() map[corev1.ResourceName]resource.Quantity {
	zeroList := make(map[corev1.ResourceName]resource.Quantity)
	zeroList[corev1.ResourceCPU] = resource.Quantity{}
	zeroList[constants.ResourceNvidiaGPU] = resource.Quantity{}
	zeroList[corev1.ResourceMemory] = resource.Quantity{}
	zeroList[corev1.ResourceEphemeralStorage] = resource.Quantity{}
	return zeroList
}

// ConvertToResourceList converts MA resource spec
// into K8s resource list
func ConvertToResourceList(
	spec *v2pb.ResourceSpec,
) (corev1.ResourceList, error) {
	rList := newZeroResourceList()
	if spec == nil {
		return rList, nil
	}

	cpu, err := resource.ParseQuantity(strconv.Itoa(int(spec.Cpu)))
	if err != nil {
		return nil, err
	}
	rList[corev1.ResourceCPU] = cpu

	gpu, err := resource.ParseQuantity(strconv.Itoa(int(spec.Gpu)))
	if err != nil {
		return nil, err
	}
	rList[constants.ResourceNvidiaGPU] = gpu

	if spec.Memory != "" {
		memory, parseErr := resource.ParseQuantity(spec.Memory)
		if parseErr != nil {
			return nil, parseErr
		}
		rList[corev1.ResourceMemory] = memory
	}

	diskSize := _defaultEphemeralStorage
	if spec.DiskSize != "" {
		diskSize = spec.DiskSize
	}
	rList[corev1.ResourceEphemeralStorage], err = resource.ParseQuantity(diskSize)
	if err != nil {
		return nil, err
	}

	return rList, nil
}

// IsRayWorkersFieldSpecified returns true if the `Workers` field is specified for the Ray job.
// Note that currently only the heterogeneous ray job spec uses the `Workers`. The homogeneous ray job spec
// still uses the `Worker` field. This is because the `Workers` field was added later on and existing field
// could not be repurposed to maintain backwards compatibility for store CRDs. We will eventually deprecate the
// `Worker` field and converge to the `Workers` field.
func IsRayWorkersFieldSpecified(job *v2pb.RayCluster) bool {
	return len(job.Spec.Workers) > 0
}

// NumRayWorkers returns the number of Ray workers for the given Ray job
func NumRayWorkers(job *v2pb.RayCluster) int {
	cnt := 0
	if IsRayWorkersFieldSpecified(job) {
		for _, worker := range job.Spec.Workers {
			cnt += int(worker.MaxInstances)
		}
	}
	return cnt
}

// IsHeterogeneousRayJob returns true if it's a heterogeneous Ray job
func IsHeterogeneousRayJob(job *v2pb.RayCluster) bool {
	return len(job.Spec.Workers) > 1
}

// IsRigidRayJob returns true if it's a rigid Ray job (homogeneous with single worker type)
func IsRigidRayJob(job *v2pb.RayCluster) bool {
	return !IsHeterogeneousRayJob(job)
}

// IsRayHeadNode returns true if the given pod is a Ray head node
func IsRayHeadNode(pod *corev1.Pod) bool {
	return pod.Labels != nil && pod.Labels[constants.RayNodeTypeLabelKey] == constants.RayHeadNodeType
}

// GetPrometheusConfigMapName provides the constructed prom configmap name for a given job
func GetPrometheusConfigMapName(jobName string) string {
	return fmt.Sprintf("prom-%s", jobName)
}
