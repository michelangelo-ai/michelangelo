package framework

import (
	"testing"

	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/cluster"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/types"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	v2beta1pb "michelangelo/api/v2beta1"
	mockFlipr "mock/code.uber.internal/rt/flipr-client-go.git/flipr/fliprmock"
)

func TestLoggerOption(t *testing.T) {
	tt := []struct {
		msg                    string
		wantBeforeModification loggerOption
		wantAfterModification  logr.Logger
	}{
		{
			msg:                    "logger option test",
			wantBeforeModification: loggerOption{log: logr.Logger{}.V(1)},
			wantAfterModification:  logr.Logger{}.V(0),
		},
	}

	for _, test := range tt {
		logger := WithLogger(logr.Logger{}.V(1))
		require.NotNil(t, logger)
		require.Equal(t, test.wantBeforeModification, logger)

		basePlugin := basePlugin{log: NewOptionBuilder().Logger().V(1)}
		require.NotNil(t, basePlugin)
		require.NotEqual(t, test.wantBeforeModification.log, basePlugin.log)

		opt := loggerOption{}
		basePlugin.Build(opt)
		require.Equal(t, test.wantAfterModification, basePlugin.log)
	}
}

func TestFliprOption(t *testing.T) {
	g := gomock.NewController(t)
	mockFlipr := mockFlipr.NewMockFliprClient(g)
	flipr := WithFlipr(mockFlipr)
	require.NotNil(t, flipr)

	basePlugin := basePlugin{flipr: NewOptionBuilder().Flipr()}
	require.NotNil(t, basePlugin)
	require.Nil(t, basePlugin.flipr)

	opt := fliprOption{}
	basePlugin.Build(opt)
	require.Nil(t, basePlugin.flipr)
}

func TestFliprConstraintsBuilderOption(t *testing.T) {
	basePlugin := basePlugin{}
	require.NotNil(t, basePlugin)
	require.Nil(t, basePlugin.fliprConstraintsBuilder)

	fliprConstraintsBuilder := WithFliprConstraintsBuilder(NewOptionBuilder().FliprConstraintsBuilder())
	require.NotNil(t, fliprConstraintsBuilder)
	basePlugin.Build(fliprConstraintsBuilder)
	require.Nil(t, basePlugin.fliprConstraintsBuilder)

	basePlugin.Build(WithFliprConstraintsBuilder(types.NewFliprConstraintsBuilder()))
	require.NotNil(t, basePlugin.fliprConstraintsBuilder)

	constraints := basePlugin.FliprConstraintsBuilder().GetFliprConstraints(map[string]interface{}{})
	require.NotNil(t, constraints)
	require.Equal(t, 1, len(constraints))
}

func TestClusterCacheOption(t *testing.T) {
	tt := []struct {
		msg           string
		wantInitial   clusterCacheOption
		wantPostApply cluster.RegisteredClustersCache
	}{
		{
			msg:           "cluster cache option test",
			wantInitial:   clusterCacheOption{},
			wantPostApply: nil,
		},
	}

	for _, test := range tt {
		basePlugin := basePlugin{}
		clusterCache := WithClusterCache(basePlugin.ClusterCache())
		require.NotNil(t, clusterCache)
		require.Equal(t, test.wantInitial, clusterCache)

		basePlugin.Build(clusterCache)
		require.NotNil(t, basePlugin)
		require.Equal(t, test.wantPostApply, basePlugin.clusterCache)
	}
}

func TestDefaultOptions(t *testing.T) {
	tt := []struct {
		msg             string
		wantGetCluster  *v2beta1pb.Cluster
		wantGetClusters *v2beta1pb.Cluster
	}{
		{
			msg:             "default cluster cache option test",
			wantGetCluster:  nil,
			wantGetClusters: nil,
		},
	}

	for _, test := range tt {
		clusterCache := noOpClusterCache{}
		require.NotNil(t, clusterCache)

		require.Nil(t, test.wantGetCluster, clusterCache.GetCluster("test"))
		require.Nil(t, test.wantGetClusters, clusterCache.GetClusters(1))
	}
}
