package cluster

import (
	"testing"

	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
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
