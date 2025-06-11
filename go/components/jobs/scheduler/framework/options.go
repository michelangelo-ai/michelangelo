package framework

import (
	"code.uber.internal/rt/flipr-client-go.git/flipr"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/cluster"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/types"
	"github.com/go-logr/logr"
)

type loggerOption struct {
	log logr.Logger
}

var _ Option = loggerOption{}

func (l loggerOption) apply(p *basePlugin) {
	p.log = l.log
}

// WithLogger adds logging to a plugin
func WithLogger(log logr.Logger) Option {
	return loggerOption{
		log: log,
	}
}

type fliprOption struct {
	flipr flipr.FliprClient
}

var _ Option = fliprOption{}

func (f fliprOption) apply(p *basePlugin) {
	p.flipr = f.flipr
}

// WithFlipr adds flipr to a plugin
func WithFlipr(flipr flipr.FliprClient) Option {
	return fliprOption{
		flipr: flipr,
	}
}

type fliprConstraintsBuilderOption struct {
	builder types.FliprConstraintsBuilder
}

var _ Option = fliprConstraintsBuilderOption{}

func (b fliprConstraintsBuilderOption) apply(p *basePlugin) {
	p.fliprConstraintsBuilder = b.builder
}

// WithFliprConstraintsBuilder adds flipr constraints builder to a plugin
func WithFliprConstraintsBuilder(builder types.FliprConstraintsBuilder) Option {
	return fliprConstraintsBuilderOption{
		builder: builder,
	}
}

type clusterCacheOption struct {
	clusterCache cluster.RegisteredClustersCache
}

var _ Option = clusterCacheOption{}

func (c clusterCacheOption) apply(p *basePlugin) {
	p.clusterCache = c.clusterCache
}

// WithClusterCache allow using the cluster cache in a plugin
func WithClusterCache(cache cluster.RegisteredClustersCache) Option {
	return clusterCacheOption{
		clusterCache: cache,
	}
}
