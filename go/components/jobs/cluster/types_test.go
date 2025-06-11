package cluster

import (
	"testing"

	v2beta1pb "michelangelo/api/v2beta1"

	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClusterMapGet(t *testing.T) {
	tt := []struct {
		items                   map[string]*Data
		expectedReadyClusters   []string
		expectedUnReadyClusters []string
		expectedClusters        []string
	}{
		{
			items: map[string]*Data{
				"cluster1": {
					cachedObj: &v2beta1pb.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name: "cluster1",
						},
					},
					clusterStatus: &v2beta1pb.ClusterStatus{
						StatusConditions: []*v2beta1pb.Condition{
							{
								Type:   constants.ClusterReady,
								Status: v2beta1pb.CONDITION_STATUS_TRUE,
							},
						},
					},
				},
				"cluster2": {
					cachedObj: &v2beta1pb.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name: "cluster2",
						},
					},
					clusterStatus: &v2beta1pb.ClusterStatus{
						StatusConditions: []*v2beta1pb.Condition{
							{
								Type:   constants.ClusterReady,
								Status: v2beta1pb.CONDITION_STATUS_FALSE,
							},
						},
					},
				},
				"cluster3": {
					cachedObj: &v2beta1pb.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name: "cluster3",
						},
					},
					clusterStatus: &v2beta1pb.ClusterStatus{
						StatusConditions: []*v2beta1pb.Condition{
							{
								Type:   constants.ClusterReady,
								Status: v2beta1pb.CONDITION_STATUS_UNKNOWN,
							},
						},
					},
				},
				"cluster4": {
					cachedObj: &v2beta1pb.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name: "cluster4",
						},
					},
				},
			},
			expectedClusters:        []string{"cluster1", "cluster2", "cluster3", "cluster4"},
			expectedReadyClusters:   []string{"cluster1"},
			expectedUnReadyClusters: []string{"cluster2", "cluster3", "cluster4"},
		},
		{
			items:                   map[string]*Data{},
			expectedClusters:        nil,
			expectedReadyClusters:   nil,
			expectedUnReadyClusters: nil,
		},
	}

	for _, test := range tt {
		clusterMap := &clusterMap{}

		for k, v := range test.items {
			clusterMap.add(k, v)
		}

		readyCluster := clusterMap.getClustersByFilter(ReadyClusters)
		requireEqualClusters(t, readyCluster, test.expectedReadyClusters)

		unReadyCluster := clusterMap.getClustersByFilter(UnreadyClusters)
		requireEqualClusters(t, unReadyCluster, test.expectedUnReadyClusters)

		clusters := clusterMap.getClustersByFilter(AllClusters)
		requireEqualClusters(t, clusters, test.expectedClusters)
	}
}

func requireEqualClusters(t *testing.T, actualCluster []*v2beta1pb.Cluster, expectedClusterNames []string) {
	var actualClusterNames []string
	for _, cluster := range actualCluster {
		if cluster != nil {
			actualClusterNames = append(actualClusterNames, cluster.Name)
		}
	}
	assert.ElementsMatch(t, expectedClusterNames, actualClusterNames)
}
