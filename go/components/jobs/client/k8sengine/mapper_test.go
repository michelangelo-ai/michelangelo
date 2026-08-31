package k8sengine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	k8sptr "k8s.io/utils/ptr"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestMapper_MapGlobalJobToLocal(t *testing.T) {
	m := Mapper{}

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
		ObjectMeta: metav1.ObjectMeta{Name: "test-job"},
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
			name:            "unsupported job type (spark) -> error",
			jobObject:       &v2pb.SparkJob{},
			clusterObject:   nil,
			expectErrSubstr: "spark job mapping not implemented",
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
	m := Mapper{}

	headPod := &corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"role": "head"}}}
	workerPod := &corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"role": "worker"}}}

	rayCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
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
				},
				Spec: rayv1.RayClusterSpec{
					HeadGroupSpec: rayv1.HeadGroupSpec{
						ServiceType:    corev1.ServiceType(rayCluster.Spec.Head.ServiceType),
						RayStartParams: rayCluster.Spec.Head.RayStartParams,
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: headPod.Labels,
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
									Labels: workerPod.Labels,
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
			name:    "spark job -> empty namespace and name",
			obj:     &v2pb.SparkJob{ObjectMeta: metav1.ObjectMeta{Name: "spark-1"}},
			expNS:   "",
			expName: "",
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

func TestMapLabels(t *testing.T) {
	controlPlaneLabels := map[string]string{
		"michelangelo/cluster-affinity": "cluster-a",
		"ma/project-name":               "proj1",
		"ma/user":                       "someone",
		"kueue.x-k8s.io/queue-name":     "user-forged-queue",
	}

	t.Run("no queue: nothing propagates", func(t *testing.T) {
		assert.Nil(t, mapLabels(controlPlaneLabels, ""))
	})

	t.Run("queue set: only the resolved queue label, never source labels", func(t *testing.T) {
		got := mapLabels(controlPlaneLabels, "ma-proj1")
		assert.Equal(t, map[string]string{"kueue.x-k8s.io/queue-name": "ma-proj1"}, got)
	})
}

func TestMapRayClusterKueueLabels(t *testing.T) {
	m := Mapper{Scheduler: maconfig.SchedulerConfig{}}
	rc := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "rc-1",
			Labels: map[string]string{
				"ma/project-name":               "proj1",
				"michelangelo/cluster-affinity": "cluster-a",
				"kueue.x-k8s.io/queue-name":     "user-forged-queue",
			},
		},
		Spec: v2pb.RayClusterSpec{Head: &v2pb.RayHeadSpec{}},
	}

	t.Run("non-kueue cluster: no labels at all", func(t *testing.T) {
		obj, err := m.MapGlobalJobClusterToLocal(rc, &v2pb.Cluster{})
		require.NoError(t, err)
		assert.Nil(t, obj.(*rayv1.RayCluster).Labels)
	})

	t.Run("nil cluster: no labels at all", func(t *testing.T) {
		obj, err := m.MapGlobalJobClusterToLocal(rc, nil)
		require.NoError(t, err)
		assert.Nil(t, obj.(*rayv1.RayCluster).Labels)
	})

	t.Run("kueue cluster: resolved queue label only", func(t *testing.T) {
		kueueCluster := &v2pb.Cluster{
			Spec: v2pb.ClusterSpec{SchedulerType: v2pb.SCHEDULER_TYPE_KUEUE},
		}
		obj, err := m.MapGlobalJobClusterToLocal(rc, kueueCluster)
		require.NoError(t, err)
		assert.Equal(t,
			map[string]string{"kueue.x-k8s.io/queue-name": "ma-proj1"},
			obj.(*rayv1.RayCluster).Labels)
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
		assert.Equal(t,
			map[string]string{"kueue.x-k8s.io/queue-name": "ma-proj-ns"},
			obj.(*rayv1.RayCluster).Labels)
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
