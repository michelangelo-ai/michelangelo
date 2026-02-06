package cluster

import (
	"testing"

	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestData_SetAndGetClusterStatus(t *testing.T) {
	data := &Data{}

	status := &v2pb.ClusterStatus{
		StatusConditions: []*apipb.Condition{},
	}

	data.SetClusterStatus(status)
	retrievedStatus := data.GetClusterStatus()

	assert.Equal(t, len(status.StatusConditions), len(retrievedStatus.StatusConditions))
}

func TestData_UpdateClusterAndStatus(t *testing.T) {
	data := &Data{
		cachedObj: &v2pb.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-cluster",
			},
		},
	}

	newStatus := &v2pb.ClusterStatus{
		StatusConditions: []*apipb.Condition{
			{
				Type:   "Ready",
				Status: apipb.CONDITION_STATUS_TRUE,
			},
		},
	}

	data.UpdateClusterAndStatus(newStatus)

	assert.Equal(t, len(newStatus.StatusConditions), len(data.GetClusterStatus().StatusConditions))
	assert.Equal(t, len(newStatus.StatusConditions), len(data.cachedObj.Status.StatusConditions))
}

func TestFilterType_Constants(t *testing.T) {
	assert.Equal(t, FilterType(0), ReadyClusters)
	assert.Equal(t, FilterType(1), UnreadyClusters)
	assert.Equal(t, FilterType(2), AllClusters)
}

// MockClustersCache for testing
type MockClustersCache struct {
	readyClusters []*v2pb.Cluster
}

func (m *MockClustersCache) GetClusters(filter FilterType) []*v2pb.Cluster {
	if filter == ReadyClusters {
		return m.readyClusters
	}
	return nil
}

func (m *MockClustersCache) GetCluster(name string) *v2pb.Cluster {
	for _, cluster := range m.readyClusters {
		if cluster.Name == name {
			return cluster
		}
	}
	return nil
}

func TestClusterMap_GetClustersByFilter_AllClusters(t *testing.T) {
	cm := &clusterMap{}

	// Create a ready cluster
	readyCluster := &v2pb.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ready-cluster",
		},
	}
	readyStatus := &v2pb.ClusterStatus{
		StatusConditions: []*apipb.Condition{
			{
				Type:   "Ready",
				Status: apipb.CONDITION_STATUS_TRUE,
			},
		},
	}
	readyData := &Data{cachedObj: readyCluster}
	readyData.SetClusterStatus(readyStatus)

	// Create an unready cluster
	unreadyCluster := &v2pb.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "unready-cluster",
		},
	}
	unreadyStatus := &v2pb.ClusterStatus{
		StatusConditions: []*apipb.Condition{
			{
				Type:   "Ready",
				Status: apipb.CONDITION_STATUS_FALSE,
			},
		},
	}
	unreadyData := &Data{cachedObj: unreadyCluster}
	unreadyData.SetClusterStatus(unreadyStatus)

	// Create a cluster with nil status
	nilStatusCluster := &v2pb.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "nil-status-cluster",
		},
	}
	nilStatusData := &Data{cachedObj: nilStatusCluster}
	nilStatusData.SetClusterStatus(nil)

	// Add clusters to the map
	cm.add("ready-cluster", readyData)
	cm.add("unready-cluster", unreadyData)
	cm.add("nil-status-cluster", nilStatusData)

	// Get all clusters
	allClusters := cm.getClustersByFilter(AllClusters)

	// Verify all clusters are returned
	assert.Len(t, allClusters, 3, "Expected all 3 clusters to be returned")

	// Verify the cluster names
	clusterNames := make(map[string]bool)
	for _, cluster := range allClusters {
		clusterNames[cluster.Name] = true
	}

	assert.True(t, clusterNames["ready-cluster"], "Expected ready-cluster to be included")
	assert.True(t, clusterNames["unready-cluster"], "Expected unready-cluster to be included")
	assert.True(t, clusterNames["nil-status-cluster"], "Expected nil-status-cluster to be included")
}
