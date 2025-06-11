package cluster

import (
	"testing"

	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestIsValidSkuConfig(t *testing.T) {
	tt := []struct {
		c     corev1.ConfigMap
		valid bool
		msg   string
	}{
		{
			c: corev1.ConfigMap{
				Data: map[string]string{
					_configMapResourceType: constants.ResourceNvidiaGPU.String(),
					_configMapSkuAlias:     "A100",
					_configMapSkuName:      "NVIDIA_Tensor_Core_A100",
				},
			},
			valid: true,
			msg:   "valid a100 sku name",
		},
		{
			c: corev1.ConfigMap{
				Data: map[string]string{
					"ca.crt": "API server cert for the namespace",
				},
			},
			valid: false,
			msg:   "non resource config map",
		},
		{
			c: corev1.ConfigMap{
				Data: map[string]string{
					_configMapResourceType: "non-gpu resource type",
					_configMapSkuAlias:     "non-gpu",
					_configMapSkuName:      "non gpu hardware sku",
				},
			},
			valid: false,
			msg:   "non gpu hardware config",
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			s := &skuConfigCache{}
			require.Equal(t, test.valid, s.isValidSkuConfig(test.c))
		})
	}
}

func TestGetCacheKey(t *testing.T) {
	tt := []struct {
		skuAlias         string
		clusterName      string
		expectedCacheKey string
	}{
		{
			skuAlias:         "P6000",
			clusterName:      "phx4-kubernetes-batch01",
			expectedCacheKey: "phx4-kubernetes-batch01-p6000",
		},
		{
			skuAlias:         "A100",
			clusterName:      "phx5-kubernetes-batch01",
			expectedCacheKey: "phx5-kubernetes-batch01-a100",
		},
	}

	for _, test := range tt {
		s := &skuConfigCache{}
		require.Equal(t, test.expectedCacheKey, s.getCacheKey(test.skuAlias, test.clusterName))
	}
}
