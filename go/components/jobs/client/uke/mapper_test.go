package uke

import (
	"errors"
	"fmt"
	"strconv"
	"testing"

	"code.uber.internal/base/ptr"
	"code.uber.internal/base/testing/contextmatcher"
	"code.uber.internal/go/envfx.git"
	computeconstants "code.uber.internal/infra/compute/compute-common/constants"
	"code.uber.internal/infra/compute/k8s-crds/apis/sparkoperator.k8s.io/v1beta2"
	kubespark "code.uber.internal/infra/compute/k8s-crds/apis/sparkoperator.k8s.io/v1beta2"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/testutils"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/types"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/utils"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/metrics"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/ray/kuberay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	v2beta1pb "michelangelo/api/v2beta1"
	"mock/code.uber.internal/rt/flipr-client-go.git/flipr/fliprmock"
	"mock/code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/skus/skusmock"
	"mock/code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/types/typesmock"
)

var _testCluster = v2beta1pb.Cluster{
	ObjectMeta: metav1.ObjectMeta{
		Name: "cluster",
	},
	Spec: v2beta1pb.ClusterSpec{
		Cluster: &v2beta1pb.ClusterSpec_Kubernetes{
			Kubernetes: &v2beta1pb.KubernetesSpec{
				Rest: &v2beta1pb.ConnectionSpec{
					Host: "https://test-k8-api-server",
					Port: "6443",
				},
			},
		},
	},
}

const (
	_expectedSpiffeID = "k8s-batch/uid/12345"
	_expectedUserID   = "12345"
)

var _command = utils.CreateTiniWrapper(utils.GetDefaultHeadCommand(utils.HeadCommandArguments{
	CPU:                  4,
	GPU:                  0,
	Memory:               4000000,
	LogPath:              "/mnt/mesos/sandbox/ray_log",
	ObjectStoreSpillPath: _objectSpillMountPoint,
}))

func TestGetLocalName(t *testing.T) {
	tt := []struct {
		msg                    string
		job                    runtime.Object
		expectedLocalName      string
		expectedLocalNamespace string
	}{
		{
			msg: "ray job",
			job: &v2beta1pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ray-job",
				},
			},
			expectedLocalName:      "ray-job",
			expectedLocalNamespace: RayLocalNamespace,
		},
		{
			msg: "spark job",
			job: &v2beta1pb.SparkJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "spark-job",
				},
			},
			expectedLocalName:      "spark-job",
			expectedLocalNamespace: SparkLocalNamespace,
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			m := Mapper{}
			ns, name := m.GetLocalName(test.job)
			require.Equal(t, test.expectedLocalNamespace, ns)
			require.Equal(t, test.expectedLocalName, name)
		})
	}
}

func TestUnknownMapGlobalToLocal(t *testing.T) {
	g := gomock.NewController(t)
	defer g.Finish()

	mockFlipr := fliprmock.NewMockFliprClient(g)
	mockFliprConstraints := typesmock.NewMockFliprConstraintsBuilder(g)
	mTLSHandler := testutils.CreateMockMTLSHandler(t, mockFlipr, mockFliprConstraints, false, false, nil, nil)

	mapper := NewUkeMapper(MapperParams{
		Env: envfx.Context{
			Environment: "production",
		},
		Scope:       tally.NoopScope,
		MTLSHandler: mTLSHandler,
	}).Mapper

	obj, err := mapper.MapGlobalToLocal(&corev1.Pod{}, nil)
	require.NotNil(t, err)
	require.EqualError(t, err, "the object must be a RayJob or a SparkJob, got:*v1.Pod")
	require.Nil(t, obj)
}

func TestMapSpark(t *testing.T) {
	tt := []struct {
		msg       string
		input     *v2beta1pb.SparkJob
		checkFunc func(cluster runtime.Object)
	}{
		{
			msg:   "map spark job to spark application",
			input: createSparkJob(nil),
			checkFunc: func(obj runtime.Object) {
				sparkApp, ok := obj.(*v1beta2.SparkApplication)
				require.True(t, ok)

				require.Equal(t, "platforms.uberai.michelangelo.ma_dev_test.pipelines.boston_housing.eval", sparkApp.Name)
				require.Equal(t, SparkLocalNamespace, sparkApp.Namespace)
				require.Equal(t, map[string]string{
					constants.ProjectNameLabelKey: "test-namespace",
				}, sparkApp.Labels)
				require.Equal(t, "", sparkApp.ResourceVersion)
				require.Nil(t, sparkApp.Finalizers)

				spec := sparkApp.Spec
				require.Equal(t, computeconstants.K8sResourceManagerSchedulerName, *spec.BatchScheduler)
				require.Equal(t, kubespark.ScalaApplicationType, spec.Type)
				require.Equal(t, "test-user", *spec.ProxyUser)
				require.Equal(t, _sparkEnv, *spec.SparkEnv)
				require.Equal(t, "localhost:15055/uber-usi/ml-code-boston-housing:bkt1-produ-1664402887-d8015", *spec.Image)
				require.Equal(t, "local:///ml-code/bazel-bin/platforms/uberai/michelangelo/ma_dev_test/path_gen.runfiles/ml_code/data/michelangelo/project_runner.py", *spec.MainApplicationFile)
				require.Equal(t, "platforms.uberai.michelangelo.ma_dev_test.pipelines.boston_housing.eval", *spec.MainClass)
				require.Equal(t, []string{
					"run",
					"--project_dir", "platforms/uberai/michelangelo/ma_dev_test",
					"--runnable_name", "platforms.uberai.michelangelo.ma_dev_test.pipelines.boston_housing.eval",
					"--mode", "CLUSTER",
					"--workdir", "mmui-test-eval-app",
				}, spec.Arguments)
				require.Equal(t, map[string]string{
					"spark.executor.memoryOverhead":            "1g",
					"spark.sql.files.maxPartitionBytes":        "537395200b",
					"spark.sql.shuffle.partitions":             "16",
					"spark.task.cpus":                          "1",
					"spark.driver.maxResultSize":               "10g",
					"spark.rdd.useDatasetWrapperRDD":           "true",
					"spark.rdd.postRDDActionHook.enabled":      "true",
					"spark.uber.k8s.logging.service":           "ml-code",
					"container.log.enableTerraBlobIntegration": "true",
				}, spec.SparkConf)
				require.Equal(t, []string{"local:///ml-code/bazel-bin/platforms/uberai/michelangelo/ma_dev_test/path_gen.runfiles/ma_jar_0_7_20220727_231929/jar/downloaded.jar"}, spec.Deps.Jars)

				executor := spec.Executor
				require.Equal(t, "2g", *executor.Memory)
				require.Equal(t, int32(1), *executor.Cores)
				require.Equal(t, int32(7), *executor.Instances)
				require.Equal(t, "PYSPARK_PYTHON", executor.Env[0].Name)
				require.Equal(t, "/ml-code/bazel-bin/data/michelangelo/examples/ma_workspace/python", executor.Env[0].Value)
				require.Equal(t, computeconstants.K8sResourceManagerSchedulerName, *executor.SchedulerName)

				executorSecrets := executor.Secrets
				require.Equal(t, 1, len(executorSecrets))

				executorSecret := executorSecrets[0]
				require.Equal(t, "ma-hadoop-platforms.uberai.michelangelo.ma_dev_test.pipelines.boston_housing.eval", executorSecret.Name)
				require.Equal(t, "/mnt/tokens", executorSecret.Path)
				require.Equal(t, kubespark.HadoopDelegationTokenSecret, executorSecret.Type)

				require.Equal(t, getTestComputeAnnotations("true"), executor.Annotations)

				driver := spec.Driver
				require.Equal(t, "sparkoperator", *driver.ServiceAccount)
				require.Equal(t, "1g", *driver.Memory)
				require.Equal(t, int32(1), *driver.Cores)
				require.Equal(t, true, *driver.HostNetwork)
				require.Equal(t, "PYSPARK_PYTHON", driver.Env[0].Name)
				require.Equal(t, "/ml-code/bazel-bin/data/michelangelo/examples/ma_workspace/python", driver.Env[0].Value)
				require.Equal(t, computeconstants.K8sResourceManagerSchedulerName, *driver.SchedulerName)

				driverSecrets := spec.Driver.Secrets
				require.Equal(t, 1, len(driverSecrets))

				driverSecret := driverSecrets[0]
				require.Equal(t, "ma-hadoop-platforms.uberai.michelangelo.ma_dev_test.pipelines.boston_housing.eval", driverSecret.Name)
				require.Equal(t, "/mnt/tokens", driverSecret.Path)
				require.Equal(t, kubespark.HadoopDelegationTokenSecret, driverSecret.Type)

				require.Equal(t, getTestComputeAnnotations("true"), driver.Annotations)
			},
		},
		{
			msg:   "test preemptible true",
			input: createSparkJob(ptr.Of(true)),
			checkFunc: func(obj runtime.Object) {
				sparkApp, _ := obj.(*v1beta2.SparkApplication)
				require.Equal(t, getTestComputeAnnotations("true"), sparkApp.Spec.Executor.Annotations)
				require.Equal(t, getTestComputeAnnotations("true"), sparkApp.Spec.Driver.Annotations)
			},
		},
		{
			msg:   "test preemptible false",
			input: createSparkJob(ptr.Of(false)),
			checkFunc: func(obj runtime.Object) {
				sparkApp, _ := obj.(*v1beta2.SparkApplication)
				require.Equal(t, getTestComputeAnnotations("false"), sparkApp.Spec.Executor.Annotations)
				require.Equal(t, getTestComputeAnnotations("false"), sparkApp.Spec.Driver.Annotations)
			},
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			g := gomock.NewController(t)
			defer g.Finish()

			mockFliprConstraints := typesmock.NewMockFliprConstraintsBuilder(g)
			mockFlipr := fliprmock.NewMockFliprClient(g)
			mTLSHandler := testutils.CreateMockMTLSHandler(t, mockFlipr, mockFliprConstraints, false, false, nil, nil)

			mapper := NewUkeMapper(MapperParams{
				Env: envfx.Context{
					Environment: "production",
				},
				Scope:       tally.NoopScope,
				MTLSHandler: mTLSHandler,
			}).Mapper

			output, err := mapper.MapGlobalToLocal(test.input, &_testCluster)
			require.NoError(t, err)
			test.checkFunc(output)
		})
	}

}

// this function will create Spark Job with Preemptible value, if its nil then preemptible will not be set
func createSparkJob(preemptible *bool) *v2beta1pb.SparkJob {
	sparkJob := &v2beta1pb.SparkJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "platforms.uberai.michelangelo.ma_dev_test.pipelines.boston_housing.eval",
			Namespace: "test-namespace",
			Finalizers: []string{
				"michelangelo/Ingester",
			},
		},
		Spec: v2beta1pb.SparkJobSpec{
			User: &v2beta1pb.UserInfo{
				ProxyUser: "test-user",
			},
			MainApplicationFile: "local:///ml-code/bazel-bin/platforms/uberai/michelangelo/ma_dev_test/path_gen.runfiles/ml_code/data/michelangelo/project_runner.py",
			MainClass:           "platforms.uberai.michelangelo.ma_dev_test.pipelines.boston_housing.eval",
			MainArgs: []string{
				"run",
				"--project_dir", "platforms/uberai/michelangelo/ma_dev_test",
				"--runnable_name", "platforms.uberai.michelangelo.ma_dev_test.pipelines.boston_housing.eval",
				"--mode", "CLUSTER",
				"--workdir", "mmui-test-eval-app",
			},
			SparkConf: map[string]string{
				"spark.executor.memoryOverhead":     "1g",
				"spark.sql.files.maxPartitionBytes": "537395200b",
				"spark.sql.shuffle.partitions":      "16",
			},
			Deps: &v2beta1pb.Dependencies{
				Jars: []string{"local:///ml-code/bazel-bin/platforms/uberai/michelangelo/ma_dev_test/path_gen.runfiles/ma_jar_0_7_20220727_231929/jar/downloaded.jar"},
			},
			Executor: &v2beta1pb.ExecutorSpec{
				Pod: &v2beta1pb.PodSpec{
					Image: "localhost:15055/uber-usi/ml-code-boston-housing:bkt1-produ-1664402887-d8015",
					Env: []*v2beta1pb.Environment{
						{
							Name:  "PYSPARK_PYTHON",
							Value: "/ml-code/bazel-bin/data/michelangelo/examples/ma_workspace/python",
						},
					},
					Resource: &v2beta1pb.ResourceSpec{
						Cpu:    1,
						Memory: "2g",
					},
				},
				Instances: 7,
			},
			Driver: &v2beta1pb.DriverSpec{
				Pod: &v2beta1pb.PodSpec{
					Image: "localhost:15055/uber-usi/ml-code-boston-housing:bkt1-produ-1664402887-d8015",
					Env: []*v2beta1pb.Environment{
						{
							Name:  "PYSPARK_PYTHON",
							Value: "/ml-code/bazel-bin/data/michelangelo/examples/ma_workspace/python",
						},
					},
					Resource: &v2beta1pb.ResourceSpec{
						Cpu:    1,
						Memory: "1g",
					},
				},
			},
		},
		Status: v2beta1pb.SparkJobStatus{
			Assignment: &v2beta1pb.AssignmentInfo{
				Cluster:      "cluster",
				ResourcePool: "resource-pool",
			},
		},
	}
	if preemptible != nil {
		sparkJob.Spec.Scheduling = &v2beta1pb.SchedulingSpec{Preemptible: *preemptible}
	}
	return sparkJob
}

func getTestComputeAnnotations(preemptible string) map[string]string {
	annotationMap := map[string]string{
		computeconstants.ResourcePoolAnnotationKey: "resource-pool",
	}
	_, err := strconv.ParseBool(preemptible)
	if err == nil {
		annotationMap[computeconstants.PreemptibleAnnotationKey] = preemptible
	}
	return annotationMap
}

func TestMapRay(t *testing.T) {
	var tt = []struct {
		msg                string
		input              *v2beta1pb.RayJob
		enableMTLS         bool
		enableRuntimeClass bool
		checkFunc          func(cluster runtime.Object)
	}{
		{
			msg: "test head pod template",
			input: &v2beta1pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ray-job",
					Namespace: "test-namespace",
					Finalizers: []string{
						"michelangelo/Ingester",
					},
					Labels: map[string]string{
						constants.UOwnLabelKey:         "d6190288-07ae-445b-91e6-2cfd6894fd0c",
						constants.OwnerServiceLabelKey: constants.MAOwnerServiceLabelValue,
					},
				},
				Spec: v2beta1pb.RayJobSpec{
					User: &v2beta1pb.UserInfo{
						Name: "test-user",
					},
					Head: &v2beta1pb.HeadSpec{
						Pod: &v2beta1pb.PodSpec{
							Resource: &v2beta1pb.ResourceSpec{
								Cpu:    4,
								Gpu:    1,
								Memory: "4M",
							},
						},
					},
					Worker: &v2beta1pb.WorkerSpec{
						MinInstances: 2,
						MaxInstances: 5,
						Pod: &v2beta1pb.PodSpec{
							Resource: &v2beta1pb.ResourceSpec{
								Cpu:    1,
								Memory: "4M",
							},
							Image: "test-image",
						},
					},
				},
				Status: v2beta1pb.RayJobStatus{
					Assignment: &v2beta1pb.AssignmentInfo{
						Cluster:      "cluster",
						ResourcePool: "resource-pool",
					},
				},
			},
			checkFunc: func(obj runtime.Object) {
				cluster, ok := obj.(*kuberay.RayCluster)
				require.True(t, ok)

				// cluster namespace
				require.Equal(t, RayLocalNamespace, cluster.Namespace)

				// ray cluster labels
				require.Equal(t, map[string]string{
					constants.ProjectNameLabelKey:   "test-namespace",
					constants.JobControlPlaneEnvKey: "", // for test this string will be empty
					constants.UOwnLabelKey:          "d6190288-07ae-445b-91e6-2cfd6894fd0c",
					constants.OwnerServiceLabelKey:  constants.MAOwnerServiceLabelValue,
				}, cluster.Labels)

				require.Nil(t, cluster.Finalizers)

				headSpec := cluster.Spec.HeadGroupSpec
				headPodTemplate := headSpec.Template

				// head pod template namespace
				require.Equal(t, "", headPodTemplate.Namespace)

				// single head
				require.Equal(t, int32(1), *headSpec.Replicas)

				//gangLabelValue := strconv.FormatUint(uint64(hash.Fnv32a([]byte("test-namespace"+"test-ray-job"))), 10)

				// Labels
				labels := headPodTemplate.Labels
				require.Equal(t, map[string]string{
					constants.UserLabelKey:                 "test-user",
					constants.ProjectNameLabelKey:          "test-namespace",
					constants.JobNameLabelKey:              "test-ray-job",
					constants.UOwnLabelKey:                 "d6190288-07ae-445b-91e6-2cfd6894fd0c",
					constants.OwnerServiceLabelKey:         constants.MAOwnerServiceLabelValue,
					constants.JobControlPlaneEnvKey:        "", // for test this string will be empty
					constants.GenericSpireIdentityLabelKey: constants.GenericSpireIdentityLabelValue,
					//computeconstants.GangMemberLabelKey: gangLabelValue,
				}, labels)

				// Annotations
				annotations := headPodTemplate.Annotations
				require.Equal(t, map[string]string{
					"com.scheduler.port.RAY_PORT":                    "dynamic",
					"com.scheduler.port.RAY_CLIENT_PORT":             "dynamic",
					"com.scheduler.port.NODE_MANAGER_PORT":           "dynamic",
					"com.scheduler.port.OBJECT_MANAGER_PORT":         "dynamic",
					"com.scheduler.port.DASHBOARD_PORT":              "dynamic",
					"com.scheduler.port.DASHBOARD_AGENT_GRPC_PORT":   "dynamic",
					"com.scheduler.port.DASHBOARD_AGENT_LISTEN_PORT": "dynamic",
					"com.scheduler.port.METRICS_EXPORT_PORT":         "dynamic",
					"com.scheduler.port.JUPYTER_NOTEBOOK_PORT":       "dynamic",
					"uber.compute.resourcepool":                      "resource-pool",
					"uber.compute.preemptible":                       "true",
					constants.UserUIDAnnotationKey:                   _expectedUserID,
					constants.SpiffeAnnotationKey:                    _expectedSpiffeID,
					//computeconstants.GangMemberNumberAnnotationKey: "3",
				}, annotations)

				// Runtime class
				require.Equal(t, "nvidia", *headPodTemplate.Spec.RuntimeClassName)
			},
		},
		{
			msg:   "test head pod spec",
			input: createRayJob(ptr.Of(false), nil),
			checkFunc: func(obj runtime.Object) {
				cluster, ok := obj.(*kuberay.RayCluster)
				require.True(t, ok)

				headSpec := cluster.Spec.HeadGroupSpec
				headPodSpec := headSpec.Template.Spec

				// 1. Volumes
				volumes := headPodSpec.Volumes
				require.Equal(t, 5, len(volumes))

				secretVolume := volumes[0]
				require.Equal(t, "volume-ma-hadoop-test-ray-job", secretVolume.Name)
				require.Equal(t, "ma-hadoop-test-ray-job", secretVolume.VolumeSource.Secret.SecretName)

				objSpillVolume := volumes[1]
				require.Equal(t, "ray-obj-spill-vol", objSpillVolume.Name)
				require.Equal(t, "", string(objSpillVolume.VolumeSource.EmptyDir.Medium))

				// 2. No init containers
				require.Equal(t, 0, len(headPodSpec.InitContainers))

				// 3. Containers
				containers := headPodSpec.Containers
				require.Equal(t, 3, len(containers))

				container := containers[0]
				require.Equal(t, "ray-head", container.Name)
				require.Equal(t, "127.0.0.1:5055/test-image", container.Image)
				require.Equal(t, corev1.PullIfNotPresent, container.ImagePullPolicy)
				require.Equal(t, 15, len(container.Env))
				require.Equal(t, []string{"bash", "--norc", "-c",
					`var_tini=$(command -v tini 2> /dev/null); if [ -x "$var_tini" ]; then exec $var_tini -gwvv -- bash --norc -c "$0" "$@"; else exec bash --norc -c "$0" "$@"; fi`,
					`python -m data.michelangelo.ray_cluster.head_node_controller ray start --head --temp-dir=/mnt/mesos/sandbox/ray_log --num-cpus=4 --num-gpus=0 --memory=4000000 --port=$RAY_PORT --ray-client-server-port=$RAY_CLIENT_PORT --node-manager-port=$NODE_MANAGER_PORT --object-manager-port=$OBJECT_MANAGER_PORT --dashboard-port=$DASHBOARD_PORT --dashboard-agent-grpc-port=$DASHBOARD_AGENT_GRPC_PORT --dashboard-agent-listen-port=$DASHBOARD_AGENT_LISTEN_PORT --metrics-export-port=$METRICS_EXPORT_PORT --system-config="'{\"object_spilling_config\":\"{\\\"type\\\":\\\"filesystem\\\",\\\"params\\\":{\\\"directory_path\\\":\\\"/ray/obj_store_spill\\\"}}\"}'" --dashboard-host="0.0.0.0" --block`,
				}, container.Command)
				require.Nil(t, container.SecurityContext)

				resourecRequests := container.Resources.Requests
				resourceLimits := container.Resources.Limits
				require.Equal(t, resourceLimits[corev1.ResourceCPU], resourecRequests[corev1.ResourceCPU])

				// 4. scheduler name
				require.Equal(t, computeconstants.K8sResourceManagerSchedulerName, headPodSpec.SchedulerName)

				// EnbableServiceLink flag is disabled
				require.NotNil(t, headPodSpec.EnableServiceLinks)
				require.False(t, *headPodSpec.EnableServiceLinks)
			},
		},
		{
			msg: "test ptrace enabled",
			input: func() *v2beta1pb.RayJob {
				rayJob := createRayJob(ptr.Of(false), nil)
				rayJob.Annotations = map[string]string{
					PtraceEnabledAnnotation: "true",
				}
				return rayJob
			}(),
			checkFunc: func(obj runtime.Object) {
				cluster, ok := obj.(*kuberay.RayCluster)
				require.True(t, ok)

				assertSecurityContext := func(securityContext *corev1.SecurityContext) {
					require.NotNil(t, securityContext)

					capabilities := securityContext.Capabilities
					require.NotNil(t, capabilities)

					addedCapabilities := capabilities.Add
					require.Equal(t, 1, len(addedCapabilities))

					require.Equal(t, "SYS_PTRACE", string(addedCapabilities[0]))
				}

				headContainer := cluster.Spec.HeadGroupSpec.Template.Spec.Containers[0]
				require.Equal(t, "ray-head", headContainer.Name)
				assertSecurityContext(headContainer.SecurityContext)

				require.Greater(t, len(cluster.Spec.WorkerGroupSpecs), 0)
				for _, worker := range cluster.Spec.WorkerGroupSpecs {
					workerContainer := worker.Template.Spec.Containers[0]
					require.Equal(t, "ray-worker", workerContainer.Name)
					assertSecurityContext(workerContainer.SecurityContext)
				}
			},
		},
		{
			msg:   "test hetero head pod spec",
			input: createRayHeteroJob(ptr.Of(false), nil),
			checkFunc: func(obj runtime.Object) {
				cluster, ok := obj.(*kuberay.RayCluster)
				require.True(t, ok)

				headSpec := cluster.Spec.HeadGroupSpec
				headPodSpec := headSpec.Template.Spec

				// 1. Volumes
				volumes := headPodSpec.Volumes
				require.Equal(t, 5, len(volumes))

				secretVolume := volumes[0]
				require.Equal(t, "volume-ma-hadoop-test-ray-job", secretVolume.Name)
				require.Equal(t, "ma-hadoop-test-ray-job", secretVolume.VolumeSource.Secret.SecretName)

				objSpillVolume := volumes[1]
				require.Equal(t, "ray-obj-spill-vol", objSpillVolume.Name)
				require.Equal(t, "", string(objSpillVolume.VolumeSource.EmptyDir.Medium))

				// 2. No init containers
				require.Equal(t, 0, len(headPodSpec.InitContainers))

				// 3. Containers
				containers := headPodSpec.Containers
				require.Equal(t, 3, len(containers))

				container := containers[0]
				require.Equal(t, "ray-head", container.Name)
				require.Equal(t, "127.0.0.1:5055/test-image", container.Image)
				require.Equal(t, corev1.PullIfNotPresent, container.ImagePullPolicy)
				require.Equal(t, 15, len(container.Env))
				require.Equal(t, []string{"bash", "--norc", "-c",
					`var_tini=$(command -v tini 2> /dev/null); if [ -x "$var_tini" ]; then exec $var_tini -gwvv -- bash --norc -c "$0" "$@"; else exec bash --norc -c "$0" "$@"; fi`,
					`python -m data.michelangelo.ray_cluster.head_node_controller ray start --head --temp-dir=/mnt/mesos/sandbox/ray_log --num-cpus=4 --num-gpus=0 --memory=4000000 --port=$RAY_PORT --ray-client-server-port=$RAY_CLIENT_PORT --node-manager-port=$NODE_MANAGER_PORT --object-manager-port=$OBJECT_MANAGER_PORT --dashboard-port=$DASHBOARD_PORT --dashboard-agent-grpc-port=$DASHBOARD_AGENT_GRPC_PORT --dashboard-agent-listen-port=$DASHBOARD_AGENT_LISTEN_PORT --metrics-export-port=$METRICS_EXPORT_PORT --system-config="'{\"object_spilling_config\":\"{\\\"type\\\":\\\"filesystem\\\",\\\"params\\\":{\\\"directory_path\\\":\\\"/ray/obj_store_spill\\\"}}\"}'" --dashboard-host="0.0.0.0" --block --resources="'{\"HEAD_NODE_0\":1}'"`,
				}, container.Command)

				// 4. scheduler name
				require.Equal(t, computeconstants.K8sResourceManagerSchedulerName, headPodSpec.SchedulerName)

				// EnableServiceLink flag is disabled
				require.NotNil(t, headPodSpec.EnableServiceLinks)
				require.False(t, *headPodSpec.EnableServiceLinks)
			},
		},
		{
			msg: "test head pod spec command override",
			input: &v2beta1pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ray-job",
					Namespace: "test-namespace",
					Finalizers: []string{
						"michelangelo/Ingester",
					},
				},
				Spec: v2beta1pb.RayJobSpec{
					User: &v2beta1pb.UserInfo{
						Name: "test-user",
					},
					Head: &v2beta1pb.HeadSpec{
						Pod: &v2beta1pb.PodSpec{
							Resource: &v2beta1pb.ResourceSpec{
								Cpu:    4,
								Gpu:    1,
								Memory: "4M",
							},
							Command: []string{
								"ray start",
							},
						},
					},
					Worker: &v2beta1pb.WorkerSpec{
						MinInstances: 2,
						MaxInstances: 5,
						Pod: &v2beta1pb.PodSpec{
							Resource: &v2beta1pb.ResourceSpec{
								Cpu:    1,
								Memory: "4M",
							},
							Image: "test-image",
						},
					},
				},
				Status: v2beta1pb.RayJobStatus{
					Assignment: &v2beta1pb.AssignmentInfo{
						Cluster:      "cluster",
						ResourcePool: "resource-pool",
					},
				},
			},
			checkFunc: func(obj runtime.Object) {
				cluster, ok := obj.(*kuberay.RayCluster)
				require.True(t, ok)

				headSpec := cluster.Spec.HeadGroupSpec
				headPodSpec := headSpec.Template.Spec
				containers := headPodSpec.Containers
				require.Equal(t, 3, len(containers))

				container := containers[0]
				require.Equal(t, []string{
					"bash", "--norc", "-c",
					"var_tini=$(command -v tini 2> /dev/null); if [ -x \"$var_tini\" ]; then exec $var_tini -gwvv -- bash --norc -c \"$0\" \"$@\"; else exec bash --norc -c \"$0\" \"$@\"; fi",
					"ray start",
				}, container.Command)
			},
		},
		{
			msg: "test worker pod template",
			input: &v2beta1pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-ray-job",
					Labels: map[string]string{
						constants.UOwnLabelKey:         "d6190288-07ae-445b-91e6-2cfd6894fd0c",
						constants.OwnerServiceLabelKey: constants.MAOwnerServiceLabelValue,
					},
				},
				Spec: v2beta1pb.RayJobSpec{
					User: &v2beta1pb.UserInfo{
						Name: "test-user",
					},
					Head: &v2beta1pb.HeadSpec{
						Pod: &v2beta1pb.PodSpec{
							Resource: &v2beta1pb.ResourceSpec{
								Cpu:    4,
								Memory: "4M",
							},
							Image: "test-image",
						},
					},
					Worker: &v2beta1pb.WorkerSpec{
						MinInstances: 4,
						MaxInstances: 10,
						Pod: &v2beta1pb.PodSpec{
							Resource: &v2beta1pb.ResourceSpec{
								Cpu:    1,
								Gpu:    4,
								Memory: "4M",
							},
							Image: "test-image",
						},
					},
				},
				Status: v2beta1pb.RayJobStatus{
					Assignment: &v2beta1pb.AssignmentInfo{
						Cluster:      "cluster",
						ResourcePool: "resource-pool",
					},
				},
			},
			checkFunc: func(obj runtime.Object) {
				cluster, ok := obj.(*kuberay.RayCluster)
				require.True(t, ok)

				// cluster namespace
				require.Equal(t, RayLocalNamespace, cluster.Namespace)

				// ray cluster labels)
				require.Equal(t, map[string]string{
					constants.ProjectNameLabelKey:   "test-namespace",
					constants.JobControlPlaneEnvKey: "", // for test this string will be empty
					constants.UOwnLabelKey:          "d6190288-07ae-445b-91e6-2cfd6894fd0c",
					constants.OwnerServiceLabelKey:  constants.MAOwnerServiceLabelValue,
				}, cluster.Labels)

				workerGroupSpec := cluster.Spec.WorkerGroupSpecs
				require.Equal(t, 1, len(workerGroupSpec))

				workerPodTemplate := workerGroupSpec[0].Template

				// worker pod template namespace
				require.Equal(t, "", workerPodTemplate.Namespace)

				// instances
				require.Equal(t, int32(4), *workerGroupSpec[0].MinReplicas)
				require.Equal(t, int32(10), *workerGroupSpec[0].MaxReplicas)
				require.Equal(t, int32(10), *workerGroupSpec[0].Replicas)

				//gangLabelValue := strconv.FormatUint(uint64(hash.Fnv32a([]byte("test-namespace"+"test-ray-job"))), 10)

				// Labels
				labels := workerPodTemplate.Labels
				require.Equal(t, map[string]string{
					constants.UserLabelKey:                 "test-user",
					constants.ProjectNameLabelKey:          "test-namespace",
					constants.JobNameLabelKey:              "test-ray-job",
					constants.UOwnLabelKey:                 "d6190288-07ae-445b-91e6-2cfd6894fd0c",
					constants.OwnerServiceLabelKey:         constants.MAOwnerServiceLabelValue,
					constants.JobControlPlaneEnvKey:        "", // for test this string will be empty
					constants.GenericSpireIdentityLabelKey: constants.GenericSpireIdentityLabelValue,
					//computeconstants.GangMemberLabelKey: gangLabelValue,
				}, labels)

				// Annotations
				annotations := workerPodTemplate.Annotations
				require.Equal(t, map[string]string{
					"com.uber.secrets_volume_name":                 "usecret",
					"com.uber.scp.service.id":                      "michelangelo-ray-init",
					"org.apache.aurora.metadata.usecrets.enable":   "true",
					"org.apache.aurora.metadata.usecrets.regional": "true",
					// Ports
					"com.scheduler.port.OBJECT_MANAGER_PORT": "dynamic",
					"com.scheduler.port.RAY_PORT":            "dynamic",
					"com.scheduler.port.METRICS_EXPORT_PORT": "dynamic",
					"com.scheduler.port.W_0":                 "dynamic",
					"com.scheduler.port.W_1":                 "dynamic",
					"com.scheduler.port.W_2":                 "dynamic",
					"com.scheduler.port.W_3":                 "dynamic",
					"com.scheduler.port.W_4":                 "dynamic",
					"uber.compute.resourcepool":              "resource-pool",
					"uber.compute.preemptible":               "true",
					constants.UserUIDAnnotationKey:           _expectedUserID,
					constants.SpiffeAnnotationKey:            _expectedSpiffeID,
					//computeconstants.GangMemberNumberAnnotationKey: "5",
				}, annotations)

				// Runtime class
				require.Equal(t, "nvidia", *workerPodTemplate.Spec.RuntimeClassName)
			},
		},
		{
			msg: "test worker pod spec",
			input: &v2beta1pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-ray-job",
				},
				Spec: v2beta1pb.RayJobSpec{
					User: &v2beta1pb.UserInfo{
						Name: "test-user",
					},
					Head: &v2beta1pb.HeadSpec{
						Pod: &v2beta1pb.PodSpec{
							Name: "head-pod",
							Resource: &v2beta1pb.ResourceSpec{
								Cpu:    4,
								Memory: "4M",
							},
							Image: "127.0.0.1:5055/test-image",
						},
					},
					Worker: &v2beta1pb.WorkerSpec{
						Pod: &v2beta1pb.PodSpec{
							Resource: &v2beta1pb.ResourceSpec{
								Cpu:    1,
								Memory: "4M",
							},
							Image: "127.0.0.1:5055/test-image",
						},
					},
				},
				Status: v2beta1pb.RayJobStatus{
					Assignment: &v2beta1pb.AssignmentInfo{
						Cluster:      "cluster",
						ResourcePool: "resource-pool",
					},
				},
			},
			checkFunc: func(obj runtime.Object) {
				cluster, ok := obj.(*kuberay.RayCluster)
				require.True(t, ok)

				workerGroupSpec := cluster.Spec.WorkerGroupSpecs
				require.Equal(t, 1, len(workerGroupSpec))

				// Group name
				require.Equal(t, "worker-test-ray-job", workerGroupSpec[0].GroupName)

				workerPodSpec := workerGroupSpec[0].Template.Spec

				// Volumes
				volumes := workerPodSpec.Volumes
				require.Equal(t, 11, len(volumes))

				secretVolume := volumes[0]
				require.Equal(t, "volume-ma-hadoop-test-ray-job", secretVolume.Name)
				require.Equal(t, "ma-hadoop-test-ray-job", secretVolume.VolumeSource.Secret.SecretName)

				spillVolume := volumes[1]
				require.Equal(t, "ray-obj-spill-vol", spillVolume.Name)
				require.Equal(t, resource.MustParse("140Gi"), *spillVolume.VolumeSource.EmptyDir.SizeLimit)

				// Init containers
				require.Equal(t, 1, len(workerPodSpec.InitContainers))
				initC := workerPodSpec.InitContainers[0]

				require.Equal(t, "ray-init", initC.Name)
				require.Equal(t, constants.InitContainerImage, initC.Image)
				require.Equal(t, 6, len(initC.VolumeMounts))

				// Test init container's environment variables.
				checkInitContainerEnv(t, initC)

				// Containers
				containers := workerPodSpec.Containers
				require.Equal(t, 2, len(containers))

				container := containers[0]
				require.Equal(t, "ray-worker", container.Name)
				require.Equal(t, "127.0.0.1:5055/test-image", container.Image)
				require.Equal(t, corev1.PullIfNotPresent, container.ImagePullPolicy)
				require.Equal(t, []string{
					"bash",
					"--norc",
					"-c",
					`var_tini=$(command -v tini 2> /dev/null); if [ -x "$var_tini" ]; then exec $var_tini -gwvv -- bash --norc -c "$0" "$@"; else exec bash --norc -c "$0" "$@"; fi`,
					"export $(cat /data/head_info.env | tr -d ' ' | xargs -L 1) && echo ray start && python -m data.michelangelo.ray_cluster.worker_node_kubernetes --head_addr $RAY_IP --cpu_count 1 --gpu_count 0 --kubernetes_job_id test-ray-job",
				}, container.Command)
				require.Equal(t, 10, len(container.Env))

				// Scheduler name
				require.Equal(t, computeconstants.K8sResourceManagerSchedulerName, workerPodSpec.SchedulerName)

				// EnableServiceLink flag is disabled
				require.NotNil(t, workerPodSpec.EnableServiceLinks)
				require.False(t, *workerPodSpec.EnableServiceLinks)

				// Runtime class
				require.Nil(t, workerPodSpec.RuntimeClassName)
			},
		},
		{
			msg:   "test hetero worker pod spec",
			input: createRayHeteroJob(nil, ptr.Of("gpu-sku")),
			checkFunc: func(obj runtime.Object) {
				cluster, ok := obj.(*kuberay.RayCluster)
				require.True(t, ok)

				workerGroupSpec := cluster.Spec.WorkerGroupSpecs
				require.Equal(t, 2, len(workerGroupSpec))

				expectedNodeLabels := []string{"DATA_NODE", "TRAINER_NODE"}
				expectedGroupName := []string{"worker-data-node", "worker-trainer-node"}
				expectedObjectStoreMemory := []uint64{800000, 1600000}

				for idx, wks := range workerGroupSpec {
					// Group name
					require.Equal(t, expectedGroupName[idx], wks.GroupName)

					workerPodSpec := wks.Template.Spec

					// Volumes
					volumes := workerPodSpec.Volumes
					require.Equal(t, 11, len(volumes))

					secretVolume := volumes[0]
					require.Equal(t, "volume-ma-hadoop-test-ray-job", secretVolume.Name)
					require.Equal(t, "ma-hadoop-test-ray-job", secretVolume.VolumeSource.Secret.SecretName)

					spillVolume := volumes[1]
					require.Equal(t, "ray-obj-spill-vol", spillVolume.Name)
					require.Equal(t, resource.MustParse("140Gi"), *spillVolume.VolumeSource.EmptyDir.SizeLimit)

					// Init containers
					require.Equal(t, 1, len(workerPodSpec.InitContainers))
					initC := workerPodSpec.InitContainers[0]

					require.Equal(t, "ray-init", initC.Name)
					require.Equal(t, constants.InitContainerImage, initC.Image)
					require.Equal(t, 6, len(initC.VolumeMounts))

					// Test init container's environment variables.
					checkInitContainerEnv(t, initC)

					// Containers
					containers := workerPodSpec.Containers
					require.Equal(t, 2, len(containers))

					container := containers[0]
					require.Equal(t, "ray-worker", container.Name)
					require.Equal(t, "127.0.0.1:5055/test-image", container.Image)
					require.Equal(t, corev1.PullIfNotPresent, container.ImagePullPolicy)

					gpuCount := 0
					var runtimeClass *string
					// trainer node type
					if idx == 1 {
						gpuCount = 1
						runtimeClass = ptr.String(constants.GPURuntimeClassName)
					}

					require.Equal(t, []string{
						"bash",
						"--norc",
						"-c",
						`var_tini=$(command -v tini 2> /dev/null); if [ -x "$var_tini" ]; then exec $var_tini -gwvv -- bash --norc -c "$0" "$@"; else exec bash --norc -c "$0" "$@"; fi`,
						fmt.Sprintf("export $(cat /data/head_info.env | tr -d ' ' | xargs -L 1) && echo ray start && python -m data.michelangelo.ray_cluster.worker_node_kubernetes --head_addr $RAY_IP --cpu_count 1 --gpu_count %d --kubernetes_job_id test-ray-job --node_label %s --object_store_memory %d",
							gpuCount, expectedNodeLabels[idx], expectedObjectStoreMemory[idx]),
					}, container.Command)
					require.Equal(t, 10, len(container.Env))

					// Scheduler name
					require.Equal(t, computeconstants.K8sResourceManagerSchedulerName, workerPodSpec.SchedulerName)

					// EnableServiceLink flag is disabled
					require.NotNil(t, workerPodSpec.EnableServiceLinks)
					require.False(t, *workerPodSpec.EnableServiceLinks)

					// Runtime class
					require.Equal(t, runtimeClass, workerPodSpec.RuntimeClassName)
				}
			},
		},
		{
			msg: "test worker pod spec command override",
			input: &v2beta1pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ray-job",
					Namespace: "test-namespace",
					Finalizers: []string{
						"michelangelo/Ingester",
					},
				},
				Spec: v2beta1pb.RayJobSpec{
					User: &v2beta1pb.UserInfo{
						Name: "test-user",
					},
					Head: &v2beta1pb.HeadSpec{
						Pod: &v2beta1pb.PodSpec{
							Resource: &v2beta1pb.ResourceSpec{
								Cpu:    4,
								Gpu:    1,
								Memory: "4M",
							},
						},
					},
					Worker: &v2beta1pb.WorkerSpec{
						MinInstances: 2,
						MaxInstances: 5,
						Pod: &v2beta1pb.PodSpec{
							Resource: &v2beta1pb.ResourceSpec{
								Cpu:    1,
								Memory: "4M",
							},
							Image: "test-image",
							Command: []string{
								"ray start",
							},
						},
					},
				},
				Status: v2beta1pb.RayJobStatus{
					Assignment: &v2beta1pb.AssignmentInfo{
						Cluster:      "cluster",
						ResourcePool: "resource-pool",
					},
				},
			},
			checkFunc: func(obj runtime.Object) {
				cluster, ok := obj.(*kuberay.RayCluster)
				require.True(t, ok)

				workerSpec := cluster.Spec.WorkerGroupSpecs[0]
				workerPodSpec := workerSpec.Template.Spec
				containers := workerPodSpec.Containers
				require.Equal(t, 2, len(containers))

				container := containers[0]
				require.Equal(t, []string{
					"bash", "--norc", "-c",
					"var_tini=$(command -v tini 2> /dev/null); if [ -x \"$var_tini\" ]; then exec $var_tini -gwvv -- bash --norc -c \"$0\" \"$@\"; else exec bash --norc -c \"$0\" \"$@\"; fi",
					"ray start",
				}, container.Command)
			},
		},
		{
			msg: "test worker pod spec command override for heterogeneous cluster",
			input: &v2beta1pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ray-job",
					Namespace: "test-namespace",
					Finalizers: []string{
						"michelangelo/Ingester",
					},
				},
				Spec: v2beta1pb.RayJobSpec{
					User: &v2beta1pb.UserInfo{
						Name: "test-user",
					},
					Head: &v2beta1pb.HeadSpec{
						Pod: &v2beta1pb.PodSpec{
							Resource: &v2beta1pb.ResourceSpec{
								Cpu:    4,
								Gpu:    1,
								Memory: "4M",
							},
						},
					},
					Workers: []*v2beta1pb.WorkerSpec{
						{
							MinInstances: 2,
							MaxInstances: 5,
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    1,
									Memory: "4M",
								},
								Image: "test-image",
								Command: []string{
									"ray start",
									"data node",
								},
							},
							NodeType: "DATA_NODE",
						},
						{
							MinInstances: 2,
							MaxInstances: 5,
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    1,
									Memory: "4M",
								},
								Image: "test-image",
								Command: []string{
									"ray start",
									"trainer node",
								},
							},
							NodeType: "TRAINER_NODE",
						},
					},
				},
				Status: v2beta1pb.RayJobStatus{
					Assignment: &v2beta1pb.AssignmentInfo{
						Cluster:      "cluster",
						ResourcePool: "resource-pool",
					},
				},
			},
			checkFunc: func(obj runtime.Object) {
				cluster, ok := obj.(*kuberay.RayCluster)
				require.True(t, ok)

				require.Equal(t, 2, len(cluster.Spec.WorkerGroupSpecs))

				cmdTypes := []string{"data node", "trainer node"}
				for i, workerSpec := range cluster.Spec.WorkerGroupSpecs {
					workerPodSpec := workerSpec.Template.Spec
					containers := workerPodSpec.Containers
					require.Equal(t, 2, len(containers))

					container := containers[0]
					require.Equal(t, []string{
						"bash", "--norc", "-c",
						"var_tini=$(command -v tini 2> /dev/null); if [ -x \"$var_tini\" ]; then exec $var_tini -gwvv -- bash --norc -c \"$0\" \"$@\"; else exec bash --norc -c \"$0\" \"$@\"; fi",
						"ray start", cmdTypes[i]}, container.Command)
				}
			},
		},
		{
			msg:   "test head pod spec with sidecar container",
			input: createRayJob(nil, nil),
			checkFunc: func(obj runtime.Object) {
				cluster, ok := obj.(*kuberay.RayCluster)
				require.True(t, ok)
				require.Equal(t, "true", cluster.Spec.HeadGroupSpec.Template.Annotations[computeconstants.PreemptibleAnnotationKey])
				require.Equal(t, "true", cluster.Spec.WorkerGroupSpecs[0].Template.Annotations[computeconstants.PreemptibleAnnotationKey])
				assertHeadGroupSpec(cluster, t)
				assertWorkerGroupSpec(cluster, t)
			},
		},
		{
			msg:   "test preemptible true",
			input: createRayJob(ptr.Of(true), nil),
			checkFunc: func(obj runtime.Object) {
				cluster, ok := obj.(*kuberay.RayCluster)
				require.True(t, ok)
				require.Equal(t, "true", cluster.Spec.HeadGroupSpec.Template.Annotations[computeconstants.PreemptibleAnnotationKey])
				require.Equal(t, "true", cluster.Spec.WorkerGroupSpecs[0].Template.Annotations[computeconstants.PreemptibleAnnotationKey])
			},
		},
		{
			msg:   "test gpu sku selection related labels",
			input: createRayJob(ptr.Of(false), ptr.Of("gpu-sku")),
			checkFunc: func(obj runtime.Object) {
				cluster, ok := obj.(*kuberay.RayCluster)
				require.True(t, ok)
				require.Equal(t, "gpu-sku", cluster.Spec.HeadGroupSpec.Template.Labels[computeconstants.GPUNodeLabelMajor])
				require.Equal(t, "gpu-sku", cluster.Spec.WorkerGroupSpecs[0].Template.Labels[computeconstants.GPUNodeLabelMajor])

				// head node selector
				require.NotNil(t, cluster.Spec.HeadGroupSpec.Template.Spec.NodeSelector)
				nodeSelectorValue, ok := cluster.Spec.HeadGroupSpec.Template.Spec.NodeSelector[_gpuNodeSelectorKey]
				require.True(t, ok)
				require.Equal(t, "gpu-sku-full-name", nodeSelectorValue)

				// worker node selector
				require.Equal(t, len(cluster.Spec.WorkerGroupSpecs), 1)
				require.NotNil(t, cluster.Spec.WorkerGroupSpecs[0].Template.Spec.NodeSelector)
				nodeSelectorValue, ok = cluster.Spec.WorkerGroupSpecs[0].Template.Spec.NodeSelector[_gpuNodeSelectorKey]
				require.True(t, ok)
				require.Equal(t, "gpu-sku-full-name", nodeSelectorValue)
			},
		},
		{
			msg:   "test gpu sku selection related labels for hetero cluster",
			input: createRayHeteroJob(ptr.Of(false), ptr.Of("gpu-sku")),
			checkFunc: func(obj runtime.Object) {
				cluster, ok := obj.(*kuberay.RayCluster)
				require.True(t, ok)
				require.Equal(t, "gpu-sku", cluster.Spec.HeadGroupSpec.Template.Labels[computeconstants.GPUNodeLabelMajor])
				require.Equal(t, "gpu-sku", cluster.Spec.WorkerGroupSpecs[0].Template.Labels[computeconstants.GPUNodeLabelMajor])

				// head node selector
				require.NotNil(t, cluster.Spec.HeadGroupSpec.Template.Spec.NodeSelector)
				nodeSelectorValue, ok := cluster.Spec.HeadGroupSpec.Template.Spec.NodeSelector[_gpuNodeSelectorKey]
				require.True(t, ok)
				require.Equal(t, "gpu-sku-full-name", nodeSelectorValue)

				// worker node selector
				require.Equal(t, len(cluster.Spec.WorkerGroupSpecs), 2)
				// data node is index 0
				require.Nil(t, cluster.Spec.WorkerGroupSpecs[0].Template.Spec.NodeSelector)
				// trainer node is index 1
				require.NotNil(t, cluster.Spec.WorkerGroupSpecs[1].Template.Spec.NodeSelector)
				nodeSelectorValue, ok = cluster.Spec.WorkerGroupSpecs[1].Template.Spec.NodeSelector[_gpuNodeSelectorKey]
				require.True(t, ok)
				require.Equal(t, "gpu-sku-full-name", nodeSelectorValue)
			},
		},
		{
			msg:                "test MTLS enabled, Runtime Class enabled",
			input:              createRayJob(ptr.Of(false), nil),
			enableMTLS:         true,
			enableRuntimeClass: true,
			checkFunc: func(obj runtime.Object) {
				cluster, ok := obj.(*kuberay.RayCluster)
				require.True(t, ok)

				// Check cluster has MTLS label
				require.Equal(t, constants.SecureServiceMeshMTLSValue, cluster.Labels[constants.SecureServiceMeshKey])

				// Check runtime class is set
				require.NotNil(t, cluster.Spec.HeadGroupSpec.Template.Spec.RuntimeClassName)
				require.Equal(t, "runc-with-hooks", *cluster.Spec.HeadGroupSpec.Template.Spec.RuntimeClassName)

				// Basic assertions on the rest of the structure
				assertHeadGroupSpec(cluster, t)
				assertWorkerGroupSpec(cluster, t)
			},
		},
		{
			msg:                "test MTLS disabled, Runtime Class disabled",
			input:              createRayJob(ptr.Of(false), nil),
			enableMTLS:         false,
			enableRuntimeClass: false,
			checkFunc: func(obj runtime.Object) {
				cluster, ok := obj.(*kuberay.RayCluster)
				require.True(t, ok)

				// Check cluster doesn't have MTLS label
				_, hasLabel := cluster.Labels[constants.SecureServiceMeshKey]
				require.False(t, hasLabel)

				// Check runtime class is not set
				require.Nil(t, cluster.Spec.HeadGroupSpec.Template.Spec.RuntimeClassName)

				// Basic assertions on the rest of the structure
				assertHeadGroupSpec(cluster, t)
				assertWorkerGroupSpec(cluster, t)
			},
		},
		{
			msg:                "test enableMTLS error handling",
			input:              createRayJob(ptr.Of(false), nil),
			enableMTLS:         true,
			enableRuntimeClass: true,
			checkFunc: func(obj runtime.Object) {
				cluster, ok := obj.(*kuberay.RayCluster)
				require.True(t, ok)
				require.Equal(t, "", cluster.Labels[constants.SecureServiceMeshKey])
				require.Nil(t, cluster.Spec.HeadGroupSpec.Template.Spec.RuntimeClassName)

				for _, worker := range cluster.Spec.WorkerGroupSpecs {
					require.Nil(t, worker.Template.Spec.RuntimeClassName)
				}

				assertHeadGroupSpec(cluster, t)
				assertWorkerGroupSpec(cluster, t)
			},
		},
		{
			msg:                "test enableMTLSRuntimeClass error handling",
			input:              createRayJob(ptr.Of(false), nil),
			enableMTLS:         true,
			enableRuntimeClass: true,
			checkFunc: func(obj runtime.Object) {
				cluster, ok := obj.(*kuberay.RayCluster)
				require.True(t, ok)
				require.Equal(t, "", cluster.Labels[constants.SecureServiceMeshKey])
				require.Nil(t, cluster.Spec.HeadGroupSpec.Template.Spec.RuntimeClassName)

				for _, worker := range cluster.Spec.WorkerGroupSpecs {
					require.Nil(t, worker.Template.Spec.RuntimeClassName)
				}

				// Basic assertions on the rest of the structure
				assertHeadGroupSpec(cluster, t)
				assertWorkerGroupSpec(cluster, t)
			},
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			g := gomock.NewController(t)
			defer g.Finish()

			mockFliprConstraints := typesmock.NewMockFliprConstraintsBuilder(g)
			mockFlipr := fliprmock.NewMockFliprClient(g)
			mockSkuCache := skusmock.NewMockSkuConfigCache(g)

			// head behavior
			mockFliprConstraints.EXPECT().GetFliprConstraints(map[string]interface{}{
				_diskSpillGpuSkuKeyName: test.input.Spec.GetHead().GetPod().GetResource().GetGpuSku(),
			})
			mockFlipr.EXPECT().GetStringValue(contextmatcher.Any(), _diskSpillFliprName, gomock.Any(), "").
				Return("140Gi", nil)
			if test.input.Spec.GetHead().GetPod().GetResource().GetGpuSku() != "" {
				mockSkuCache.EXPECT().GetSkuName("gpu-sku", _testCluster.Name).Return("gpu-sku-full-name", nil)
			}

			// worker behavior
			if test.input.Spec.GetWorker() != nil {
				mockFliprConstraints.EXPECT().GetFliprConstraints(map[string]interface{}{
					_diskSpillGpuSkuKeyName: test.input.Spec.GetWorker().GetPod().GetResource().GetGpuSku(),
				})
				mockFlipr.EXPECT().GetStringValue(contextmatcher.Any(), _diskSpillFliprName, gomock.Any(), "").
					Return("140Gi", nil)
			}
			if test.input.Spec.GetWorker().GetPod().GetResource().GetGpuSku() != "" {
				mockSkuCache.EXPECT().GetSkuName("gpu-sku", _testCluster.Name).Return("gpu-sku-full-name", nil)
			}

			// hetero workers behavior
			for _, w := range test.input.Spec.GetWorkers() {
				mockFliprConstraints.EXPECT().GetFliprConstraints(map[string]interface{}{
					_diskSpillGpuSkuKeyName: w.GetPod().GetResource().GetGpuSku(),
				})
				mockFlipr.EXPECT().GetStringValue(contextmatcher.Any(), _diskSpillFliprName, gomock.Any(), "").
					Return("140Gi", nil)
				if w.GetPod().GetResource().GetGpuSku() != "" {
					mockSkuCache.EXPECT().GetSkuName("gpu-sku", _testCluster.Name).Return("gpu-sku-full-name", nil)
				}
			}

			var mTLSHandler types.MTLSHandler
			if test.msg == "test enableMTLS error handling" {
				mTLSHandler = testutils.CreateMockMTLSHandler(t, mockFlipr, mockFliprConstraints, false, false, fmt.Errorf("error with enableMTLS"), fmt.Errorf("error with enableMTLS"))
			} else if test.msg == "test enableMTLSRuntimeClass error handling" {
				mTLSHandler = testutils.CreateMockMTLSHandler(t, mockFlipr, mockFliprConstraints, false, false, fmt.Errorf("error with enableMTLS"), fmt.Errorf("error with enableMTLS"))
			} else {
				mTLSHandler = testutils.CreateMockMTLSHandler(t, mockFlipr, mockFliprConstraints, test.enableMTLS, test.enableRuntimeClass, nil, nil)
			}

			mapper := NewUkeMapper(MapperParams{
				Env:                     envfx.Context{Environment: "production"},
				SkuCache:                mockSkuCache,
				FliprClient:             mockFlipr,
				FliprConstraintsBuilder: mockFliprConstraints,
				Scope:                   tally.NoopScope,
				MTLSHandler:             mTLSHandler,
				SpiffeProvider:          MockSpiffeIDProvider{},
			}).Mapper

			output, err := mapper.MapGlobalToLocal(test.input, &_testCluster)
			require.NoError(t, err)
			test.checkFunc(output)
		})
	}
}

func assertWorkerGroupSpec(cluster *kuberay.RayCluster, t *testing.T) {
	workerGroupSpec := cluster.Spec.WorkerGroupSpecs
	require.Equal(t, 1, len(workerGroupSpec))
	workerPodSpec := workerGroupSpec[0].Template.Spec
	volumes := workerPodSpec.Volumes
	require.Equal(t, 11, len(volumes))
	containers := workerPodSpec.Containers
	require.Equal(t, 2, len(containers))
	for _, c := range containers {
		assertWorkerContainer(c, t, workerPodSpec)
	}
}

func assertWorkerContainer(c corev1.Container, t *testing.T, workerPodSpec corev1.PodSpec) {
	if c.Name == constants.WorkerContainerName {
		require.Equal(t, constants.WorkerContainerName, c.Name)
		require.Equal(t, "127.0.0.1:5055/test-image", c.Image)
		require.Equal(t, corev1.PullIfNotPresent, c.ImagePullPolicy)
		require.Equal(t, []string{
			"bash",
			"--norc",
			"-c",
			`var_tini=$(command -v tini 2> /dev/null); if [ -x "$var_tini" ]; then exec $var_tini -gwvv -- bash --norc -c "$0" "$@"; else exec bash --norc -c "$0" "$@"; fi`,
			"export $(cat /data/head_info.env | tr -d ' ' | xargs -L 1) && echo ray start && python -m data.michelangelo.ray_cluster.worker_node_kubernetes --head_addr $RAY_IP --cpu_count 1 --gpu_count 0 --kubernetes_job_id test-ray-job",
		}, c.Command)
		require.Equal(t, 10, len(c.Env))
		require.Equal(t, computeconstants.K8sResourceManagerSchedulerName, workerPodSpec.SchedulerName)
		//check env variable set
		require.Contains(t, c.Env, corev1.EnvVar{Name: _rayContainerIdentifierEnvKey, Value: "true"})
		//check volume got mounted
		volumes := c.VolumeMounts
		for _, v := range volumes {
			if v.Name == computeconstants.SandboxSharedVolumeName {
				require.Equal(t, v.MountPath, _rayContainerSandboxMountPath)
			}
		}
	} else {
		require.Equal(t, c.Name, computeconstants.KubeSandboxSidecarName) //check sidecar container added
	}
}

func assertHeadGroupSpec(cluster *kuberay.RayCluster, t *testing.T) {
	headSpec := cluster.Spec.HeadGroupSpec
	headPodSpec := headSpec.Template.Spec

	volumes := headPodSpec.Volumes
	require.Equal(t, 5, len(volumes))

	containers := headPodSpec.Containers
	require.Equal(t, 3, len(containers))
	for _, c := range containers {
		if c.Name == constants.HeadContainerName {
			assertHeadContainer(t, c, headPodSpec)
		}
	}
}

func assertHeadContainer(t *testing.T, c corev1.Container, headPodSpec corev1.PodSpec) {
	require.Equal(t, c.Name, constants.HeadContainerName)
	require.Equal(t, "127.0.0.1:5055/test-image", c.Image)
	require.Equal(t, corev1.PullIfNotPresent, c.ImagePullPolicy)

	require.Equal(t, _command, c.Command)
	env := c.Env
	require.Equal(t, 15, len(env))
	require.Equal(t, computeconstants.K8sResourceManagerSchedulerName, headPodSpec.SchedulerName)
	//check env variable set
	require.Contains(t, env, corev1.EnvVar{Name: _rayContainerIdentifierEnvKey, Value: "true"})
	volumes := c.VolumeMounts
	for _, v := range volumes {
		if v.Name == computeconstants.SandboxSharedVolumeName {
			require.Equal(t, v.MountPath, _rayContainerSandboxMountPath)
		}
	}
}

// this function will create Ray Job with Preemptible value, if its nil then preemptible will not be set
func createRayJob(preemptible *bool, gpuSku *string) *v2beta1pb.RayJob {
	rayJob := &v2beta1pb.RayJob{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-namespace",
			Name:      "test-ray-job",
		},
		Spec: v2beta1pb.RayJobSpec{
			User: &v2beta1pb.UserInfo{
				Name: "test-user",
			},
			Head: &v2beta1pb.HeadSpec{
				Pod: &v2beta1pb.PodSpec{
					Resource: &v2beta1pb.ResourceSpec{
						Cpu:    4,
						Memory: "4M",
					},
					Image: "test-image",
				},
			},
			Worker: &v2beta1pb.WorkerSpec{
				Pod: &v2beta1pb.PodSpec{
					Resource: &v2beta1pb.ResourceSpec{
						Cpu:    1,
						Memory: "4M",
					},
					Image: "test-image",
				},
			},
		},
		Status: v2beta1pb.RayJobStatus{
			Assignment: &v2beta1pb.AssignmentInfo{
				Cluster:      "cluster",
				ResourcePool: "resource-pool",
			},
		},
	}
	//we need to keep the below code to convert bool to validate cases where preemptible is not set
	// so if you pass any string other than true/false it will not set the preemptible.
	if preemptible != nil {
		rayJob.Spec.Scheduling = &v2beta1pb.SchedulingSpec{Preemptible: *preemptible}
	}

	if gpuSku != nil {
		rayJob.Spec.Head.Pod.Resource.GpuSku = *gpuSku
		rayJob.Spec.Head.Pod.Resource.Gpu = 1

		rayJob.Spec.Worker.Pod.Resource.GpuSku = *gpuSku
		rayJob.Spec.Worker.Pod.Resource.Gpu = 1
	}
	return rayJob
}

func createRayHeteroJob(preemptible *bool, gpuSku *string) *v2beta1pb.RayJob {
	job := createRayJob(preemptible, gpuSku)
	skuValue := ""
	if gpuSku != nil {
		skuValue = *gpuSku
	}

	job.Spec.Worker = nil
	job.Spec.Workers = []*v2beta1pb.WorkerSpec{
		{
			Pod: &v2beta1pb.PodSpec{
				Resource: &v2beta1pb.ResourceSpec{
					Cpu:    1,
					Memory: "4M",
				},
				Image: "test-image",
			},
			NodeType:               "DATA_NODE",
			ObjectStoreMemoryRatio: 0.2,
		},
		{
			Pod: &v2beta1pb.PodSpec{
				Resource: &v2beta1pb.ResourceSpec{
					Cpu:    1,
					Gpu:    1,
					GpuSku: skuValue,
					Memory: "4M",
				},
				Image: "test-image",
			},
			NodeType:               "TRAINER_NODE",
			ObjectStoreMemoryRatio: 0.4,
		},
	}

	return job
}

func checkInitContainerEnv(t *testing.T, initContainer corev1.Container) {
	require.Equal(t, 5, len(initContainer.Env))

	envMap := map[string]corev1.EnvVar{}
	for _, env := range initContainer.Env {
		envMap[env.Name] = env
	}
	checkEnv(t, envMap, "SECRETS_PATH", corev1.EnvVar{Name: "SECRETS_PATH", Value: "/usecret/current/michelangelo-ray-init/"})
	checkEnv(t, envMap, "UDEPLOY_APP_ID", corev1.EnvVar{Name: "UDEPLOY_APP_ID", Value: "michelangelo-ray-init"})
	checkEnv(t, envMap, "KUBERNETES_SERVICE_HOST", corev1.EnvVar{Name: "KUBERNETES_SERVICE_HOST", Value: "test-k8-api-server"})
	checkEnv(t, envMap, "KUBERNETES_SERVICE_PORT", corev1.EnvVar{Name: "KUBERNETES_SERVICE_PORT", Value: "6443"})
	checkEnv(t, envMap, "RAY_HEAD_NODE", corev1.EnvVar{Name: "RAY_HEAD_NODE", Value: "head-test-ray-job"})

}

func checkEnv(t *testing.T, envMap map[string]corev1.EnvVar, expectedKey string, expectedValue corev1.EnvVar) {
	actualValue, ok := envMap[expectedKey]
	require.True(t, ok, "Key %s not present in env", expectedKey)
	require.Equal(t, expectedValue, actualValue, "Mismatch in values in env for key %s, expected value %s, actual value %s", expectedKey, expectedValue, actualValue)
}

func TestGetRayLogPath(t *testing.T) {
	m := Mapper{}
	require.Equal(t, "/mnt/mesos/sandbox/ray_log", m.getRayLogPath())
}

func TestGetDiskSpillVolumeSize(t *testing.T) {
	tt := []struct {
		msg               string
		gpuSku            string
		mockSetup         func(g *gomock.Controller, spillValue string) Mapper
		wantError         string
		wantResult        resource.Quantity
		spillValue        string
		wantFailedMetrics bool
	}{
		{
			msg: "no gpu sku",
			mockSetup: func(g *gomock.Controller, spillValue string) Mapper {
				mockFliprConstraints := typesmock.NewMockFliprConstraintsBuilder(g)
				mockFliprConstraints.EXPECT().GetFliprConstraints(map[string]interface{}{
					"gpu_sku": "",
				})
				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFlipr.EXPECT().GetStringValue(contextmatcher.Any(), _diskSpillFliprName, gomock.Any(), "").
					Return(spillValue, nil)

				return Mapper{
					fliprConstraintsBuilder: mockFliprConstraints,
					fliprClient:             mockFlipr,
				}
			},
			spillValue: "140Gi",
			wantResult: resource.MustParse("140Gi"),
		},
		{
			msg:    "A100 gpu sku has higher spill limit",
			gpuSku: "A100",
			mockSetup: func(g *gomock.Controller, spillValue string) Mapper {
				mockFliprConstraints := typesmock.NewMockFliprConstraintsBuilder(g)
				mockFliprConstraints.EXPECT().GetFliprConstraints(map[string]interface{}{
					"gpu_sku": "a100",
				})
				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFlipr.EXPECT().GetStringValue(contextmatcher.Any(), _diskSpillFliprName, gomock.Any(), "").
					Return(spillValue, nil)

				return Mapper{
					fliprConstraintsBuilder: mockFliprConstraints,
					fliprClient:             mockFlipr,
				}
			},
			spillValue: "512Gi",
			wantResult: resource.MustParse("512Gi"),
		},
		{
			msg: "flipr returns error",
			mockSetup: func(g *gomock.Controller, spillValue string) Mapper {
				mockFliprConstraints := typesmock.NewMockFliprConstraintsBuilder(g)
				mockFliprConstraints.EXPECT().GetFliprConstraints(map[string]interface{}{
					"gpu_sku": "",
				})
				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFlipr.EXPECT().GetStringValue(contextmatcher.Any(), _diskSpillFliprName, gomock.Any(), "").
					Return(spillValue, assert.AnError)

				return Mapper{
					fliprConstraintsBuilder: mockFliprConstraints,
					fliprClient:             mockFlipr,
				}
			},
			wantError: "assert.AnError",
		},
		{
			msg: "invalid quantity in the flipr - check metrics",
			mockSetup: func(g *gomock.Controller, spillValue string) Mapper {
				mockFliprConstraints := typesmock.NewMockFliprConstraintsBuilder(g)
				mockFliprConstraints.EXPECT().GetFliprConstraints(map[string]interface{}{
					"gpu_sku": "",
				})
				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFlipr.EXPECT().GetStringValue(contextmatcher.Any(), _diskSpillFliprName, gomock.Any(), "").
					Return(spillValue, nil)

				return Mapper{
					fliprConstraintsBuilder: mockFliprConstraints,
					fliprClient:             mockFlipr,
					metrics: metrics.NewControllerMetrics(
						tally.NewTestScope("test", map[string]string{}),
						_mapperName),
				}
			},
			spillValue:        "140g",
			wantFailedMetrics: true,
			wantError:         "invalid value for gpu sku",
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			g := gomock.NewController(t)
			defer g.Finish()

			mockFliprConstraints := typesmock.NewMockFliprConstraintsBuilder(g)
			mockFlipr := fliprmock.NewMockFliprClient(g)
			mTLSHandler := testutils.CreateMockMTLSHandler(t, mockFlipr, mockFliprConstraints, false, false, nil, nil)
			mapper := test.mockSetup(g, test.spillValue)
			mapper.mTLSHandler = mTLSHandler

			result, err := mapper.getDiskSpillVolumeSize(test.gpuSku)
			if test.wantError != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, test.wantError)
			} else {
				require.Equal(t, test.wantResult, result)
			}

			if test.wantFailedMetrics {
				testScope := mapper.metrics.MetricsScope.(tally.TestScope)
				require.Equal(t, 1, len(testScope.Snapshot().Counters()))

				expectedMetrics := map[string]int64{
					"test.ukeMapper.invalid_flipr_disk_spill_value+controller=ukeMapper,ray_disk_spill_value=140g": int64(1),
				}
				expectedMetricTag := map[string]string{
					constants.ControllerTag: _mapperName,
					_diskSpillMetricKeyName: test.spillValue,
				}
				for k, v := range testScope.Snapshot().Counters() {
					val, ok := expectedMetrics[k]
					require.True(t, ok)
					require.Equal(t, fmt.Sprintf("%s.%s.%s", "test", _mapperName, _badFliprValueMetricName), v.Name())
					require.Equal(t, val, v.Value())

					require.Equal(t, expectedMetricTag, v.Tags())
				}
			}
		})
	}
}

func TestGetSandboxVolume(t *testing.T) {
	tt := []struct {
		podTemplate   corev1.PodTemplateSpec
		volumeName    string
		expectedError bool
		expectedSize  resource.Quantity
		msg           string
	}{
		{
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "vol1",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{
									SizeLimit: resource.NewScaledQuantity(5, 9),
								},
							},
						},
						{
							Name: "vol2",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{
									SizeLimit: resource.NewScaledQuantity(6, 9),
								},
							},
						},
					},
				},
			},
			volumeName:   "vol1",
			expectedSize: *resource.NewScaledQuantity(5, 9),
			msg:          "vol1 found",
		},
		{
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "vol3",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{
									SizeLimit: resource.NewScaledQuantity(5, 9),
								},
							},
						},
						{
							Name: "vol2",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{
									SizeLimit: resource.NewScaledQuantity(6, 9),
								},
							},
						},
					},
				},
			},
			volumeName:    "vol1",
			expectedError: true,
			msg:           "vol1 not found",
		},
		{
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{},
			},
			volumeName:    "vol1",
			expectedError: true,
			msg:           "no volumes present",
		},
	}

	for _, test := range tt {
		m := Mapper{}
		v, err := m.getVolume(test.podTemplate, test.volumeName)
		if test.expectedError {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.NotNil(t, v.EmptyDir)
			require.NotNil(t, v.EmptyDir.SizeLimit)
			require.True(t, test.expectedSize.Cmp(*v.EmptyDir.SizeLimit) == 0)
		}
	}
}

func TestAdjustRequirementsForVolumes(t *testing.T) {
	tt := []struct {
		msg              string
		podTemplate      corev1.PodTemplateSpec
		expectedDiskSize string
	}{
		{
			msg:              "head pod container size raised",
			podTemplate:      createPodTemplateSpecForTestAdjustRequirementsForVolumes(constants.HeadContainerName, "160Gi"),
			expectedDiskSize: "240Gi",
		},
		{
			msg:              "head pod container size stays the same at limit",
			podTemplate:      createPodTemplateSpecForTestAdjustRequirementsForVolumes(constants.HeadContainerName, "240Gi"),
			expectedDiskSize: "240Gi",
		},
		{
			msg:              "head pod container size stays the same higher than limit",
			podTemplate:      createPodTemplateSpecForTestAdjustRequirementsForVolumes(constants.HeadContainerName, "250Gi"),
			expectedDiskSize: "250Gi",
		},
		{
			msg:              "worker pod container size raise",
			podTemplate:      createPodTemplateSpecForTestAdjustRequirementsForVolumes(constants.WorkerContainerName, "160Gi"),
			expectedDiskSize: "240Gi",
		},
		{
			msg:              "worker pod container size stays the same at limit",
			podTemplate:      createPodTemplateSpecForTestAdjustRequirementsForVolumes(constants.WorkerContainerName, "240Gi"),
			expectedDiskSize: "240Gi",
		},
		{
			msg:              "worker pod container size stays the same higher than limit",
			podTemplate:      createPodTemplateSpecForTestAdjustRequirementsForVolumes(constants.WorkerContainerName, "250Gi"),
			expectedDiskSize: "250Gi",
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			spillMountSize := resource.MustParse("140Gi")
			m := Mapper{}
			sandboxVolSize := resource.MustParse(computeconstants.SandboxSharedVolumeSizeLimit)
			adjustedTemplate := m.adjustRequirementsForVolumes(test.podTemplate,
				corev1.Volume{
					Name: _objectSpillVolumeMount,
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							Medium:    corev1.StorageMediumDefault,
							SizeLimit: &spillMountSize,
						},
					},
				},
				corev1.Volume{
					Name: computeconstants.SandboxSharedVolumeName,
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							SizeLimit: &sandboxVolSize,
						},
					},
				})

			// check other fields have not changed
			require.Equal(t, "template1", adjustedTemplate.Name)
			require.Equal(t, 2, len(adjustedTemplate.Spec.Volumes))
			require.Equal(t, "volume1", adjustedTemplate.Spec.Volumes[0].Name)
			require.Equal(t, "volume2", adjustedTemplate.Spec.Volumes[1].Name)
			require.True(t, adjustedTemplate.Spec.HostNetwork)
			require.Equal(t, corev1.RestartPolicyNever, adjustedTemplate.Spec.RestartPolicy)
			require.Equal(t, 2, len(adjustedTemplate.Spec.Containers))
			require.Equal(t, test.podTemplate.Spec.Containers[0].Name, adjustedTemplate.Spec.Containers[0].Name)
			require.Equal(t, "shared-data", adjustedTemplate.Spec.Containers[1].Name)

			expectedDiskQuantity := resource.MustParse(test.expectedDiskSize)

			// check adjusted Ray container requests and limits
			require.Equal(t, expectedDiskQuantity, adjustedTemplate.Spec.Containers[0].Resources.Requests[corev1.ResourceEphemeralStorage])
			require.Equal(t, expectedDiskQuantity, adjustedTemplate.Spec.Containers[0].Resources.Limits[corev1.ResourceEphemeralStorage])

			// other container requests and limits should stay the same
			otherContainerDiskQuantity := resource.MustParse("50Gi")
			require.Equal(t, otherContainerDiskQuantity, adjustedTemplate.Spec.Containers[1].Resources.Requests[corev1.ResourceEphemeralStorage])
			require.Equal(t, otherContainerDiskQuantity, adjustedTemplate.Spec.Containers[1].Resources.Limits[corev1.ResourceEphemeralStorage])
		})
	}
}

func createPodTemplateSpecForTestAdjustRequirementsForVolumes(containerName, diskSize string) corev1.PodTemplateSpec {
	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name: "template1",
		},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{
					Name: "volume1",
				},
				{
					Name: "volume2",
				},
			},
			Containers: []corev1.Container{
				{
					Name: containerName,
					Resources: corev1.ResourceRequirements{
						Requests: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceEphemeralStorage: resource.MustParse(diskSize),
						},
						Limits: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceEphemeralStorage: resource.MustParse(diskSize),
						},
					},
				},
				{
					Name: "shared-data",
					Resources: corev1.ResourceRequirements{
						Requests: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceEphemeralStorage: resource.MustParse("50Gi"),
						},
						Limits: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceEphemeralStorage: resource.MustParse("50Gi"),
						},
					},
				},
			},
			HostNetwork:   true,
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}
}

func TestGetRuntimeClass(t *testing.T) {
	tests := []struct {
		name       string
		enableMTLS bool
		resources  *v2beta1pb.ResourceSpec
		expected   *string
	}{
		{
			name:       "MTLS enabled with GPU",
			enableMTLS: true,
			resources:  &v2beta1pb.ResourceSpec{Gpu: 1},
			expected:   ptr.String(constants.MTLSGPURuntimeClassName),
		},
		{
			name:       "MTLS enabled without GPU",
			enableMTLS: true,
			resources:  &v2beta1pb.ResourceSpec{Gpu: 0},
			expected:   ptr.String(constants.MTLSRuntimeClassName),
		},
		{
			name:       "MTLS disabled with GPU",
			enableMTLS: false,
			resources:  &v2beta1pb.ResourceSpec{Gpu: 1},
			expected:   ptr.String(constants.GPURuntimeClassName),
		},
		{
			name:       "MTLS disabled without GPU",
			enableMTLS: false,
			resources:  &v2beta1pb.ResourceSpec{Gpu: 0},
			expected:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper := Mapper{}
			result := mapper.getRuntimeClass(tt.enableMTLS, tt.resources)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestGetNodeSelectorFromResource(t *testing.T) {
	tests := []struct {
		name        string
		resources   *v2beta1pb.ResourceSpec
		setupMock   func(g *gomock.Controller) *Mapper
		expected    map[string]string
		expectError bool
	}{
		{
			name:      "GPU SKU present, should call getNodeSelector",
			resources: &v2beta1pb.ResourceSpec{GpuSku: "gpu-sku"},
			setupMock: func(g *gomock.Controller) *Mapper {
				mockSkuCache := skusmock.NewMockSkuConfigCache(g)
				mockSkuCache.EXPECT().
					GetSkuName("gpu-sku", gomock.Any()).
					Return("gpu-sku-full-name", nil).
					AnyTimes()

				return &Mapper{
					skuCache: mockSkuCache,
				}
			},
			expected: map[string]string{
				_gpuNodeSelectorKey: "gpu-sku-full-name",
			},
			expectError: false,
		},
		{
			name:      "Error in getNodeSelector",
			resources: &v2beta1pb.ResourceSpec{GpuSku: "gpu-sku"},
			setupMock: func(g *gomock.Controller) *Mapper {
				mockSkuCache := skusmock.NewMockSkuConfigCache(g)
				mockSkuCache.EXPECT().
					GetSkuName("gpu-sku", gomock.Any()).
					Return("", errors.New("SKU lookup error")).
					AnyTimes()

				return &Mapper{
					skuCache: mockSkuCache,
				}
			},
			expected:    nil,
			expectError: true,
		},
		{
			name:      "No GPU SKU, should return nil selector",
			resources: &v2beta1pb.ResourceSpec{},
			setupMock: func(g *gomock.Controller) *Mapper {
				return &Mapper{}
			},
			expected:    nil,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomock.NewController(t)
			defer g.Finish()

			mapper := tt.setupMock(g)

			nodeSelector, err := mapper.getNodeSelectorFromResource(tt.resources, &_testCluster)

			if tt.expectError {
				require.Error(t, err)
				require.Nil(t, nodeSelector)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected, nodeSelector)
			}
		})
	}
}

type MockSpiffeIDProvider struct {
	mockLookupFunc func(ldap string) string
}

func (m MockSpiffeIDProvider) GetSpiffeID(ldap string) string {
	if m.mockLookupFunc != nil {
		return m.mockLookupFunc(ldap)
	}
	return _expectedSpiffeID
}

func (m MockSpiffeIDProvider) GetUserID(ldap string) string {
	if m.mockLookupFunc != nil {
		return m.mockLookupFunc(ldap)
	}
	return _expectedUserID
}
