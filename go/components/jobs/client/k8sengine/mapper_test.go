package k8sengine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

func TestMapper_MapGlobalJobToLocal(t *testing.T) {
	m := Mapper{}

	headPod := &corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"role": "head"}}}
	workerPod := &corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"role": "worker"}}}
	submitterPod := headPod.DeepCopy()
	submitterPod.Spec.RestartPolicy = corev1.RestartPolicyNever

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
					SubmitterPodTemplate: submitterPod,
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
