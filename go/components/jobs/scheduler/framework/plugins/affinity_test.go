package plugins

import (
	"context"
	"errors"
	"testing"

	infraCrds "code.uber.internal/infra/compute/k8s-crds/apis/compute.uber.com/v1beta2"
	"code.uber.internal/rt/flipr-client-go.git/flipr"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/cluster"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/types"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/scheduler/framework"
	sharedconstants "code.uber.internal/uberai/michelangelo/shared/constants"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v2beta1pb "michelangelo/api/v2beta1"
	"mock/code.uber.internal/rt/flipr-client-go.git/flipr/fliprmock"
	"mock/code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/cluster/clustermock"
	"mock/code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/types/typesmock"
)

func TestFilterUsingAffinity(t *testing.T) {

	tt := []struct {
		msg               string
		affinity          *v2beta1pb.ResourceAffinity
		pools             []*cluster.ResourcePoolInfo
		expectedPoolNames []string
	}{
		{
			msg: "matches only one label",
			affinity: &v2beta1pb.ResourceAffinity{
				Selector: &v1.LabelSelector{
					MatchLabels: map[string]string{
						"key1": "value1",
					},
				},
			},
			pools: []*cluster.ResourcePoolInfo{
				{
					ClusterName: "test-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool1",
							Labels: map[string]string{
								"key1": "value1",
								"key2": "value2",
							},
						},
					},
				},
				{
					ClusterName: "test-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool2",
							Labels: map[string]string{
								"key2": "value2",
							},
						},
					},
				},
			},
			expectedPoolNames: []string{
				"pool1",
			},
		},
		{
			msg: "match resourcepool matching all labels",
			affinity: &v2beta1pb.ResourceAffinity{
				Selector: &v1.LabelSelector{
					MatchLabels: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
				},
			},
			pools: []*cluster.ResourcePoolInfo{
				{
					ClusterName: "test-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool1",
							Labels: map[string]string{
								"key1": "value1",
							},
						},
					},
				},
				{
					ClusterName: "test-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool2",
							Labels: map[string]string{
								"key2": "value2",
							},
						},
					},
				},
				{
					ClusterName: "test-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool3",
							Labels: map[string]string{
								"key1": "value1",
								"key2": "value2",
							},
						},
					},
				},
			},
			expectedPoolNames: []string{
				"pool3",
			},
		},
		{
			msg:      "match all resource pools if affinity is nil",
			affinity: nil,
			pools: []*cluster.ResourcePoolInfo{
				{
					ClusterName: "test-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool1",
							Labels: map[string]string{
								"key1": "value1",
							},
						},
					},
				},
				{
					ClusterName: "test-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool2",
							Labels: map[string]string{
								"key2": "value2",
							},
						},
					},
				},
				{
					ClusterName: "test-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool3",
							Labels: map[string]string{
								"key1": "value1",
								"key2": "value2",
							},
						},
					},
				},
			},
			expectedPoolNames: []string{
				"pool1",
				"pool2",
				"pool3",
			},
		},
		{
			msg:      "match all resource pools if affinity is empty",
			affinity: &v2beta1pb.ResourceAffinity{},
			pools: []*cluster.ResourcePoolInfo{
				{
					ClusterName: "test-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool1",
							Labels: map[string]string{
								"key1": "value1",
							},
						},
					},
				},
				{
					ClusterName: "test-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool2",
							Labels: map[string]string{
								"key2": "value2",
							},
						},
					},
				},
				{
					ClusterName: "test-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool3",
							Labels: map[string]string{
								"key1": "value1",
								"key2": "value2",
							},
						},
					},
				},
			},
			expectedPoolNames: []string{
				"pool1",
				"pool2",
				"pool3",
			},
		},
		{
			msg: "match all resource pools if selector is empty",
			affinity: &v2beta1pb.ResourceAffinity{
				Selector: &v1.LabelSelector{},
			},
			pools: []*cluster.ResourcePoolInfo{
				{
					ClusterName: "test-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool1",
							Labels: map[string]string{
								"key1": "value1",
							},
						},
					},
				},
				{
					ClusterName: "test-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool2",
							Labels: map[string]string{
								"key2": "value2",
							},
						},
					},
				},
				{
					ClusterName: "test-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool3",
							Labels: map[string]string{
								"key1": "value1",
								"key2": "value2",
							},
						},
					},
				},
			},
			expectedPoolNames: []string{
				"pool1",
				"pool2",
				"pool3",
			},
		},
		{
			msg: "match resource pool with name",
			affinity: &v2beta1pb.ResourceAffinity{
				Selector: &v1.LabelSelector{
					MatchLabels: map[string]string{
						_resourceNameLabelKey: "/pool1",
					},
				},
			},
			pools: []*cluster.ResourcePoolInfo{
				{
					ClusterName: "test-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool1",
							Labels: map[string]string{
								"key1": "value1",
							},
						},
						Status: infraCrds.ResourcePoolStatus{
							Path: "/pool1",
						},
					},
				},
				{
					ClusterName: "test-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool2",
							Labels: map[string]string{
								"key2": "value2",
							},
						},
						Status: infraCrds.ResourcePoolStatus{
							Path: "/pool2",
						},
					},
				},
				{
					ClusterName: "test-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool3",
							Labels: map[string]string{
								"key1": "value1",
								"key2": "value2",
							},
						},
					},
				},
			},
			expectedPoolNames: []string{
				"pool1",
			},
		},
		{
			msg: "case insensitive comparison",
			affinity: &v2beta1pb.ResourceAffinity{
				Selector: &v1.LabelSelector{
					MatchLabels: map[string]string{
						_resourceNameLabelKey: "/Pool1",
						"key1":                "Value1",
					},
				},
			},
			pools: []*cluster.ResourcePoolInfo{
				{
					ClusterName: "test-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool1",
							Labels: map[string]string{
								"key1": "value1",
							},
						},
						Status: infraCrds.ResourcePoolStatus{
							Path: "/pool1",
						},
					},
				},
				{
					ClusterName: "test-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool2",
							Labels: map[string]string{
								"key2": "value2",
							},
						},
						Status: infraCrds.ResourcePoolStatus{
							Path: "/pool2",
						},
					},
				},
				{
					ClusterName: "test-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool3",
							Labels: map[string]string{
								"key1": "value1",
								"key2": "value2",
							},
						},
					},
				},
			},
			expectedPoolNames: []string{
				"pool1",
			},
		},
		{
			msg: "check implicit gpu anti affinity",
			affinity: &v2beta1pb.ResourceAffinity{
				Selector: &v1.LabelSelector{
					MatchLabels: map[string]string{
						"key1": "value1",
					},
				},
			},
			pools: []*cluster.ResourcePoolInfo{
				{
					ClusterName: "test-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool1",
							Labels: map[string]string{
								"key1":          "value1",
								_gpuAffinityKey: "true",
							},
						},
						Status: infraCrds.ResourcePoolStatus{
							Path: "/pool1",
						},
					},
				},
				{
					ClusterName: "test-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool2",
							Labels: map[string]string{
								"key1":          "value1",
								_gpuAffinityKey: "false",
							},
						},
						Status: infraCrds.ResourcePoolStatus{
							Path: "/pool2",
						},
					},
				},
				{
					ClusterName: "test-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool3",
							Labels: map[string]string{
								"key1": "value1",
							},
						},
					},
				},
			},
			expectedPoolNames: []string{
				"pool2",
				"pool3",
			},
		},
		{
			msg: "check implicit specific gpu anti affinity",
			affinity: &v2beta1pb.ResourceAffinity{
				Selector: &v1.LabelSelector{
					MatchLabels: map[string]string{
						_gpuAffinityKey: "true",
					},
				},
			},
			pools: []*cluster.ResourcePoolInfo{
				{
					ClusterName: "test-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool1",
							Labels: map[string]string{
								_gpuAffinityKey: "true",
								constants.ResourcePoolSpecialResourceAlias: "P6000",
							},
						},
						Status: infraCrds.ResourcePoolStatus{
							Path: "/pool1",
						},
					},
				},
				{
					ClusterName: "test-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool2",
							Labels: map[string]string{
								_gpuAffinityKey: "true",
							},
						},
						Status: infraCrds.ResourcePoolStatus{
							Path: "/pool2",
						},
					},
				},
			},
			expectedPoolNames: []string{
				"pool2",
			},
		},
		{
			msg: "check that zone dca60 can be used when specified",
			affinity: &v2beta1pb.ResourceAffinity{
				Selector: &v1.LabelSelector{
					MatchLabels: map[string]string{
						"key1":              "value1",
						ClusterZoneLabelKey: "dca60",
					},
				},
			},
			pools: []*cluster.ResourcePoolInfo{
				{
					ClusterName: "test-cluster-phx5",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool1",
							Labels: map[string]string{
								"key1":              "value1",
								ClusterZoneLabelKey: "phx5",
							},
						},
						Status: infraCrds.ResourcePoolStatus{
							Path: "/pool1",
						},
					},
				},
				{
					ClusterName: "test-cluster-dca60",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool2",
							Labels: map[string]string{
								"key1":              "value1",
								ClusterZoneLabelKey: "dca60",
							},
						},
						Status: infraCrds.ResourcePoolStatus{
							Path: "/pool2",
						},
					},
				},
			},
			expectedPoolNames: []string{
				"pool2",
			},
		},
		{
			msg: "check that zone dca20 can be used when specified",
			affinity: &v2beta1pb.ResourceAffinity{
				Selector: &v1.LabelSelector{
					MatchLabels: map[string]string{
						"key1":              "value1",
						ClusterZoneLabelKey: "dca20",
					},
				},
			},
			pools: []*cluster.ResourcePoolInfo{
				{
					ClusterName: "test-cluster-phx5",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool1",
							Labels: map[string]string{
								"key1":              "value1",
								ClusterZoneLabelKey: "phx5",
							},
						},
						Status: infraCrds.ResourcePoolStatus{
							Path: "/pool1",
						},
					},
				},
				{
					ClusterName: "test-cluster-dca60",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool2",
							Labels: map[string]string{
								"key1":              "value1",
								ClusterZoneLabelKey: "dca20",
							},
						},
						Status: infraCrds.ResourcePoolStatus{
							Path: "/pool2",
						},
					},
				},
			},
			expectedPoolNames: []string{
				"pool2",
			},
		},
		{
			msg: "check that zone phx60 can be used when specified",
			affinity: &v2beta1pb.ResourceAffinity{
				Selector: &v1.LabelSelector{
					MatchLabels: map[string]string{
						"key1":              "value1",
						ClusterZoneLabelKey: "phx60",
					},
				},
			},
			pools: []*cluster.ResourcePoolInfo{
				{
					ClusterName: "test-cluster-phx5",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool1",
							Labels: map[string]string{
								"key1":              "value1",
								ClusterZoneLabelKey: "phx5",
							},
						},
						Status: infraCrds.ResourcePoolStatus{
							Path: "/pool1",
						},
					},
				},
				{
					ClusterName: "test-cluster-dca60",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool2",
							Labels: map[string]string{
								"key1":              "value1",
								ClusterZoneLabelKey: "phx60",
							},
						},
						Status: infraCrds.ResourcePoolStatus{
							Path: "/pool2",
						},
					},
				},
			},
			expectedPoolNames: []string{
				"pool2",
			},
		},
		{
			msg: "check that A100 can be used when zone is not specified if sku is specified",
			affinity: &v2beta1pb.ResourceAffinity{
				Selector: &v1.LabelSelector{
					MatchLabels: map[string]string{
						"key1": "value1",
						constants.ResourcePoolSpecialResourceAlias: "A100",
					},
				},
			},
			pools: []*cluster.ResourcePoolInfo{
				{
					ClusterName: "test-cluster-phx5",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool1",
							Labels: map[string]string{
								"key1":              "value1",
								ClusterZoneLabelKey: "phx5",
							},
						},
						Status: infraCrds.ResourcePoolStatus{
							Path: "/pool1",
						},
					},
				},
				{
					ClusterName: "test-cluster-phx8",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool2",
							Labels: map[string]string{
								"key1":              "value1",
								ClusterZoneLabelKey: "phx8",
								constants.ResourcePoolSpecialResourceAlias: "A100",
							},
						},
						Status: infraCrds.ResourcePoolStatus{
							Path: "/pool2",
						},
					},
				},
			},
			expectedPoolNames: []string{
				"pool2",
			},
		},
		{
			msg: "V1 GPU job without SKU label blocked by special GPU pools (can only use default RTX5000 pools)",
			affinity: &v2beta1pb.ResourceAffinity{
				Selector: &v1.LabelSelector{
					MatchLabels: map[string]string{
						"key1":          "value1",
						_gpuAffinityKey: "true",
						// No ClusterRegionProviderLabelKey - this is a V1 job
						// No ResourcePoolSpecialResourceAlias - V1 job without explicit GPU SKU
					},
				},
			},
			pools: []*cluster.ResourcePoolInfo{
				{
					ClusterName: "test-cluster-phx5",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool1",
							Labels: map[string]string{
								"key1":          "value1",
								_gpuAffinityKey: "true",
								constants.ResourcePoolSpecialResourceAlias: "A100", // Special GPU pool
							},
						},
						Status: infraCrds.ResourcePoolStatus{
							Path: "/pool1",
						},
					},
				},
				{
					ClusterName: "test-cluster-phx8",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool2",
							Labels: map[string]string{
								"key1":          "value1",
								_gpuAffinityKey: "true",
								// No special resource alias - generic GPU pool
							},
						},
						Status: infraCrds.ResourcePoolStatus{
							Path: "/pool2",
						},
					},
				},
			},
			expectedPoolNames: []string{
				"pool2", // Only generic GPU pool, special GPU pool blocked by anti-affinity
			},
		},
		{
			msg: "check that zone dca60 cannot be used when not explicitly specified",
			affinity: &v2beta1pb.ResourceAffinity{
				Selector: &v1.LabelSelector{
					MatchLabels: map[string]string{
						"key1": "value1",
					},
				},
			},
			pools: []*cluster.ResourcePoolInfo{
				{
					ClusterName: "test-cluster-phx5",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool1",
							Labels: map[string]string{
								"key1":              "value1",
								ClusterZoneLabelKey: "phx5",
							},
						},
						Status: infraCrds.ResourcePoolStatus{
							Path: "/pool1",
						},
					},
				},
				{
					ClusterName: "test-cluster-dca60",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool2",
							Labels: map[string]string{
								"key1":              "value1",
								ClusterZoneLabelKey: "dca60",
							},
						},
						Status: infraCrds.ResourcePoolStatus{
							Path: "/pool2",
						},
					},
				},
			},
			expectedPoolNames: []string{
				"pool1",
			},
		},
		{
			msg: "check that zone dca20 cannot be used when not explicitly specified",
			affinity: &v2beta1pb.ResourceAffinity{
				Selector: &v1.LabelSelector{
					MatchLabels: map[string]string{
						"key1": "value1",
					},
				},
			},
			pools: []*cluster.ResourcePoolInfo{
				{
					ClusterName: "test-cluster-phx5",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool1",
							Labels: map[string]string{
								"key1":              "value1",
								ClusterZoneLabelKey: "phx5",
							},
						},
						Status: infraCrds.ResourcePoolStatus{
							Path: "/pool1",
						},
					},
				},
				{
					ClusterName: "test-cluster-dca60",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool2",
							Labels: map[string]string{
								"key1":              "value1",
								ClusterZoneLabelKey: "dca20",
							},
						},
						Status: infraCrds.ResourcePoolStatus{
							Path: "/pool2",
						},
					},
				},
			},
			expectedPoolNames: []string{
				"pool1",
			},
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			g := gomock.NewController(t)
			clusterCache := clustermock.NewMockRegisteredClustersCache(g)

			for _, pool := range test.pools {
				poolCluster := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: _defaultJobRegion,
					},
				}

				if zone, ok := pool.Pool.Labels[ClusterZoneLabelKey]; ok {
					poolCluster.Spec.Zone = zone
				}
				clusterCache.EXPECT().GetCluster(pool.ClusterName).Return(poolCluster).AnyTimes()
			}

			mockFlipr := fliprmock.NewMockFliprClient(g)
			mockFlipr.EXPECT().GetStringValue(gomock.Any(), _fliprRayJobsInCloud, gomock.Any(), "").
				Return("", nil)

			mockConstraints := typesmock.NewMockFliprConstraintsBuilder(g)
			mockConstraints.EXPECT().GetFliprConstraints(gomock.Any()).Return(flipr.Constraints{})

			opts := framework.NewOptionBuilder()
			opts.Build(framework.WithClusterCache(clusterCache), framework.WithFlipr(mockFlipr), framework.WithFliprConstraintsBuilder(mockConstraints))
			affinityFilter := AffinityFilter{
				OptionBuilder: opts,
			}
			matches, err := affinityFilter.Filter(
				context.Background(),
				framework.BatchRayJob{
					RayJob: &v2beta1pb.RayJob{
						Spec: v2beta1pb.RayJobSpec{
							Affinity: &v2beta1pb.Affinity{
								ResourceAffinity: test.affinity,
							},
						},
					}}, test.pools)
			require.NoError(t, err)
			require.Equal(t, len(test.expectedPoolNames), len(matches))

			for i, name := range test.expectedPoolNames {
				require.Equal(t, name, matches[i].Pool.Name)
			}
		})
	}
}

func TestMatchClusterSelector(t *testing.T) {
	tt := []struct {
		msg             string
		labels          map[string]string
		candidates      []*cluster.ResourcePoolInfo
		setup           func(g *gomock.Controller) cluster.RegisteredClustersCache
		expectedMatches []string
	}{
		{
			msg: "select with specified region",
			labels: map[string]string{
				ClusterRegionLabelKey: "phx",
			},
			candidates: []*cluster.ResourcePoolInfo{
				createResourcePool("cluster1"),
				createResourcePool("cluster2"),
			},
			setup: func(g *gomock.Controller) cluster.RegisteredClustersCache {
				mockCache := clustermock.NewMockRegisteredClustersCache(g)

				cluster1 := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "phx",
						Zone:   "phx5",
					},
				}
				cluster1.SetName("cluster1")
				mockCache.EXPECT().GetCluster("cluster1").Return(cluster1).AnyTimes()

				cluster2 := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "dca",
						Zone:   "dca11",
					},
				}
				cluster2.SetName("cluster2")
				mockCache.EXPECT().GetCluster("cluster2").Return(cluster2).AnyTimes()
				return mockCache
			},
			expectedMatches: []string{
				"cluster1",
			},
		},
		{
			msg: "case insensitive comparison for region",
			labels: map[string]string{
				ClusterRegionLabelKey: "PHX",
			},
			candidates: []*cluster.ResourcePoolInfo{
				createResourcePool("cluster1"),
				createResourcePool("cluster2"),
			},
			setup: func(g *gomock.Controller) cluster.RegisteredClustersCache {
				mockCache := clustermock.NewMockRegisteredClustersCache(g)

				cluster1 := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "phx",
						Zone:   "phx5",
					},
				}
				cluster1.SetName("cluster1")
				mockCache.EXPECT().GetCluster("cluster1").Return(cluster1).AnyTimes()

				cluster2 := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "dca",
						Zone:   "dca11",
					},
				}
				cluster2.SetName("cluster2")
				mockCache.EXPECT().GetCluster("cluster2").Return(cluster2).AnyTimes()
				return mockCache
			},
			expectedMatches: []string{
				"cluster1",
			},
		},
		{
			msg: "select with specified zone",
			labels: map[string]string{
				ClusterZoneLabelKey: "dca11",
			},
			candidates: []*cluster.ResourcePoolInfo{
				createResourcePool("cluster1"),
				createResourcePool("cluster2"),
			},
			setup: func(g *gomock.Controller) cluster.RegisteredClustersCache {
				mockCache := clustermock.NewMockRegisteredClustersCache(g)

				cluster1 := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "phx",
						Zone:   "phx5",
					},
				}
				cluster1.SetName("cluster1")
				mockCache.EXPECT().GetCluster("cluster1").Return(cluster1).AnyTimes()

				cluster2 := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "dca",
						Zone:   "dca11",
					},
				}
				cluster2.SetName("cluster2")
				mockCache.EXPECT().GetCluster("cluster2").Return(cluster2).AnyTimes()
				return mockCache
			},
			expectedMatches: []string{
				"cluster2",
			},
		},
		{
			msg: "case insensitive comparison for region",
			labels: map[string]string{
				ClusterZoneLabelKey: "PHX5",
			},
			candidates: []*cluster.ResourcePoolInfo{
				createResourcePool("cluster1"),
				createResourcePool("cluster2"),
			},
			setup: func(g *gomock.Controller) cluster.RegisteredClustersCache {
				mockCache := clustermock.NewMockRegisteredClustersCache(g)

				cluster1 := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "phx",
						Zone:   "phx5",
					},
				}
				cluster1.SetName("cluster1")
				mockCache.EXPECT().GetCluster("cluster1").Return(cluster1).AnyTimes()

				cluster2 := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "dca",
						Zone:   "dca11",
					},
				}
				cluster2.SetName("cluster2")
				mockCache.EXPECT().GetCluster("cluster2").Return(cluster2).AnyTimes()
				return mockCache
			},
			expectedMatches: []string{
				"cluster1",
			},
		},
		{
			msg: "select with specified cluster",
			labels: map[string]string{
				ClusterNameLabelKey: "cluster1",
			},
			candidates: []*cluster.ResourcePoolInfo{
				createResourcePool("cluster1"),
				createResourcePool("cluster2"),
			},
			setup: func(g *gomock.Controller) cluster.RegisteredClustersCache {
				mockCache := clustermock.NewMockRegisteredClustersCache(g)

				cluster1 := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "phx",
						Zone:   "phx5",
					},
				}
				cluster1.SetName("cluster1")
				mockCache.EXPECT().GetCluster("cluster1").Return(cluster1).AnyTimes()

				cluster2 := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "dca",
						Zone:   "dca11",
					},
				}
				cluster2.SetName("cluster2")
				mockCache.EXPECT().GetCluster("cluster2").Return(cluster2).AnyTimes()
				return mockCache
			},
			expectedMatches: []string{
				"cluster1",
			},
		},
		{
			msg: "case insensitive comparison for cluster",
			labels: map[string]string{
				ClusterNameLabelKey: "CLUSTER2",
			},
			candidates: []*cluster.ResourcePoolInfo{
				createResourcePool("cluster1"),
				createResourcePool("cluster2"),
			},
			setup: func(g *gomock.Controller) cluster.RegisteredClustersCache {
				mockCache := clustermock.NewMockRegisteredClustersCache(g)

				cluster1 := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "phx",
						Zone:   "phx5",
					},
				}
				cluster1.SetName("cluster1")
				mockCache.EXPECT().GetCluster("cluster1").Return(cluster1).AnyTimes()

				cluster2 := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "dca",
						Zone:   "dca11",
					},
				}
				cluster2.SetName("cluster2")
				mockCache.EXPECT().GetCluster("cluster2").Return(cluster2).AnyTimes()
				return mockCache
			},
			expectedMatches: []string{
				"cluster2",
			},
		},
		{
			msg: "multiple selectors provided",
			labels: map[string]string{
				ClusterRegionLabelKey: "phx",
				ClusterZoneLabelKey:   "phx6",
			},
			candidates: []*cluster.ResourcePoolInfo{
				createResourcePool("cluster1"),
				createResourcePool("cluster2"),
				createResourcePool("cluster3"),
			},
			setup: func(g *gomock.Controller) cluster.RegisteredClustersCache {
				mockCache := clustermock.NewMockRegisteredClustersCache(g)

				cluster1 := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "phx",
						Zone:   "phx5",
					},
				}
				cluster1.SetName("cluster1")
				mockCache.EXPECT().GetCluster("cluster1").Return(cluster1).AnyTimes()

				cluster2 := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "dca",
						Zone:   "dca11",
					},
				}
				cluster2.SetName("cluster2")
				mockCache.EXPECT().GetCluster("cluster2").Return(cluster2).AnyTimes()

				cluster3 := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "phx",
						Zone:   "phx6",
					},
				}
				cluster3.SetName("cluster3")
				mockCache.EXPECT().GetCluster("cluster3").Return(cluster3).AnyTimes()
				return mockCache
			},
			expectedMatches: []string{
				"cluster3",
			},
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			g := gomock.NewController(t)
			cache := test.setup(g)

			builder := framework.NewOptionBuilder()
			builder.Build(framework.WithClusterCache(cache))
			affinityFilter := AffinityFilter{
				OptionBuilder: builder,
			}

			selector := createLabelSelector(test.labels)
			matches := affinityFilter.matchClusterSelector(selector, test.candidates)
			require.Equal(t, len(test.expectedMatches), len(matches))
			for i := range matches {
				require.Equal(t, test.expectedMatches[i], matches[i].ClusterName)
			}
		})
	}
}

func TestGetSelectorWithClusterAffinity(t *testing.T) {
	tt := []struct {
		job              framework.BatchJob
		expectedSelector *v1.LabelSelector
		msg              string
	}{
		{
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: createLabelSelector(map[string]string{
									ClusterRegionLabelKey: "dca",
								}),
							},
						},
					},
				},
			},
			expectedSelector: createLabelSelector(map[string]string{
				ClusterRegionLabelKey: "dca",
			}),
			msg: "cluster region already specified to a non-default region",
		},
		{
			job: framework.BatchSparkJob{
				SparkJob: &v2beta1pb.SparkJob{
					Spec: v2beta1pb.SparkJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: createLabelSelector(map[string]string{
									ClusterRegionLabelKey: "phx",
								}),
							},
						},
					},
				},
			},
			expectedSelector: createLabelSelector(map[string]string{
				ClusterRegionLabelKey: "phx",
			}),
			msg: "cluster region already specified to the default region",
		},
		{
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{},
				},
			},
			expectedSelector: createLabelSelector(map[string]string{
				ClusterRegionLabelKey: "phx",
			}),
			msg: "no affinity specified should created selector with default region",
		},
		{
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: &v1.LabelSelector{
									MatchLabels: map[string]string{
										"resourcepool.michelangelo/support-resource-type-cpu": "true",
										"resourcepool.michelangelo/support-env-dev":           "true",
									},
								},
							},
						},
					},
				},
			},
			expectedSelector: createLabelSelector(map[string]string{
				ClusterRegionLabelKey: "phx",
				"resourcepool.michelangelo/support-resource-type-cpu": "true",
				"resourcepool.michelangelo/support-env-dev":           "true",
			}),
			msg: "non cluster affinity should add the default region to selector",
		},
		{
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: createLabelSelector(map[string]string{
									ClusterZoneLabelKey: "dca11",
								}),
							},
						},
					},
				},
			},
			expectedSelector: createLabelSelector(map[string]string{
				ClusterZoneLabelKey: "dca11",
			}),
			msg: "cluster zone already specified - selector should not change",
		},
		{
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: createLabelSelector(map[string]string{
									ClusterNameLabelKey: "phx5-kubernetes-batch01",
								}),
							},
						},
					},
				},
			},
			expectedSelector: createLabelSelector(map[string]string{
				ClusterNameLabelKey: "phx5-kubernetes-batch01",
			}),
			msg: "cluster name already specified - selector should not change",
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			a := AffinityFilter{}
			selector := a.getSelectorWithClusterAffinity(test.job.GetAffinity().GetResourceAffinity().GetSelector())
			require.Equal(t, test.expectedSelector, selector)
		})
	}
}

func createLabelSelector(labels map[string]string) *v1.LabelSelector {
	return &v1.LabelSelector{
		MatchLabels: labels,
	}
}

func createResourcePool(clusterName string) *cluster.ResourcePoolInfo {
	return &cluster.ResourcePoolInfo{
		ClusterName: clusterName,
	}
}

func TestAddCloudZoneToAffinityBasedOnFlipr(t *testing.T) {
	tt := []struct {
		job              framework.BatchJob
		setupFliprMock   func(g *gomock.Controller) (flipr.FliprClient, types.FliprConstraintsBuilder)
		expectError      bool
		expectedSelector *v1.LabelSelector
		msg              string
	}{
		{
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-job",
						Namespace: "test-ns",
					},
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: createLabelSelector(map[string]string{
									ClusterRegionLabelKey: "dca",
								}),
							},
						},
					},
				},
			},
			setupFliprMock: func(g *gomock.Controller) (flipr.FliprClient, types.FliprConstraintsBuilder) {
				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFlipr.EXPECT().GetStringValue(gomock.Any(), _fliprRayJobsInCloud, gomock.Any(), "").
					Return("", nil)

				mockConstraints := typesmock.NewMockFliprConstraintsBuilder(g)
				mockConstraints.EXPECT().GetFliprConstraints(map[string]interface{}{
					_fliprProjectCPUPropertyName: "test-ns",
				}).Return(flipr.Constraints{})
				return mockFlipr, mockConstraints
			},
			expectedSelector: createLabelSelector(map[string]string{
				ClusterRegionLabelKey: "dca",
			}),
			msg: "no match - return unchanged",
		},
		{
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-job",
						Namespace: "test-ns",
						Annotations: map[string]string{
							"a":                                    "b",
							sharedconstants.RunnableNameAnnotation: "runnable",
						},
					},
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: createLabelSelector(map[string]string{
									ClusterRegionLabelKey: "dca",
								}),
							},
						},
					},
				},
			},
			expectError: true,
			setupFliprMock: func(g *gomock.Controller) (flipr.FliprClient, types.FliprConstraintsBuilder) {
				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFlipr.EXPECT().GetStringValue(gomock.Any(), _fliprRayJobsInCloud, gomock.Any(), "").
					Return("", errors.New("random error"))

				mockConstraints := typesmock.NewMockFliprConstraintsBuilder(g)
				mockConstraints.EXPECT().GetFliprConstraints(map[string]interface{}{
					_fliprProjectCPUPropertyName: "test-ns",
					_fliprRunnablePropertyName:   "runnable",
				}).Return(flipr.Constraints{})
				return mockFlipr, mockConstraints
			},
			expectedSelector: createLabelSelector(map[string]string{
				ClusterRegionLabelKey: "dca",
			}),
			msg: "runnable annotation present but querying flipr gives an error - return unchanged",
		},
		{
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-job",
						Namespace: "test-ns",
						Annotations: map[string]string{
							"a":                                    "b",
							sharedconstants.RunnableNameAnnotation: "runnable",
						},
					},
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: createLabelSelector(map[string]string{
									ClusterRegionLabelKey: "dca",
								}),
							},
						},
					},
				},
			},
			setupFliprMock: func(g *gomock.Controller) (flipr.FliprClient, types.FliprConstraintsBuilder) {
				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFlipr.EXPECT().GetStringValue(gomock.Any(), _fliprRayJobsInCloud, gomock.Any(), "").
					Return("", nil)

				mockConstraints := typesmock.NewMockFliprConstraintsBuilder(g)
				mockConstraints.EXPECT().GetFliprConstraints(map[string]interface{}{
					_fliprProjectCPUPropertyName: "test-ns",
					_fliprRunnablePropertyName:   "runnable",
				}).Return(flipr.Constraints{})
				return mockFlipr, mockConstraints
			},
			expectedSelector: createLabelSelector(map[string]string{
				ClusterRegionLabelKey: "dca",
			}),
			msg: "runnable annotation present but not not enabled to run in cloud in flipr - return unchanged",
		},
		{
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-job",
						Namespace: "test-ns",
						Annotations: map[string]string{
							"a":                                    "b",
							sharedconstants.RunnableNameAnnotation: "runnable",
						},
					},
					Spec: v2beta1pb.RayJobSpec{},
				},
			},
			setupFliprMock: func(g *gomock.Controller) (flipr.FliprClient, types.FliprConstraintsBuilder) {
				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFlipr.EXPECT().GetStringValue(gomock.Any(), _fliprRayJobsInCloud, gomock.Any(), "").
					Return("dca60", nil)

				mockConstraints := typesmock.NewMockFliprConstraintsBuilder(g)
				mockConstraints.EXPECT().GetFliprConstraints(map[string]interface{}{
					_fliprProjectCPUPropertyName: "test-ns",
					_fliprRunnablePropertyName:   "runnable",
				}).Return(flipr.Constraints{})
				return mockFlipr, mockConstraints
			},
			expectedSelector: createLabelSelector(map[string]string{
				ClusterZoneLabelKey: "dca60",
			}),
			msg: "runnable annotation present and enabled to run in cloud in flipr - existing selector is nil",
		},
		{
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-job",
						Namespace: "test-ns",
						Annotations: map[string]string{
							"a":                                    "b",
							sharedconstants.RunnableNameAnnotation: "runnable",
						},
					},
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: &v1.LabelSelector{},
							},
						},
					},
				},
			},
			setupFliprMock: func(g *gomock.Controller) (flipr.FliprClient, types.FliprConstraintsBuilder) {
				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFlipr.EXPECT().GetStringValue(gomock.Any(), _fliprRayJobsInCloud, gomock.Any(), "").
					Return("dca60", nil)

				mockConstraints := typesmock.NewMockFliprConstraintsBuilder(g)
				mockConstraints.EXPECT().GetFliprConstraints(map[string]interface{}{
					_fliprProjectCPUPropertyName: "test-ns",
					_fliprRunnablePropertyName:   "runnable",
				}).Return(flipr.Constraints{})
				return mockFlipr, mockConstraints
			},
			expectedSelector: createLabelSelector(map[string]string{
				ClusterZoneLabelKey: "dca60",
			}),
			msg: "runnable annotation present and enabled to run in cloud in flipr - existing selector MatchLabels is nil",
		},
		{
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-job",
						Namespace: "test-ns",
						Annotations: map[string]string{
							"a":                                    "b",
							sharedconstants.RunnableNameAnnotation: "runnable",
						},
					},
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: createLabelSelector(map[string]string{
									ClusterRegionLabelKey: "dca",
								}),
							},
						},
					},
				},
			},
			setupFliprMock: func(g *gomock.Controller) (flipr.FliprClient, types.FliprConstraintsBuilder) {
				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFlipr.EXPECT().GetStringValue(gomock.Any(), _fliprRayJobsInCloud, gomock.Any(), "").
					Return("dca60", nil)

				mockConstraints := typesmock.NewMockFliprConstraintsBuilder(g)
				mockConstraints.EXPECT().GetFliprConstraints(map[string]interface{}{
					_fliprProjectCPUPropertyName: "test-ns",
					_fliprRunnablePropertyName:   "runnable",
				}).Return(flipr.Constraints{})
				return mockFlipr, mockConstraints
			},
			expectedSelector: createLabelSelector(map[string]string{
				ClusterRegionLabelKey: "dca",
				ClusterZoneLabelKey:   "dca60",
			}),
			msg: "runnable annotation present and enabled to run in cloud in flipr",
		},
		{
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-job",
						Namespace: "test-ns",
						Annotations: map[string]string{
							"a":                                    "b",
							sharedconstants.RunnableNameAnnotation: "runnable",
						},
					},
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: createLabelSelector(map[string]string{
									ClusterZoneLabelKey: "dca11",
								}),
							},
						},
					},
				},
			},
			setupFliprMock: func(g *gomock.Controller) (flipr.FliprClient, types.FliprConstraintsBuilder) {
				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFlipr.EXPECT().GetStringValue(gomock.Any(), _fliprRayJobsInCloud, gomock.Any(), "").
					Return("dca60", nil)

				mockConstraints := typesmock.NewMockFliprConstraintsBuilder(g)
				mockConstraints.EXPECT().GetFliprConstraints(map[string]interface{}{
					_fliprProjectCPUPropertyName: "test-ns",
					_fliprRunnablePropertyName:   "runnable",
				}).Return(flipr.Constraints{})
				return mockFlipr, mockConstraints
			},
			expectedSelector: createLabelSelector(map[string]string{
				ClusterZoneLabelKey: "dca60",
			}),
			msg: "runnable annotation present and enabled to run in cloud in flipr - override existing zone",
		},
		{
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-job",
						Namespace: "test-ns",
					},
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: createLabelSelector(map[string]string{
									ClusterZoneLabelKey: "dca11",
								}),
							},
						},
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    2,
									Memory: "100Gi",
								},
							},
						},
					},
				},
			},
			setupFliprMock: func(g *gomock.Controller) (flipr.FliprClient, types.FliprConstraintsBuilder) {
				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFlipr.EXPECT().GetStringValue(gomock.Any(), _fliprRayJobsInCloud, gomock.Any(), "").
					Return("phx60", nil)

				mockConstraints := typesmock.NewMockFliprConstraintsBuilder(g)
				mockConstraints.EXPECT().GetFliprConstraints(map[string]interface{}{
					_fliprProjectCPUPropertyName: "test-ns",
				}).Return(flipr.Constraints{})
				return mockFlipr, mockConstraints
			},
			expectedSelector: createLabelSelector(map[string]string{
				ClusterZoneLabelKey: "phx60",
			}),
			msg: "match based on project name for CPU only job - override existing zone",
		},
		{
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: createLabelSelector(map[string]string{
									ClusterZoneLabelKey: "dca11",
								}),
							},
						},
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    2,
									Memory: "100gi",
								},
							},
						},
					},
				},
			},
			setupFliprMock: func(g *gomock.Controller) (flipr.FliprClient, types.FliprConstraintsBuilder) {
				mockFlipr := fliprmock.NewMockFliprClient(g)
				return mockFlipr, nil
			},
			expectError: true,
			msg:         "bad memory spec causes error",
		},
		{
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-job",
						Namespace: "test-ns",
					},
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: createLabelSelector(map[string]string{
									ClusterZoneLabelKey: "dca11",
								}),
							},
						},
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    2,
									Memory: "100Gi",
								},
							},
						},
					},
				},
			},
			setupFliprMock: func(g *gomock.Controller) (flipr.FliprClient, types.FliprConstraintsBuilder) {
				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFlipr.EXPECT().GetStringValue(gomock.Any(), _fliprRayJobsInCloud, gomock.Any(), "").
					Return("phx60", nil)

				mockConstraints := typesmock.NewMockFliprConstraintsBuilder(g)
				mockConstraints.EXPECT().GetFliprConstraints(map[string]interface{}{
					_fliprProjectCPUPropertyName: "test-ns",
				}).Return(flipr.Constraints{})
				return mockFlipr, mockConstraints
			},
			expectedSelector: createLabelSelector(map[string]string{
				ClusterZoneLabelKey: "phx60",
			}),
			msg: "match based on project name for a CPU only job - override existing zone",
		},
		{
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-job",
						Namespace: "test-ns",
						Annotations: map[string]string{
							sharedconstants.PipelineNameAnnotation: "pipeline-name",
						},
					},
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: createLabelSelector(map[string]string{
									ClusterZoneLabelKey: "dca11",
								}),
							},
						},
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    2,
									Memory: "100Gi",
								},
							},
						},
					},
				},
			},
			setupFliprMock: func(g *gomock.Controller) (flipr.FliprClient, types.FliprConstraintsBuilder) {
				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFlipr.EXPECT().GetStringValue(gomock.Any(), _fliprRayJobsInCloud, gomock.Any(), "").
					Return("dca60", nil)

				mockConstraints := typesmock.NewMockFliprConstraintsBuilder(g)
				mockConstraints.EXPECT().GetFliprConstraints(map[string]interface{}{
					_fliprPipelinePropertyName:   "test-ns/pipeline-name",
					_fliprProjectCPUPropertyName: "test-ns",
				}).Return(flipr.Constraints{})
				return mockFlipr, mockConstraints
			},
			expectedSelector: createLabelSelector(map[string]string{
				ClusterZoneLabelKey: "dca60",
			}),
			msg: "match based on pipeline name for a CPU only job - override existing zone",
		},
		{
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-job",
						Namespace: "test-ns",
					},
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: createLabelSelector(map[string]string{
									ClusterZoneLabelKey: "dca11",
								}),
							},
						},
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    2,
									Gpu:    1,
									Memory: "100Gi",
								},
							},
						},
						Worker: &v2beta1pb.WorkerSpec{
							MinInstances: 1,
							MaxInstances: 1,
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    2,
									Gpu:    1,
									Memory: "100Gi",
								},
							},
						},
					},
				},
			},
			setupFliprMock: func(g *gomock.Controller) (flipr.FliprClient, types.FliprConstraintsBuilder) {
				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFlipr.EXPECT().GetStringValue(gomock.Any(), _fliprRayJobsInCloud, gomock.Any(), "").
					Return("phx60", nil)

				mockConstraints := typesmock.NewMockFliprConstraintsBuilder(g)
				mockConstraints.EXPECT().GetFliprConstraints(map[string]interface{}{
					_fliprProjectGPUPropertyName: "test-ns",
				}).Return(flipr.Constraints{})
				return mockFlipr, mockConstraints
			},
			expectedSelector: createLabelSelector(map[string]string{
				ClusterZoneLabelKey: "phx60",
			}),
			msg: "match based on project name for a GPU job - override existing zone",
		},
		{
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-job",
						Namespace: "test-ns",
						Annotations: map[string]string{
							sharedconstants.PipelineNameAnnotation: "pipeline-name",
						},
					},
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: createLabelSelector(map[string]string{
									ClusterZoneLabelKey: "dca11",
								}),
							},
						},
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    2,
									Gpu:    1,
									Memory: "100Gi",
								},
							},
						},
						Worker: &v2beta1pb.WorkerSpec{
							MinInstances: 1,
							MaxInstances: 1,
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    2,
									Gpu:    1,
									Memory: "100Gi",
								},
							},
						},
					},
				},
			},
			setupFliprMock: func(g *gomock.Controller) (flipr.FliprClient, types.FliprConstraintsBuilder) {
				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFlipr.EXPECT().GetStringValue(gomock.Any(), _fliprRayJobsInCloud, gomock.Any(), "").
					Return("phx60", nil)

				mockConstraints := typesmock.NewMockFliprConstraintsBuilder(g)
				mockConstraints.EXPECT().GetFliprConstraints(map[string]interface{}{
					_fliprPipelinePropertyName:   "test-ns/pipeline-name",
					_fliprProjectGPUPropertyName: "test-ns",
				}).Return(flipr.Constraints{})
				return mockFlipr, mockConstraints
			},
			expectedSelector: createLabelSelector(map[string]string{
				ClusterZoneLabelKey: "phx60",
			}),
			msg: "match based on pipeline name for a GPU job - override existing zone",
		},
		{
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-job",
						Namespace: "test-ns",
					},
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: createLabelSelector(map[string]string{
									ClusterZoneLabelKey: "dca11",
								}),
							},
						},
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    2,
									Gpu:    1,
									Memory: "100Gi",
								},
							},
						},
						Workers: []*v2beta1pb.WorkerSpec{
							{
								MinInstances: 1,
								MaxInstances: 1,
								Pod: &v2beta1pb.PodSpec{
									Resource: &v2beta1pb.ResourceSpec{
										Cpu:    2,
										Memory: "100Gi",
									},
								},
								NodeType: "DATA_NODE",
							},
							{
								MinInstances: 1,
								MaxInstances: 1,
								Pod: &v2beta1pb.PodSpec{
									Resource: &v2beta1pb.ResourceSpec{
										Cpu:    2,
										Gpu:    1,
										Memory: "100Gi",
									},
								},
								NodeType: "TRAINER_NODE",
							},
						},
					},
				},
			},
			setupFliprMock: func(g *gomock.Controller) (flipr.FliprClient, types.FliprConstraintsBuilder) {
				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFlipr.EXPECT().GetStringValue(gomock.Any(), _fliprRayJobsInCloud, gomock.Any(), "").
					Return("phx60", nil)

				mockConstraints := typesmock.NewMockFliprConstraintsBuilder(g)
				mockConstraints.EXPECT().GetFliprConstraints(map[string]interface{}{
					_fliprProjectGPUPropertyName: "test-ns",
				}).Return(flipr.Constraints{})
				return mockFlipr, mockConstraints
			},
			expectedSelector: createLabelSelector(map[string]string{
				ClusterZoneLabelKey: "phx60",
			}),
			msg: "match based on project name for a heterogeneous GPU job - override existing zone",
		},
		{
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-job",
						Namespace: "test-ns",
						Annotations: map[string]string{
							sharedconstants.PipelineNameAnnotation: "pipeline-name",
						},
					},
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: createLabelSelector(map[string]string{
									ClusterZoneLabelKey: "dca11",
								}),
							},
						},
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    2,
									Gpu:    1,
									Memory: "100Gi",
								},
							},
						},
						Workers: []*v2beta1pb.WorkerSpec{
							{
								MinInstances: 1,
								MaxInstances: 1,
								Pod: &v2beta1pb.PodSpec{
									Resource: &v2beta1pb.ResourceSpec{
										Cpu:    2,
										Memory: "100Gi",
									},
								},
								NodeType: "DATA_NODE",
							},
							{
								MinInstances: 1,
								MaxInstances: 1,
								Pod: &v2beta1pb.PodSpec{
									Resource: &v2beta1pb.ResourceSpec{
										Cpu:    2,
										Gpu:    1,
										Memory: "100Gi",
									},
								},
								NodeType: "TRAINER_NODE",
							},
						},
					},
				},
			},
			setupFliprMock: func(g *gomock.Controller) (flipr.FliprClient, types.FliprConstraintsBuilder) {
				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFlipr.EXPECT().GetStringValue(gomock.Any(), _fliprRayJobsInCloud, gomock.Any(), "").
					Return("phx60", nil)

				mockConstraints := typesmock.NewMockFliprConstraintsBuilder(g)
				mockConstraints.EXPECT().GetFliprConstraints(map[string]interface{}{
					_fliprPipelinePropertyName:   "test-ns/pipeline-name",
					_fliprProjectGPUPropertyName: "test-ns",
				}).Return(flipr.Constraints{})
				return mockFlipr, mockConstraints
			},
			expectedSelector: createLabelSelector(map[string]string{
				ClusterZoneLabelKey: "phx60",
			}),
			msg: "match based on pipeline name for a heterogeneous GPU job - override existing zone",
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			g := gomock.NewController(t)
			mockFlipr, mockConstraints := test.setupFliprMock(g)

			builder := framework.NewOptionBuilder()
			builder.Build(framework.WithFlipr(mockFlipr), framework.WithFliprConstraintsBuilder(mockConstraints))
			a := AffinityFilter{
				OptionBuilder: builder,
			}
			selector, err := a.addCloudZoneToAffinityBasedOnFlipr(context.Background(), test.job)
			if test.expectError {
				require.Error(t, err)
				require.Nil(t, selector)
			} else {
				require.Equal(t, test.expectedSelector, selector)
			}
		})
	}
}

func TestIsGpuJob(t *testing.T) {
	tt := []struct {
		job            framework.BatchJob
		expectedResult bool
		expectedError  error
		msg            string
	}{
		{
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: createLabelSelector(map[string]string{
									ClusterZoneLabelKey: "dca11",
								}),
							},
						},
					},
				},
			},
			expectedResult: false,
			msg:            "no resource spec",
		},
		{
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: createLabelSelector(map[string]string{
									ClusterZoneLabelKey: "dca11",
								}),
							},
						},
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    2,
									Memory: "100Gi",
								},
							},
						},
					},
				},
			},
			expectedResult: false,
			msg:            "CPU only job",
		},
		{
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: createLabelSelector(map[string]string{
									ClusterZoneLabelKey: "dca11",
								}),
							},
						},
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    2,
									Gpu:    1,
									Memory: "100Gi",
								},
							},
						},
						Worker: &v2beta1pb.WorkerSpec{
							MinInstances: 1,
							MaxInstances: 1,
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    2,
									Gpu:    1,
									Memory: "100Gi",
								},
							},
						},
					},
				},
			},
			expectedResult: true,
			msg:            "GPU homogeneous Ray job",
		},
		{
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: createLabelSelector(map[string]string{
									ClusterZoneLabelKey: "dca11",
								}),
							},
						},
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    2,
									Gpu:    0,
									Memory: "100Gi",
								},
							},
						},
						Worker: &v2beta1pb.WorkerSpec{
							MinInstances: 1,
							MaxInstances: 1,
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    2,
									Gpu:    0,
									Memory: "100Gi",
								},
							},
						},
					},
				},
			},
			msg: "GPU homogeneous Ray job with 0 gpu explicitly specified",
		},
		{
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: createLabelSelector(map[string]string{
									ClusterZoneLabelKey: "dca11",
								}),
							},
						},
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    2,
									Gpu:    1,
									Memory: "100gi",
								},
							},
						},
						Worker: &v2beta1pb.WorkerSpec{
							MinInstances: 1,
							MaxInstances: 1,
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    2,
									Gpu:    1,
									Memory: "100Gi",
								},
							},
						},
					},
				},
			},
			expectedError: errors.New("error getting job's resource requirement: quantities must match the regular expression '^([+-]?[0-9.]+)([eEinumkKMGTP]*[-+]?[0-9]*)$'"),
			msg:           "Bad resource requirements",
		},
		{
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: createLabelSelector(map[string]string{
									ClusterZoneLabelKey: "dca11",
								}),
							},
						},
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    2,
									Gpu:    1,
									Memory: "100Gi",
								},
							},
						},
						Workers: []*v2beta1pb.WorkerSpec{
							{
								MinInstances: 1,
								MaxInstances: 1,
								Pod: &v2beta1pb.PodSpec{
									Resource: &v2beta1pb.ResourceSpec{
										Cpu:    2,
										Memory: "100Gi",
									},
								},
								NodeType: "DATA_NODE",
							},
							{
								MinInstances: 1,
								MaxInstances: 1,
								Pod: &v2beta1pb.PodSpec{
									Resource: &v2beta1pb.ResourceSpec{
										Cpu:    2,
										Gpu:    1,
										Memory: "100Gi",
									},
								},
								NodeType: "TRAINER_NODE",
							},
						},
					},
				},
			},
			expectedResult: true,
			msg:            "GPU heterogeneous Ray job",
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			a := AffinityFilter{}
			isGpuJob, err := a.isGpuJob(test.job)
			require.Equal(t, test.expectedResult, isGpuJob)
			require.Equal(t, test.expectedError, err)
		})
	}
}

func TestMatchClusterSelectorOnRegionProvider(t *testing.T) {
	tt := []struct {
		msg             string
		selector        *v1.LabelSelector
		candidates      []*cluster.ResourcePoolInfo
		setup           func(g *gomock.Controller) cluster.RegisteredClustersCache
		expectedMatches []string
	}{
		{
			msg: "match using region-provider - phx-gcp, only one regional cluster",
			selector: createLabelSelector(map[string]string{
				ClusterRegionProviderLabelKey: "phx-gcp",
			}),
			candidates: []*cluster.ResourcePoolInfo{
				createResourcePool("cluster1"),
				createResourcePool("cluster2"),
				createResourcePool("cluster3"),
			},
			setup: func(g *gomock.Controller) cluster.RegisteredClustersCache {
				mockCache := clustermock.NewMockRegisteredClustersCache(g)

				cluster1 := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "phx",
						Zone:   "phx5",
						Dc:     v2beta1pb.DC_TYPE_CLOUD_GCP,
					},
				}
				cluster1.SetName("cluster1")
				mockCache.EXPECT().GetCluster("cluster1").Return(cluster1).AnyTimes()

				cluster2 := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "phx",
						Dc:     v2beta1pb.DC_TYPE_CLOUD_GCP,
					},
				}
				cluster2.SetName("cluster2")
				mockCache.EXPECT().GetCluster("cluster2").Return(cluster2).AnyTimes()

				cluster3 := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "phx",
						Zone:   "phx60",
					},
				}
				cluster3.SetName("cluster3")
				mockCache.EXPECT().GetCluster("cluster3").Return(cluster3).AnyTimes()
				return mockCache
			},
			expectedMatches: []string{
				"cluster2",
			},
		},
		{
			msg: "match using region-provider - phx-gcp",
			selector: createLabelSelector(map[string]string{
				ClusterRegionProviderLabelKey: "phx-gcp",
			}),
			candidates: []*cluster.ResourcePoolInfo{
				createResourcePool("cluster1"),
				createResourcePool("cluster2"),
				createResourcePool("cluster3"),
			},
			setup: func(g *gomock.Controller) cluster.RegisteredClustersCache {
				mockCache := clustermock.NewMockRegisteredClustersCache(g)

				cluster1 := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "phx",
						Dc:     v2beta1pb.DC_TYPE_ON_PREM,
					},
				}
				cluster1.SetName("cluster1")
				mockCache.EXPECT().GetCluster("cluster1").Return(cluster1).AnyTimes()

				cluster2 := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "phx",
						Dc:     v2beta1pb.DC_TYPE_CLOUD_GCP,
					},
				}
				cluster2.SetName("cluster2")
				mockCache.EXPECT().GetCluster("cluster2").Return(cluster2).AnyTimes()

				cluster3 := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "DCA",
						Dc:     v2beta1pb.DC_TYPE_CLOUD_GCP,
					},
				}
				cluster3.SetName("cluster3")
				mockCache.EXPECT().GetCluster("cluster3").Return(cluster3).AnyTimes()
				return mockCache
			},
			expectedMatches: []string{
				"cluster2",
			},
		},
		{
			msg: "multiple matches using region-provider - dca-gcp",
			selector: createLabelSelector(map[string]string{
				ClusterRegionProviderLabelKey: "dca-gcp",
			}),
			candidates: []*cluster.ResourcePoolInfo{
				createResourcePool("cluster1"),
				createResourcePool("cluster2"),
				createResourcePool("cluster3"),
				createResourcePool("cluster4"),
			},
			setup: func(g *gomock.Controller) cluster.RegisteredClustersCache {
				mockCache := clustermock.NewMockRegisteredClustersCache(g)

				cluster1 := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "dca",
						Dc:     v2beta1pb.DC_TYPE_ON_PREM,
					},
				}
				cluster1.SetName("cluster1")
				mockCache.EXPECT().GetCluster("cluster1").Return(cluster1).AnyTimes()

				cluster2 := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "dca",
						Dc:     v2beta1pb.DC_TYPE_CLOUD_GCP,
					},
				}
				cluster2.SetName("cluster2")
				mockCache.EXPECT().GetCluster("cluster2").Return(cluster2).AnyTimes()

				cluster3 := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "dca",
						Dc:     v2beta1pb.DC_TYPE_CLOUD_GCP,
					},
				}
				cluster3.SetName("cluster3")
				mockCache.EXPECT().GetCluster("cluster3").Return(cluster3).AnyTimes()

				cluster4 := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "phx",
						Dc:     v2beta1pb.DC_TYPE_CLOUD_GCP,
					},
				}
				cluster4.SetName("cluster4")
				mockCache.EXPECT().GetCluster("cluster4").Return(cluster4).AnyTimes()
				return mockCache
			},
			expectedMatches: []string{
				"cluster2",
				"cluster3",
			},
		},
		{
			msg: "no matches for valid region-provider",
			selector: createLabelSelector(map[string]string{
				ClusterRegionProviderLabelKey: "phx-onprem",
			}),
			candidates: []*cluster.ResourcePoolInfo{
				createResourcePool("cluster1"),
				createResourcePool("cluster2"),
			},
			setup: func(g *gomock.Controller) cluster.RegisteredClustersCache {
				mockCache := clustermock.NewMockRegisteredClustersCache(g)

				cluster1 := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "phx",
						Dc:     v2beta1pb.DC_TYPE_CLOUD_GCP,
					},
				}
				cluster1.SetName("cluster1")
				mockCache.EXPECT().GetCluster("cluster1").Return(cluster1).AnyTimes()

				cluster2 := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "dca",
						Dc:     v2beta1pb.DC_TYPE_CLOUD_GCP,
					},
				}
				cluster2.SetName("cluster2")
				mockCache.EXPECT().GetCluster("cluster2").Return(cluster2).AnyTimes()
				return mockCache
			},
			expectedMatches: []string{},
		},
		{
			msg: "handle nil cluster from cache",
			selector: createLabelSelector(map[string]string{
				ClusterRegionProviderLabelKey: "phx-gcp",
			}),
			candidates: []*cluster.ResourcePoolInfo{
				createResourcePool("cluster1"),
				createResourcePool("cluster2"),
				createResourcePool("cluster3"),
			},
			setup: func(g *gomock.Controller) cluster.RegisteredClustersCache {
				mockCache := clustermock.NewMockRegisteredClustersCache(g)

				cluster1 := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "phx",
						Dc:     v2beta1pb.DC_TYPE_ON_PREM,
					},
				}
				cluster1.SetName("cluster1")
				mockCache.EXPECT().GetCluster("cluster1").Return(cluster1).AnyTimes()

				cluster2 := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "phx",
						Dc:     v2beta1pb.DC_TYPE_CLOUD_GCP,
					},
				}
				cluster2.SetName("cluster2")
				mockCache.EXPECT().GetCluster("cluster2").Return(cluster2).AnyTimes()

				// Simulating a deleted cluster by returning nil
				mockCache.EXPECT().GetCluster("cluster3").Return(nil).AnyTimes()
				return mockCache
			},
			expectedMatches: []string{
				"cluster2",
			},
		},
		{
			msg: "empty candidates list",
			selector: createLabelSelector(map[string]string{
				ClusterRegionProviderLabelKey: "phx-gcp",
			}),
			candidates: []*cluster.ResourcePoolInfo{},
			setup: func(g *gomock.Controller) cluster.RegisteredClustersCache {
				mockCache := clustermock.NewMockRegisteredClustersCache(g)
				return mockCache
			},
			expectedMatches: []string{},
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			g := gomock.NewController(t)

			cache := test.setup(g)

			builder := framework.NewOptionBuilder()
			builder.Build(framework.WithClusterCache(cache))
			affinityFilter := AffinityFilter{
				OptionBuilder: builder,
			}

			matches := affinityFilter.matchClusterSelector(test.selector, test.candidates)

			require.Equal(t, len(test.expectedMatches), len(matches))
			for i, match := range matches {
				require.Equal(t, test.expectedMatches[i], match.ClusterName)
			}
		})
	}
}

func TestIsClusterAffinityPresent(t *testing.T) {
	tt := []struct {
		msg      string
		selector *v1.LabelSelector
		expected bool
	}{
		{
			msg:      "nil selector",
			selector: nil,
			expected: false,
		},
		{
			msg:      "empty selector",
			selector: &v1.LabelSelector{},
			expected: false,
		},
		{
			msg: "empty match labels",
			selector: &v1.LabelSelector{
				MatchLabels: map[string]string{},
			},
			expected: false,
		},
		{
			msg: "cluster name affinity present",
			selector: createLabelSelector(map[string]string{
				ClusterNameLabelKey: "cluster1",
			}),
			expected: true,
		},
		{
			msg: "cluster region affinity present",
			selector: createLabelSelector(map[string]string{
				ClusterRegionLabelKey: "phx",
			}),
			expected: true,
		},
		{
			msg: "cluster zone affinity present",
			selector: createLabelSelector(map[string]string{
				ClusterZoneLabelKey: "phx5",
			}),
			expected: true,
		},
		{
			msg: "non-cluster affinity labels",
			selector: createLabelSelector(map[string]string{
				"foo": "bar",
				"baz": "qux",
			}),
			expected: false,
		},
		{
			msg: "empty cluster affinity value",
			selector: createLabelSelector(map[string]string{
				ClusterRegionLabelKey: "",
			}),
			expected: false,
		},
		{
			msg: "region-provider affinity present - valid format",
			selector: createLabelSelector(map[string]string{
				ClusterRegionProviderLabelKey: "phx-gcp",
			}),
			expected: true,
		},
		{
			msg: "region-provider affinity only - no other affinity keys",
			selector: createLabelSelector(map[string]string{
				ClusterRegionProviderLabelKey: "phx-gcp",
				"foo":                         "bar",
			}),
			expected: true,
		},
		{
			msg: "region-provider affinity with other cluster affinity keys",
			selector: createLabelSelector(map[string]string{
				ClusterRegionProviderLabelKey: "phx-gcp",
				ClusterRegionLabelKey:         "phx",
			}),
			expected: true,
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			a := AffinityFilter{}
			result := a.isClusterAffinityPresent(test.selector)
			require.Equal(t, test.expected, result)
		})
	}
}

func TestIsFedV2Job(t *testing.T) {
	tt := []struct {
		msg      string
		job      framework.BatchJob
		expected bool
	}{
		{
			msg: "job with ray-region-provider annotation",
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: v1.ObjectMeta{
						Annotations: map[string]string{
							sharedconstants.RayRegionProviderAnnotation: "phx-gcp",
						},
					},
				},
			},
			expected: true,
		},
		{
			msg: "job with empty ray-region-provider annotation",
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: v1.ObjectMeta{
						Annotations: map[string]string{
							sharedconstants.RayRegionProviderAnnotation: "",
						},
					},
				},
			},
			expected: false,
		},
		{
			msg: "job without ray-region-provider annotation",
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: v1.ObjectMeta{
						Annotations: map[string]string{
							"other-annotation": "value",
						},
					},
				},
			},
			expected: false,
		},
		{
			msg: "job with nil annotations",
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: v1.ObjectMeta{
						Annotations: nil,
					},
				},
			},
			expected: false,
		},
		{
			msg: "job with no annotations field",
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: v1.ObjectMeta{},
				},
			},
			expected: false,
		},
		{
			msg: "job with multiple annotations including ray-region-provider",
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: v1.ObjectMeta{
						Annotations: map[string]string{
							"other-annotation":                          "value",
							sharedconstants.RayRegionProviderAnnotation: "dca-gcp",
							"another-annotation":                        "another-value",
						},
					},
				},
			},
			expected: true,
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			a := AffinityFilter{}
			result := a.isFedV2Job(test.job)
			require.Equal(t, test.expected, result)
		})
	}
}

func TestAddFedV2Affinity(t *testing.T) {
	tt := []struct {
		msg              string
		job              framework.BatchJob
		inputSelector    *v1.LabelSelector
		expectedSelector *v1.LabelSelector
	}{
		{
			msg: "nil selector with ray-region-provider annotation",
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-job",
						Namespace: "test-namespace",
						Annotations: map[string]string{
							sharedconstants.RayRegionProviderAnnotation: "phx-gcp",
						},
					},
				},
			},
			inputSelector: nil,
			expectedSelector: createLabelSelector(map[string]string{
				ClusterRegionProviderLabelKey: "phx-gcp",
			}),
		},
		{
			msg: "empty selector with ray-region-provider annotation",
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-job",
						Namespace: "test-namespace",
						Annotations: map[string]string{
							sharedconstants.RayRegionProviderAnnotation: "dca-gcp",
						},
					},
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: &v1.LabelSelector{},
							},
						},
					},
				},
			},
			inputSelector: &v1.LabelSelector{},
			expectedSelector: createLabelSelector(map[string]string{
				ClusterRegionProviderLabelKey: "dca-gcp",
			}),
		},
		{
			msg: "selector with existing labels and ray-region-provider annotation",
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-job",
						Namespace: "test-namespace",
						Annotations: map[string]string{
							sharedconstants.RayRegionProviderAnnotation: "phx-gcp",
						},
					},
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: createLabelSelector(map[string]string{
									"existing-label": "existing-value",
								}),
							},
						},
					},
				},
			},
			inputSelector: createLabelSelector(map[string]string{
				"existing-label": "existing-value",
			}),
			expectedSelector: createLabelSelector(map[string]string{
				"existing-label":              "existing-value",
				ClusterRegionProviderLabelKey: "phx-gcp",
			}),
		},
		{
			msg: "selector with existing region-provider label gets overwritten",
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-job",
						Namespace: "test-namespace",
						Annotations: map[string]string{
							sharedconstants.RayRegionProviderAnnotation: "dca-gcp",
						},
					},
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: createLabelSelector(map[string]string{
									ClusterRegionProviderLabelKey: "old-value",
									"other-label":                 "other-value",
								}),
							},
						},
					},
				},
			},
			inputSelector: createLabelSelector(map[string]string{
				ClusterRegionProviderLabelKey: "old-value",
				"other-label":                 "other-value",
			}),
			expectedSelector: createLabelSelector(map[string]string{
				ClusterRegionProviderLabelKey: "dca-gcp",
				"other-label":                 "other-value",
			}),
		},
		{
			msg: "selector with nil MatchLabels gets initialized",
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-job",
						Namespace: "test-namespace",
						Annotations: map[string]string{
							sharedconstants.RayRegionProviderAnnotation: "phx-gcp",
						},
					},
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: &v1.LabelSelector{
									MatchLabels: nil,
								},
							},
						},
					},
				},
			},
			inputSelector: &v1.LabelSelector{
				MatchLabels: nil,
			},
			expectedSelector: createLabelSelector(map[string]string{
				ClusterRegionProviderLabelKey: "phx-gcp",
			}),
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			builder := framework.NewOptionBuilder()
			builder.Build()
			a := AffinityFilter{
				OptionBuilder: builder,
			}
			result := a.addFedV2Affinity(test.job)
			require.Equal(t, test.expectedSelector, result)
		})
	}
}

func TestIsFedV2JobFromSelector(t *testing.T) {
	tt := []struct {
		msg      string
		selector *v1.LabelSelector
		expected bool
	}{
		{
			msg:      "nil selector",
			selector: nil,
			expected: false,
		},
		{
			msg:      "empty selector",
			selector: &v1.LabelSelector{},
			expected: false,
		},
		{
			msg: "selector with nil MatchLabels",
			selector: &v1.LabelSelector{
				MatchLabels: nil,
			},
			expected: false,
		},
		{
			msg: "selector with empty MatchLabels",
			selector: &v1.LabelSelector{
				MatchLabels: map[string]string{},
			},
			expected: false,
		},
		{
			msg: "selector with region provider - valid value",
			selector: createLabelSelector(map[string]string{
				ClusterRegionProviderLabelKey: "phx-gcp",
			}),
			expected: true,
		},
		{
			msg: "selector with region provider - empty value",
			selector: createLabelSelector(map[string]string{
				ClusterRegionProviderLabelKey: "",
			}),
			expected: false,
		},
		{
			msg: "selector with other labels but no region provider",
			selector: createLabelSelector(map[string]string{
				"other-label": "other-value",
			}),
			expected: false,
		},
		{
			msg: "selector with region provider and other labels",
			selector: createLabelSelector(map[string]string{
				ClusterRegionProviderLabelKey: "dca-gcp",
				"other-label":                 "other-value",
			}),
			expected: true,
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			a := AffinityFilter{}
			result := a.isFedV2JobFromSelector(test.selector)
			require.Equal(t, test.expected, result)
		})
	}
}

func TestFilterFedV2Jobs(t *testing.T) {
	tt := []struct {
		msg               string
		job               framework.BatchJob
		pools             []*cluster.ResourcePoolInfo
		setup             func(g *gomock.Controller) (cluster.RegisteredClustersCache, flipr.FliprClient, types.FliprConstraintsBuilder)
		expectedPoolNames []string
	}{
		{
			msg: "FedV2 job with region-provider annotation matches regional cluster",
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-job",
						Namespace: "test-namespace",
						Annotations: map[string]string{
							sharedconstants.RayRegionProviderAnnotation: "phx-gcp",
						},
					},
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: createLabelSelector(map[string]string{
									"test-label": "test-value",
								}),
							},
						},
					},
				},
			},
			pools: []*cluster.ResourcePoolInfo{
				{
					ClusterName: "phx-onprem-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool1",
							Labels: map[string]string{
								"test-label": "test-value",
							},
						},
					},
				},
				{
					ClusterName: "phx-gcp-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool2",
							Labels: map[string]string{
								"test-label": "test-value",
							},
						},
					},
				},
				{
					ClusterName: "dca-gcp-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool3",
							Labels: map[string]string{
								"test-label": "test-value",
							},
						},
					},
				},
			},
			setup: func(g *gomock.Controller) (cluster.RegisteredClustersCache, flipr.FliprClient, types.FliprConstraintsBuilder) {
				clusterCache := clustermock.NewMockRegisteredClustersCache(g)

				onpremCluster := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "phx",
						Zone:   "phx5", // Non-regional cluster
						Dc:     v2beta1pb.DC_TYPE_ON_PREM,
					},
				}
				onpremCluster.SetName("phx-onprem-cluster")
				clusterCache.EXPECT().GetCluster("phx-onprem-cluster").Return(onpremCluster).AnyTimes()

				gcpCluster := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "PHX",
						// No zone - this makes it a regional cluster
						Dc: v2beta1pb.DC_TYPE_CLOUD_GCP,
					},
				}
				gcpCluster.SetName("phx-gcp-cluster")
				clusterCache.EXPECT().GetCluster("phx-gcp-cluster").Return(gcpCluster).AnyTimes()

				dcaCluster := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "DCA",
						// No zone - this makes it a regional cluster
						Dc: v2beta1pb.DC_TYPE_CLOUD_GCP,
					},
				}
				dcaCluster.SetName("dca-gcp-cluster")
				clusterCache.EXPECT().GetCluster("dca-gcp-cluster").Return(dcaCluster).AnyTimes()

				// FedV2 jobs bypass Flipr logic, so we don't expect Flipr calls
				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockConstraints := typesmock.NewMockFliprConstraintsBuilder(g)

				return clusterCache, mockFlipr, mockConstraints
			},
			expectedPoolNames: []string{"pool2"},
		},
		{
			msg: "FedV2 job with different region-provider matches different cluster",
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-job",
						Namespace: "test-namespace",
						Annotations: map[string]string{
							sharedconstants.RayRegionProviderAnnotation: "dca-gcp",
						},
					},
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: createLabelSelector(map[string]string{
									"test-label": "test-value",
								}),
							},
						},
					},
				},
			},
			pools: []*cluster.ResourcePoolInfo{
				{
					ClusterName: "phx-gcp-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool1",
							Labels: map[string]string{
								"test-label": "test-value",
							},
						},
					},
				},
				{
					ClusterName: "dca-gcp-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool2",
							Labels: map[string]string{
								"test-label": "test-value",
							},
						},
					},
				},
			},
			setup: func(g *gomock.Controller) (cluster.RegisteredClustersCache, flipr.FliprClient, types.FliprConstraintsBuilder) {
				clusterCache := clustermock.NewMockRegisteredClustersCache(g)

				phxCluster := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "phx",
						// No zone - this makes it a regional cluster
						Dc: v2beta1pb.DC_TYPE_CLOUD_GCP,
					},
				}
				phxCluster.SetName("phx-gcp-cluster")
				clusterCache.EXPECT().GetCluster("phx-gcp-cluster").Return(phxCluster).AnyTimes()

				dcaCluster := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "DCA",
						// No zone - this makes it a regional cluster
						Dc: v2beta1pb.DC_TYPE_CLOUD_GCP,
					},
				}
				dcaCluster.SetName("dca-gcp-cluster")
				clusterCache.EXPECT().GetCluster("dca-gcp-cluster").Return(dcaCluster).AnyTimes()

				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockConstraints := typesmock.NewMockFliprConstraintsBuilder(g)

				return clusterCache, mockFlipr, mockConstraints
			},
			expectedPoolNames: []string{"pool2"},
		},
		{
			msg: "non-FedV2 job uses traditional Flipr logic",
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-job",
						Namespace: "test-namespace",
						// No ray-region-provider annotation
					},
					Spec: v2beta1pb.RayJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: createLabelSelector(map[string]string{
									"test-label": "test-value",
								}),
							},
						},
						Head: &v2beta1pb.HeadSpec{
							Pod: &v2beta1pb.PodSpec{
								Resource: &v2beta1pb.ResourceSpec{
									Cpu:    2,
									Memory: "100Gi",
								},
							},
						},
					},
				},
			},
			pools: []*cluster.ResourcePoolInfo{
				{
					ClusterName: "test-cluster",
					Pool: infraCrds.ResourcePool{
						ObjectMeta: v1.ObjectMeta{
							Name: "pool1",
							Labels: map[string]string{
								"test-label": "test-value",
							},
						},
					},
				},
			},
			setup: func(g *gomock.Controller) (cluster.RegisteredClustersCache, flipr.FliprClient, types.FliprConstraintsBuilder) {
				clusterCache := clustermock.NewMockRegisteredClustersCache(g)

				cluster := &v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: _defaultJobRegion,
					},
				}
				cluster.SetName("test-cluster")
				clusterCache.EXPECT().GetCluster("test-cluster").Return(cluster).AnyTimes()

				// Non-FedV2 jobs should use Flipr logic
				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFlipr.EXPECT().GetStringValue(gomock.Any(), _fliprRayJobsInCloud, gomock.Any(), "").
					Return("", nil)

				mockConstraints := typesmock.NewMockFliprConstraintsBuilder(g)
				mockConstraints.EXPECT().GetFliprConstraints(gomock.Any()).Return(flipr.Constraints{})

				return clusterCache, mockFlipr, mockConstraints
			},
			expectedPoolNames: []string{"pool1"},
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			g := gomock.NewController(t)
			clusterCache, mockFlipr, mockConstraints := test.setup(g)

			opts := framework.NewOptionBuilder()
			opts.Build(framework.WithClusterCache(clusterCache), framework.WithFlipr(mockFlipr), framework.WithFliprConstraintsBuilder(mockConstraints))
			affinityFilter := AffinityFilter{
				OptionBuilder: opts,
			}

			matches, err := affinityFilter.Filter(context.Background(), test.job, test.pools)
			require.NoError(t, err)
			require.Equal(t, len(test.expectedPoolNames), len(matches))

			for i, name := range test.expectedPoolNames {
				require.Equal(t, name, matches[i].Pool.Name)
			}
		})
	}
}
