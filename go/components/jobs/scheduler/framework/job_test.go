package framework

import (
	"testing"

	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	sharedConstants "code.uber.internal/uberai/michelangelo/shared/constants"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	v2beta1pb "michelangelo/api/v2beta1"
)

// ConvertResourceMapToResourceList is a test helper function that aggregates all resource requirements
// from a ResourceSKU map into a single ResourceList.
// This is copied from plugins/utils.go but reimplemented here to avoid circular dependencies.
func ConvertResourceMapToResourceList(resourcesByKey map[string]v1.ResourceList) v1.ResourceList {
	total := make(v1.ResourceList)

	for _, resourceList := range resourcesByKey {
		total = quotav1.Add(total, resourceList)
	}

	return total
}

// TestGetResourceRequirement validates the GetResourceRequirement method for various job types and resource configurations,
// ensuring correct calculation of total resources and expected resource SKUs, as well as appropriate error handling for invalid cases.
func TestGetResourceRequirement(t *testing.T) {
	tests := []struct {
		name                 string
		job                  BatchJob
		expectedTotal        v1.ResourceList
		expectedResourceSKUs []string
		expectError          bool
	}{
		{
			name: "empty ray job spec",
			job: BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{},
							},
						},
						Worker: &v2beta1pb.WorkerSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{},
							},
						},
						Scheduling: &v2beta1pb.SchedulingSpec{Preemptible: true},
					},
				},
			},
			expectedTotal: v1.ResourceList{
				"cpu":               resource.Quantity{Format: "DecimalSI"}, // 0 CPU
				"nvidia.com/gpu":    resource.Quantity{Format: "DecimalSI"}, // 0 GPU
				"memory":            resource.Quantity{Format: "DecimalSI"}, // 0 memory
				"ephemeral-storage": resource.MustParse("100Gi"),            // 2 * 50Gi (head + worker)
			},
			expectedResourceSKUs: []string{"default"}, // Empty specs still get default key with default resources
			expectError:          false,
		},
		{
			name: "valid ray job spec",
			job: BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    4,
									Gpu:    1,
									Memory: "100Mi",
								},
							},
						},
						Worker: &v2beta1pb.WorkerSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    4,
									Gpu:    1,
									Memory: "100Mi",
								},
							},
							MinInstances: 2,
							MaxInstances: 2,
						},
						Scheduling: &v2beta1pb.SchedulingSpec{Preemptible: true},
					},
				},
			},
			expectedTotal: v1.ResourceList{
				"cpu":            *resource.NewQuantity(12, resource.DecimalSI),
				"nvidia.com/gpu": *resource.NewQuantity(3, resource.DecimalSI),
				"memory":         resource.MustParse("300Mi"),
			},
			expectedResourceSKUs: []string{"RTX5000"}, // No GPU SKU specified, uses RTX5000 for v1 compatibility
			expectError:          false,
		},
		{
			job: BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    4,
									Memory: "100Mi",
								},
							},
						},
						Workers: []*v2beta1pb.WorkerSpec{
							{
								Pod: &v2beta1pb.PodSpec{
									Resource: &v2beta1pb.ResourceSpec{
										Cpu:    4,
										Memory: "100Mi",
									},
								},
								MinInstances: 4,
								MaxInstances: 4,
								NodeType:     "DATA_NODE",
							},
							{
								Pod: &v2beta1pb.PodSpec{
									Resource: &v2beta1pb.ResourceSpec{
										Cpu:    8,
										Gpu:    1,
										Memory: "150Mi",
									},
								},
								MinInstances: 1,
								MaxInstances: 1,
								NodeType:     "TRAINER_NODE",
							},
						},
						Scheduling: &v2beta1pb.SchedulingSpec{Preemptible: true},
					},
				},
			},
			name: "valid heterogeneous ray job spec",
			expectedTotal: v1.ResourceList{
				"cpu":            *resource.NewQuantity(28, resource.DecimalSI),
				"nvidia.com/gpu": *resource.NewQuantity(1, resource.DecimalSI),
				"memory":         resource.MustParse("650Mi"),
			},
			expectedResourceSKUs: []string{"default", "RTX5000"}, // Non-GPU workers use default, GPU worker without SKU uses RTX5000
			expectError:          false,
		},
		{
			name: "empty spark job spec",
			job: BatchSparkJob{
				SparkJob: &v2beta1pb.SparkJob{
					Spec: v2beta1pb.SparkJobSpec{
						Scheduling: &v2beta1pb.SchedulingSpec{Preemptible: true},
					},
				},
			},
			expectedTotal: v1.ResourceList{
				"cpu":               resource.Quantity{Format: "DecimalSI"}, // 0 CPU
				"nvidia.com/gpu":    resource.Quantity{Format: "DecimalSI"}, // 0 GPU
				"memory":            resource.Quantity{Format: "DecimalSI"}, // 0 memory
				"ephemeral-storage": resource.Quantity{Format: "DecimalSI"}, // 0 (no driver/executor)
			},
			expectedResourceSKUs: []string{"default"}, // Empty specs still get default key
			expectError:          false,
		},
		{
			name: "spark job with driver and executors",
			job: BatchSparkJob{
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
							Instances: 3,
						},
					},
				},
			},
			expectedTotal: v1.ResourceList{
				"cpu":    *resource.NewQuantity(10, resource.DecimalSI), // 4 (driver) + 6 (2*3 executors) = 10
				"memory": resource.MustParse("20Gi"),                    // 8Gi + 12Gi = 20Gi total
			},
			expectedResourceSKUs: []string{"default"}, // Spark always uses default
			expectError:          false,
		},
		{
			name: "ray job with same GPU SKU",
			job: BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    4,
									Memory: "8Gi",
									GpuSku: "A100",
									Gpu:    1,
								},
							},
						},
						Worker: &v2beta1pb.WorkerSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    2,
									Memory: "4Gi",
									GpuSku: "A100",
									Gpu:    1,
								},
							},
							MinInstances: 2,
						},
					},
				},
			},
			expectedTotal: v1.ResourceList{
				"cpu":            *resource.NewQuantity(8, resource.DecimalSI), // 4 (head) + 4 (2*2 workers) = 8
				"nvidia.com/gpu": *resource.NewQuantity(3, resource.DecimalSI), // 1 (head) + 2 (1*2 workers) = 3
				"memory":         resource.MustParse("16Gi"),                   // 8Gi + 8Gi = 16Gi total
			},
			expectedResourceSKUs: []string{"A100"}, // All components use same GPU SKU
			expectError:          false,
		},
		{
			name: "ray job with nil RayJob",
			job: BatchRayJob{
				RayJob: nil,
			},
			expectedTotal:        v1.ResourceList{}, // Nil job returns empty map
			expectedResourceSKUs: []string{},        // Nil job returns empty map
			expectError:          false,
		},
		{
			name: "ray job with invalid memory format",
			job: BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    4,
									Memory: "invalid-memory-format", // Invalid format
									Gpu:    1,
								},
							},
						},
					},
				},
			},
			expectError: true, // Should return error due to invalid memory format
		},
		{
			job: BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    4,
									Memory: "8Gi",
									GpuSku: "A100",
									Gpu:    1,
								},
							},
						},
						Workers: []*v2beta1pb.WorkerSpec{
							{
								Pod: &v2beta1pb.PodSpec{
									Resource: &v2beta1pb.ResourceSpec{
										Cpu:    2,
										Memory: "4Gi",
										// No GPU SKU - uses DefaultCPU
									},
								},
								MinInstances: 2,
								NodeType:     "DATA_NODE",
							},
							{
								Pod: &v2beta1pb.PodSpec{
									Resource: &v2beta1pb.ResourceSpec{
										Cpu:    8,
										Memory: "16Gi",
										GpuSku: "RTX5000",
										Gpu:    2,
									},
								},
								MinInstances: 1,
								NodeType:     "TRAINER_NODE",
							},
						},
					},
				},
			},
			name: "ray job with heterogeneous workers",
			expectedTotal: v1.ResourceList{
				"cpu":            *resource.NewQuantity(16, resource.DecimalSI), // 4 (head) + 4 (2*2 cpu workers) + 8 (1*8 gpu workers) = 16
				"nvidia.com/gpu": *resource.NewQuantity(3, resource.DecimalSI),  // 1 (head) + 2 (1*2 gpu workers) = 3
				"memory":         resource.MustParse("32Gi"),                    // 8Gi + 8Gi + 16Gi = 32Gi total
			},
			expectedResourceSKUs: []string{"A100", "default", "RTX5000"}, // Three different resource SKUs
			expectError:          false,
		},
		{
			name: "ray job with GPU but no SKU uses default",
			job: BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    4,
									Memory: "8Gi",
									Gpu:    1, // GPU specified but no SKU
									// GpuSku is empty
								},
							},
						},
					},
				},
			},
			expectedTotal: v1.ResourceList{
				"cpu":            *resource.NewQuantity(4, resource.DecimalSI),
				"nvidia.com/gpu": *resource.NewQuantity(1, resource.DecimalSI),
				"memory":         resource.MustParse("8Gi"),
			},
			expectedResourceSKUs: []string{"RTX5000"}, // GPU without SKU uses RTX5000 for v1 compatibility
			expectError:          false,               // Should work for backward compatibility
		},
		{
			name: "spark job with nil driver resource",
			job: BatchSparkJob{
				SparkJob: &v2beta1pb.SparkJob{
					Spec: v2beta1pb.SparkJobSpec{
						Driver: &v2beta1pb.DriverSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: nil, // Nil resource
							},
						},
					},
				},
			},
			expectedTotal: v1.ResourceList{
				"cpu":               resource.Quantity{Format: "DecimalSI"}, // 0 resources
				"nvidia.com/gpu":    resource.Quantity{Format: "DecimalSI"},
				"memory":            resource.Quantity{Format: "DecimalSI"},
				"ephemeral-storage": resource.Quantity{Format: "DecimalSI"},
			},
			expectedResourceSKUs: []string{"default"}, // Spark always uses default
			expectError:          false,               // Should handle nil gracefully
		},
		{
			name: "spark job with nil executor resource",
			job: BatchSparkJob{
				SparkJob: &v2beta1pb.SparkJob{
					Spec: v2beta1pb.SparkJobSpec{
						Executor: &v2beta1pb.ExecutorSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: nil, // Nil resource
							},
							Instances: 2,
						},
					},
				},
			},
			expectedTotal: v1.ResourceList{
				"cpu":               resource.Quantity{Format: "DecimalSI"}, // 0 resources
				"nvidia.com/gpu":    resource.Quantity{Format: "DecimalSI"},
				"memory":            resource.Quantity{Format: "DecimalSI"},
				"ephemeral-storage": resource.Quantity{Format: "DecimalSI"},
			},
			expectedResourceSKUs: []string{"default"}, // Spark always uses default
			expectError:          false,               // Should handle nil gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Helper()

			resourcesByKey, err := tt.job.GetResourceRequirement()

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Verify that all expected resource SKUs are present
			require.Equal(t, len(tt.expectedResourceSKUs), len(resourcesByKey),
				"Expected %d resource SKUs, got %d", len(tt.expectedResourceSKUs), len(resourcesByKey))

			for _, expectedSKU := range tt.expectedResourceSKUs {
				_, exists := resourcesByKey[expectedSKU]
				require.True(t, exists, "Expected resource SKU %s not found", expectedSKU)
			}

			// Verify that no unexpected keys are present
			for actualSKU := range resourcesByKey {
				found := false
				for _, expectedSKU := range tt.expectedResourceSKUs {
					if actualSKU == expectedSKU {
						found = true
						break
					}
				}
				require.True(t, found, "Unexpected resource SKU %s found", actualSKU)
			}

			// Aggregate all resources for comparison with expected total
			aggregatedResources := ConvertResourceMapToResourceList(resourcesByKey)

			// Verify that aggregated resources match expected values
			require.Equal(t, tt.expectedTotal.Cpu(), aggregatedResources.Cpu())

			// Check other resources if they exist in expected
			if gpuExpected := tt.expectedTotal.Name("nvidia.com/gpu", resource.DecimalSI); gpuExpected != nil && !gpuExpected.IsZero() {
				gpuActual := aggregatedResources.Name("nvidia.com/gpu", resource.DecimalSI)
				require.True(t, gpuExpected.Equal(*gpuActual),
					"Expected GPU %s, got %s", gpuExpected.String(), gpuActual.String())
			}

			if memoryExpected := tt.expectedTotal.Memory(); memoryExpected != nil && !memoryExpected.IsZero() {
				memoryActual := aggregatedResources.Memory()
				require.True(t, memoryExpected.Equal(*memoryActual),
					"Expected Memory %s, got %s", memoryExpected.String(), memoryActual.String())
			}
		})
	}
}

func TestGetters(t *testing.T) {
	tt := []struct {
		desc                      string
		job                       BatchJob
		wantAffinity              *v2beta1pb.Affinity
		wantAssignment            *v2beta1pb.AssignmentInfo
		wantConditions            *[]*v2beta1pb.Condition
		wantGeneration            int64
		wantName                  string
		wantNamespace             string
		wantUser                  string
		wantSchedulingPreemptible bool
		wantJobEnv                string
		wantLabels                map[string]string
		wantAnnotations           map[string]string
		wantTerminationType       v2beta1pb.TerminationType
	}{
		{
			desc: "valid ray job spec",
			job: BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ClusterAffinity: &v2beta1pb.ClusterAffinity{},
						},
						User: &v2beta1pb.UserInfo{
							Name: "dummyUser",
						},
						Scheduling: &v2beta1pb.SchedulingSpec{
							Preemptible: true,
						},
						Termination: &v2beta1pb.TerminationSpec{
							Type: v2beta1pb.TERMINATION_TYPE_SUCCEEDED,
						},
					},
					Status: v2beta1pb.RayJobStatus{StatusConditions: []*v2beta1pb.Condition{{}}},
					ObjectMeta: v1meta.ObjectMeta{
						Generation: 1,
						Namespace:  "dummyNamespace",
						Name:       "dummyName",
						Labels: map[string]string{
							sharedConstants.EnvironmentLabel: constants.Production,
						},
						Annotations: map[string]string{
							"runnable": "test_runnable",
						},
					},
				},
			},
			wantAffinity:              &v2beta1pb.Affinity{ClusterAffinity: &v2beta1pb.ClusterAffinity{}},
			wantAssignment:            &v2beta1pb.AssignmentInfo{},
			wantConditions:            &[]*v2beta1pb.Condition{{}},
			wantGeneration:            1,
			wantName:                  "dummyName",
			wantNamespace:             "dummyNamespace",
			wantUser:                  "dummyUser",
			wantSchedulingPreemptible: true,
			wantJobEnv:                v2beta1pb.ENV_TYPE_PRODUCTION.String(),
			wantLabels: map[string]string{
				sharedConstants.EnvironmentLabel: constants.Production,
			},
			wantAnnotations: map[string]string{
				"runnable": "test_runnable",
			},
			wantTerminationType: v2beta1pb.TERMINATION_TYPE_SUCCEEDED,
		},
		{
			desc: "valid spark job spec",
			job: BatchSparkJob{
				SparkJob: &v2beta1pb.SparkJob{
					Spec: v2beta1pb.SparkJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ClusterAffinity: &v2beta1pb.ClusterAffinity{},
						},
						User: &v2beta1pb.UserInfo{
							Name: "dummyUser",
						},
						Scheduling: &v2beta1pb.SchedulingSpec{
							Preemptible: false,
						},
						Termination: &v2beta1pb.TerminationSpec{
							Type: v2beta1pb.TERMINATION_TYPE_SUCCEEDED,
						},
					},
					Status: v2beta1pb.SparkJobStatus{StatusConditions: []*v2beta1pb.Condition{}},
					ObjectMeta: v1meta.ObjectMeta{
						Generation: 1,
						Namespace:  "dummyNamespace",
						Name:       "dummyName",
						Labels: map[string]string{
							sharedConstants.EnvironmentLabel: v2beta1pb.ENV_TYPE_DEVELOPMENT.String(),
						},
						Annotations: map[string]string{
							"runnable": "test_runnable",
						},
					},
				},
			},
			wantAffinity:              &v2beta1pb.Affinity{ClusterAffinity: &v2beta1pb.ClusterAffinity{}},
			wantAssignment:            &v2beta1pb.AssignmentInfo{},
			wantConditions:            &[]*v2beta1pb.Condition{},
			wantGeneration:            1,
			wantName:                  "dummyName",
			wantNamespace:             "dummyNamespace",
			wantUser:                  "dummyUser",
			wantSchedulingPreemptible: false,
			wantJobEnv:                v2beta1pb.ENV_TYPE_DEVELOPMENT.String(),
			wantLabels: map[string]string{
				sharedConstants.EnvironmentLabel: v2beta1pb.ENV_TYPE_DEVELOPMENT.String(),
			},
			wantAnnotations: map[string]string{
				"runnable": "test_runnable",
			},
			wantTerminationType: v2beta1pb.TERMINATION_TYPE_SUCCEEDED,
		},
	}

	for _, test := range tt {
		affinity := test.job.GetAffinity()
		require.Equal(t, test.wantAffinity, affinity)

		assignment := test.job.GetAssignmentInfo()
		require.Equal(t, test.wantAssignment, assignment)

		conditions := test.job.GetConditions()
		require.Equal(t, test.wantConditions, conditions)

		generation := test.job.GetGeneration()
		require.Equal(t, test.wantGeneration, generation)

		name := test.job.GetName()
		require.Equal(t, test.wantName, name)

		namespace := test.job.GetNamespace()
		require.Equal(t, test.wantNamespace, namespace)

		user := test.job.GetUserName()
		require.Equal(t, test.wantUser, user)

		isPreemptibleJob := test.job.IsPreemptibleJob()
		require.Equal(t, test.wantSchedulingPreemptible, isPreemptibleJob)

		env := test.job.GetEnvironmentLabel()
		require.Equal(t, test.wantJobEnv, env)

		require.Equal(t, test.wantLabels, test.job.GetLabels())

		require.Equal(t, test.wantAnnotations, test.job.GetAnnotations())

		require.Equal(t, test.wantTerminationType, test.job.GetTerminationSpec().Type)
	}
}

// TestAddResourcesByResourceSKU verifies that the AddResourcesByResourceSKU function
// correctly processes PodSpecs and aggregates resources with proper resource SKU determination,
// scaling, and aggregation logic.
func TestAddResourcesByResourceSKU(t *testing.T) {
	tests := []struct {
		name                 string
		initialAggregated    map[string]v1.ResourceList
		podSpec              *v2beta1pb.PodSpec
		instances            int64
		expectedAggregated   map[string]v1.ResourceList
		expectedResourceSKUs []string
		expectError          bool
	}{
		{
			name:              "nil pod spec",
			initialAggregated: make(map[string]v1.ResourceList),
			podSpec:           nil,
			instances:         1,
			expectedAggregated: map[string]v1.ResourceList{
				"default": {
					"cpu":               resource.Quantity{Format: "DecimalSI"}, // 0 CPU
					"nvidia.com/gpu":    resource.Quantity{Format: "DecimalSI"}, // 0 GPU
					"memory":            resource.Quantity{Format: "DecimalSI"}, // 0 memory
					"ephemeral-storage": resource.Quantity{Format: "DecimalSI"}, // 0 ephemeral storage for nil spec
				},
			},
			expectedResourceSKUs: []string{"default"},
			expectError:          false,
		},
		{
			name:              "empty resource spec",
			initialAggregated: make(map[string]v1.ResourceList),
			podSpec: &v2beta1pb.PodSpec{
				Resource: &v2beta1pb.ResourceSpec{},
			},
			instances: 2,
			expectedAggregated: map[string]v1.ResourceList{
				"default": {
					"cpu":               resource.Quantity{Format: "DecimalSI"}, // 0 CPU
					"nvidia.com/gpu":    resource.Quantity{Format: "DecimalSI"}, // 0 GPU
					"memory":            resource.Quantity{Format: "DecimalSI"}, // 0 memory
					"ephemeral-storage": resource.MustParse("100Gi"),            // 2 * 50Gi
				},
			},
			expectedResourceSKUs: []string{"default"},
			expectError:          false,
		},
		{
			name:              "CPU-only resources with scaling",
			initialAggregated: make(map[string]v1.ResourceList),
			podSpec: &v2beta1pb.PodSpec{
				Resource: &v2beta1pb.ResourceSpec{
					Cpu:    4,
					Memory: "8Gi",
				},
			},
			instances: 3,
			expectedAggregated: map[string]v1.ResourceList{
				"default": {
					"cpu":               *resource.NewQuantity(12, resource.DecimalSI), // 4 * 3 = 12
					"nvidia.com/gpu":    resource.Quantity{Format: "DecimalSI"},        // 0 GPU
					"memory":            resource.MustParse("24Gi"),                    // 8Gi * 3 = 24Gi
					"ephemeral-storage": resource.MustParse("150Gi"),                   // 50Gi * 3 = 150Gi
				},
			},
			expectedResourceSKUs: []string{"default"},
			expectError:          false,
		},
		{
			name:              "GPU with SKU",
			initialAggregated: make(map[string]v1.ResourceList),
			podSpec: &v2beta1pb.PodSpec{
				Resource: &v2beta1pb.ResourceSpec{
					Cpu:    4,
					Memory: "8Gi",
					GpuSku: "A100",
					Gpu:    2,
				},
			},
			instances: 1,
			expectedAggregated: map[string]v1.ResourceList{
				"A100": {
					"cpu":               *resource.NewQuantity(4, resource.DecimalSI),
					"nvidia.com/gpu":    *resource.NewQuantity(2, resource.DecimalSI),
					"memory":            resource.MustParse("8Gi"),
					"ephemeral-storage": resource.MustParse("50Gi"),
				},
			},
			expectedResourceSKUs: []string{"A100"},
			expectError:          false,
		},
		{
			name:              "GPU without SKU defaults to RTX5000",
			initialAggregated: make(map[string]v1.ResourceList),
			podSpec: &v2beta1pb.PodSpec{
				Resource: &v2beta1pb.ResourceSpec{
					Cpu:    4,
					Memory: "8Gi",
					Gpu:    1,
					// No GpuSku specified
				},
			},
			instances: 1,
			expectedAggregated: map[string]v1.ResourceList{
				"RTX5000": {
					"cpu":               *resource.NewQuantity(4, resource.DecimalSI),
					"nvidia.com/gpu":    *resource.NewQuantity(1, resource.DecimalSI),
					"memory":            resource.MustParse("8Gi"),
					"ephemeral-storage": resource.MustParse("50Gi"),
				},
			},
			expectedResourceSKUs: []string{"RTX5000"},
			expectError:          false,
		},
		{
			name: "aggregation with existing resources",
			initialAggregated: map[string]v1.ResourceList{
				"A100": {
					"cpu":               *resource.NewQuantity(2, resource.DecimalSI),
					"nvidia.com/gpu":    *resource.NewQuantity(1, resource.DecimalSI),
					"memory":            resource.MustParse("4Gi"),
					"ephemeral-storage": resource.MustParse("50Gi"),
				},
			},
			podSpec: &v2beta1pb.PodSpec{
				Resource: &v2beta1pb.ResourceSpec{
					Cpu:    4,
					Memory: "8Gi",
					GpuSku: "A100",
					Gpu:    1,
				},
			},
			instances: 2,
			expectedAggregated: map[string]v1.ResourceList{
				"A100": {
					"cpu":               *resource.NewQuantity(10, resource.DecimalSI), // 2 + (4 * 2) = 10
					"nvidia.com/gpu":    *resource.NewQuantity(3, resource.DecimalSI),  // 1 + (1 * 2) = 3
					"memory":            resource.MustParse("20Gi"),                    // 4Gi + (8Gi * 2) = 20Gi
					"ephemeral-storage": resource.MustParse("150Gi"),                   // 50Gi + (50Gi * 2) = 150Gi
				},
			},
			expectedResourceSKUs: []string{"A100"},
			expectError:          false,
		},
		{
			name: "aggregation with different resource SKUs",
			initialAggregated: map[string]v1.ResourceList{
				"default": {
					"cpu":               *resource.NewQuantity(2, resource.DecimalSI),
					"nvidia.com/gpu":    resource.Quantity{Format: "DecimalSI"},
					"memory":            resource.MustParse("4Gi"),
					"ephemeral-storage": resource.MustParse("50Gi"),
				},
			},
			podSpec: &v2beta1pb.PodSpec{
				Resource: &v2beta1pb.ResourceSpec{
					Cpu:    4,
					Memory: "8Gi",
					GpuSku: "A100",
					Gpu:    1,
				},
			},
			instances: 1,
			expectedAggregated: map[string]v1.ResourceList{
				"default": {
					"cpu":               *resource.NewQuantity(2, resource.DecimalSI),
					"nvidia.com/gpu":    resource.Quantity{Format: "DecimalSI"},
					"memory":            resource.MustParse("4Gi"),
					"ephemeral-storage": resource.MustParse("50Gi"),
				},
				"A100": {
					"cpu":               *resource.NewQuantity(4, resource.DecimalSI),
					"nvidia.com/gpu":    *resource.NewQuantity(1, resource.DecimalSI),
					"memory":            resource.MustParse("8Gi"),
					"ephemeral-storage": resource.MustParse("50Gi"),
				},
			},
			expectedResourceSKUs: []string{"default", "A100"},
			expectError:          false,
		},
		{
			name:              "invalid memory format should return error",
			initialAggregated: make(map[string]v1.ResourceList),
			podSpec: &v2beta1pb.PodSpec{
				Resource: &v2beta1pb.ResourceSpec{
					Cpu:    4,
					Memory: "invalid-memory-format",
					Gpu:    1,
				},
			},
			instances:   1,
			expectError: true,
		},
		{
			name:              "zero instances",
			initialAggregated: make(map[string]v1.ResourceList),
			podSpec: &v2beta1pb.PodSpec{
				Resource: &v2beta1pb.ResourceSpec{
					Cpu:    4,
					Memory: "8Gi",
					Gpu:    1,
				},
			},
			instances: 0,
			expectedAggregated: map[string]v1.ResourceList{
				"RTX5000": {
					"cpu":               resource.Quantity{Format: "DecimalSI"}, // 0 CPU (scaled by 0)
					"nvidia.com/gpu":    resource.Quantity{Format: "DecimalSI"}, // 0 GPU (scaled by 0)
					"memory":            resource.Quantity{Format: "DecimalSI"}, // 0 memory (scaled by 0)
					"ephemeral-storage": resource.Quantity{Format: "DecimalSI"}, // 0 ephemeral (scaled by 0)
				},
			},
			expectedResourceSKUs: []string{"RTX5000"},
			expectError:          false,
		},
		{
			name:              "high instance count scaling",
			initialAggregated: make(map[string]v1.ResourceList),
			podSpec: &v2beta1pb.PodSpec{
				Resource: &v2beta1pb.ResourceSpec{
					Cpu:    1,
					Memory: "1Gi",
					GpuSku: "V100",
					Gpu:    1,
				},
			},
			instances: 100,
			expectedAggregated: map[string]v1.ResourceList{
				"V100": {
					"cpu":               *resource.NewQuantity(100, resource.DecimalSI),
					"nvidia.com/gpu":    *resource.NewQuantity(100, resource.DecimalSI),
					"memory":            resource.MustParse("100Gi"),
					"ephemeral-storage": resource.MustParse("5000Gi"), // 50Gi * 100 = 5000Gi
				},
			},
			expectedResourceSKUs: []string{"V100"},
			expectError:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Helper()

			err := addResourcesByResourceSKU(tt.initialAggregated, tt.podSpec, tt.instances)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Verify that all expected resource SKUs are present
			require.Equal(t, len(tt.expectedResourceSKUs), len(tt.initialAggregated),
				"Expected %d resource SKUs, got %d", len(tt.expectedResourceSKUs), len(tt.initialAggregated))

			for _, expectedSKU := range tt.expectedResourceSKUs {
				_, exists := tt.initialAggregated[expectedSKU]
				require.True(t, exists, "Expected resource SKU %s not found", expectedSKU)
			}

			// Verify that no unexpected keys are present
			for actualSKU := range tt.initialAggregated {
				found := false
				for _, expectedSKU := range tt.expectedResourceSKUs {
					if actualSKU == expectedSKU {
						found = true
						break
					}
				}
				require.True(t, found, "Unexpected resource SKU %s found", actualSKU)
			}

			// Verify resource quantities for each expected resource SKU
			for _, resourceSKU := range tt.expectedResourceSKUs {
				expectedResources := tt.expectedAggregated[resourceSKU]
				actualResources := tt.initialAggregated[resourceSKU]

				// Check CPU
				expectedCPU := expectedResources.Cpu()
				actualCPU := actualResources.Cpu()
				require.True(t, expectedCPU.Equal(*actualCPU),
					"Resource SKU %s: Expected CPU %s, got %s", resourceSKU, expectedCPU.String(), actualCPU.String())

				// Check GPU if present
				if gpuExpected := expectedResources.Name("nvidia.com/gpu", resource.DecimalSI); gpuExpected != nil && !gpuExpected.IsZero() {
					gpuActual := actualResources.Name("nvidia.com/gpu", resource.DecimalSI)
					require.True(t, gpuExpected.Equal(*gpuActual),
						"Resource SKU %s: Expected GPU %s, got %s", resourceSKU, gpuExpected.String(), gpuActual.String())
				}

				// Check Memory if present
				if memoryExpected := expectedResources.Memory(); memoryExpected != nil && !memoryExpected.IsZero() {
					memoryActual := actualResources.Memory()
					require.True(t, memoryExpected.Equal(*memoryActual),
						"Resource SKU %s: Expected Memory %s, got %s", resourceSKU, memoryExpected.String(), memoryActual.String())
				}

				// Check Ephemeral Storage if present
				if ephemeralExpected := expectedResources.Name("ephemeral-storage", resource.DecimalSI); ephemeralExpected != nil && !ephemeralExpected.IsZero() {
					ephemeralActual := actualResources.Name("ephemeral-storage", resource.DecimalSI)
					require.True(t, ephemeralExpected.Equal(*ephemeralActual),
						"Resource SKU %s: Expected Ephemeral Storage %s, got %s", resourceSKU, ephemeralExpected.String(), ephemeralActual.String())
				}
			}
		})
	}
}
