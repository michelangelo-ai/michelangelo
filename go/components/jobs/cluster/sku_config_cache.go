package cluster

import (
	"fmt"
	"strings"
	"sync"

	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/skus"
	"go.uber.org/fx"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
)

// sku config map related constants
const (
	_configMapResourceType = "resourceType"
	_configMapSkuAlias     = "skuAlias"
	_configMapSkuName      = "skuName"
)

var _requiredSkuConfigMapDataKeys = []string{_configMapResourceType, _configMapSkuAlias, _configMapSkuName}

// SkuConfigCacheParams is the parameter for constructing new cache
type SkuConfigCacheParams struct {
	fx.In

	Log *zap.Logger
}

// NewSkuConfigCache constructs a SkuConfigCache
func NewSkuConfigCache(p SkuConfigCacheParams) skus.SkuConfigCache {
	return &skuConfigCache{
		log: p.Log.With(zap.String(constants.Component, "SkuConfigCache")),
	}
}

var _ skus.SkuConfigCache = (*skuConfigCache)(nil)

type skuConfigCache struct {
	m sync.Map

	log *zap.Logger
}

// GetSkuName return the sku name for a given sku alias in a cluster
func (s *skuConfigCache) GetSkuName(skuAlias string, clusterName string) (string, error) {
	key := s.getCacheKey(skuAlias, clusterName)
	sku, ok := s.m.Load(key)
	if !ok {
		return "", fmt.Errorf("skuAlias %s not found for cluster %s", skuAlias, clusterName)
	}

	return sku.(string), nil
}

func (s *skuConfigCache) addSkuMaps(configs []corev1.ConfigMap, clusterName string) {
	// Update with the latest values. We don't handle the case of removing stale gpu aliases
	// from the cache because it's unlikely to happen. In addition, every deployment will clear up
	// this cache.
	for _, c := range configs {
		if !s.isValidSkuConfig(c) {
			s.log.Info("configMap is not a valid sku config",
				zap.String("config_map", c.String()))
			continue
		}

		key := s.getCacheKey(c.Data[_configMapSkuAlias], clusterName)

		// always update with the latest value
		s.m.Swap(key, c.Data[_configMapSkuName])
	}
}

// Ideally we shouldn't need to do this. The Compute team should validate and sanitize this list
// before adding the config map and provide a way to filter out any maps that are not a valid sku config.
func (s *skuConfigCache) isValidSkuConfig(c corev1.ConfigMap) bool {
	for _, k := range _requiredSkuConfigMapDataKeys {
		if _, ok := c.Data[k]; !ok {
			return false
		}
	}

	return c.Data[_configMapResourceType] == constants.ResourceNvidiaGPU.String()
}

func (s *skuConfigCache) getCacheKey(skuAlias string, clusterName string) string {
	return strings.ToLower(clusterName + "-" + skuAlias)
}
