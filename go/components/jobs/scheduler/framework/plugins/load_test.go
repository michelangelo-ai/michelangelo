package plugins

import (
	"context"
	"testing"

	infraCrds "code.uber.internal/infra/compute/k8s-crds/apis/compute.uber.com/v1beta1"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/cluster"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/scheduler/framework"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v2beta1pb "michelangelo/api/v2beta1"
)

var _testPools = []*cluster.ResourcePoolInfo{
	{
		Pool: infraCrds.ResourcePool{
			ObjectMeta: metav1.ObjectMeta{
				Name: "One",
			},
			Spec: infraCrds.ResourcePoolSpec{
				Resources: []infraCrds.ResourceConfig{
					{
						Kind:        corev1.ResourceCPU.String(),
						Reservation: *resource.NewQuantity(200, resource.DecimalSI),
					},
					{
						Kind:        corev1.ResourceMemory.String(),
						Reservation: *resource.NewScaledQuantity(100, 9),
					},
					{
						Kind:        corev1.ResourceEphemeralStorage.String(),
						Reservation: *resource.NewQuantity(130, resource.DecimalSI),
					},
				},
			},
			Status: infraCrds.ResourcePoolStatus{
				Usage: corev1.ResourceList{
					// 190 available
					corev1.ResourceCPU: *resource.NewQuantity(10, resource.DecimalSI),
					// 60 available
					corev1.ResourceMemory: *resource.NewScaledQuantity(40, 9),
					// 20 available
					corev1.ResourceEphemeralStorage: *resource.NewQuantity(110, resource.DecimalSI),
				},
			},
		},
	},
	{
		Pool: infraCrds.ResourcePool{
			ObjectMeta: metav1.ObjectMeta{
				Name: "Two",
			},
			Spec: infraCrds.ResourcePoolSpec{
				Resources: []infraCrds.ResourceConfig{
					{
						Kind:        corev1.ResourceCPU.String(),
						Reservation: *resource.NewQuantity(500, resource.DecimalSI),
					},
					{
						Kind:        corev1.ResourceMemory.String(),
						Reservation: *resource.NewScaledQuantity(100, 9),
					},
					{
						Kind:        constants.ResourceNvidiaGPU.String(),
						Reservation: *resource.NewQuantity(50, resource.DecimalSI),
					},
					{
						Kind:        corev1.ResourceEphemeralStorage.String(),
						Reservation: *resource.NewQuantity(120, resource.DecimalSI),
					},
				},
			},
			Status: infraCrds.ResourcePoolStatus{
				Usage: corev1.ResourceList{
					// 50 available
					corev1.ResourceCPU: *resource.NewQuantity(450, resource.DecimalSI),
					// 50 available
					corev1.ResourceMemory: *resource.NewScaledQuantity(50, 9),
					// 30 available
					constants.ResourceNvidiaGPU: *resource.NewQuantity(20, resource.DecimalSI),
					// 30 available
					corev1.ResourceEphemeralStorage: *resource.NewQuantity(90, resource.DecimalSI),
				},
			},
		},
	},
	{
		Pool: infraCrds.ResourcePool{
			ObjectMeta: metav1.ObjectMeta{
				Name: "Three",
			},
			Spec: infraCrds.ResourcePoolSpec{
				Resources: []infraCrds.ResourceConfig{
					{
						Kind:        corev1.ResourceCPU.String(),
						Reservation: *resource.NewQuantity(500, resource.DecimalSI),
					},
					{
						Kind:        corev1.ResourceMemory.String(),
						Reservation: *resource.NewScaledQuantity(100, 9),
					},
					{
						Kind:        constants.ResourceNvidiaGPU.String(),
						Reservation: *resource.NewQuantity(100, resource.DecimalSI),
					},
					{
						Kind:        corev1.ResourceEphemeralStorage.String(),
						Reservation: *resource.NewQuantity(100, resource.DecimalSI),
					},
				},
			},
			Status: infraCrds.ResourcePoolStatus{
				Usage: corev1.ResourceList{
					// 100 available
					corev1.ResourceCPU: *resource.NewQuantity(400, resource.DecimalSI),
					// 80 available
					corev1.ResourceMemory: *resource.NewScaledQuantity(20, 9),
					// 20 available
					constants.ResourceNvidiaGPU: *resource.NewQuantity(80, resource.DecimalSI),
					// 40 available
					corev1.ResourceEphemeralStorage: *resource.NewQuantity(60, resource.DecimalSI),
				},
			},
		},
	},
}

func TestSortCandidatesByPolicy(t *testing.T) {
	tt := []struct {
		policy        scoringPolicy
		expectedOrder []string
		msg           string
	}{
		{
			policy:        scoreByAvailableCPU,
			expectedOrder: []string{"One", "Three", "Two"},
			msg:           "ScoreByAvailableCPU",
		},
		{
			policy:        scoreByAvailableMemory,
			expectedOrder: []string{"Three", "One", "Two"},
			msg:           "ScoreByAvailableMemory",
		},
		{
			policy:        scoreByAvailableGPU,
			expectedOrder: []string{"Two", "Three", "One"},
			msg:           "ScoreByAvailableGPU",
		},
		{
			policy:        scoreByAvailableDiskSize,
			expectedOrder: []string{"Three", "Two", "One"},
			msg:           "ScoreByAvailableDiskSize",
		},
	}
	headPodSpec := &v2beta1pb.PodSpec{
		Resource: &v2beta1pb.ResourceSpec{
			Cpu:      1,
			Gpu:      1,
			GpuSku:   "a100",
			Memory:   "1",
			DiskSize: "150",
		},
	}
	workerPodSpec := &v2beta1pb.PodSpec{
		Resource: &v2beta1pb.ResourceSpec{
			Cpu:      1,
			Gpu:      1,
			GpuSku:   "a100",
			Memory:   "1",
			DiskSize: "150",
		},
	}
	testJob := framework.BatchRayJob{
		RayJob: &v2beta1pb.RayJob{
			Spec: v2beta1pb.RayJobSpec{
				Head: &v2beta1pb.HeadSpec{
					Pod: headPodSpec,
				},
				Worker: &v2beta1pb.WorkerSpec{
					Pod:          workerPodSpec,
					MaxInstances: int32(1),
				},
			},
		}}
	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			l := LoadScorer{
				OptionBuilder: framework.NewOptionBuilder(),
			}
			result, err := l.sortCandidatesByPolicy(testJob, _testPools, test.policy)
			require.NoError(t, err)
			require.Equal(t, len(test.expectedOrder), len(result))

			for i, poolName := range test.expectedOrder {
				require.Equal(t, poolName, result[i].Pool.Name)
			}
		})
	}
}

func TestScore(t *testing.T) {
	tt := []struct {
		msg           string
		job           framework.BatchJob
		candidates    []*cluster.ResourcePoolInfo
		expectedOrder []string
	}{
		{
			msg: "Job fits all resource pools, resource pools are sorted by MEM dominant resource",
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:      1,
									Memory:   "1",
									DiskSize: "1",
								},
							},
						},
						Worker: &v2beta1pb.WorkerSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:      1,
									Memory:   "1",
									DiskSize: "1",
								},
							},
							MaxInstances: int32(1),
						},
					},
				}},
			candidates:    _testPools,
			expectedOrder: []string{"Three", "One", "Two"},
		},
		{
			msg: "Job doesn't fit any resource pools, resource pools are sorted by MEM dominant resource",
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:      100000, // large CPU wont fit anywhere
									Gpu:      1,
									GpuSku:   "a100",
									Memory:   "1",
									DiskSize: "150",
								},
							},
						},
						Worker: &v2beta1pb.WorkerSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:      1,
									Gpu:      1,
									GpuSku:   "a100",
									Memory:   "1",
									DiskSize: "150",
								},
							},
							MaxInstances: int32(1),
						},
					},
				}},
			candidates:    _testPools,
			expectedOrder: []string{"Three", "One", "Two"},
		},
		{
			// Tests if there is one resource pools where the job can fit then it sorted before the other pools.
			msg: "Job fits resource pool 'one' and the rest of resource pools are sorted by MEM dominant resource",
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:      150, // only RP "one" has enough CPUs
									Memory:   "1",
									DiskSize: "1",
								},
							},
						},
						Worker: &v2beta1pb.WorkerSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:      1,
									Memory:   "1",
									DiskSize: "150",
								},
							},
							MaxInstances: int32(1),
						},
					},
				}},
			candidates:    _testPools,
			expectedOrder: []string{"One", "Three", "Two"},
		},
		{
			// Tests if there are multiple resource pools where the job can fit, those resource pools are sorted by dominant resource.
			msg: "Job fits resource pool 'three' and 'two'  and resource pools are sorted by MEM dominant resource",
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:      10,
									Memory:   "1",
									Gpu:      1,
									GpuSku:   "a100",
									DiskSize: "1",
								},
							},
						},
						Worker: &v2beta1pb.WorkerSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:      1,
									Memory:   "1",
									Gpu:      1,
									GpuSku:   "a100",
									DiskSize: "1",
								},
							},
							MaxInstances: int32(1),
						},
					},
				}},
			candidates:    _testPools,
			expectedOrder: []string{"Three", "Two", "One"},
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			builder := framework.NewOptionBuilder()
			builder.Build(framework.WithLogger(logr.Logger{}.V(1)))
			scorer := LoadScorer{
				OptionBuilder: builder,
			}
			result, err := scorer.Score(context.Background(), test.job, test.candidates)
			require.NoError(t, err)
			require.Equal(t, len(test.expectedOrder), len(result))

			for i, poolName := range test.expectedOrder {
				require.Equal(t, poolName, result[i].Pool.Name)
			}
		})
	}
}
