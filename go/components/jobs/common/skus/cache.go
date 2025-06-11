package skus

// SkuConfigCache stores the configuration of sku alias to sku name map
type SkuConfigCache interface {
	// GetSkuName gets the name of the sku corresponding to a sku alias for
	// a given cluster
	GetSkuName(skuAlias string, clusterName string) (string, error)
}
