package framework

import (
	"context"

	"code.uber.internal/rt/flipr-client-go.git/flipr"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/cluster"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/types"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Plugin is the parent type for all the scheduling framework plugins.
type Plugin interface {
	// Name returns the name of the plugin
	Name() string
}

// FilterPlugin is a plugin to filter the resource pools for assigment
type FilterPlugin interface {
	Plugin

	// Filter is called by the scheduling framework.
	// Filter takes in the job to filter and a list of candidate resource pools.
	// It returns a list of feasible resource pools or an error indicating failure.
	Filter(ctx context.Context, job BatchJob,
		candidates []*cluster.ResourcePoolInfo) ([]*cluster.ResourcePoolInfo, error)
}

// ScorePlugin is a plugin to score the resource pools for assigment
type ScorePlugin interface {
	Plugin

	// Score is called by the scheduling framework
	// Score takes a list of filtered resource pools and scores each resource pool and returns a list of
	// resource pools in the descending order of the rank of the resource pool
	Score(ctx context.Context, job BatchJob,
		candidates []*cluster.ResourcePoolInfo) ([]*cluster.ResourcePoolInfo, error)
}

// OptionBuilder allows adding options
type OptionBuilder interface {
	// Build will apply the options
	Build(opts ...Option)

	// Logger returns a logger
	Logger() logr.Logger

	// Flipr return a flipr
	Flipr() flipr.FliprClient

	// FliprConstraintsBuilder returns a FliprConstraintsBuilder
	FliprConstraintsBuilder() types.FliprConstraintsBuilder

	// ClusterCache returns the cluster cache
	ClusterCache() cluster.RegisteredClustersCache
}

// Module provides OptionBuilder
var Module = fx.Provide(NewOptionBuilder)

// NewOptionBuilder provides default implementation for OptionBuilder
func NewOptionBuilder() OptionBuilder {
	return &basePlugin{
		log: zapr.NewLogger(zap.NewNop()),
		// we explicitly provide nil because a noOpFlipr cannot be implemented. This is because
		// the FliprClient interface uses non-visible types in method arguments.
		flipr: nil,
		// this will always be used in conjunction with the flipr client, therefore should be explicitly provided
		// along with it.
		fliprConstraintsBuilder: nil,
		clusterCache:            noOpClusterCache{},
	}
}

// BasePlugin implements building options that other plugins can use
type basePlugin struct {
	log                     logr.Logger
	flipr                   flipr.FliprClient
	fliprConstraintsBuilder types.FliprConstraintsBuilder
	clusterCache            cluster.RegisteredClustersCache
}

var _ OptionBuilder = &basePlugin{}

// Build the plugin with options
func (b *basePlugin) Build(opts ...Option) {
	for _, o := range opts {
		o.apply(b)
	}
}

// Logger returns the configured logger
func (b *basePlugin) Logger() logr.Logger {
	return b.log
}

// Flipr returns the configured flipr
func (b *basePlugin) Flipr() flipr.FliprClient {
	return b.flipr
}

// FliprConstraintsBuilder returns the configured flipr constraints builder
func (b *basePlugin) FliprConstraintsBuilder() types.FliprConstraintsBuilder {
	return b.fliprConstraintsBuilder
}

// ClusterCache return the configured cluster cache
func (b *basePlugin) ClusterCache() cluster.RegisteredClustersCache {
	return b.clusterCache
}

// Option is an operation that can be applied to a plugin
type Option interface {
	apply(plugin *basePlugin)
}
