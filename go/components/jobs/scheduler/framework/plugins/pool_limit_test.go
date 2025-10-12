package plugins

import (
	"context"
	"testing"

	computecommonconstants "code.uber.internal/infra/compute/compute-common/constants"
	infraCrds "code.uber.internal/infra/compute/k8s-crds/apis/compute.uber.com/v1beta2"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/cluster"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/scheduler/framework"
	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v2beta1 "michelangelo/api/v2beta1"
)

func TestName(t *testing.T) {
	p := PoolLimitFilter{}
	require.Equal(t, "PoolLimitFilter", p.Name())
}

func TestFilter(t *testing.T) {
	tt := []struct {
		msg           string
		job           framework.BatchJob
		candidates    []*cluster.ResourcePoolInfo
		filteredPools []string
	}{
		{
			msg: "GPU job",
			job: framework.BatchRayJob{
				RayJob: &v2beta1.RayJob{
					Spec: v2beta1.RayJobSpec{
						Head: &v2beta1.HeadSpec{
							Pod: &v2beta1.PodSpec{
								Resource: &v2beta1.ResourceSpec{
									Cpu:      4,
									Memory:   "100G",
									DiskSize: "200G",
									Gpu:      3,
								},
							},
						},
					},
				},
			},
			candidates: []*cluster.ResourcePoolInfo{
				{
					Pool: infraCrds.ResourcePool{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pool1",
							Namespace: "test-ns",
						},
						Spec: infraCrds.ResourcePoolSpec{
							Resources: []infraCrds.ResourceConfig{
								{
									Kind:        corev1.ResourceCPU.String(),
									Limit:       *resource.NewQuantity(100, resource.DecimalSI),
									Reservation: *resource.NewQuantity(50, resource.DecimalSI),
								},
								{
									Kind:        constants.ResourceNvidiaGPU.String(),
									Limit:       *resource.NewQuantity(10, resource.DecimalSI),
									Reservation: *resource.NewQuantity(5, resource.DecimalSI),
								},
								{
									Kind:        corev1.ResourceMemory.String(),
									Limit:       *resource.NewScaledQuantity(100, 9),
									Reservation: *resource.NewScaledQuantity(100, 9),
								},
								{
									Kind:        corev1.ResourceEphemeralStorage.String(),
									Limit:       *resource.NewScaledQuantity(200, 9),
									Reservation: *resource.NewScaledQuantity(200, 9),
								},
							},
						},
					},
				},
				{
					Pool: infraCrds.ResourcePool{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pool2",
							Namespace: "test-ns",
						},
						Spec: infraCrds.ResourcePoolSpec{
							Resources: []infraCrds.ResourceConfig{
								{
									Kind:        corev1.ResourceCPU.String(),
									Limit:       *resource.NewQuantity(100, resource.DecimalSI),
									Reservation: *resource.NewQuantity(50, resource.DecimalSI),
								},
								{
									Kind:        constants.ResourceNvidiaGPU.String(),
									Limit:       *resource.NewQuantity(10, resource.DecimalSI),
									Reservation: *resource.NewQuantity(5, resource.DecimalSI),
								},
								{
									Kind:        corev1.ResourceMemory.String(),
									Limit:       *resource.NewScaledQuantity(90, 9),
									Reservation: *resource.NewScaledQuantity(90, 9),
								},
								{
									Kind:        corev1.ResourceEphemeralStorage.String(),
									Limit:       *resource.NewScaledQuantity(200, 9),
									Reservation: *resource.NewScaledQuantity(200, 9),
								},
							},
						},
					},
				},
				{
					Pool: infraCrds.ResourcePool{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pool3",
							Namespace: "test-ns",
						},
						Spec: infraCrds.ResourcePoolSpec{
							Resources: []infraCrds.ResourceConfig{
								{
									Kind:        corev1.ResourceCPU.String(),
									Limit:       *resource.NewQuantity(100, resource.DecimalSI),
									Reservation: *resource.NewQuantity(50, resource.DecimalSI),
								},
								{
									Kind:        corev1.ResourceMemory.String(),
									Limit:       *resource.NewScaledQuantity(100, 9),
									Reservation: *resource.NewScaledQuantity(100, 9),
								},
								{
									Kind:        corev1.ResourceEphemeralStorage.String(),
									Limit:       *resource.NewScaledQuantity(200, 9),
									Reservation: *resource.NewScaledQuantity(200, 9),
								},
							},
						},
					},
				},
			},
			filteredPools: []string{"pool1"},
		},
		{
			msg: "CPU job",
			job: framework.BatchRayJob{
				RayJob: &v2beta1.RayJob{
					Spec: v2beta1.RayJobSpec{
						Head: &v2beta1.HeadSpec{
							Pod: &v2beta1.PodSpec{
								Resource: &v2beta1.ResourceSpec{
									Cpu:      4,
									Memory:   "100G",
									DiskSize: "200G",
								},
							},
						},
					},
				},
			},
			candidates: []*cluster.ResourcePoolInfo{
				{
					Pool: infraCrds.ResourcePool{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pool1",
							Namespace: "test-ns",
						},
						Spec: infraCrds.ResourcePoolSpec{
							Resources: []infraCrds.ResourceConfig{
								{
									Kind:        corev1.ResourceCPU.String(),
									Limit:       *resource.NewQuantity(100, resource.DecimalSI),
									Reservation: *resource.NewQuantity(50, resource.DecimalSI),
								},
								{
									Kind:        constants.ResourceNvidiaGPU.String(),
									Limit:       *resource.NewQuantity(10, resource.DecimalSI),
									Reservation: *resource.NewQuantity(5, resource.DecimalSI),
								},
								{
									Kind:        corev1.ResourceMemory.String(),
									Limit:       *resource.NewScaledQuantity(100, 9),
									Reservation: *resource.NewScaledQuantity(100, 9),
								},
								{
									Kind:        corev1.ResourceEphemeralStorage.String(),
									Limit:       *resource.NewScaledQuantity(200, 9),
									Reservation: *resource.NewScaledQuantity(200, 9),
								},
							},
						},
					},
				},
				{
					Pool: infraCrds.ResourcePool{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pool2",
							Namespace: "test-ns",
						},
						Spec: infraCrds.ResourcePoolSpec{
							Resources: []infraCrds.ResourceConfig{
								{
									Kind:        corev1.ResourceCPU.String(),
									Limit:       *resource.NewQuantity(100, resource.DecimalSI),
									Reservation: *resource.NewQuantity(50, resource.DecimalSI),
								},
								{
									Kind:        constants.ResourceNvidiaGPU.String(),
									Limit:       *resource.NewQuantity(10, resource.DecimalSI),
									Reservation: *resource.NewQuantity(5, resource.DecimalSI),
								},
								{
									Kind:        corev1.ResourceMemory.String(),
									Limit:       *resource.NewScaledQuantity(90, 9),
									Reservation: *resource.NewScaledQuantity(90, 9),
								},
								{
									Kind:        corev1.ResourceEphemeralStorage.String(),
									Limit:       *resource.NewScaledQuantity(200, 9),
									Reservation: *resource.NewScaledQuantity(200, 9),
								},
							},
						},
					},
				},
				{
					Pool: infraCrds.ResourcePool{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pool3",
							Namespace: "test-ns",
						},
						Spec: infraCrds.ResourcePoolSpec{
							Resources: []infraCrds.ResourceConfig{
								{
									Kind:        corev1.ResourceCPU.String(),
									Limit:       *resource.NewQuantity(100, resource.DecimalSI),
									Reservation: *resource.NewQuantity(50, resource.DecimalSI),
								},
								{
									Kind:        corev1.ResourceMemory.String(),
									Limit:       *resource.NewScaledQuantity(100, 9),
									Reservation: *resource.NewScaledQuantity(100, 9),
								},
								{
									Kind:        corev1.ResourceEphemeralStorage.String(),
									Limit:       *resource.NewScaledQuantity(200, 9),
									Reservation: *resource.NewScaledQuantity(200, 9),
								},
							},
						},
					},
				},
			},
			filteredPools: []string{"pool1", "pool3"},
		},
		{
			msg: "Ray job with v2 ResourceMap - GPU SKU specified",
			job: framework.BatchRayJob{
				RayJob: &v2beta1.RayJob{
					Spec: v2beta1.RayJobSpec{
						Head: &v2beta1.HeadSpec{
							Pod: &v2beta1.PodSpec{
								Resource: &v2beta1.ResourceSpec{
									Cpu:    4,
									Memory: "50G",
									Gpu:    1,
									GpuSku: "A100",
								},
							},
						},
					},
				},
			},
			candidates: []*cluster.ResourcePoolInfo{
				{
					Pool: infraCrds.ResourcePool{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "v2-pool-with-gpu",
							Namespace: "test-ns",
						},
						Spec: infraCrds.ResourcePoolSpec{
							ResourceMap: map[string]infraCrds.ResourceConfiguration{
								"A100": {
									Resources: []infraCrds.ResourceConfig{
										{
											Kind:  corev1.ResourceCPU.String(),
											Limit: *resource.NewQuantity(100, resource.DecimalSI),
										},
										{
											Kind:  constants.ResourceNvidiaGPU.String(),
											Limit: *resource.NewQuantity(10, resource.DecimalSI),
										},
										{
											Kind:  corev1.ResourceMemory.String(),
											Limit: *resource.NewScaledQuantity(100, 9),
										},
										{
											Kind:  corev1.ResourceEphemeralStorage.String(),
											Limit: *resource.NewScaledQuantity(100, 9),
										},
									},
								},
							},
						},
					},
				},
				{
					Pool: infraCrds.ResourcePool{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "v2-pool-cpu-only",
							Namespace: "test-ns",
						},
						Spec: infraCrds.ResourcePoolSpec{
							ResourceMap: map[string]infraCrds.ResourceConfiguration{
								computecommonconstants.DefaultCPU: {
									Resources: []infraCrds.ResourceConfig{
										{
											Kind:  corev1.ResourceCPU.String(),
											Limit: *resource.NewQuantity(100, resource.DecimalSI),
										},
									},
								},
							},
						},
					},
				},
			},
			filteredPools: []string{"v2-pool-with-gpu"}, // Only GPU pool should match
		},
		{
			msg: "Ray job with v2 ResourceMap - exceeds CPU limit",
			job: framework.BatchRayJob{
				RayJob: &v2beta1.RayJob{
					Spec: v2beta1.RayJobSpec{
						Head: &v2beta1.HeadSpec{
							Pod: &v2beta1.PodSpec{
								Resource: &v2beta1.ResourceSpec{
									Cpu:    200, // Exceeds limit of 100
									Memory: "50G",
									Gpu:    1,
									GpuSku: "A100",
								},
							},
						},
					},
				},
			},
			candidates: []*cluster.ResourcePoolInfo{
				{
					Pool: infraCrds.ResourcePool{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "v2-pool-limited",
							Namespace: "test-ns",
						},
						Spec: infraCrds.ResourcePoolSpec{
							ResourceMap: map[string]infraCrds.ResourceConfiguration{
								"A100": {
									Resources: []infraCrds.ResourceConfig{
										{
											Kind:  corev1.ResourceCPU.String(),
											Limit: *resource.NewQuantity(100, resource.DecimalSI), // Lower than job requirement
										},
										{
											Kind:  constants.ResourceNvidiaGPU.String(),
											Limit: *resource.NewQuantity(10, resource.DecimalSI),
										},
									},
								},
							},
						},
					},
				},
			},
			filteredPools: nil, // Should be filtered out due to CPU limit
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			opts := framework.NewOptionBuilder()
			opts.Build(framework.WithLogger(zapr.NewLogger(zaptest.NewLogger(t))))
			pl := PoolLimitFilter{
				OptionBuilder: opts,
			}
			result, err := pl.Filter(context.TODO(), test.job, test.candidates)
			require.NoError(t, err)
			var poolNames []string
			for _, pool := range result {
				poolNames = append(poolNames, pool.Pool.Name)
			}
			require.Equal(t, test.filteredPools, poolNames)
		})
	}
}
