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

func TestMapper_MapGlobalToLocal(t *testing.T) {
	m := Mapper{}

	headPod := &corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"role": "head"}}}
	workerPod := &corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"role": "worker"}}}

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
		name              string
		jobObject         any
		clusterObject     any
		expectJobType     bool
		expectClusterType bool
		expectErrSubstr   string
		check             func(t *testing.T, job k8sruntime.Object, cluster k8sruntime.Object)
	}{
		{
			name:              "ray job and cluster -> both mapped",
			jobObject:         rayJob,
			clusterObject:     rayCluster,
			expectJobType:     true,
			expectClusterType: true,
			check: func(t *testing.T, job k8sruntime.Object, cluster k8sruntime.Object) {
				rj, ok := job.(*rayv1.RayJob)
				require.True(t, ok)
				assert.Equal(t, rayJob.Name, rj.Name)
				assert.Equal(t, RayLocalNamespace, rj.Namespace)
				assert.Equal(t, RayJobKind, rj.TypeMeta.Kind)
				assert.Equal(t, RayAPIVersion, rj.TypeMeta.APIVersion)
				// ClusterSelector
				assert.Equal(t, rayCluster.Name, rj.Spec.ClusterSelector["ray.io/cluster"])
				assert.Equal(t, RayLocalNamespace, rj.Spec.ClusterSelector["rayClusterNamespace"])
				// Entrypoint
				assert.Equal(t, rayJob.Spec.Entrypoint, rj.Spec.Entrypoint)

				rc, ok := cluster.(*rayv1.RayCluster)
				require.True(t, ok)
				assert.Equal(t, rayCluster.Name, rc.Name)
				assert.Equal(t, RayLocalNamespace, rc.Namespace)
				assert.Equal(t, RayClusterKind, rc.TypeMeta.Kind)
				assert.Equal(t, RayAPIVersion, rc.TypeMeta.APIVersion)
				// Head group
				assert.Equal(t, corev1.ServiceType(rayCluster.Spec.Head.ServiceType), rc.Spec.HeadGroupSpec.ServiceType)
				assert.Equal(t, rayCluster.Spec.Head.RayStartParams, rc.Spec.HeadGroupSpec.RayStartParams)
				assert.Equal(t, headPod.Labels, rc.Spec.HeadGroupSpec.Template.Labels)
				// Worker groups
				require.Len(t, rc.Spec.WorkerGroupSpecs, len(rayCluster.Spec.Workers))
				wg := rc.Spec.WorkerGroupSpecs[0]
				assert.Equal(t, RayWorkerNodePrefix+rayCluster.Name, wg.GroupName)
				require.NotNil(t, wg.Replicas)
				require.NotNil(t, wg.MinReplicas)
				require.NotNil(t, wg.MaxReplicas)
				assert.EqualValues(t, rayCluster.Spec.Workers[0].MinInstances, *wg.Replicas)
				assert.EqualValues(t, rayCluster.Spec.Workers[0].MinInstances, *wg.MinReplicas)
				assert.EqualValues(t, rayCluster.Spec.Workers[0].MaxInstances, *wg.MaxReplicas)
				assert.Equal(t, rayCluster.Spec.Workers[0].RayStartParams, wg.RayStartParams)
				assert.Equal(t, workerPod.Labels, wg.Template.Labels)
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
			name:              "only cluster provided -> cluster mapped",
			jobObject:         nil,
			clusterObject:     rayCluster,
			expectJobType:     false,
			expectClusterType: true,
			check: func(t *testing.T, job k8sruntime.Object, cluster k8sruntime.Object) {
				assert.Nil(t, job)
				_, ok := cluster.(*rayv1.RayCluster)
				require.True(t, ok)
			},
		},
		{
			name:            "unsupported cluster object type -> error",
			jobObject:       nil,
			clusterObject:   &v2pb.SparkJob{},
			expectErrSubstr: "unsupported cluster object type",
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

			lj, lc, err := m.MapGlobalToLocal(jobObj, clusterObj, nil)
			if tt.expectErrSubstr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErrSubstr)
				assert.Nil(t, lj)
				assert.Nil(t, lc)
				return
			}

			require.NoError(t, err)
			if tt.expectJobType {
				_, ok := lj.(*rayv1.RayJob)
				require.True(t, ok)
			} else {
				assert.Nil(t, lj)
			}
			if tt.expectClusterType {
				_, ok := lc.(*rayv1.RayCluster)
				require.True(t, ok)
			} else if lc != nil {
				// some tests still pass cluster to map job; allow both when provided
				_, ok := lc.(*rayv1.RayCluster)
				require.True(t, ok)
			}

			if tt.check != nil {
				tt.check(t, lj, lc)
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
