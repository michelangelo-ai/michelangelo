package k8sengine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	sparkv1beta2 "github.com/michelangelo-ai/michelangelo/go/thirdparty/k8s-crds/apis/sparkoperator.k8s.io/v1beta2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// sparkMapper is the Mapper the Spark tests map with: log persistence off, no
// Kueue queue mapping configured, so a resolved queue name is purely derived
// from the project identity.
func sparkMapper() Mapper {
	return Mapper{
		Scheduler: maconfig.SchedulerConfig{},
		Env:       env.Context{RuntimeEnvironment: "staging"},
	}
}

// sparkControlPlaneLabels are the labels every mapped Spark object must carry
// for namespace "proj1" under RuntimeEnvironment "staging". The owner differs
// from Ray's: it is what keeps the two federated watches from crossing over.
var sparkControlPlaneLabels = map[string]string{
	"ma/owner-service":     "michelangelo-spark",
	"ma/control-plane-env": "staging",
	"ma/project-name":      "proj1",
}

func kueueCluster() *v2pb.Cluster {
	return &v2pb.Cluster{Spec: v2pb.ClusterSpec{SchedulerType: v2pb.SCHEDULER_TYPE_KUEUE}}
}

// newSparkJob is a fully populated SparkJob in namespace "proj1".
func newSparkJob() *v2pb.SparkJob {
	return &v2pb.SparkJob{
		ObjectMeta: metav1.ObjectMeta{Name: "spark-1", Namespace: "proj1"},
		Spec: v2pb.SparkJobSpec{
			SparkVersion:        "3.5.0",
			MainClass:           "com.uber.Main",
			MainApplicationFile: "local:///opt/app.jar",
			MainArgs:            []string{"--flag", "value"},
			SparkConf:           map[string]string{"spark.executor.instances": "2"},
			Driver: &v2pb.DriverSpec{
				Pod: &v2pb.PodSpec{
					Image:              "spark:3.5.0",
					ImagePullingPolicy: "IfNotPresent",
					Resource:           &v2pb.ResourceSpec{Cpu: 1, Memory: "2g"},
					Env:                []*v2pb.Environment{{Name: "DRIVER_ENV", Value: "d"}},
				},
			},
			Executor: &v2pb.ExecutorSpec{
				Instances: 2,
				Pod: &v2pb.PodSpec{
					Resource: &v2pb.ResourceSpec{Cpu: 2, Memory: "4g", Gpu: 1, GpuSku: "nvidia-a100"},
					Env:      []*v2pb.Environment{{Name: "EXECUTOR_ENV", Value: "e"}},
				},
			},
			Deps: &v2pb.Dependencies{
				Jars:    []string{"local:///opt/dep.jar"},
				Files:   []string{"local:///opt/conf.yaml"},
				PyFiles: []string{"local:///opt/lib.py"},
			},
		},
	}
}

// mapSpark maps job and asserts the result is a SparkApplication.
func mapSpark(t *testing.T, m Mapper, job *v2pb.SparkJob, cluster *v2pb.Cluster) *sparkv1beta2.SparkApplication {
	t.Helper()
	obj, err := m.MapGlobalJobToLocal(job, nil, cluster)
	require.NoError(t, err)
	local, ok := obj.(*sparkv1beta2.SparkApplication)
	require.True(t, ok, "expected *sparkv1beta2.SparkApplication, got %T", obj)
	return local
}

func TestMapper_MapSparkJobToLocal(t *testing.T) {
	local := mapSpark(t, sparkMapper(), newSparkJob(), &v2pb.Cluster{})

	// The compute cluster's API server dispatches on TypeMeta, and every Spark
	// job lands in one namespace regardless of the CR's own.
	assert.Equal(t, SparkApplicationKind, local.Kind)
	assert.Equal(t, SparkAPIVersion, local.APIVersion)
	assert.Equal(t, SparkLocalNamespace, local.Namespace)
	assert.Equal(t, "spark-1", local.Name)

	assert.Equal(t, sparkv1beta2.JavaApplicationType, local.Spec.Type)
	assert.Equal(t, sparkv1beta2.ClusterMode, local.Spec.Mode)
	assert.Equal(t, "3.5.0", local.Spec.SparkVersion)
	assert.Equal(t, []string{"--flag", "value"}, local.Spec.Arguments)
	assert.Equal(t, map[string]string{"spark.executor.instances": "2"}, local.Spec.SparkConf)

	require.NotNil(t, local.Spec.MainApplicationFile)
	assert.Equal(t, "local:///opt/app.jar", *local.Spec.MainApplicationFile)
	require.NotNil(t, local.Spec.MainClass)
	assert.Equal(t, "com.uber.Main", *local.Spec.MainClass)

	// The application-level image comes off the driver pod; the standalone path
	// does the same. The operator applies it to executors too unless they set
	// their own, which TestMapSparkJob_ExecutorImage covers. This fixture's
	// executor names no image, so it inherits this one.
	assert.Nil(t, local.Spec.Executor.Image)
	require.NotNil(t, local.Spec.Image)
	assert.Equal(t, "spark:3.5.0", *local.Spec.Image)
	require.NotNil(t, local.Spec.ImagePullPolicy)
	assert.Equal(t, "IfNotPresent", *local.Spec.ImagePullPolicy)

	assert.Equal(t, []string{"local:///opt/dep.jar"}, local.Spec.Deps.Jars)
	assert.Equal(t, []string{"local:///opt/conf.yaml"}, local.Spec.Deps.Files)
	assert.Equal(t, []string{"local:///opt/lib.py"}, local.Spec.Deps.PyFiles)

	require.NotNil(t, local.Spec.Driver.Cores)
	assert.EqualValues(t, 1, *local.Spec.Driver.Cores)
	require.NotNil(t, local.Spec.Driver.Memory)
	assert.Equal(t, "2g", *local.Spec.Driver.Memory)
	// Only the driver needs to create and watch its own executors.
	require.NotNil(t, local.Spec.Driver.ServiceAccount)
	assert.Equal(t, sparkDriverServiceAccount, *local.Spec.Driver.ServiceAccount)
	assert.Nil(t, local.Spec.Executor.ServiceAccount)

	require.NotNil(t, local.Spec.Executor.Instances)
	assert.EqualValues(t, 2, *local.Spec.Executor.Instances)
	// The fixture's SKU is "nvidia-a100", which must not leak into the resource
	// name: TestMapSparkJob_GPU covers the rest of the matrix.
	require.NotNil(t, local.Spec.Executor.GPU)
	assert.Equal(t, "nvidia.com/gpu", local.Spec.Executor.GPU.Name)
	assert.EqualValues(t, 1, local.Spec.Executor.GPU.Quantity)

	// The executor's own sizing, not just the driver's.
	require.NotNil(t, local.Spec.Executor.Cores)
	assert.EqualValues(t, 2, *local.Spec.Executor.Cores)
	require.NotNil(t, local.Spec.Executor.Memory)
	assert.Equal(t, "4g", *local.Spec.Executor.Memory)

	require.Len(t, local.Spec.Driver.Env, 1)
	assert.Equal(t, corev1.EnvVar{Name: "DRIVER_ENV", Value: "d"}, local.Spec.Driver.Env[0])
	require.Len(t, local.Spec.Executor.Env, 1)
	assert.Equal(t, corev1.EnvVar{Name: "EXECUTOR_ENV", Value: "e"}, local.Spec.Executor.Env[0])
}

// SparkApplicationSpec.Type is required by the CRD and v2pb.SparkJobSpec has no
// equivalent, so the main file's extension is the only signal available.
func TestSparkApplicationType(t *testing.T) {
	tests := []struct {
		mainApplicationFile string
		expect              sparkv1beta2.SparkApplicationType
	}{
		{"local:///opt/app.py", sparkv1beta2.PythonApplicationType},
		{"s3a://bucket/job.R", sparkv1beta2.RApplicationType},
		{"s3a://bucket/job.r", sparkv1beta2.RApplicationType},
		{"local:///opt/app.jar", sparkv1beta2.JavaApplicationType},
		{"", sparkv1beta2.JavaApplicationType},
	}
	for _, tt := range tests {
		t.Run(tt.mainApplicationFile, func(t *testing.T) {
			assert.Equal(t, tt.expect, sparkApplicationType(tt.mainApplicationFile))
		})
	}
}

func TestMapSparkJobLabels(t *testing.T) {
	m := sparkMapper()

	// Every label here is attacker-controlled: the source CR's labels come from
	// the job author. None of them may reach the compute cluster verbatim. The
	// job has no UID, so no ma/job-uid label may appear either: the one on the
	// source CR is a forgery, and the tests below that expect exactly the
	// control-plane labels fail if it is copied.
	job := newSparkJob()
	job.Labels = map[string]string{
		"ma/owner-service":          "not-michelangelo",
		"ma/control-plane-env":      "production",
		"kueue.x-k8s.io/queue-name": "user-forged-queue",
		"ma/job-uid":                "forged-uid",
	}

	t.Run("non-kueue cluster: no queue label anywhere", func(t *testing.T) {
		local := mapSpark(t, m, job, &v2pb.Cluster{})
		assert.Equal(t, sparkControlPlaneLabels, local.Labels)
		assert.Equal(t, sparkControlPlaneLabels, local.Spec.Driver.Labels)
		assert.Equal(t, sparkControlPlaneLabels, local.Spec.Executor.Labels)
	})

	t.Run("nil cluster: no queue label anywhere", func(t *testing.T) {
		local := mapSpark(t, m, job, nil)
		assert.Equal(t, sparkControlPlaneLabels, local.Labels)
		assert.NotContains(t, local.Spec.Driver.Labels, "kueue.x-k8s.io/queue-name")
	})

	// Kueue's pod integration would treat labelled pods as separately
	// admissible workloads, so the queue label stays on the SparkApplication.
	t.Run("kueue cluster: queue label on the object, not on the pods", func(t *testing.T) {
		local := mapSpark(t, m, job, kueueCluster())

		assert.Equal(t, "ma-proj1", local.Labels["kueue.x-k8s.io/queue-name"],
			"a forged queue label must not redirect admission")
		assert.NotContains(t, local.Spec.Driver.Labels, "kueue.x-k8s.io/queue-name")
		assert.NotContains(t, local.Spec.Executor.Labels, "kueue.x-k8s.io/queue-name")
	})

	// The control-plane labels go on the pod specs too, so the driver and
	// executor pods carry them whatever the operator copies from the
	// application.
	t.Run("driver and executor carry the control-plane labels", func(t *testing.T) {
		local := mapSpark(t, m, job, kueueCluster())
		assert.Equal(t, sparkControlPlaneLabels, local.Spec.Driver.Labels)
		assert.Equal(t, sparkControlPlaneLabels, local.Spec.Executor.Labels)
		assert.Equal(t, "michelangelo-spark", local.Labels["ma/owner-service"],
			"Spark's owner must differ from Ray's or the two watches cross over")
	})

	// Sharing one map would make a later edit to either pod spec's labels show
	// up on the other.
	t.Run("driver and executor do not share one label map", func(t *testing.T) {
		local := mapSpark(t, m, job, &v2pb.Cluster{})
		local.Spec.Driver.Labels["probe"] = "driver-only"
		assert.NotContains(t, local.Spec.Executor.Labels, "probe")
		assert.NotContains(t, local.Labels, "probe")
	})

	// The job's UID is stamped by the control plane from the CR's own metadata,
	// so a forged label on the source cannot stand in for it.
	t.Run("a forged job-uid label never replaces the real UID", func(t *testing.T) {
		withUID := newSparkJob()
		withUID.Labels = job.Labels
		withUID.UID = "real-uid"
		local := mapSpark(t, m, withUID, &v2pb.Cluster{})
		assert.Equal(t, "real-uid", local.Labels["ma/job-uid"])
		assert.NotContains(t, local.Spec.Driver.Labels, "ma/job-uid")
		assert.NotContains(t, local.Spec.Executor.Labels, "ma/job-uid")
	})

	t.Run("kueue cluster without project label falls back to namespace", func(t *testing.T) {
		unlabeled := &v2pb.SparkJob{
			ObjectMeta: metav1.ObjectMeta{Name: "spark-2", Namespace: "proj-ns"},
		}
		local := mapSpark(t, m, unlabeled, kueueCluster())
		assert.Equal(t, "ma-proj-ns", local.Labels["kueue.x-k8s.io/queue-name"])
	})

	t.Run("kueue cluster without any project identity fails dispatch", func(t *testing.T) {
		anonymous := &v2pb.SparkJob{ObjectMeta: metav1.ObjectMeta{Name: "spark-3"}}
		_, err := m.MapGlobalJobToLocal(anonymous, nil, kueueCluster())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "LocalQueue")
	})
}

// A mapper that panics on a sparse CR would take the control-plane process
// down with it — exactly what making Spark-shaped input safe was meant to
// prevent. The standalone path dereferences Driver.Pod.Image and
// Resource.Cpu unguarded; this one must not.
func TestMapSparkJob_SparseCR(t *testing.T) {
	m := sparkMapper()

	tests := []struct {
		name string
		job  *v2pb.SparkJob
	}{
		{
			name: "no spec at all",
			job:  &v2pb.SparkJob{ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "proj1"}},
		},
		{
			name: "nil driver and executor",
			job: &v2pb.SparkJob{
				ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "proj1"},
				Spec:       v2pb.SparkJobSpec{MainApplicationFile: "local:///a.py"},
			},
		},
		{
			name: "nil pods",
			job: &v2pb.SparkJob{
				ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "proj1"},
				Spec: v2pb.SparkJobSpec{
					Driver:   &v2pb.DriverSpec{},
					Executor: &v2pb.ExecutorSpec{},
				},
			},
		},
		{
			name: "nil resources",
			job: &v2pb.SparkJob{
				ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "proj1"},
				Spec: v2pb.SparkJobSpec{
					Driver:   &v2pb.DriverSpec{Pod: &v2pb.PodSpec{}},
					Executor: &v2pb.ExecutorSpec{Pod: &v2pb.PodSpec{}},
				},
			},
		},
		{
			name: "env-from source with neither ref set",
			job: &v2pb.SparkJob{
				ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "proj1"},
				Spec: v2pb.SparkJobSpec{
					Driver: &v2pb.DriverSpec{Pod: &v2pb.PodSpec{
						EnvFrom: []*v2pb.EnvFromSource{{Prefix: "P_"}},
					}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotPanics(t, func() {
				local := mapSpark(t, m, tt.job, kueueCluster())
				// Whatever is missing, the projected object is still routable.
				assert.Equal(t, SparkLocalNamespace, local.Namespace)
				assert.Equal(t, sparkControlPlaneLabels, local.Spec.Driver.Labels)
			})
		})
	}

	// MapGlobalJobToLocal's own guard only catches a nil interface; a typed nil
	// pointer passes it and reaches the mapper.
	t.Run("typed nil SparkJob errors rather than panicking", func(t *testing.T) {
		require.NotPanics(t, func() {
			_, err := m.MapGlobalJobToLocal((*v2pb.SparkJob)(nil), nil, &v2pb.Cluster{})
			require.Error(t, err)
		})
	})

	// Cores has Minimum=1 in the CRD and a zero-quantity GPU would still emit a
	// resource limit for a device the job never asked for.
	t.Run("zero-valued resources are omitted, not zeroed", func(t *testing.T) {
		job := &v2pb.SparkJob{
			ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "proj1"},
			Spec: v2pb.SparkJobSpec{
				Driver: &v2pb.DriverSpec{Pod: &v2pb.PodSpec{
					Resource: &v2pb.ResourceSpec{Cpu: 0, Memory: "", Gpu: 0},
				}},
				Executor: &v2pb.ExecutorSpec{Instances: 0},
			},
		}
		local := mapSpark(t, m, job, &v2pb.Cluster{})
		assert.Nil(t, local.Spec.Driver.Cores)
		assert.Nil(t, local.Spec.Driver.Memory)
		assert.Nil(t, local.Spec.Driver.GPU)
		assert.Nil(t, local.Spec.Executor.Instances)
		assert.Nil(t, local.Spec.Image, "an unset image stays absent rather than serializing as \"\"")
	})
}

// SparkPodSpec has no Volumes of its own, so the sidecar's emptyDir has to be
// hoisted onto the application — once, though both pods ask for it.
func TestMapSparkJob_CollectorSidecar(t *testing.T) {
	m := sparkMapper()
	m.LogPersistence = LogPersistenceConfig{
		Enabled:           true,
		StorageEndpoint:   "minio:9091",
		Bucket:            "ray-history",
		CredentialsSecret: "minio-credentials",
		CollectorImage:    "quay.io/kuberay/collector:v1.7.1",
	}

	local := mapSpark(t, m, newSparkJob(), &v2pb.Cluster{})

	require.Len(t, local.Spec.Driver.Sidecars, 1)
	require.Len(t, local.Spec.Executor.Sidecars, 1)
	assert.Equal(t, "collector", local.Spec.Driver.Sidecars[0].Name)

	require.Len(t, local.Spec.Volumes, 1, "driver and executor share one declaration")
	require.NotNil(t, local.Spec.Volumes[0].EmptyDir)

	// The main container's own env and mounts survive injection.
	driverEnv, _, _ := splitEnv(local.Spec.Driver.Env)
	assert.Equal(t, "d", driverEnv["DRIVER_ENV"])
	assert.NotEmpty(t, local.Spec.Driver.VolumeMounts)

	// RAY_ROLE distinguishes the two collectors; neither is "Head", so neither
	// picks up the head-only dashboard-poll vars.
	sidecarEnv, _, _ := splitEnv(local.Spec.Driver.Sidecars[0].Env)
	assert.Equal(t, sparkDriverRole, sidecarEnv["RAY_ROLE"])
	executorEnv, _, _ := splitEnv(local.Spec.Executor.Sidecars[0].Env)
	assert.Equal(t, sparkExecutorRole, executorEnv["RAY_ROLE"])
}

func TestMapSparkJob_NoSidecarWhenDisabled(t *testing.T) {
	local := mapSpark(t, sparkMapper(), newSparkJob(), &v2pb.Cluster{})
	assert.Empty(t, local.Spec.Driver.Sidecars)
	assert.Empty(t, local.Spec.Executor.Sidecars)
	assert.Empty(t, local.Spec.Volumes)
}

// v2pb.SparkJobStatus has no state enum, so the application state travels in
// StatusConditions — the control plane's real state machine.
func TestMapper_MapLocalJobStatusToGlobal_SparkApplication(t *testing.T) {
	m := Mapper{}

	mkApp := func(state sparkv1beta2.ApplicationStateType, errMsg string) *sparkv1beta2.SparkApplication {
		app := &sparkv1beta2.SparkApplication{}
		app.Status.SparkApplicationID = "spark-app-id"
		app.Status.DriverInfo.WebUIIngressAddress = "http://spark-ui"
		app.Status.AppState = sparkv1beta2.ApplicationState{State: state, ErrorMessage: errMsg}
		return app
	}

	condition := func(t *testing.T, conditions []*apipb.Condition, conditionType string) *apipb.Condition {
		t.Helper()
		for _, c := range conditions {
			if c.Type == conditionType {
				return c
			}
		}
		return nil
	}

	t.Run("identity and URL are carried across", func(t *testing.T) {
		status, err := m.MapLocalJobStatusToGlobal(mkApp(sparkv1beta2.RunningState, ""))
		require.NoError(t, err)
		require.NotNil(t, status)
		require.NotNil(t, status.Spark)
		assert.Equal(t, "spark-app-id", status.Spark.ApplicationId)
		assert.Equal(t, "http://spark-ui", status.Spark.JobUrl)
	})

	tests := []struct {
		name       string
		state      sparkv1beta2.ApplicationStateType
		errMsg     string
		expStatus  apipb.ConditionStatus
		expRunning bool
	}{
		{"completed", sparkv1beta2.CompletedState, "", apipb.CONDITION_STATUS_TRUE, false},
		{"failed", sparkv1beta2.FailedState, "boom", apipb.CONDITION_STATUS_FALSE, false},
		{"submission failed", sparkv1beta2.FailedSubmissionState, "rejected", apipb.CONDITION_STATUS_FALSE, false},
		{"failing", sparkv1beta2.FailingState, "dying", apipb.CONDITION_STATUS_UNKNOWN, false},
		{"running", sparkv1beta2.RunningState, "", apipb.CONDITION_STATUS_UNKNOWN, true},
		{"submitted", sparkv1beta2.SubmittedState, "", apipb.CONDITION_STATUS_UNKNOWN, false},
		{"new", sparkv1beta2.NewState, "", apipb.CONDITION_STATUS_UNKNOWN, false},
		{"unknown", sparkv1beta2.UnknownState, "", apipb.CONDITION_STATUS_UNKNOWN, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := m.MapLocalJobStatusToGlobal(mkApp(tt.state, tt.errMsg))
			require.NoError(t, err)
			require.NotNil(t, status.Spark)

			// The state is the reason, the error message is the message.
			succeeded := condition(t, status.Spark.StatusConditions, "Succeeded")
			require.NotNil(t, succeeded, "a Succeeded condition is emitted on every call")
			assert.Equal(t, tt.expStatus, succeeded.Status)
			assert.Equal(t, string(tt.state), succeeded.Reason)
			assert.Equal(t, tt.errMsg, succeeded.Message)

			running := condition(t, status.Spark.StatusConditions, "SparkAppRunning")
			if tt.expRunning {
				require.NotNil(t, running)
				assert.Equal(t, apipb.CONDITION_STATUS_TRUE, running.Status)
			} else {
				assert.Nil(t, running)
			}
		})
	}
}

// TestMapSparkJobQueueLabelPlacement is the label placement rule in one table:
// wherever the job is headed, the Kueue queue label belongs on the
// SparkApplication object and nowhere else, while the three control-plane
// labels go on everything.
//
// Both directions fail quietly, which is why they are asserted together. Omit
// the label from the object and the SparkApplication is created but never
// admitted. Add it to the pod templates and Kueue's pod integration treats
// each driver and executor as an admissible workload of its own, double
// counting against the quota the job was already admitted under.
func TestMapSparkJobQueueLabelPlacement(t *testing.T) {
	tests := []struct {
		name string
		// cluster is the target the job is being projected for.
		cluster *v2pb.Cluster
		// wantQueueName is the LocalQueue the object must be labelled with;
		// empty means the object must carry no queue label at all.
		wantQueueName string
	}{
		{
			name:          "kueue-managed cluster",
			cluster:       kueueCluster(),
			wantQueueName: "ma-proj1",
		},
		{
			name:    "cluster with the default scheduler",
			cluster: &v2pb.Cluster{},
		},
		{
			name:    "no target cluster yet",
			cluster: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			local := mapSpark(t, sparkMapper(), newSparkJob(), tt.cluster)

			// Direction 1 — the workload object is the only thing Kueue admits.
			if tt.wantQueueName == "" {
				assert.NotContains(t, local.Labels, constants.KueueQueueNameLabelKey,
					"a job on a non-Kueue cluster must carry no queue label")
			} else {
				assert.Equal(t, tt.wantQueueName, local.Labels[constants.KueueQueueNameLabelKey],
					"the SparkApplication must name the LocalQueue it is admitted under")
			}

			// Direction 2 — the pod templates carry the control-plane labels
			// the watch selects on, and never the queue label.
			pods := []struct {
				role   string
				labels map[string]string
			}{
				{"driver", local.Spec.Driver.Labels},
				{"executor", local.Spec.Executor.Labels},
			}
			for _, pod := range pods {
				assert.Equal(t, sparkControlPlaneLabels, pod.labels,
					"the %s template carries exactly the three control-plane labels", pod.role)
				assert.NotContains(t, pod.labels, constants.KueueQueueNameLabelKey,
					"the %s template must not be separately admissible", pod.role)
			}
		})
	}
}

// Nothing but the application's own TTL reaps a finished SparkApplication, so
// every projected one carries it. The literal is the chosen TTL (24h); a change
// to the constant has to change this test deliberately.
func TestMapSparkJob_TimeToLive(t *testing.T) {
	local := mapSpark(t, sparkMapper(), newSparkJob(), &v2pb.Cluster{})

	require.NotNil(t, local.Spec.TimeToLiveSeconds, "an application without a TTL is never garbage-collected")
	assert.EqualValues(t, 86400, *local.Spec.TimeToLiveSeconds)

	t.Run("sparse CR still carries the TTL", func(t *testing.T) {
		sparse := &v2pb.SparkJob{ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "proj1"}}
		got := mapSpark(t, sparkMapper(), sparse, nil)
		require.NotNil(t, got.Spec.TimeToLiveSeconds)
		assert.EqualValues(t, SparkApplicationTTLSeconds, *got.Spec.TimeToLiveSeconds)
	})

	// A shared pointer would let a write through one application's spec change
	// every other's.
	t.Run("each application gets its own pointer", func(t *testing.T) {
		first := mapSpark(t, sparkMapper(), newSparkJob(), nil)
		second := mapSpark(t, sparkMapper(), newSparkJob(), nil)
		*first.Spec.TimeToLiveSeconds = 1
		assert.EqualValues(t, SparkApplicationTTLSeconds, *second.Spec.TimeToLiveSeconds)
	})
}

// The job's UID rides on the SparkApplication as ma/job-uid, so that the
// client and the watcher can tell the job's own application from a same-named
// one that another job left behind or that was created outside Michelangelo.
func TestMapSparkJob_JobUIDLabel(t *testing.T) {
	m := sparkMapper()

	t.Run("present on the application, with the job's UID", func(t *testing.T) {
		job := newSparkJob()
		job.UID = "11111111-2222-3333-4444-555555555555"
		local := mapSpark(t, m, job, &v2pb.Cluster{})

		assert.Equal(t, "11111111-2222-3333-4444-555555555555", local.Labels["ma/job-uid"])
		// The three control-plane labels are still all there.
		for key, want := range sparkControlPlaneLabels {
			assert.Equal(t, want, local.Labels[key], key)
		}
	})

	// The mapper leaves it off the pod specs. Spark Operator 2.x copies it onto
	// the pods with the application's other labels, and nothing reads it there.
	t.Run("not on the pod specs", func(t *testing.T) {
		job := newSparkJob()
		job.UID = "uid-1"
		local := mapSpark(t, m, job, &v2pb.Cluster{})

		assert.Equal(t, sparkControlPlaneLabels, local.Spec.Driver.Labels)
		assert.Equal(t, sparkControlPlaneLabels, local.Spec.Executor.Labels)
	})

	t.Run("present alongside the queue label on a kueue cluster", func(t *testing.T) {
		job := newSparkJob()
		job.UID = "uid-2"
		local := mapSpark(t, m, job, kueueCluster())

		assert.Equal(t, "uid-2", local.Labels["ma/job-uid"])
		assert.Equal(t, "ma-proj1", local.Labels["kueue.x-k8s.io/queue-name"])
	})

	// An object that never went through the API server has no UID, so there is
	// nothing to label the application with: none is written.
	t.Run("absent when the job has no UID", func(t *testing.T) {
		local := mapSpark(t, m, newSparkJob(), &v2pb.Cluster{})

		assert.NotContains(t, local.Labels, "ma/job-uid")
		assert.Equal(t, sparkControlPlaneLabels, local.Labels)
	})

	// The source CR's labels belong to the job author. The UID comes from the CR's
	// metadata alone, so a forged label is dropped when there is no UID and is
	// overwritten when there is one.
	t.Run("a forged label on the source does not survive", func(t *testing.T) {
		forged := newSparkJob()
		forged.Labels = map[string]string{"ma/job-uid": "someone-elses-uid"}
		local := mapSpark(t, m, forged, &v2pb.Cluster{})
		assert.NotContains(t, local.Labels, "ma/job-uid")

		forged.UID = "my-uid"
		local = mapSpark(t, m, forged, &v2pb.Cluster{})
		assert.Equal(t, "my-uid", local.Labels["ma/job-uid"])
	})
}

// The GPU resource name is nvidia.com/gpu whatever gpu_sku says. The SKU is
// a GPU model ("nvidia-tesla-v100"), never an extended-resource name, and the
// Uniflow tasks leave it empty -- the operator drops a GPUSpec with no name.
// A pod that asks for a GPU also gets the NVIDIA toleration; one that does not
// is left alone so the taint keeps reserving GPU nodes.
func TestMapSparkJob_GPU(t *testing.T) {
	m := sparkMapper()

	job := func(driver, executor *v2pb.ResourceSpec) *v2pb.SparkJob {
		return &v2pb.SparkJob{
			ObjectMeta: metav1.ObjectMeta{Name: "gpu-job", Namespace: "proj1"},
			Spec: v2pb.SparkJobSpec{
				Driver:   &v2pb.DriverSpec{Pod: &v2pb.PodSpec{Resource: driver}},
				Executor: &v2pb.ExecutorSpec{Pod: &v2pb.PodSpec{Resource: executor}},
			},
		}
	}

	// wantGPU is the expected quantity, 0 for "no GPUSpec at all".
	tests := []struct {
		name             string
		driver, executor *v2pb.ResourceSpec
		wantDriverGPU    int64
		wantExecutorGPU  int64
	}{
		{
			name:            "executor with a SKU",
			driver:          &v2pb.ResourceSpec{Cpu: 1},
			executor:        &v2pb.ResourceSpec{Gpu: 2, GpuSku: "nvidia-a100"},
			wantExecutorGPU: 2,
		},
		{
			name:            "executor without a SKU (what Uniflow sends)",
			driver:          &v2pb.ResourceSpec{Cpu: 1},
			executor:        &v2pb.ResourceSpec{Gpu: 1},
			wantExecutorGPU: 1,
		},
		{
			name:          "driver with a SKU",
			driver:        &v2pb.ResourceSpec{Gpu: 1, GpuSku: "nvidia-tesla-v100"},
			executor:      &v2pb.ResourceSpec{Cpu: 2},
			wantDriverGPU: 1,
		},
		{
			name:          "driver without a SKU",
			driver:        &v2pb.ResourceSpec{Gpu: 3},
			executor:      &v2pb.ResourceSpec{Cpu: 2},
			wantDriverGPU: 3,
		},
		{
			name:            "both",
			driver:          &v2pb.ResourceSpec{Gpu: 1, GpuSku: "nvidia-a100"},
			executor:        &v2pb.ResourceSpec{Gpu: 4},
			wantDriverGPU:   1,
			wantExecutorGPU: 4,
		},
		{
			name:     "neither",
			driver:   &v2pb.ResourceSpec{Cpu: 1},
			executor: &v2pb.ResourceSpec{Cpu: 2},
		},
		{
			// A SKU is not a request: it names a model, the count asks for one.
			name:     "a SKU alone requests no GPU",
			driver:   &v2pb.ResourceSpec{GpuSku: "nvidia-a100"},
			executor: &v2pb.ResourceSpec{GpuSku: "nvidia-a100"},
		},
	}

	check := func(t *testing.T, role string, pod sparkv1beta2.SparkPodSpec, wantGPU int64) {
		t.Helper()
		if wantGPU == 0 {
			assert.Nil(t, pod.GPU, "%s: no GPU asked for, none projected", role)
			assert.Nil(t, pod.Tolerations, "%s: a pod without a GPU is left to the taint", role)
			return
		}
		require.NotNil(t, pod.GPU, role)
		assert.Equal(t, "nvidia.com/gpu", pod.GPU.Name, role)
		assert.EqualValues(t, wantGPU, pod.GPU.Quantity, role)
		assert.Equal(t, []corev1.Toleration{gpuToleration}, pod.Tolerations,
			"%s: exactly the NVIDIA GPU toleration", role)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			local := mapSpark(t, m, job(tt.driver, tt.executor), &v2pb.Cluster{})
			check(t, "driver", local.Spec.Driver.SparkPodSpec, tt.wantDriverGPU)
			check(t, "executor", local.Spec.Executor.SparkPodSpec, tt.wantExecutorGPU)
		})
	}

	// Tolerations on the pods must be the pods' own: editing one reaches neither
	// the other pod nor the package-level toleration every mapping starts from.
	t.Run("tolerations are not shared", func(t *testing.T) {
		local := mapSpark(t, m, job(&v2pb.ResourceSpec{Gpu: 1}, &v2pb.ResourceSpec{Gpu: 1}), &v2pb.Cluster{})
		require.Len(t, local.Spec.Driver.Tolerations, 1)
		require.Len(t, local.Spec.Executor.Tolerations, 1)

		local.Spec.Driver.Tolerations[0].Key = "edited"

		assert.Equal(t, "nvidia.com/gpu", local.Spec.Executor.Tolerations[0].Key)
		assert.Equal(t, "nvidia.com/gpu", nvidiaGPUToleration.Key)
		again := mapSpark(t, m, job(&v2pb.ResourceSpec{Gpu: 1}, nil), &v2pb.Cluster{})
		assert.Equal(t, "nvidia.com/gpu", again.Spec.Driver.Tolerations[0].Key)
	})
}

// An optional envFrom source must stay optional. Federation makes "present
// on the host cluster, absent on the member" likelier than the single-cluster
// path was, and a required source the member lacks keeps the pod from starting.
func TestMapSparkJob_EnvFrom(t *testing.T) {
	m := sparkMapper()

	sources := []*v2pb.EnvFromSource{
		{
			Prefix:       "SEC_",
			SecretRef:    &v2pb.SecretEnvSource{LocalObjectReference: &v2pb.LocalObjectReference{Name: "required-secret"}},
			ConfigMapRef: &v2pb.ConfigMapEnvSource{LocalObjectReference: &v2pb.LocalObjectReference{Name: "optional-config"}, Optional: true},
		},
		{
			SecretRef: &v2pb.SecretEnvSource{LocalObjectReference: &v2pb.LocalObjectReference{Name: "optional-secret"}, Optional: true},
		},
		{
			ConfigMapRef: &v2pb.ConfigMapEnvSource{LocalObjectReference: &v2pb.LocalObjectReference{Name: "required-config"}},
		},
		// A source naming neither reference survives without panicking and keeps
		// its prefix.
		{Prefix: "ONLY_"},
	}

	job := &v2pb.SparkJob{
		ObjectMeta: metav1.ObjectMeta{Name: "envfrom-job", Namespace: "proj1"},
		Spec: v2pb.SparkJobSpec{
			Driver:   &v2pb.DriverSpec{Pod: &v2pb.PodSpec{EnvFrom: sources}},
			Executor: &v2pb.ExecutorSpec{Pod: &v2pb.PodSpec{EnvFrom: sources}},
		},
	}
	local := mapSpark(t, m, job, &v2pb.Cluster{})

	want := []corev1.EnvFromSource{
		{
			Prefix:       "SEC_",
			SecretRef:    &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "required-secret"}},
			ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "optional-config"}, Optional: ptr.To(true)},
		},
		{
			SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "optional-secret"}, Optional: ptr.To(true)},
		},
		{
			ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "required-config"}},
		},
		{Prefix: "ONLY_"},
	}

	assert.Equal(t, want, local.Spec.Driver.EnvFrom, "driver")
	assert.Equal(t, want, local.Spec.Executor.EnvFrom, "executor")

	// proto3 cannot express "explicitly false", so required sources keep a nil
	// pointer rather than a pointer to false.
	assert.Nil(t, local.Spec.Driver.EnvFrom[0].SecretRef.Optional)
	assert.Nil(t, local.Spec.Driver.EnvFrom[2].ConfigMapRef.Optional)
}

// The application-level image is the driver's, or the executor's when the
// driver names none; an executor image that differs is set on the executor.
func TestMapSparkJob_ExecutorImage(t *testing.T) {
	m := sparkMapper()

	job := func(driverImage, executorImage string) *v2pb.SparkJob {
		return &v2pb.SparkJob{
			ObjectMeta: metav1.ObjectMeta{Name: "image-job", Namespace: "proj1"},
			Spec: v2pb.SparkJobSpec{
				Driver:   &v2pb.DriverSpec{Pod: &v2pb.PodSpec{Image: driverImage, ImagePullingPolicy: "Always"}},
				Executor: &v2pb.ExecutorSpec{Pod: &v2pb.PodSpec{Image: executorImage, ImagePullingPolicy: "Never"}},
			},
		}
	}

	tests := []struct {
		name                       string
		driverImage, executorImage string
		wantAppImage               string // "" means no application-level image
		wantExecutorImage          string // "" means the executor inherits
	}{
		{name: "same image: executor inherits", driverImage: "spark:1", executorImage: "spark:1", wantAppImage: "spark:1"},
		{name: "executor names none: inherits", driverImage: "spark:1", wantAppImage: "spark:1"},
		{name: "different executor image is set per executor", driverImage: "spark:1", executorImage: "spark-gpu:1", wantAppImage: "spark:1", wantExecutorImage: "spark-gpu:1"},
		{name: "driver names none: the executor's becomes the application's", executorImage: "spark-gpu:1", wantAppImage: "spark-gpu:1"},
		{name: "no image at all", wantAppImage: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			local := mapSpark(t, m, job(tt.driverImage, tt.executorImage), &v2pb.Cluster{})

			if tt.wantAppImage == "" {
				assert.Nil(t, local.Spec.Image)
			} else {
				require.NotNil(t, local.Spec.Image)
				assert.Equal(t, tt.wantAppImage, *local.Spec.Image)
			}
			if tt.wantExecutorImage == "" {
				assert.Nil(t, local.Spec.Executor.Image)
			} else {
				require.NotNil(t, local.Spec.Executor.Image)
				assert.Equal(t, tt.wantExecutorImage, *local.Spec.Executor.Image)
			}
			// The driver never carries a per-pod image: the application-level one is
			// its image.
			assert.Nil(t, local.Spec.Driver.Image)
		})
	}

	// SparkPodSpec has no pull policy, so the executor's cannot be carried and the
	// application-level one is the driver pod's.
	t.Run("pull policy comes from the driver pod", func(t *testing.T) {
		local := mapSpark(t, m, job("spark:1", "spark-gpu:1"), &v2pb.Cluster{})
		require.NotNil(t, local.Spec.ImagePullPolicy)
		assert.Equal(t, "Always", *local.Spec.ImagePullPolicy)
	})

	t.Run("a driver pod that is absent does not hide the executor image", func(t *testing.T) {
		noDriver := &v2pb.SparkJob{
			ObjectMeta: metav1.ObjectMeta{Name: "image-job", Namespace: "proj1"},
			Spec: v2pb.SparkJobSpec{
				Executor: &v2pb.ExecutorSpec{Pod: &v2pb.PodSpec{Image: "spark-gpu:1"}},
			},
		}
		local := mapSpark(t, m, noDriver, &v2pb.Cluster{})
		require.NotNil(t, local.Spec.Image)
		assert.Equal(t, "spark-gpu:1", *local.Spec.Image)
		assert.Nil(t, local.Spec.Executor.Image)
	})
}

// The collector design only works if three names agree: the volume the
// application declares, the mount on the main container (which is what makes the
// operator attach a hoisted volume to the pod at all), and the mount on the
// sidecar. Break any link and the pod is invalid or the collector reads an empty
// directory, and the sidecar test above stays green.
func TestMapSparkJob_CollectorSidecarVolumeLinkage(t *testing.T) {
	m := sparkMapper()
	m.LogPersistence = LogPersistenceConfig{
		Enabled:           true,
		StorageEndpoint:   "minio:9091",
		Bucket:            "ray-history",
		CredentialsSecret: "minio-credentials",
		CollectorImage:    "quay.io/kuberay/collector:v1.7.1",
	}

	local := mapSpark(t, m, newSparkJob(), &v2pb.Cluster{})

	require.Len(t, local.Spec.Volumes, 1)
	volume := local.Spec.Volumes[0]
	require.NotNil(t, volume.EmptyDir)

	mountNamed := func(t *testing.T, mounts []corev1.VolumeMount, name string) corev1.VolumeMount {
		t.Helper()
		for _, mount := range mounts {
			if mount.Name == name {
				return mount
			}
		}
		require.Failf(t, "no mount of the application's volume", "volume %q, mounts %v", name, mounts)
		return corev1.VolumeMount{}
	}

	for _, pod := range []struct {
		role     string
		spec     sparkv1beta2.SparkPodSpec
		wantRole string
	}{
		{"driver", local.Spec.Driver.SparkPodSpec, "Driver"},
		{"executor", local.Spec.Executor.SparkPodSpec, "Executor"},
	} {
		t.Run(pod.role, func(t *testing.T) {
			// The main container mounts the volume the application declares.
			mainMount := mountNamed(t, pod.spec.VolumeMounts, volume.Name)
			assert.Equal(t, "/tmp/ray", mainMount.MountPath)

			// So does the sidecar, at the same path, or it would read an empty one.
			require.Len(t, pod.spec.Sidecars, 1)
			sidecar := pod.spec.Sidecars[0]
			sidecarMount := mountNamed(t, sidecar.VolumeMounts, volume.Name)
			assert.Equal(t, mainMount.MountPath, sidecarMount.MountPath)

			// RAY_ROLE is asserted against literals: comparing it to the constants
			// production passes would stay green if the role were renamed "Head".
			env, _, _ := splitEnv(sidecar.Env)
			assert.Equal(t, pod.wantRole, env["RAY_ROLE"])
			assert.NotContains(t, env, "RAY_DASHBOARD_ADDRESS", "only a Head collector polls the dashboard")
		})
	}
}

// The status rows the original table never exercised, and the message a failed
// application gets when the operator gives none.
func TestMapper_MapLocalJobStatusToGlobal_SparkApplicationEdges(t *testing.T) {
	m := Mapper{}

	succeeded := func(t *testing.T, status *v2pb.SparkJobStatus) *apipb.Condition {
		t.Helper()
		for _, c := range status.StatusConditions {
			if c.Type == "Succeeded" {
				return c
			}
		}
		require.Fail(t, "no Succeeded condition")
		return nil
	}

	// A nil application has no state, which is the state UNKNOWN: non-terminal,
	// so the job is neither finished nor failed on the strength of nothing.
	t.Run("nil application reads as UNKNOWN", func(t *testing.T) {
		got, err := m.MapLocalJobStatusToGlobal((*sparkv1beta2.SparkApplication)(nil))
		require.NoError(t, err)
		require.NotNil(t, got.Spark)

		condition := succeeded(t, got.Spark)
		assert.Equal(t, apipb.CONDITION_STATUS_UNKNOWN, condition.Status)
		assert.Equal(t, "UNKNOWN", condition.Reason)
		assert.Empty(t, condition.Message)
		assert.Empty(t, got.Spark.ApplicationId)
		assert.Len(t, got.Spark.StatusConditions, 1, "no SparkAppRunning for nothing")
	})

	// SUCCEEDING and the rerun states move on to a terminal state or back to
	// running; none of them is the outcome.
	for _, state := range []sparkv1beta2.ApplicationStateType{
		sparkv1beta2.SucceedingState,
		sparkv1beta2.PendingRerunState,
		sparkv1beta2.InvalidatingState,
	} {
		t.Run("transitional "+string(state), func(t *testing.T) {
			app := &sparkv1beta2.SparkApplication{}
			app.Status.AppState = sparkv1beta2.ApplicationState{State: state}

			got, err := m.MapLocalJobStatusToGlobal(app)
			require.NoError(t, err)

			condition := succeeded(t, got.Spark)
			assert.Equal(t, apipb.CONDITION_STATUS_UNKNOWN, condition.Status)
			assert.Equal(t, string(state), condition.Reason)
			assert.Len(t, got.Spark.StatusConditions, 1, "only a RUNNING application reports SparkAppRunning")
		})
	}

	// The operator does not always say why an application failed, and an empty
	// message shows up downstream as a bare "FAILED:". The reason keeps the real
	// state either way.
	for _, state := range []sparkv1beta2.ApplicationStateType{
		sparkv1beta2.FailedState,
		sparkv1beta2.FailedSubmissionState,
	} {
		t.Run("failure without a message: "+string(state), func(t *testing.T) {
			app := &sparkv1beta2.SparkApplication{}
			app.Status.AppState = sparkv1beta2.ApplicationState{State: state}

			got, err := m.MapLocalJobStatusToGlobal(app)
			require.NoError(t, err)

			condition := succeeded(t, got.Spark)
			assert.Equal(t, apipb.CONDITION_STATUS_FALSE, condition.Status)
			assert.Equal(t, "Spark job failed", condition.Message)
			assert.Equal(t, string(state), condition.Reason)
		})

		t.Run("failure with a message keeps it: "+string(state), func(t *testing.T) {
			app := &sparkv1beta2.SparkApplication{}
			app.Status.AppState = sparkv1beta2.ApplicationState{State: state, ErrorMessage: "driver exited 137"}

			got, err := m.MapLocalJobStatusToGlobal(app)
			require.NoError(t, err)

			assert.Equal(t, "driver exited 137", succeeded(t, got.Spark).Message)
		})
	}

	// The default message belongs to failures only: a state that is not an
	// outcome yet must not be given one.
	t.Run("the default message is not given to non-failures", func(t *testing.T) {
		for _, state := range []sparkv1beta2.ApplicationStateType{
			sparkv1beta2.NewState, sparkv1beta2.RunningState, sparkv1beta2.CompletedState, sparkv1beta2.FailingState,
		} {
			app := &sparkv1beta2.SparkApplication{}
			app.Status.AppState = sparkv1beta2.ApplicationState{State: state}

			got, err := m.MapLocalJobStatusToGlobal(app)
			require.NoError(t, err)
			assert.Empty(t, succeeded(t, got.Spark).Message, string(state))
		}
	})
}
