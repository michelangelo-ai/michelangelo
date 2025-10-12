package plugins

import (
	"context"
	"fmt"
	"testing"

	infraCrds "code.uber.internal/infra/compute/k8s-crds/apis/compute.uber.com/v1beta2"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/cluster"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	matypes "code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/types"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/scheduler/framework"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v2beta1pb "michelangelo/api/v2beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func TestCanResourcePoolFitJobWithResourceMap(t *testing.T) {
	tests := []struct {
		name     string
		job      framework.BatchJob
		poolInfo *cluster.ResourcePoolInfo
		expected bool
	}{
		{
			name: "Spark job with driver - should return true when pool has sufficient resources",
			job: framework.BatchSparkJob{
				SparkJob: &v2beta1pb.SparkJob{
					Spec: v2beta1pb.SparkJobSpec{
						Driver: &v2beta1pb.DriverSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    4,
									Memory: "8Gi",
								},
							},
						},
					},
				},
			},
			poolInfo: &cluster.ResourcePoolInfo{
				Pool: infraCrds.ResourcePool{
					Spec: infraCrds.ResourcePoolSpec{
						ResourceMap: map[string]infraCrds.ResourceConfiguration{
							"default": {
								Resources: []infraCrds.ResourceConfig{
									{
										Kind:        corev1.ResourceCPU.String(),
										Reservation: *resource.NewQuantity(10, resource.DecimalSI),
									},
									{
										Kind:        corev1.ResourceMemory.String(),
										Reservation: *resource.NewScaledQuantity(16, 9), // 16Gi - sufficient for 8Gi job
									},
									{
										Kind:        constants.ResourceNvidiaGPU.String(),
										Reservation: *resource.NewQuantity(0, resource.DecimalSI), // 0 GPU
									},
									{
										Kind:        corev1.ResourceEphemeralStorage.String(),
										Reservation: resource.MustParse("100Gi"),
									},
								},
							},
						},
					},
					Status: infraCrds.ResourcePoolStatus{
						UsageMap: map[string]corev1.ResourceList{
							"default": {
								corev1.ResourceCPU:              *resource.NewQuantity(0, resource.DecimalSI),
								corev1.ResourceMemory:           resource.MustParse("0Gi"),
								constants.ResourceNvidiaGPU:     *resource.NewQuantity(0, resource.DecimalSI),
								corev1.ResourceEphemeralStorage: resource.MustParse("0Gi"),
							},
						},
					},
				},
			},
			expected: true, // Spark job (4 CPU, 8Gi) fits in pool (10 CPU, 16Gi)
		},
		{
			name: "Ray job with nil RayJob - returns true for empty components",
			job: framework.BatchRayJob{
				RayJob: nil,
			},
			poolInfo: &cluster.ResourcePoolInfo{
				Pool: infraCrds.ResourcePool{
					Spec: infraCrds.ResourcePoolSpec{
						ResourceMap: map[string]infraCrds.ResourceConfiguration{
							"default": {
								Resources: []infraCrds.ResourceConfig{
									{
										Kind:        corev1.ResourceCPU.String(),
										Reservation: *resource.NewQuantity(10, resource.DecimalSI),
									},
								},
							},
						},
					},
				},
			},
			expected: true, // Empty components should return true
		},
		{
			name: "Ray job with resource SKU not in pool - triggers canHost false",
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    1,
									Memory: "1Gi",
									GpuSku: "RTX5000", // Not available in pool
									Gpu:    1,
								},
							},
						},
					},
				},
			},
			poolInfo: &cluster.ResourcePoolInfo{
				Pool: infraCrds.ResourcePool{
					Spec: infraCrds.ResourcePoolSpec{
						ResourceMap: map[string]infraCrds.ResourceConfiguration{
							"A100": { // Only A100 available
								Resources: []infraCrds.ResourceConfig{
									{
										Kind:        corev1.ResourceCPU.String(),
										Reservation: *resource.NewQuantity(10, resource.DecimalSI),
									},
								},
							},
						},
					},
				},
			},
			expected: false, // Should return false when canHost is false
		},
		{
			name: "Ray job with sufficient resources",
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-ray-job",
						Namespace: "test-ns",
					},
					Spec: v2beta1pb.RayJobSpec{
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    1,
									Memory: "1Gi",
									GpuSku: "A100",
									Gpu:    1,
								},
							},
						},
					},
				},
			},
			poolInfo: &cluster.ResourcePoolInfo{
				Pool: infraCrds.ResourcePool{
					Spec: infraCrds.ResourcePoolSpec{
						ResourceMap: map[string]infraCrds.ResourceConfiguration{
							"A100": {
								Resources: []infraCrds.ResourceConfig{
									{
										Kind:        corev1.ResourceCPU.String(),
										Reservation: *resource.NewQuantity(10, resource.DecimalSI),
									},
									{
										Kind:        constants.ResourceNvidiaGPU.String(),
										Reservation: *resource.NewQuantity(5, resource.DecimalSI),
									},
									{
										Kind:        corev1.ResourceMemory.String(),
										Reservation: resource.MustParse("10Gi"),
									},
									{
										Kind:        corev1.ResourceEphemeralStorage.String(),
										Reservation: resource.MustParse("100Gi"),
									},
								},
							},
						},
					},
					Status: infraCrds.ResourcePoolStatus{
						UsageMap: map[string]corev1.ResourceList{
							"A100": {
								corev1.ResourceCPU:              *resource.NewQuantity(0, resource.DecimalSI),
								constants.ResourceNvidiaGPU:     *resource.NewQuantity(0, resource.DecimalSI),
								corev1.ResourceMemory:           resource.MustParse("0Gi"),
								corev1.ResourceEphemeralStorage: resource.MustParse("0Gi"),
							},
						},
					},
				},
			},
			expected: true, // Should fit: sufficient capacity for all resources
		},
		{
			name: "Ray job exceeds CPU capacity - covers resource checking failure path",
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-ray-job-cpu-exceed",
						Namespace: "test-ns",
					},
					Spec: v2beta1pb.RayJobSpec{
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    1,
									Memory: "1Gi",
									GpuSku: "A100",
									Gpu:    1,
								},
							},
						},
					},
				},
			},
			poolInfo: &cluster.ResourcePoolInfo{
				Pool: infraCrds.ResourcePool{
					Spec: infraCrds.ResourcePoolSpec{
						ResourceMap: map[string]infraCrds.ResourceConfiguration{
							"A100": {
								Resources: []infraCrds.ResourceConfig{
									{
										Kind:        corev1.ResourceCPU.String(),
										Reservation: *resource.NewQuantity(1, resource.DecimalSI), // Only 1 CPU
									},
									{
										Kind:        constants.ResourceNvidiaGPU.String(),
										Reservation: *resource.NewQuantity(5, resource.DecimalSI),
									},
								},
							},
						},
					},
					Status: infraCrds.ResourcePoolStatus{
						UsageMap: map[string]corev1.ResourceList{
							"A100": {
								corev1.ResourceCPU:          *resource.NewQuantity(1, resource.DecimalSI), // 0 CPU available
								constants.ResourceNvidiaGPU: *resource.NewQuantity(0, resource.DecimalSI),
							},
						},
					},
				},
			},
			expected: false, // Should not fit: needs 1 CPU, but 0 available
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Helper()

			result := canV2ResourcePoolFitJob(tt.job, tt.poolInfo)
			require.Equal(t, tt.expected, result)
		})
	}
}

// mockJobWithError is a mock job that returns an error from GetResourceRequirement
type mockJobWithError struct{}

func (m *mockJobWithError) GetResourceRequirement() (map[string]corev1.ResourceList, error) {
	return nil, fmt.Errorf("mock error from GetResourceRequirement")
}

func (m *mockJobWithError) GetGeneration() int64                           { return 0 }
func (m *mockJobWithError) GetName() string                                { return "mock-job" }
func (m *mockJobWithError) GetNamespace() string                           { return "mock-ns" }
func (m *mockJobWithError) GetAffinity() *v2beta1pb.Affinity               { return nil }
func (m *mockJobWithError) GetConditions() *[]*v2beta1pb.Condition         { return nil }
func (m *mockJobWithError) GetAssignmentInfo() *v2beta1pb.AssignmentInfo   { return nil }
func (m *mockJobWithError) GetObject() client.Object                       { return nil }
func (m *mockJobWithError) GetLabels() map[string]string                   { return nil }
func (m *mockJobWithError) GetAnnotations() map[string]string              { return nil }
func (m *mockJobWithError) GetUserName() string                            { return "mock-user" }
func (m *mockJobWithError) GetTerminationSpec() *v2beta1pb.TerminationSpec { return nil }
func (m *mockJobWithError) IsPreemptibleJob() bool                         { return false }
func (m *mockJobWithError) GetEnvironmentLabel() string                    { return "" }
func (m *mockJobWithError) GetJobType() matypes.JobType                    { return matypes.RayJob }

// TestGetAvailableForPoolJobAware validates the job-aware pool capacity calculation that considers
// only resource SKUs relevant to the job. It verifies v1/v2 pool handling, constraint-aware scoring
// (single SKU optimization, multi-SKU minimum), error handling, and edge cases like negative capacity and empty requirements.
func TestGetAvailableForPoolJobAware(t *testing.T) {
	tests := []struct {
		name         string
		resourceName corev1.ResourceName
		poolInfo     *cluster.ResourcePoolInfo
		job          framework.BatchJob
		expected     resource.Quantity
	}{
		{
			name:         "v1 pool - delegates to getAvailableV1",
			resourceName: corev1.ResourceCPU,
			poolInfo: &cluster.ResourcePoolInfo{
				Pool: infraCrds.ResourcePool{
					Spec: infraCrds.ResourcePoolSpec{
						Resources: []infraCrds.ResourceConfig{
							{
								Kind:        corev1.ResourceCPU.String(),
								Reservation: *resource.NewQuantity(100, resource.DecimalSI),
							},
						},
					},
					Status: infraCrds.ResourcePoolStatus{
						Usage: corev1.ResourceList{
							corev1.ResourceCPU: *resource.NewQuantity(30, resource.DecimalSI),
						},
					},
				},
			},
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    2,
									Memory: "4Gi",
								},
							},
						},
					},
				},
			},
			expected: *resource.NewQuantity(70, resource.DecimalSI), // 100 - 30
		},
		{
			name:         "v2 pool - job with single resource SKU",
			resourceName: corev1.ResourceCPU,
			poolInfo: &cluster.ResourcePoolInfo{
				Pool: infraCrds.ResourcePool{
					Spec: infraCrds.ResourcePoolSpec{
						ResourceMap: map[string]infraCrds.ResourceConfiguration{
							"A100": {
								Resources: []infraCrds.ResourceConfig{
									{
										Kind:        corev1.ResourceCPU.String(),
										Reservation: *resource.NewQuantity(50, resource.DecimalSI),
									},
								},
							},
							"RTX5000": {
								Resources: []infraCrds.ResourceConfig{
									{
										Kind:        corev1.ResourceCPU.String(),
										Reservation: *resource.NewQuantity(30, resource.DecimalSI),
									},
								},
							},
						},
					},
					Status: infraCrds.ResourcePoolStatus{
						UsageMap: map[string]corev1.ResourceList{
							"A100": {
								corev1.ResourceCPU: *resource.NewQuantity(10, resource.DecimalSI),
							},
							"RTX5000": {
								corev1.ResourceCPU: *resource.NewQuantity(5, resource.DecimalSI),
							},
						},
					},
				},
			},
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    2,
									Memory: "4Gi",
									GpuSku: "A100", // Only needs A100 resource SKU
									Gpu:    1,
								},
							},
						},
					},
				},
			},
			expected: *resource.NewQuantity(40, resource.DecimalSI), // Only A100: 50 - 10 = 40
		},
		{
			name:         "v2 pool - job with multiple resource SKUs, returns minimum (constraint-aware)",
			resourceName: corev1.ResourceCPU,
			poolInfo: &cluster.ResourcePoolInfo{
				Pool: infraCrds.ResourcePool{
					Spec: infraCrds.ResourcePoolSpec{
						ResourceMap: map[string]infraCrds.ResourceConfiguration{
							"A100": {
								Resources: []infraCrds.ResourceConfig{
									{
										Kind:        corev1.ResourceCPU.String(),
										Reservation: *resource.NewQuantity(50, resource.DecimalSI),
									},
								},
							},
							"RTX5000": {
								Resources: []infraCrds.ResourceConfig{
									{
										Kind:        corev1.ResourceCPU.String(),
										Reservation: *resource.NewQuantity(30, resource.DecimalSI),
									},
								},
							},
						},
					},
					Status: infraCrds.ResourcePoolStatus{
						UsageMap: map[string]corev1.ResourceList{
							"A100": {
								corev1.ResourceCPU: *resource.NewQuantity(10, resource.DecimalSI),
							},
							"RTX5000": {
								corev1.ResourceCPU: *resource.NewQuantity(5, resource.DecimalSI),
							},
						},
					},
				},
			},
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    2,
									Memory: "4Gi",
									GpuSku: "A100",
									Gpu:    1,
								},
							},
						},
						Worker: &v2beta1pb.WorkerSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    1,
									Memory: "2Gi",
									GpuSku: "RTX5000", // Different resource SKU
									Gpu:    1,
								},
							},
							MinInstances: 1,
						},
					},
				},
			},
			expected: *resource.NewQuantity(25, resource.DecimalSI), // min(A100: 40, RTX5000: 25) = 25
		},
		{
			name:         "v2 pool - job with no resource requirements",
			resourceName: corev1.ResourceCPU,
			poolInfo: &cluster.ResourcePoolInfo{
				Pool: infraCrds.ResourcePool{
					Spec: infraCrds.ResourcePoolSpec{
						ResourceMap: map[string]infraCrds.ResourceConfiguration{
							"A100": {
								Resources: []infraCrds.ResourceConfig{
									{
										Kind:        corev1.ResourceCPU.String(),
										Reservation: *resource.NewQuantity(50, resource.DecimalSI),
									},
								},
							},
						},
					},
				},
			},
			job: framework.BatchRayJob{
				RayJob: nil, // No RayJob spec - results in empty resource requirements
			},
			expected: resource.Quantity{}, // Zero quantity for empty requirements
		},
		{
			name:         "v2 pool - job resource SKU not available in pool",
			resourceName: corev1.ResourceCPU,
			poolInfo: &cluster.ResourcePoolInfo{
				Pool: infraCrds.ResourcePool{
					Spec: infraCrds.ResourcePoolSpec{
						ResourceMap: map[string]infraCrds.ResourceConfiguration{
							"A100": {
								Resources: []infraCrds.ResourceConfig{
									{
										Kind:        corev1.ResourceCPU.String(),
										Reservation: *resource.NewQuantity(50, resource.DecimalSI),
									},
								},
							},
						},
					},
				},
			},
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    2,
									Memory: "4Gi",
									GpuSku: "V100", // Not available in pool
									Gpu:    1,
								},
							},
						},
					},
				},
			},
			expected: resource.Quantity{}, // Zero quantity when resource SKU not available
		},
		{
			name:         "v2 pool - mixed availability across resource SKUs",
			resourceName: corev1.ResourceMemory,
			poolInfo: &cluster.ResourcePoolInfo{
				Pool: infraCrds.ResourcePool{
					Spec: infraCrds.ResourcePoolSpec{
						ResourceMap: map[string]infraCrds.ResourceConfiguration{
							"A100": {
								Resources: []infraCrds.ResourceConfig{
									{
										Kind:        corev1.ResourceMemory.String(),
										Reservation: resource.MustParse("32Gi"),
									},
								},
							},
							"RTX5000": {
								Resources: []infraCrds.ResourceConfig{
									{
										Kind:        corev1.ResourceMemory.String(),
										Reservation: resource.MustParse("16Gi"),
									},
								},
							},
							"default": {
								Resources: []infraCrds.ResourceConfig{
									{
										Kind:        corev1.ResourceMemory.String(),
										Reservation: resource.MustParse("8Gi"),
									},
								},
							},
						},
					},
					Status: infraCrds.ResourcePoolStatus{
						UsageMap: map[string]corev1.ResourceList{
							"A100": {
								corev1.ResourceMemory: resource.MustParse("16Gi"),
							},
							"RTX5000": {
								corev1.ResourceMemory: resource.MustParse("4Gi"),
							},
							"default": {
								corev1.ResourceMemory: resource.MustParse("2Gi"),
							},
						},
					},
				},
			},
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    2,
									Memory: "4Gi",
									GpuSku: "A100",
									Gpu:    1,
								},
							},
						},
						Workers: []*v2beta1pb.WorkerSpec{
							{
								Pod: &v2beta1pb.PodSpec{
									Resource: &v2beta1pb.ResourceSpec{
										Cpu:    1,
										Memory: "2Gi",
										GpuSku: "RTX5000",
										Gpu:    1,
									},
								},
								MinInstances: 2,
							},
							{
								Pod: &v2beta1pb.PodSpec{
									Resource: &v2beta1pb.ResourceSpec{
										Cpu:    1,
										Memory: "1Gi",
										// No GPU - uses default resource SKU
									},
								},
								MinInstances: 1,
							},
						},
					},
				},
			},
			// Available: A100: 32-16=16Gi, RTX5000: 16-4=12Gi, default: 8-2=6Gi
			expected: resource.MustParse("6Gi"), // min(16Gi, 12Gi, 6Gi) = 6Gi
		},
		{
			name:         "v2 pool - negative available capacity",
			resourceName: corev1.ResourceCPU,
			poolInfo: &cluster.ResourcePoolInfo{
				Pool: infraCrds.ResourcePool{
					Spec: infraCrds.ResourcePoolSpec{
						ResourceMap: map[string]infraCrds.ResourceConfiguration{
							"A100": {
								Resources: []infraCrds.ResourceConfig{
									{
										Kind:        corev1.ResourceCPU.String(),
										Reservation: *resource.NewQuantity(10, resource.DecimalSI),
									},
								},
							},
						},
					},
					Status: infraCrds.ResourcePoolStatus{
						UsageMap: map[string]corev1.ResourceList{
							"A100": {
								corev1.ResourceCPU: *resource.NewQuantity(15, resource.DecimalSI), // Usage > reservation
							},
						},
					},
				},
			},
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    1,
									GpuSku: "A100",
									Gpu:    1,
								},
							},
						},
					},
				},
			},
			expected: *resource.NewQuantity(-5, resource.DecimalSI), // 10 - 15 = -5 (elastic sharing)
		},
		{
			name:         "v2 pool - spark job with default resource SKU",
			resourceName: corev1.ResourceCPU,
			poolInfo: &cluster.ResourcePoolInfo{
				Pool: infraCrds.ResourcePool{
					Spec: infraCrds.ResourcePoolSpec{
						ResourceMap: map[string]infraCrds.ResourceConfiguration{
							"default": {
								Resources: []infraCrds.ResourceConfig{
									{
										Kind:        corev1.ResourceCPU.String(),
										Reservation: *resource.NewQuantity(100, resource.DecimalSI),
									},
								},
							},
						},
					},
					Status: infraCrds.ResourcePoolStatus{
						UsageMap: map[string]corev1.ResourceList{
							"default": {
								corev1.ResourceCPU: *resource.NewQuantity(20, resource.DecimalSI),
							},
						},
					},
				},
			},
			job: framework.BatchSparkJob{
				SparkJob: &v2beta1pb.SparkJob{
					Spec: v2beta1pb.SparkJobSpec{
						Driver: &v2beta1pb.DriverSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    4,
									Memory: "8Gi",
								},
							},
						},
						Executor: &v2beta1pb.ExecutorSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    2,
									Memory: "4Gi",
								},
							},
							Instances: 2,
						},
					},
				},
			},
			expected: *resource.NewQuantity(80, resource.DecimalSI), // 100 - 20 = 80
		},
		{
			name:         "v2 pool - error in GetResourceRequirement returns zero",
			resourceName: corev1.ResourceCPU,
			poolInfo: &cluster.ResourcePoolInfo{
				Pool: infraCrds.ResourcePool{
					Spec: infraCrds.ResourcePoolSpec{
						ResourceMap: map[string]infraCrds.ResourceConfiguration{
							"A100": {
								Resources: []infraCrds.ResourceConfig{
									{
										Kind:        corev1.ResourceCPU.String(),
										Reservation: *resource.NewQuantity(50, resource.DecimalSI),
									},
								},
							},
						},
					},
				},
			},
			job:      &mockJobWithError{}, // Mock job that returns error from GetResourceRequirement
			expected: resource.Quantity{}, // Zero quantity on error
		},
		{
			name:         "v2 pool - all resource SKUs have zero availability",
			resourceName: corev1.ResourceCPU,
			poolInfo: &cluster.ResourcePoolInfo{
				Pool: infraCrds.ResourcePool{
					Spec: infraCrds.ResourcePoolSpec{
						ResourceMap: map[string]infraCrds.ResourceConfiguration{
							"A100": {
								Resources: []infraCrds.ResourceConfig{
									{
										Kind:        corev1.ResourceCPU.String(),
										Reservation: *resource.NewQuantity(10, resource.DecimalSI),
									},
								},
							},
							"RTX5000": {
								Resources: []infraCrds.ResourceConfig{
									{
										Kind:        corev1.ResourceCPU.String(),
										Reservation: *resource.NewQuantity(10, resource.DecimalSI),
									},
								},
							},
						},
					},
					Status: infraCrds.ResourcePoolStatus{
						UsageMap: map[string]corev1.ResourceList{
							"A100": {
								corev1.ResourceCPU: *resource.NewQuantity(10, resource.DecimalSI), // Full usage
							},
							"RTX5000": {
								corev1.ResourceCPU: *resource.NewQuantity(10, resource.DecimalSI), // Full usage
							},
						},
					},
				},
			},
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    1,
									GpuSku: "A100",
									Gpu:    1,
								},
							},
						},
						Worker: &v2beta1pb.WorkerSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    1,
									GpuSku: "RTX5000",
									Gpu:    1,
								},
							},
							MinInstances: 1,
						},
					},
				},
			},
			expected: resource.Quantity{}, // min(0, 0) = 0
		},
		{
			name:         "v2 pool - constraint-aware scoring prevents misleading optimism",
			resourceName: corev1.ResourceMemory,
			poolInfo: &cluster.ResourcePoolInfo{
				Pool: infraCrds.ResourcePool{
					Spec: infraCrds.ResourcePoolSpec{
						ResourceMap: map[string]infraCrds.ResourceConfiguration{
							"A100": {
								Resources: []infraCrds.ResourceConfig{
									{
										Kind:        corev1.ResourceCPU.String(),
										Reservation: *resource.NewQuantity(10, resource.DecimalSI),
									},
									{
										Kind:        corev1.ResourceMemory.String(),
										Reservation: resource.MustParse("8Gi"), // Low memory
									},
								},
							},
							"RTX5000": {
								Resources: []infraCrds.ResourceConfig{
									{
										Kind:        corev1.ResourceCPU.String(),
										Reservation: *resource.NewQuantity(4, resource.DecimalSI), // Low CPU
									},
									{
										Kind:        corev1.ResourceMemory.String(),
										Reservation: resource.MustParse("64Gi"), // High memory
									},
								},
							},
						},
					},
					Status: infraCrds.ResourcePoolStatus{
						UsageMap: map[string]corev1.ResourceList{
							"A100": {
								corev1.ResourceCPU:    *resource.NewQuantity(2, resource.DecimalSI),
								corev1.ResourceMemory: resource.MustParse("4Gi"),
							},
							"RTX5000": {
								corev1.ResourceCPU:    *resource.NewQuantity(2, resource.DecimalSI),
								corev1.ResourceMemory: resource.MustParse("32Gi"),
							},
						},
					},
				},
			},
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    6, // Needs more CPU than A100 has available (8 CPU available)
									Memory: "2Gi",
									GpuSku: "A100",
									Gpu:    1,
								},
							},
						},
						Worker: &v2beta1pb.WorkerSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    1,
									Memory: "16Gi", // Needs more memory than RTX5000 has available (32Gi available)
									GpuSku: "RTX5000",
									Gpu:    1,
								},
							},
							MinInstances: 1,
						},
					},
				},
			},
			// Available: A100: 8-4=4Gi, RTX5000: 64-32=32Gi
			// Job cannot actually fit: A100 needs 6 CPU (only 8 available) + 2Gi memory (4Gi available ✓)
			//                          RTX5000 needs 1 CPU (2 available ) + 16Gi memory (32Gi available ✓)
			// Constraint-aware scoring: min(4Gi, 32Gi) = 4Gi (reflects the bottleneck)
			expected: resource.MustParse("4Gi"), // min(4Gi, 32Gi) = 4Gi - reflects constraint reality
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Helper()

			result := getAvailable(tt.resourceName, tt.poolInfo, tt.job)
			require.True(t, tt.expected.Equal(result),
				"Expected %s, got %s", tt.expected.String(), result.String())
		})
	}
}

func TestGetAvailableV2(t *testing.T) {
	tests := []struct {
		name         string
		resourceName corev1.ResourceName
		poolInfo     *cluster.ResourcePoolInfo
		resourceSKU  string
		expected     resource.Quantity
	}{
		{
			name:         "get available CPU for existing resource SKU",
			resourceName: corev1.ResourceCPU,
			poolInfo: &cluster.ResourcePoolInfo{
				Pool: infraCrds.ResourcePool{
					Spec: infraCrds.ResourcePoolSpec{
						ResourceMap: map[string]infraCrds.ResourceConfiguration{
							"A100": {
								Resources: []infraCrds.ResourceConfig{
									{
										Kind:        corev1.ResourceCPU.String(),
										Reservation: *resource.NewQuantity(100, resource.DecimalSI),
									},
								},
							},
						},
					},
					Status: infraCrds.ResourcePoolStatus{
						UsageMap: map[string]corev1.ResourceList{
							"A100": {
								corev1.ResourceCPU: *resource.NewQuantity(30, resource.DecimalSI),
							},
						},
					},
				},
			},
			resourceSKU: "A100",
			expected:    *resource.NewQuantity(70, resource.DecimalSI), // 100 - 30
		},
		{
			name:         "get available GPU for existing resource SKU",
			resourceName: constants.ResourceNvidiaGPU,
			poolInfo: &cluster.ResourcePoolInfo{
				Pool: infraCrds.ResourcePool{
					Spec: infraCrds.ResourcePoolSpec{
						ResourceMap: map[string]infraCrds.ResourceConfiguration{
							"RTX5000": {
								Resources: []infraCrds.ResourceConfig{
									{
										Kind:        constants.ResourceNvidiaGPU.String(),
										Reservation: *resource.NewQuantity(8, resource.DecimalSI),
									},
								},
							},
						},
					},
					Status: infraCrds.ResourcePoolStatus{
						UsageMap: map[string]corev1.ResourceList{
							"RTX5000": {
								constants.ResourceNvidiaGPU: *resource.NewQuantity(3, resource.DecimalSI),
							},
						},
					},
				},
			},
			resourceSKU: "RTX5000",
			expected:    *resource.NewQuantity(5, resource.DecimalSI), // 8 - 3
		},
		{
			name:         "resource SKU not found - returns zero quantity",
			resourceName: corev1.ResourceCPU,
			poolInfo: &cluster.ResourcePoolInfo{
				Pool: infraCrds.ResourcePool{
					Spec: infraCrds.ResourcePoolSpec{
						ResourceMap: map[string]infraCrds.ResourceConfiguration{
							"A100": {
								Resources: []infraCrds.ResourceConfig{
									{
										Kind:        corev1.ResourceCPU.String(),
										Reservation: *resource.NewQuantity(100, resource.DecimalSI),
									},
								},
							},
						},
					},
				},
			},
			resourceSKU: "NonExistentSKU",
			expected:    resource.Quantity{}, // Should return zero for missing SKU
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Helper()

			result := getAvailableV2(tt.resourceName, tt.poolInfo, tt.resourceSKU)
			require.True(t, tt.expected.Equal(result),
				"Expected %s, got %s", tt.expected.String(), result.String())
		})
	}
}

func TestComputeAvailableFromConfig(t *testing.T) {
	tests := []struct {
		name            string
		resourceName    corev1.ResourceName
		resourceConfigs []infraCrds.ResourceConfig
		usage           corev1.ResourceList
		expected        resource.Quantity
	}{
		{
			name:         "compute available CPU",
			resourceName: corev1.ResourceCPU,
			resourceConfigs: []infraCrds.ResourceConfig{
				{
					Kind:        corev1.ResourceCPU.String(),
					Reservation: *resource.NewQuantity(100, resource.DecimalSI),
				},
				{
					Kind:        corev1.ResourceMemory.String(),
					Reservation: *resource.NewScaledQuantity(50, 9),
				},
			},
			usage: corev1.ResourceList{
				corev1.ResourceCPU:    *resource.NewQuantity(25, resource.DecimalSI),
				corev1.ResourceMemory: *resource.NewScaledQuantity(10, 9),
			},
			expected: *resource.NewQuantity(75, resource.DecimalSI), // 100 - 25
		},
		{
			name:         "compute available memory",
			resourceName: corev1.ResourceMemory,
			resourceConfigs: []infraCrds.ResourceConfig{
				{
					Kind:        corev1.ResourceCPU.String(),
					Reservation: *resource.NewQuantity(100, resource.DecimalSI),
				},
				{
					Kind:        corev1.ResourceMemory.String(),
					Reservation: *resource.NewScaledQuantity(50, 9),
				},
			},
			usage: corev1.ResourceList{
				corev1.ResourceCPU:    *resource.NewQuantity(25, resource.DecimalSI),
				corev1.ResourceMemory: *resource.NewScaledQuantity(10, 9),
			},
			expected: *resource.NewScaledQuantity(40, 9), // 50Gi - 10Gi
		},
		{
			name:         "resource not found in config - returns zero",
			resourceName: constants.ResourceNvidiaGPU,
			resourceConfigs: []infraCrds.ResourceConfig{
				{
					Kind:        corev1.ResourceCPU.String(),
					Reservation: *resource.NewQuantity(100, resource.DecimalSI),
				},
			},
			usage: corev1.ResourceList{
				corev1.ResourceCPU: *resource.NewQuantity(25, resource.DecimalSI),
			},
			expected: resource.Quantity{}, // GPU not in config, should return zero reservation
		},
		{
			name:         "negative available (oversubscribed) - elastic resource sharing",
			resourceName: corev1.ResourceCPU,
			resourceConfigs: []infraCrds.ResourceConfig{
				{
					Kind:        corev1.ResourceCPU.String(),
					Reservation: *resource.NewQuantity(100, resource.DecimalSI),
				},
			},
			usage: corev1.ResourceList{
				corev1.ResourceCPU: *resource.NewQuantity(150, resource.DecimalSI), // More usage than reservation
			},
			expected: *resource.NewQuantity(-50, resource.DecimalSI), // 100 - 150 = -50 (expected for elastic sharing)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Helper()

			result := computeAvailableFromConfig(tt.resourceName, tt.resourceConfigs, tt.usage)
			require.True(t, tt.expected.Equal(result),
				"Expected %s, got %s", tt.expected.String(), result.String())
		})
	}
}
