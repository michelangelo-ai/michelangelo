package utils

import (
	"testing"

	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	"github.com/go-test/deep"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v2beta1 "michelangelo/api/v2beta1"
)

func TestFormatDockerImage(t *testing.T) {
	tt := []struct {
		registry      string
		dockerImage   string
		expectedImage string
		msg           string
	}{
		{
			registry:      "127.0.0.1:5055",
			dockerImage:   "uber-usi/ml-code-ertp-dev-playground:bkt1-produ-1715994219-9293c",
			expectedImage: "127.0.0.1:5055/uber-usi/ml-code-ertp-dev-playground:bkt1-produ-1715994219-9293c",
			msg:           "format image",
		},
		{
			registry:      "127.0.0.1:5055",
			dockerImage:   "127.0.0.1:5055/uber-usi/ml-code-ertp-dev-playground:bkt1-produ-1715994219-9293c",
			expectedImage: "127.0.0.1:5055/uber-usi/ml-code-ertp-dev-playground:bkt1-produ-1715994219-9293c",
			msg:           "already formatted image",
		},
		{
			registry:      "",
			dockerImage:   "uber-usi/ml-code-ertp-dev-playground:bkt1-produ-1715994219-9293c",
			expectedImage: "uber-usi/ml-code-ertp-dev-playground:bkt1-produ-1715994219-9293c",
			msg:           "no registry provided",
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			formattedImage := FormatDockerImage(test.registry, test.dockerImage)
			require.Equal(t, test.expectedImage, formattedImage)
		})
	}
}

func TestCreateTiniWrapper(t *testing.T) {
	command := "ray start"
	expectedCommand := []string{
		"bash",
		"--norc",
		"-c",
		`var_tini=$(command -v tini 2> /dev/null); if [ -x "$var_tini" ]; then exec $var_tini -gwvv -- bash --norc -c "$0" "$@"; else exec bash --norc -c "$0" "$@"; fi`,
		"ray start",
	}
	require.Equal(t, expectedCommand, CreateTiniWrapper([]string{command}))
}

func TestRemoveBashInEntrypoint(t *testing.T) {
	cmd := "export USER={user} && export UBER_OWNER={uber_owner} && export RAY_DEDUP_LOGS=0 && export $(cat /data/head_info.env | tr -d ' ' | xargs -L 1) && " +
		"python3 ./tools/setup_hadoop_on_phx8.py && echo ray start && python -m tools.ray.worker --head_addr $RAY_IP --cpu_count {cpu} --gpu_count {gpu} --kubernetes_job_id dummy_job"
	inputCmd := []string{
		"bash",
		"--norc",
		"-c",
		cmd,
	}
	outputCmd := removeBashInEntrypoint(inputCmd)
	require.Equal(t, 1, len(outputCmd))
	require.Equal(t, cmd, outputCmd[0])
}

func TestGetDefaultHeadCommand(t *testing.T) {
	args := HeadCommandArguments{
		CPU:                  16,
		GPU:                  1,
		Memory:               16000000000,
		NodeLabel:            "DATA_NODE",
		LogPath:              "/mnt/mesos/sandbox/ray_log",
		ObjectStoreSpillPath: "/ray/obj-spill-vol",
	}
	require.Equal(t,
		[]string{
			"python -m data.michelangelo.ray_cluster.head_node_controller" +
				` ray start --head --temp-dir=/mnt/mesos/sandbox/ray_log` +
				" --num-cpus=16 --num-gpus=1 --memory=16000000000" +
				" --port=$RAY_PORT" +
				" --ray-client-server-port=$RAY_CLIENT_PORT" +
				" --node-manager-port=$NODE_MANAGER_PORT" +
				" --object-manager-port=$OBJECT_MANAGER_PORT" +
				" --dashboard-port=$DASHBOARD_PORT" +
				" --dashboard-agent-grpc-port=$DASHBOARD_AGENT_GRPC_PORT" +
				" --dashboard-agent-listen-port=$DASHBOARD_AGENT_LISTEN_PORT" +
				" --metrics-export-port=$METRICS_EXPORT_PORT" +
				// Note that `object_spilling_config` should be specified in json format.
				// We do not need to specify this for workers. https://docs.ray.io/en/latest/ray-core/objects/object-spilling.html#cluster-mode
				` --system-config="'{\"object_spilling_config\":\"{\\\"type\\\":\\\"filesystem\\\",\\\"params\\\":{\\\"directory_path\\\":\\\"/ray/obj-spill-vol\\\"}}\"}'"` +
				` --dashboard-host="0.0.0.0"` +
				" --block" +
				` --resources="'DATA_NODE'"`,
		},
		GetDefaultHeadCommand(args))
}

func TestGetDefaultWorkerCommand(t *testing.T) {
	tt := []struct {
		args            WorkerCommandArguments
		expectedCommand []string
		expectedError   bool
		msg             string
	}{
		{
			args: WorkerCommandArguments{
				CPU:                    16,
				Memory:                 "16G",
				GPU:                    1,
				JobName:                "ma-ra-test-ray-job",
				NodeLabel:              "DATA_NODE",
				ObjectStoreMemoryRatio: _dataNodeObjectStoreMemoryRatio,
			},
			expectedCommand: []string{
				"export $(cat /data/head_info.env | tr -d ' ' | xargs -L 1) && " +
					"echo ray start && " +
					"python -m data.michelangelo.ray_cluster.worker_node_kubernetes --head_addr $RAY_IP --cpu_count 16" +
					" --gpu_count 1 --kubernetes_job_id ma-ra-test-ray-job" +
					" --node_label DATA_NODE" +
					" --object_store_memory 9600000000",
			},
			msg: "data node command",
		},
		{
			args: WorkerCommandArguments{
				CPU:                    16,
				Memory:                 "16G",
				GPU:                    1,
				JobName:                "ma-ra-test-ray-job",
				NodeLabel:              "TRAINER_NODE",
				ObjectStoreMemoryRatio: _trainerNodeObjectStoreMemoryRatio,
			},
			expectedCommand: []string{
				"export $(cat /data/head_info.env | tr -d ' ' | xargs -L 1) && " +
					"echo ray start && " +
					"python -m data.michelangelo.ray_cluster.worker_node_kubernetes --head_addr $RAY_IP --cpu_count 16" +
					" --gpu_count 1 --kubernetes_job_id ma-ra-test-ray-job" +
					" --node_label TRAINER_NODE" +
					" --object_store_memory 8000000000",
			},
			msg: "trainer node command",
		},
		{
			args: WorkerCommandArguments{
				CPU:                    16,
				Memory:                 "16gi",
				GPU:                    1,
				JobName:                "ma-ra-test-ray-job",
				ObjectStoreMemoryRatio: 0.5,
			},
			expectedError: true,
			msg:           "invalid memory quantity",
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			cmd, err := GetDefaultWorkerCommand(test.args)
			if test.expectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.expectedCommand, cmd)
			}
		})
	}
}

func TestConvertMAResourceSpecToResourceList(t *testing.T) {
	tt := []struct {
		MASpec        v2beta1.ResourceSpec
		K8sResource   corev1.ResourceList
		errorExpected bool
		msg           string
	}{
		{
			MASpec: v2beta1.ResourceSpec{
				Cpu:    4,
				Gpu:    2,
				Memory: "100G",
			},
			K8sResource: corev1.ResourceList{
				corev1.ResourceCPU:              *resource.NewQuantity(4, resource.DecimalSI),
				constants.ResourceNvidiaGPU:     *resource.NewQuantity(2, resource.DecimalSI),
				corev1.ResourceMemory:           *resource.NewScaledQuantity(100, 9),
				corev1.ResourceEphemeralStorage: resource.MustParse("50Gi"),
			},
			msg: "100G memory and 50G disk",
		},
		{
			MASpec: v2beta1.ResourceSpec{
				Cpu:      4,
				Gpu:      2,
				Memory:   "400M",
				DiskSize: "100G",
			},
			K8sResource: corev1.ResourceList{
				corev1.ResourceCPU:              *resource.NewQuantity(4, resource.DecimalSI),
				constants.ResourceNvidiaGPU:     *resource.NewQuantity(2, resource.DecimalSI),
				corev1.ResourceMemory:           *resource.NewScaledQuantity(400, 6),
				corev1.ResourceEphemeralStorage: *resource.NewScaledQuantity(100, 9),
			},
			msg: "400M memory and 100G disk",
		},
		{
			MASpec: v2beta1.ResourceSpec{
				Cpu:    4,
				Gpu:    2,
				Memory: "80000000000",
			},
			K8sResource: corev1.ResourceList{
				corev1.ResourceCPU:              *resource.NewQuantity(4, resource.DecimalSI),
				constants.ResourceNvidiaGPU:     *resource.NewQuantity(2, resource.DecimalSI),
				corev1.ResourceMemory:           *resource.NewScaledQuantity(80, 9),
				corev1.ResourceEphemeralStorage: resource.MustParse("50Gi"),
			},
			msg: "80,000,000,000 memory and 50G disk",
		},
		{
			MASpec: v2beta1.ResourceSpec{
				Cpu:      0,
				Gpu:      0,
				Memory:   "0",
				DiskSize: "0",
			},
			K8sResource: corev1.ResourceList{
				corev1.ResourceCPU:              resource.Quantity{},
				constants.ResourceNvidiaGPU:     resource.Quantity{},
				corev1.ResourceMemory:           resource.Quantity{},
				corev1.ResourceEphemeralStorage: resource.Quantity{},
			},
			msg: "all zeroes",
		},
		{
			MASpec: v2beta1.ResourceSpec{
				Cpu: 5,
			},
			K8sResource: corev1.ResourceList{
				corev1.ResourceCPU:              *resource.NewQuantity(5, resource.DecimalSI),
				constants.ResourceNvidiaGPU:     resource.Quantity{},
				corev1.ResourceMemory:           resource.Quantity{},
				corev1.ResourceEphemeralStorage: resource.MustParse("50Gi"),
			},
			msg: "missing specs for some resources",
		},
		{
			MASpec: v2beta1.ResourceSpec{
				Cpu:    5,
				Memory: "120g",
			},
			K8sResource: corev1.ResourceList{
				corev1.ResourceCPU:              *resource.NewQuantity(5, resource.DecimalSI),
				constants.ResourceNvidiaGPU:     resource.Quantity{},
				corev1.ResourceMemory:           resource.Quantity{},
				corev1.ResourceEphemeralStorage: resource.MustParse("50Gi"),
			},
			errorExpected: true,
			msg:           "bad memory spec",
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			converted, err := ConvertToResourceList(&test.MASpec)
			if test.errorExpected {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Nil(t, deep.Equal(converted, test.K8sResource))
			}
		})
	}
}

func TestNewZeroResourceList(t *testing.T) {
	zeroList := newZeroResourceList()

	cpus, ok := zeroList[corev1.ResourceCPU]
	require.True(t, ok)
	require.Equal(t, resource.Quantity{}, cpus)

	gpus, ok := zeroList[constants.ResourceNvidiaGPU]
	require.True(t, ok)
	require.Equal(t, resource.Quantity{}, gpus)

	memory, ok := zeroList[corev1.ResourceMemory]
	require.True(t, ok)
	require.Equal(t, resource.Quantity{}, memory)

	diskSize, ok := zeroList[corev1.ResourceEphemeralStorage]
	require.True(t, ok)
	require.Equal(t, resource.Quantity{}, diskSize)
}

func TestIsPresentEnvLabel(t *testing.T) {
	tests := []struct {
		name               string
		resourcePoolLabels map[string]string
		want               bool
	}{
		{
			name: "with prod env label",
			resourcePoolLabels: map[string]string{
				constants.ResourcePoolEnvProd: "true",
			},
			want: true,
		},
		{
			name: "with dev env label",
			resourcePoolLabels: map[string]string{
				constants.ResourcePoolEnvDev: "false",
			},
			want: true,
		},
		{
			name: "with test env label",
			resourcePoolLabels: map[string]string{
				constants.ResourcePoolEnvTest: "false",
			},
			want: true,
		},
		{
			name:               "without any label",
			resourcePoolLabels: map[string]string{},
			want:               false,
		},
		{
			name: "without env label",
			resourcePoolLabels: map[string]string{
				"key": "value",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsPresentEnvLabel(tt.resourcePoolLabels); got != tt.want {
				t.Errorf("IsPresentEnvLabel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetHeadResourceLabel(t *testing.T) {
	label := `{\"HEAD_NODE_0\":1}`
	require.Equal(t, `--resources="'{\"HEAD_NODE_0\":1}'"`, getHeadResourceLabel(label))
}

func TestNumRayWorkers(t *testing.T) {
	tt := []struct {
		job             *v2beta1.RayJob
		expectedWorkers int
		msg             string
	}{
		{
			job: &v2beta1.RayJob{
				Spec: v2beta1.RayJobSpec{
					Worker: &v2beta1.WorkerSpec{
						MinInstances: 2,
						MaxInstances: 2,
					},
				},
			},
			expectedWorkers: 2,
			msg:             "homogeneous ray cluster",
		},
		{
			job: &v2beta1.RayJob{
				Spec: v2beta1.RayJobSpec{
					Workers: []*v2beta1.WorkerSpec{
						{
							MinInstances: 4,
							MaxInstances: 4,
							NodeType:     "DATA_NODE",
						},
						{
							MinInstances: 1,
							MaxInstances: 1,
							NodeType:     "TRAINER_NODE",
						},
					},
				},
			},
			expectedWorkers: 5,
			msg:             "heterogeneous ray cluster",
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			require.Equal(t, test.expectedWorkers, NumRayWorkers(test.job))
		})
	}
}

func TestIsRayHeadNode(t *testing.T) {
	tt := []struct {
		pod               *corev1.Pod
		expectedIsRayNode bool
		msg               string
	}{
		{
			pod:               &corev1.Pod{},
			expectedIsRayNode: false,
			msg:               "random pod with not labels",
		},
		{
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						constants.RayNodeLabelKey:     constants.IsRayNodeValue,
						constants.RayNodeTypeLabelKey: constants.RayHeadNodeType,
					},
				},
			},
			expectedIsRayNode: true,
			msg:               "ray head node",
		},
		{
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						constants.RayNodeLabelKey:     constants.IsRayNodeValue,
						constants.RayNodeTypeLabelKey: constants.RayWorkerNodeType,
					},
				},
			},
			expectedIsRayNode: false,
			msg:               "ray worker node",
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			require.Equal(t, test.expectedIsRayNode, IsRayHeadNode(test.pod))
		})
	}
}
