package plugins

import (
	"testing"

	infraCrds "code.uber.internal/infra/compute/k8s-crds/apis/compute.uber.com/v1beta2"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/cluster"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestGetResourceConfigAndUsage(t *testing.T) {
	tests := []struct {
		name            string
		poolInfo        *cluster.ResourcePoolInfo
		sku             string
		expectedConfig  []infraCrds.ResourceConfig
		expectedUsage   corev1.ResourceList
		expectedCanHost bool
	}{
		{
			name: "v1beta1 format - aggregated resources",
			poolInfo: &cluster.ResourcePoolInfo{
				Pool: infraCrds.ResourcePool{
					Spec: infraCrds.ResourcePoolSpec{
						Resources: []infraCrds.ResourceConfig{
							{
								Kind:        corev1.ResourceCPU.String(),
								Reservation: *resource.NewQuantity(100, resource.DecimalSI),
								Limit:       *resource.NewQuantity(200, resource.DecimalSI),
							},
							{
								Kind:        corev1.ResourceMemory.String(),
								Reservation: *resource.NewScaledQuantity(50, 9),
								Limit:       *resource.NewScaledQuantity(100, 9),
							},
						},
					},
					Status: infraCrds.ResourcePoolStatus{
						Usage: corev1.ResourceList{
							corev1.ResourceCPU:    *resource.NewQuantity(30, resource.DecimalSI),
							corev1.ResourceMemory: *resource.NewScaledQuantity(20, 9),
						},
					},
				},
			},
			sku: "A100_SXM4",
			expectedConfig: []infraCrds.ResourceConfig{
				{
					Kind:        corev1.ResourceCPU.String(),
					Reservation: *resource.NewQuantity(100, resource.DecimalSI),
					Limit:       *resource.NewQuantity(200, resource.DecimalSI),
				},
				{
					Kind:        corev1.ResourceMemory.String(),
					Reservation: *resource.NewScaledQuantity(50, 9),
					Limit:       *resource.NewScaledQuantity(100, 9),
				},
			},
			expectedUsage: corev1.ResourceList{
				corev1.ResourceCPU:    *resource.NewQuantity(30, resource.DecimalSI),
				corev1.ResourceMemory: *resource.NewScaledQuantity(20, 9),
			},
			expectedCanHost: true,
		},
		{
			name: "v1beta2 format - SKU exists with usage",
			poolInfo: &cluster.ResourcePoolInfo{
				Pool: infraCrds.ResourcePool{
					Spec: infraCrds.ResourcePoolSpec{
						ResourceMap: map[string]infraCrds.ResourceConfiguration{
							"A100_SXM4": {
								Resources: []infraCrds.ResourceConfig{
									{
										Kind:        corev1.ResourceCPU.String(),
										Reservation: *resource.NewQuantity(50, resource.DecimalSI),
										Limit:       *resource.NewQuantity(100, resource.DecimalSI),
									},
									{
										Kind:        constants.ResourceNvidiaGPU.String(),
										Reservation: *resource.NewQuantity(4, resource.DecimalSI),
										Limit:       *resource.NewQuantity(8, resource.DecimalSI),
									},
								},
							},
							"RTX5000": {
								Resources: []infraCrds.ResourceConfig{
									{
										Kind:        corev1.ResourceCPU.String(),
										Reservation: *resource.NewQuantity(30, resource.DecimalSI),
										Limit:       *resource.NewQuantity(60, resource.DecimalSI),
									},
								},
							},
						},
					},
					Status: infraCrds.ResourcePoolStatus{
						UsageMap: map[string]corev1.ResourceList{
							"A100_SXM4": {
								corev1.ResourceCPU:          *resource.NewQuantity(20, resource.DecimalSI),
								constants.ResourceNvidiaGPU: *resource.NewQuantity(2, resource.DecimalSI),
							},
						},
					},
				},
			},
			sku: "A100_SXM4",
			expectedConfig: []infraCrds.ResourceConfig{
				{
					Kind:        corev1.ResourceCPU.String(),
					Reservation: *resource.NewQuantity(50, resource.DecimalSI),
					Limit:       *resource.NewQuantity(100, resource.DecimalSI),
				},
				{
					Kind:        constants.ResourceNvidiaGPU.String(),
					Reservation: *resource.NewQuantity(4, resource.DecimalSI),
					Limit:       *resource.NewQuantity(8, resource.DecimalSI),
				},
			},
			expectedUsage: corev1.ResourceList{
				corev1.ResourceCPU:          *resource.NewQuantity(20, resource.DecimalSI),
				constants.ResourceNvidiaGPU: *resource.NewQuantity(2, resource.DecimalSI),
			},
			expectedCanHost: true,
		},
		{
			name: "v1beta2 format - SKU exists without usage",
			poolInfo: &cluster.ResourcePoolInfo{
				Pool: infraCrds.ResourcePool{
					Spec: infraCrds.ResourcePoolSpec{
						ResourceMap: map[string]infraCrds.ResourceConfiguration{
							"defaultCPU": {
								Resources: []infraCrds.ResourceConfig{
									{
										Kind:        corev1.ResourceCPU.String(),
										Reservation: *resource.NewQuantity(100, resource.DecimalSI),
										Limit:       *resource.NewQuantity(200, resource.DecimalSI),
									},
								},
							},
						},
					},
					Status: infraCrds.ResourcePoolStatus{
						// No UsageMap provided
					},
				},
			},
			sku: "defaultCPU",
			expectedConfig: []infraCrds.ResourceConfig{
				{
					Kind:        corev1.ResourceCPU.String(),
					Reservation: *resource.NewQuantity(100, resource.DecimalSI),
					Limit:       *resource.NewQuantity(200, resource.DecimalSI),
				},
			},
			expectedUsage:   corev1.ResourceList{},
			expectedCanHost: true,
		},
		{
			name: "v1beta2 format - SKU not found",
			poolInfo: &cluster.ResourcePoolInfo{
				Pool: infraCrds.ResourcePool{
					Spec: infraCrds.ResourcePoolSpec{
						ResourceMap: map[string]infraCrds.ResourceConfiguration{
							"A100_SXM4": {
								Resources: []infraCrds.ResourceConfig{
									{
										Kind:        constants.ResourceNvidiaGPU.String(),
										Reservation: *resource.NewQuantity(4, resource.DecimalSI),
									},
								},
							},
						},
					},
				},
			},
			sku:             "RTX5000",
			expectedConfig:  nil,
			expectedUsage:   nil,
			expectedCanHost: false,
		},
		{
			name: "no resources defined",
			poolInfo: &cluster.ResourcePoolInfo{
				Pool: infraCrds.ResourcePool{
					Spec: infraCrds.ResourcePoolSpec{
						// No Resources or ResourceMap
					},
				},
			},
			sku:             "defaultCPU",
			expectedConfig:  nil,
			expectedUsage:   nil,
			expectedCanHost: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Helper()

			config, usage, canHost := getResourceConfigAndUsage(tt.poolInfo, tt.sku)

			assert.Equal(t, tt.expectedCanHost, canHost)
			assert.Equal(t, tt.expectedConfig, config)
			assert.Equal(t, tt.expectedUsage, usage)
		})
	}
}

// TestConvertResourceMapToResourceList ensures that resource aggregation correctly sums resource quantities across SKUs,
// validating proper map-reduce semantics for resource management across different resource types (CPU, GPU, memory, storage).
func TestConvertResourceMapToResourceList(t *testing.T) {
	tests := []struct {
		name           string
		resourcesBySKU map[string]corev1.ResourceList
		expected       corev1.ResourceList
	}{
		{
			name:           "empty map",
			resourcesBySKU: map[string]corev1.ResourceList{},
			expected:       corev1.ResourceList{},
		},
		{
			name: "single resource SKU",
			resourcesBySKU: map[string]corev1.ResourceList{
				"A100": {
					"cpu":            *resource.NewQuantity(4, resource.DecimalSI),
					"nvidia.com/gpu": *resource.NewQuantity(1, resource.DecimalSI),
					"memory":         resource.MustParse("8Gi"),
				},
			},
			expected: corev1.ResourceList{
				"cpu":            *resource.NewQuantity(4, resource.DecimalSI),
				"nvidia.com/gpu": *resource.NewQuantity(1, resource.DecimalSI),
				"memory":         resource.MustParse("8Gi"),
			},
		},
		{
			name: "multiple resource SKUs aggregation",
			resourcesBySKU: map[string]corev1.ResourceList{
				"A100": {
					"cpu":            *resource.NewQuantity(4, resource.DecimalSI),
					"nvidia.com/gpu": *resource.NewQuantity(1, resource.DecimalSI),
					"memory":         resource.MustParse("8Gi"),
				},
				"default": {
					"cpu":    *resource.NewQuantity(2, resource.DecimalSI),
					"memory": resource.MustParse("4Gi"),
				},
				"RTX5000": {
					"cpu":            *resource.NewQuantity(8, resource.DecimalSI),
					"nvidia.com/gpu": *resource.NewQuantity(2, resource.DecimalSI),
					"memory":         resource.MustParse("16Gi"),
				},
			},
			expected: corev1.ResourceList{
				"cpu":            *resource.NewQuantity(14, resource.DecimalSI), // 4 + 2 + 8 = 14
				"nvidia.com/gpu": *resource.NewQuantity(3, resource.DecimalSI),  // 1 + 0 + 2 = 3
				"memory":         resource.MustParse("28Gi"),                    // 8 + 4 + 16 = 28
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Helper()

			result := ConvertResourceMapToResourceList(tt.resourcesBySKU)

			// Check CPU
			expectedCPU := tt.expected.Cpu()
			actualCPU := result.Cpu()
			require.True(t, expectedCPU.Equal(*actualCPU),
				"Expected CPU %s, got %s", expectedCPU.String(), actualCPU.String())

			// Check GPU if present
			if gpuExpected := tt.expected.Name("nvidia.com/gpu", resource.DecimalSI); gpuExpected != nil && !gpuExpected.IsZero() {
				gpuActual := result.Name("nvidia.com/gpu", resource.DecimalSI)
				require.True(t, gpuExpected.Equal(*gpuActual),
					"Expected GPU %s, got %s", gpuExpected.String(), gpuActual.String())
			}

			// Check Memory if present
			if memoryExpected := tt.expected.Memory(); memoryExpected != nil && !memoryExpected.IsZero() {
				memoryActual := result.Memory()
				require.True(t, memoryExpected.Equal(*memoryActual),
					"Expected Memory %s, got %s", memoryExpected.String(), memoryActual.String())
			}
		})
	}
}
