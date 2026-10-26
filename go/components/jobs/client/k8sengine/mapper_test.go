package k8sengine

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	k8sptr "k8s.io/utils/ptr"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// withControlPlaneLabels returns own plus the control-plane labels a mapped
// object is expected to carry for RuntimeEnvironment "staging" in namespace
// "proj1". As in production, the control-plane labels win over same-key labels
// in own. own is left untouched — mapping must not write back into the
// Michelangelo CR's own pod template.
func withControlPlaneLabels(own map[string]string) map[string]string {
	labels := make(map[string]string, len(own)+3)
	for key, value := range own {
		labels[key] = value
	}
	labels["ma/owner-service"] = "michelangelo-ray"
	labels["ma/control-plane-env"] = "staging"
	labels["ma/project-name"] = "proj1"
	return labels
}

func TestMapper_MapGlobalJobToLocal(t *testing.T) {
	m := Mapper{Env: env.Context{RuntimeEnvironment: "staging"}}

	headPod := &corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"role": "head"}},
		Spec: corev1.PodSpec{
			ServiceAccountName: "ray-sa",
			ImagePullSecrets:   []corev1.LocalObjectReference{{Name: "regcred"}},
			Containers: []corev1.Container{{
				Name:            "ray-head",
				Image:           "ray:test",
				ImagePullPolicy: corev1.PullIfNotPresent,
				// Head-sized (and GPU) resources that must NOT leak into the submitter.
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:                    resource.MustParse("4"),
						corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("1"),
					},
				},
			}},
		},
	}
	workerPod := &corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"role": "worker"}}}

	// The submitter reuses the head image + pull/auth settings but is right-sized
	// (KubeRay-default CPU/mem, no GPU) and never inherits the head's resources.
	submitterPod := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			RestartPolicy:      corev1.RestartPolicyNever,
			ServiceAccountName: "ray-sa",
			ImagePullSecrets:   []corev1.LocalObjectReference{{Name: "regcred"}},
			Containers: []corev1.Container{{
				Name:            "ray-job-submitter",
				Image:           "ray:test",
				ImagePullPolicy: corev1.PullIfNotPresent,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("200Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
				},
			}},
		},
	}

	rayJob := &v2pb.RayJob{
		ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: "proj1"},
		Spec:       v2pb.RayJobSpec{Entrypoint: "python main.py"},
	}
	rayCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Spec: v2pb.RayClusterSpec{
			RayVersion: "2.10.0",
			Head: &v2pb.RayHeadSpec{
				ServiceType:    string(corev1.ServiceTypeClusterIP),
				Pod:            headPod,
				RayStartParams: map[string]string{"head": "param"},
			},
			Workers: []*v2pb.RayWorkerSpec{
				{
					Pod:            workerPod,
					MinInstances:   1,
					MaxInstances:   3,
					RayStartParams: map[string]string{"worker": "param"},
				},
			},
		},
	}

	tests := []struct {
		name                string
		jobObject           any
		clusterObject       any
		expectErrSubstr     string
		expectedLocalObject k8sruntime.Object
	}{
		{
			name:          "ray job with cluster -> job mapped",
			jobObject:     rayJob,
			clusterObject: rayCluster,
			expectedLocalObject: &rayv1.RayJob{
				TypeMeta: metav1.TypeMeta{
					Kind:       RayJobKind,
					APIVersion: RayAPIVersion,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      rayJob.Name,
					Namespace: RayLocalNamespace,
					// Routing labels: how the federated watchers find this
					// object and map its events back to the RayJob CR.
					Labels: map[string]string{
						"ma/owner-service":     "michelangelo-ray",
						"ma/control-plane-env": "staging",
						"ma/project-name":      "proj1",
					},
				},
				Spec: rayv1.RayJobSpec{
					Entrypoint: rayJob.Spec.Entrypoint,
					ClusterSelector: map[string]string{
						"ray.io/cluster":      rayCluster.Name,
						"rayClusterNamespace": RayLocalNamespace,
					},
					TTLSecondsAfterFinished:  int32(300),
					ShutdownAfterJobFinishes: true,
					SubmitterPodTemplate:     submitterPod,
				},
			},
		},
		{
			name:            "ray job without cluster -> error",
			jobObject:       rayJob,
			clusterObject:   nil,
			expectErrSubstr: "ray job requires associated RayCluster object",
		},
		{
			name:            "ray job with wrong cluster type -> error",
			jobObject:       rayJob,
			clusterObject:   &v2pb.SparkJob{},
			expectErrSubstr: "expected *v2pb.RayCluster",
		},
		{
			// Spark needs no job-cluster object: the job is itself the
			// scheduled unit. The full projection is asserted in spark_test.go.
			name:          "spark job without cluster -> job mapped",
			jobObject:     &v2pb.SparkJob{},
			clusterObject: nil,
		},
		{
			name:            "nil job object -> error",
			jobObject:       nil,
			clusterObject:   rayCluster,
			expectErrSubstr: "jobObject cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var jobObj, clusterObj k8sruntime.Object
			if tt.jobObject != nil {
				jobObj = tt.jobObject.(k8sruntime.Object)
			}
			if tt.clusterObject != nil {
				clusterObj = tt.clusterObject.(k8sruntime.Object)
			}

			lj, err := m.MapGlobalJobToLocal(jobObj, clusterObj, nil)
			if tt.expectErrSubstr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErrSubstr)
				assert.Nil(t, lj)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, lj)

			if tt.expectedLocalObject != nil {
				require.Equal(t, tt.expectedLocalObject, lj)
			}
		})
	}
}

func TestMapper_MapGlobalJobClusterToLocal(t *testing.T) {
	m := Mapper{Env: env.Context{RuntimeEnvironment: "staging"}}

	headPod := &corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"role": "head"}}}
	workerPod := &corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"role": "worker"}}}

	rayCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "proj1"},
		Spec: v2pb.RayClusterSpec{
			RayVersion: "2.3.1",
			Head: &v2pb.RayHeadSpec{
				ServiceType:    string(corev1.ServiceTypeClusterIP),
				Pod:            headPod,
				RayStartParams: map[string]string{"head": "param"},
			},
			Workers: []*v2pb.RayWorkerSpec{
				{
					Pod:            workerPod,
					MinInstances:   1,
					MaxInstances:   3,
					RayStartParams: map[string]string{"worker": "param"},
				},
			},
		},
	}

	// Helper variables for expected object
	minReplicas := int32(rayCluster.Spec.Workers[0].MinInstances)
	maxReplicas := int32(rayCluster.Spec.Workers[0].MaxInstances)

	tests := []struct {
		name                string
		clusterObject       any
		expectErrSubstr     string
		expectedLocalObject k8sruntime.Object
	}{
		{
			name:          "ray cluster -> cluster mapped",
			clusterObject: rayCluster,
			expectedLocalObject: &rayv1.RayCluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       RayClusterKind,
					APIVersion: RayAPIVersion,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      rayCluster.Name,
					Namespace: RayLocalNamespace,
					// Routing labels: how the federated watchers find this
					// object and map its events back to the RayCluster CR.
					Labels: map[string]string{
						"ma/owner-service":     "michelangelo-ray",
						"ma/control-plane-env": "staging",
						"ma/project-name":      "proj1",
					},
				},
				Spec: rayv1.RayClusterSpec{
					HeadGroupSpec: rayv1.HeadGroupSpec{
						ServiceType:    corev1.ServiceType(rayCluster.Spec.Head.ServiceType),
						RayStartParams: rayCluster.Spec.Head.RayStartParams,
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								// KubeRay copies these onto the head pod, which
								// is the only way the pod watch can see it.
								Labels: withControlPlaneLabels(headPod.Labels),
							},
						},
					},
					RayVersion: rayCluster.Spec.RayVersion,
					WorkerGroupSpecs: []rayv1.WorkerGroupSpec{
						{
							GroupName:      RayWorkerNodePrefix + rayCluster.Name,
							Replicas:       &minReplicas,
							MinReplicas:    &minReplicas,
							MaxReplicas:    &maxReplicas,
							RayStartParams: rayCluster.Spec.Workers[0].RayStartParams,
							Template: corev1.PodTemplateSpec{
								ObjectMeta: metav1.ObjectMeta{
									Labels: withControlPlaneLabels(workerPod.Labels),
								},
							},
						},
					},
				},
			},
		},
		{
			name:            "unsupported cluster object type -> error",
			clusterObject:   &v2pb.SparkJob{},
			expectErrSubstr: "unsupported cluster object type",
		},
		{
			name:            "nil cluster object -> error",
			clusterObject:   nil,
			expectErrSubstr: "jobClusterObject cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var clusterObj k8sruntime.Object
			if tt.clusterObject != nil {
				clusterObj = tt.clusterObject.(k8sruntime.Object)
			}

			lc, err := m.MapGlobalJobClusterToLocal(clusterObj, nil)
			if tt.expectErrSubstr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErrSubstr)
				assert.Nil(t, lc)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, lc)

			if tt.expectedLocalObject != nil {
				require.Equal(t, tt.expectedLocalObject, lc)
			}
		})
	}
}

// TestGetHeadGroupSpec_RayStartParams verifies rayStartParams handling: an unset
// (nil) map becomes a non-nil empty map so it serializes to `{}` rather than
// `null` (which the RayCluster CRD rejects — see nonNilRayStartParams), while a
// populated map is passed through unchanged.
func TestGetHeadGroupSpec_RayStartParams(t *testing.T) {
	t.Run("nil -> empty non-nil map", func(t *testing.T) {
		got := getHeadGroupSpec(&v2pb.RayHeadSpec{ServiceType: string(corev1.ServiceTypeClusterIP)})
		require.NotNil(t, got.RayStartParams)
		assert.Equal(t, map[string]string{}, got.RayStartParams)
	})
	t.Run("populated -> preserved", func(t *testing.T) {
		got := getHeadGroupSpec(&v2pb.RayHeadSpec{RayStartParams: map[string]string{"dashboard-host": "0.0.0.0"}})
		assert.Equal(t, map[string]string{"dashboard-host": "0.0.0.0"}, got.RayStartParams)
	})
}

// TestGetWorkerGroupSpecs_RayStartParams mirrors the head-group check for worker
// groups: nil rayStartParams must map to a non-nil empty map, populated is kept.
func TestGetWorkerGroupSpecs_RayStartParams(t *testing.T) {
	t.Run("nil -> empty non-nil map", func(t *testing.T) {
		got := getWorkerGroupSpecs("test-cluster", []*v2pb.RayWorkerSpec{{MinInstances: 1, MaxInstances: 2}})
		require.Len(t, got, 1)
		require.NotNil(t, got[0].RayStartParams)
		assert.Equal(t, map[string]string{}, got[0].RayStartParams)
	})
	t.Run("populated -> preserved", func(t *testing.T) {
		got := getWorkerGroupSpecs("test-cluster", []*v2pb.RayWorkerSpec{{
			MinInstances:   1,
			MaxInstances:   2,
			RayStartParams: map[string]string{"metrics-export-port": "8080"},
		}})
		require.Len(t, got, 1)
		assert.Equal(t, map[string]string{"metrics-export-port": "8080"}, got[0].RayStartParams)
	})
}

func TestMapper_GetLocalName(t *testing.T) {
	m := Mapper{}

	tests := []struct {
		name    string
		obj     any
		expNS   string
		expName string
	}{
		{
			name:    "ray job -> returns namespace and name",
			obj:     &v2pb.RayJob{ObjectMeta: metav1.ObjectMeta{Name: "ray-1"}},
			expNS:   RayLocalNamespace,
			expName: "ray-1",
		},
		{
			name:    "spark job -> returns namespace and name",
			obj:     &v2pb.SparkJob{ObjectMeta: metav1.ObjectMeta{Name: "spark-1"}},
			expNS:   SparkLocalNamespace,
			expName: "spark-1",
		},
		{
			name:    "unknown type -> empty namespace and name",
			obj:     &struct{}{},
			expNS:   "",
			expName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var obj k8sruntime.Object
			switch v := tt.obj.(type) {
			case k8sruntime.Object:
				obj = v
			default:
				// non-runtime.Object types
			}
			ns, name := m.GetLocalName(obj)
			assert.Equal(t, tt.expNS, ns)
			assert.Equal(t, tt.expName, name)
		})
	}
}

func TestMapper_MapLocalJobStatusToGlobal(t *testing.T) {
	m := Mapper{}

	mkRayV1 := func(jobStatus rayv1.JobStatus) *rayv1.RayJob {
		r := &rayv1.RayJob{}
		r.Status.JobStatus = jobStatus
		return r
	}

	tests := []struct {
		name         string
		job          k8sruntime.Object
		expectStatus string
		expectMsg    string
	}{
		{
			name:         "running RayJob -> RUNNING",
			job:          mkRayV1(rayv1.JobStatusRunning),
			expectStatus: string(rayv1.JobStatusRunning),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			js, err := m.MapLocalJobStatusToGlobal(tt.job)
			require.NoError(t, err)
			require.NotNil(t, js)
			require.NotNil(t, js.Ray)
			assert.Equal(t, tt.expectStatus, js.Ray.JobStatus)
			assert.Equal(t, tt.expectMsg, js.Ray.Message)
		})
	}
}

func TestMapLocalClusterStatusToGlobal_WithConditions(t *testing.T) {
	m := Mapper{}

	tests := []struct {
		name            string
		kubeRayState    rayv1.ClusterState
		statusReason    string
		suspend         *bool
		conditions      []metav1.Condition
		expectState     v2pb.RayClusterState
		expectPodErrors int
		expectReason    string
	}{
		{
			name:         "no conditions uses deprecated Status.Reason",
			kubeRayState: rayv1.Ready,
			statusReason: "AllReady",
			expectState:  v2pb.RAY_CLUSTER_STATE_READY,
			expectReason: "AllReady",
		},
		{
			name:         "HeadPodReady=False with CrashLoopBackOff",
			kubeRayState: "",
			conditions: []metav1.Condition{
				{
					Type:    string(rayv1.HeadPodReady),
					Status:  metav1.ConditionFalse,
					Reason:  "CrashLoopBackOff",
					Message: "container ray-head is crashing",
				},
			},
			expectState:     v2pb.RAY_CLUSTER_STATE_UNKNOWN,
			expectPodErrors: 1,
			expectReason:    "CrashLoopBackOff",
		},
		{
			name:         "ReplicaFailure=True with FailedCreateHeadPod",
			kubeRayState: "",
			conditions: []metav1.Condition{
				{
					Type:    string(rayv1.RayClusterReplicaFailure),
					Status:  metav1.ConditionTrue,
					Reason:  "FailedCreateHeadPod",
					Message: "quota exceeded",
				},
			},
			expectState:     v2pb.RAY_CLUSTER_STATE_UNKNOWN,
			expectPodErrors: 1,
			expectReason:    "FailedCreateHeadPod",
		},
		{
			name:         "HeadPodReady=False with RayClusterPodsProvisioning is not a failure",
			kubeRayState: "",
			conditions: []metav1.Condition{
				{
					Type:    string(rayv1.HeadPodReady),
					Status:  metav1.ConditionFalse,
					Reason:  rayv1.RayClusterPodsProvisioning,
					Message: "pods are being created",
				},
			},
			expectState:     v2pb.RAY_CLUSTER_STATE_UNKNOWN,
			expectPodErrors: 0,
			expectReason:    "",
		},
		{
			name:         "Suspended state maps to SUSPENDED",
			kubeRayState: rayv1.Suspended,
			expectState:  v2pb.RAY_CLUSTER_STATE_SUSPENDED,
			expectReason: "ClusterSuspended",
		},
		{
			name:         "spec.suspend alone maps to SUSPENDED before status catches up",
			kubeRayState: "",
			suspend:      k8sptr.To(true),
			expectState:  v2pb.RAY_CLUSTER_STATE_SUSPENDED,
			expectReason: "ClusterSuspended",
		},
		{
			name:         "spec.suspend=false does not suspend",
			kubeRayState: rayv1.Ready,
			suspend:      k8sptr.To(false),
			expectState:  v2pb.RAY_CLUSTER_STATE_READY,
		},
		{
			// The Kueue admission-gating window: pods are intentionally absent,
			// so HeadPodNotFound must not surface as a (terminal) pod error.
			name:         "suspended cluster suppresses HeadPodNotFound pod errors",
			kubeRayState: rayv1.Suspended,
			suspend:      k8sptr.To(true),
			conditions: []metav1.Condition{
				{
					Type:    string(rayv1.HeadPodReady),
					Status:  metav1.ConditionFalse,
					Reason:  "HeadPodNotFound",
					Message: "head pod not found",
				},
			},
			expectState:     v2pb.RAY_CLUSTER_STATE_SUSPENDED,
			expectPodErrors: 0,
			expectReason:    "ClusterSuspended",
		},
		{
			name:         "suspended cluster suppresses ReplicaFailure pod errors",
			kubeRayState: rayv1.Suspended,
			conditions: []metav1.Condition{
				{
					Type:   string(rayv1.RayClusterReplicaFailure),
					Status: metav1.ConditionTrue,
					Reason: "FailedDeleteHeadPod",
				},
			},
			expectState:     v2pb.RAY_CLUSTER_STATE_SUSPENDED,
			expectPodErrors: 0,
			expectReason:    "ClusterSuspended",
		},
		{
			name:         "RayClusterSuspending condition maps to SUSPENDED with suspending reason",
			kubeRayState: "",
			conditions: []metav1.Condition{
				{
					Type:   rayClusterSuspendingConditionType,
					Status: metav1.ConditionTrue,
					Reason: "RayClusterSuspending",
				},
			},
			expectState:  v2pb.RAY_CLUSTER_STATE_SUSPENDED,
			expectReason: "ClusterSuspending",
		},
		{
			name:         "RayClusterSuspended condition maps to SUSPENDED",
			kubeRayState: "",
			conditions: []metav1.Condition{
				{
					Type:   rayClusterSuspendedConditionType,
					Status: metav1.ConditionTrue,
					Reason: "RayClusterSuspended",
				},
			},
			expectState:  v2pb.RAY_CLUSTER_STATE_SUSPENDED,
			expectReason: "ClusterSuspended",
		},
		{
			// Regression guard: once a cluster is NOT suspended, HeadPodNotFound
			// is a real failure again.
			name:         "HeadPodNotFound without suspension stays a failure",
			kubeRayState: "",
			conditions: []metav1.Condition{
				{
					Type:    string(rayv1.HeadPodReady),
					Status:  metav1.ConditionFalse,
					Reason:  "HeadPodNotFound",
					Message: "head pod not found",
				},
			},
			expectState:     v2pb.RAY_CLUSTER_STATE_UNKNOWN,
			expectPodErrors: 1,
			expectReason:    "HeadPodNotFound",
		},
		{
			name:         "condition reason takes priority over deprecated Status.Reason",
			kubeRayState: "",
			statusReason: "DeprecatedReason",
			conditions: []metav1.Condition{
				{
					Type:    string(rayv1.HeadPodReady),
					Status:  metav1.ConditionFalse,
					Reason:  "ImagePullBackOff",
					Message: "cannot pull image",
				},
			},
			expectState:     v2pb.RAY_CLUSTER_STATE_UNKNOWN,
			expectPodErrors: 1,
			expectReason:    "ImagePullBackOff",
		},
		{
			name:         "ReplicaFailure reason has priority over HeadPodReady reason",
			kubeRayState: "",
			conditions: []metav1.Condition{
				{
					Type:   string(rayv1.HeadPodReady),
					Status: metav1.ConditionFalse,
					Reason: "CrashLoopBackOff",
				},
				{
					Type:   string(rayv1.RayClusterReplicaFailure),
					Status: metav1.ConditionTrue,
					Reason: "FailedCreateWorkerPod",
				},
			},
			expectState:     v2pb.RAY_CLUSTER_STATE_UNKNOWN,
			expectPodErrors: 2,
			expectReason:    "FailedCreateWorkerPod",
		},
		{
			name:         "HeadPodReady=False with ContainersNotReady (actual KubeRay reason)",
			kubeRayState: "",
			conditions: []metav1.Condition{
				{
					Type:    string(rayv1.HeadPodReady),
					Status:  metav1.ConditionFalse,
					Reason:  "ContainersNotReady",
					Message: "containers with unready status: [head]",
				},
			},
			expectState:     v2pb.RAY_CLUSTER_STATE_UNKNOWN,
			expectPodErrors: 1,
			expectReason:    "ContainersNotReady",
		},
		{
			name:         "HeadPodReady=True is not a failure",
			kubeRayState: rayv1.Ready,
			conditions: []metav1.Condition{
				{
					Type:   string(rayv1.HeadPodReady),
					Status: metav1.ConditionTrue,
					Reason: rayv1.HeadPodRunningAndReady,
				},
			},
			expectState:     v2pb.RAY_CLUSTER_STATE_READY,
			expectPodErrors: 0,
			expectReason:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rayCluster := &rayv1.RayCluster{
				Spec: rayv1.RayClusterSpec{
					Suspend: tt.suspend,
				},
				Status: rayv1.RayClusterStatus{
					State:      tt.kubeRayState,
					Reason:     tt.statusReason,
					Conditions: tt.conditions,
				},
			}
			result, err := m.MapLocalClusterStatusToGlobal(rayCluster)
			require.NoError(t, err)
			require.NotNil(t, result.Ray)

			assert.Equal(t, tt.expectState, result.Ray.State)
			assert.Len(t, result.Ray.PodErrors, tt.expectPodErrors)
			assert.Equal(t, tt.expectReason, result.Reason)
		})
	}
}

func TestControlPlaneLabels(t *testing.T) {
	m := Mapper{Env: env.Context{RuntimeEnvironment: "staging"}}

	t.Run("identifies the owning control plane and namespace", func(t *testing.T) {
		assert.Equal(t, map[string]string{
			"ma/owner-service":     "michelangelo-ray",
			"ma/control-plane-env": "staging",
			"ma/project-name":      "proj1",
		}, m.controlPlaneLabels("proj1", "michelangelo-ray"))
	})

	t.Run("no queue: no queue label", func(t *testing.T) {
		assert.NotContains(t, m.objectLabels("proj1", "michelangelo-ray", ""), "kueue.x-k8s.io/queue-name")
	})

	t.Run("queue set: resolved queue label is added", func(t *testing.T) {
		assert.Equal(t, "ma-proj1", m.objectLabels("proj1", "michelangelo-ray", "ma-proj1")["kueue.x-k8s.io/queue-name"])
	})
}

// The runtime environment is stamped as a label value on every Ray object and
// selected on by every watch, so a value Kubernetes rejects must stop the
// service at startup rather than fail every dispatch.
func TestNewMapper_RuntimeEnvironment(t *testing.T) {
	tests := []struct {
		name               string
		runtimeEnvironment string
		wantPanic          bool
	}{
		{name: "unset", runtimeEnvironment: "", wantPanic: false},
		{name: "valid label value", runtimeEnvironment: "staging", wantPanic: false},
		{name: "contains a slash", runtimeEnvironment: "prod/us", wantPanic: true},
		{name: "longer than 63 characters", runtimeEnvironment: strings.Repeat("a", 64), wantPanic: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newMapper := func() {
				NewMapper(LogPersistenceConfig{}, maconfig.SchedulerConfig{}, env.Context{RuntimeEnvironment: tt.runtimeEnvironment})
			}
			if tt.wantPanic {
				assert.Panics(t, newMapper)
			} else {
				assert.NotPanics(t, newMapper)
			}
		})
	}
}

func TestMapRayClusterLabels(t *testing.T) {
	m := Mapper{Scheduler: maconfig.SchedulerConfig{}, Env: env.Context{RuntimeEnvironment: "staging"}}
	// Every label here is attacker-controlled: the source CR's labels come from
	// the job author. None of them may reach the compute cluster.
	rc := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rc-1",
			Namespace: "proj1",
			Labels: map[string]string{
				"ma/project-name":               "victim-namespace",
				"ma/owner-service":              "not-michelangelo",
				"ma/control-plane-env":          "production",
				"michelangelo/cluster-affinity": "cluster-a",
				"kueue.x-k8s.io/queue-name":     "user-forged-queue",
			},
		},
		Spec: v2pb.RayClusterSpec{
			Head: &v2pb.RayHeadSpec{},
			Workers: []*v2pb.RayWorkerSpec{
				{Pod: &corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"role": "worker"}},
				}},
			},
		},
	}

	controlPlane := map[string]string{
		"ma/owner-service":     "michelangelo-ray",
		"ma/control-plane-env": "staging",
		"ma/project-name":      "proj1",
	}

	t.Run("non-kueue cluster: control-plane labels only", func(t *testing.T) {
		obj, err := m.MapGlobalJobClusterToLocal(rc, &v2pb.Cluster{})
		require.NoError(t, err)
		assert.Equal(t, controlPlane, obj.(*rayv1.RayCluster).Labels)
	})

	t.Run("nil cluster: control-plane labels only", func(t *testing.T) {
		obj, err := m.MapGlobalJobClusterToLocal(rc, nil)
		require.NoError(t, err)
		assert.Equal(t, controlPlane, obj.(*rayv1.RayCluster).Labels)
	})

	// The pod watch selects on these; KubeRay only propagates what the group
	// templates already carry.
	t.Run("head and worker templates carry the control-plane labels", func(t *testing.T) {
		obj, err := m.MapGlobalJobClusterToLocal(rc, &v2pb.Cluster{})
		require.NoError(t, err)
		local := obj.(*rayv1.RayCluster)

		assert.Equal(t, controlPlane, local.Spec.HeadGroupSpec.Template.Labels)

		require.Len(t, local.Spec.WorkerGroupSpecs, 1)
		worker := local.Spec.WorkerGroupSpecs[0].Template.Labels
		for key, value := range controlPlane {
			assert.Equal(t, value, worker[key])
		}
		assert.Equal(t, "worker", worker["role"], "template's own labels are preserved")
	})

	// The mapped template starts as a shallow copy of the CR's pod spec, so a
	// merge that wrote through its label map would stamp control-plane labels
	// onto the caller's object.
	t.Run("mapping does not label the source CR's pod template", func(t *testing.T) {
		_, err := m.MapGlobalJobClusterToLocal(rc, &v2pb.Cluster{})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"role": "worker"}, rc.Spec.Workers[0].Pod.Labels)
	})

	// A job author can label their own pod templates, and the pod watch routes
	// pod events by ma/project-name, so a template's copy of a control-plane
	// label must lose to the control plane's own value.
	t.Run("control-plane labels override the templates' own", func(t *testing.T) {
		forged := &v2pb.RayCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "rc-4", Namespace: "proj1"},
			Spec: v2pb.RayClusterSpec{
				Head: &v2pb.RayHeadSpec{Pod: &corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"ma/project-name": "victim", "role": "head"}},
				}},
				Workers: []*v2pb.RayWorkerSpec{
					{Pod: &corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"ma/project-name": "victim", "role": "worker"}},
					}},
				},
			},
		}
		obj, err := m.MapGlobalJobClusterToLocal(forged, &v2pb.Cluster{})
		require.NoError(t, err)
		local := obj.(*rayv1.RayCluster)

		head := local.Spec.HeadGroupSpec.Template.Labels
		assert.Equal(t, "proj1", head["ma/project-name"], "the CR's namespace, not the template's forged value")
		assert.Equal(t, "head", head["role"], "the template's other labels are kept")
		assert.Equal(t, withControlPlaneLabels(map[string]string{"role": "head"}), head)

		require.Len(t, local.Spec.WorkerGroupSpecs, 1)
		worker := local.Spec.WorkerGroupSpecs[0].Template.Labels
		assert.Equal(t, "proj1", worker["ma/project-name"], "the CR's namespace, not the template's forged value")
		assert.Equal(t, "worker", worker["role"], "the template's other labels are kept")
		assert.Equal(t, withControlPlaneLabels(map[string]string{"role": "worker"}), worker)
	})

	// Kueue's pod integration would treat labelled pods as separately admissible
	// workloads, so the queue label stays on the RayCluster.
	t.Run("kueue cluster: queue label on the object, not on the pods", func(t *testing.T) {
		kueueCluster := &v2pb.Cluster{
			Spec: v2pb.ClusterSpec{SchedulerType: v2pb.SCHEDULER_TYPE_KUEUE},
		}
		obj, err := m.MapGlobalJobClusterToLocal(rc, kueueCluster)
		require.NoError(t, err)
		local := obj.(*rayv1.RayCluster)

		assert.Contains(t, local.Labels, "kueue.x-k8s.io/queue-name")
		assert.NotContains(t, local.Spec.HeadGroupSpec.Template.Labels, "kueue.x-k8s.io/queue-name")
		require.Len(t, local.Spec.WorkerGroupSpecs, 1)
		assert.NotContains(t, local.Spec.WorkerGroupSpecs[0].Template.Labels, "kueue.x-k8s.io/queue-name")
	})

	// Pins PRE-EXISTING behaviour; it does not endorse it. Queue resolution
	// prefers the CR's own ma/project-name over its namespace, and that label
	// is author-supplied, so a job can be admitted against another project's
	// LocalQueue and quota. Changing the precedence is a separate follow-up;
	// this case only keeps the routing-label work from changing it unnoticed.
	t.Run("pre-existing, forgeable: kueue queue resolution prefers the CR's own ma/project-name", func(t *testing.T) {
		kueueCluster := &v2pb.Cluster{
			Spec: v2pb.ClusterSpec{SchedulerType: v2pb.SCHEDULER_TYPE_KUEUE},
		}
		obj, err := m.MapGlobalJobClusterToLocal(rc, kueueCluster)
		require.NoError(t, err)

		assert.Equal(t, "ma-victim-namespace", obj.(*rayv1.RayCluster).Labels["kueue.x-k8s.io/queue-name"])
	})

	t.Run("routing labels ignore the source CR's own labels", func(t *testing.T) {
		kueueCluster := &v2pb.Cluster{
			Spec: v2pb.ClusterSpec{SchedulerType: v2pb.SCHEDULER_TYPE_KUEUE},
		}
		obj, err := m.MapGlobalJobClusterToLocal(rc, kueueCluster)
		require.NoError(t, err)
		local := obj.(*rayv1.RayCluster)

		assert.Equal(t, "proj1", local.Labels["ma/project-name"],
			"a forged ma/project-name must not redirect the watcher at another namespace")
		assert.Equal(t, "michelangelo-ray", local.Labels["ma/owner-service"])
		assert.Equal(t, "staging", local.Labels["ma/control-plane-env"])
		assert.NotContains(t, local.Labels, "michelangelo/cluster-affinity")
	})

	t.Run("kueue cluster without project label falls back to namespace", func(t *testing.T) {
		unlabeled := &v2pb.RayCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "rc-2", Namespace: "proj-ns"},
			Spec:       v2pb.RayClusterSpec{Head: &v2pb.RayHeadSpec{}},
		}
		kueueCluster := &v2pb.Cluster{
			Spec: v2pb.ClusterSpec{SchedulerType: v2pb.SCHEDULER_TYPE_KUEUE},
		}
		obj, err := m.MapGlobalJobClusterToLocal(unlabeled, kueueCluster)
		require.NoError(t, err)
		assert.Equal(t, "ma-proj-ns", obj.(*rayv1.RayCluster).Labels["kueue.x-k8s.io/queue-name"])
	})

	t.Run("kueue cluster without any project identity fails dispatch", func(t *testing.T) {
		anonymous := &v2pb.RayCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "rc-3"},
			Spec:       v2pb.RayClusterSpec{Head: &v2pb.RayHeadSpec{}},
		}
		kueueCluster := &v2pb.Cluster{
			Spec: v2pb.ClusterSpec{SchedulerType: v2pb.SCHEDULER_TYPE_KUEUE},
		}
		_, err := m.MapGlobalJobClusterToLocal(anonymous, kueueCluster)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "LocalQueue")
	})
}
