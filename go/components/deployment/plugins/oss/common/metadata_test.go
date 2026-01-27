package common

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
)

func TestGetClusterMetadata(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *apipb.Condition
		expected *ClusterMetadata
	}{
		{
			name: "nil metadata returns nil",
			setup: func() *apipb.Condition {
				return &apipb.Condition{Metadata: nil}
			},
			expected: nil,
		},
		{
			name: "invalid any type returns nil",
			setup: func() *apipb.Condition {
				anyVal, _ := types.MarshalAny(&types.StringValue{Value: "not a struct"})
				return &apipb.Condition{Metadata: anyVal}
			},
			expected: nil,
		},
		{
			name: "empty struct returns nil",
			setup: func() *apipb.Condition {
				anyVal, _ := types.MarshalAny(&types.Struct{})
				return &apipb.Condition{Metadata: anyVal}
			},
			expected: nil,
		},
		{
			name: "struct without clusters field returns nil",
			setup: func() *apipb.Condition {
				structVal := &types.Struct{
					Fields: map[string]*types.Value{
						"backend_type": {Kind: &types.Value_StringValue{StringValue: "triton"}},
					},
				}
				anyVal, _ := types.MarshalAny(structVal)
				return &apipb.Condition{Metadata: anyVal}
			},
			expected: nil,
		},
		{
			name: "valid metadata returns ClusterMetadata",
			setup: func() *apipb.Condition {
				condition := &apipb.Condition{}
				metadata := &ClusterMetadata{
					BackendType: "triton",
					Clusters: []ClusterEntry{
						{
							ClusterID: "cluster-1",
							Host:      "api.cluster1.example.com",
							Port:      "6443",
							TokenTag:  "token-1",
							CaDataTag: "ca-1",
							State:     ClusterStatePending,
						},
					},
					CurrentIndex: 0,
				}
				_ = SetClusterMetadata(condition, metadata)
				return condition
			},
			expected: &ClusterMetadata{
				BackendType: "triton",
				Clusters: []ClusterEntry{
					{
						ClusterID: "cluster-1",
						Host:      "api.cluster1.example.com",
						Port:      "6443",
						TokenTag:  "token-1",
						CaDataTag: "ca-1",
						State:     ClusterStatePending,
					},
				},
				CurrentIndex: 0,
			},
		},
		{
			name: "multiple clusters returns all",
			setup: func() *apipb.Condition {
				condition := &apipb.Condition{}
				metadata := &ClusterMetadata{
					BackendType: "triton",
					Clusters: []ClusterEntry{
						{ClusterID: "cluster-1", State: ClusterStateDeployed},
						{ClusterID: "cluster-2", State: ClusterStateDeploymentInProgress},
						{ClusterID: "cluster-3", State: ClusterStatePending},
					},
					CurrentIndex: 1,
				}
				_ = SetClusterMetadata(condition, metadata)
				return condition
			},
			expected: &ClusterMetadata{
				BackendType: "triton",
				Clusters: []ClusterEntry{
					{ClusterID: "cluster-1", State: ClusterStateDeployed},
					{ClusterID: "cluster-2", State: ClusterStateDeploymentInProgress},
					{ClusterID: "cluster-3", State: ClusterStatePending},
				},
				CurrentIndex: 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition := tt.setup()
			result := GetClusterMetadata(condition)

			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.expected.BackendType, result.BackendType)
				assert.Equal(t, tt.expected.CurrentIndex, result.CurrentIndex)
				require.Len(t, result.Clusters, len(tt.expected.Clusters))
				for i, expectedCluster := range tt.expected.Clusters {
					assert.Equal(t, expectedCluster.ClusterID, result.Clusters[i].ClusterID)
					assert.Equal(t, expectedCluster.Host, result.Clusters[i].Host)
					assert.Equal(t, expectedCluster.Port, result.Clusters[i].Port)
					assert.Equal(t, expectedCluster.TokenTag, result.Clusters[i].TokenTag)
					assert.Equal(t, expectedCluster.CaDataTag, result.Clusters[i].CaDataTag)
					assert.Equal(t, expectedCluster.State, result.Clusters[i].State)
				}
			}
		})
	}
}

func TestSetClusterMetadata(t *testing.T) {
	tests := []struct {
		name        string
		metadata    *ClusterMetadata
		expectErr   bool
		verifyRound bool
	}{
		{
			name: "sets empty clusters",
			metadata: &ClusterMetadata{
				BackendType:  "triton",
				Clusters:     []ClusterEntry{},
				CurrentIndex: 0,
			},
			expectErr:   false,
			verifyRound: true,
		},
		{
			name: "sets single cluster",
			metadata: &ClusterMetadata{
				BackendType: "triton",
				Clusters: []ClusterEntry{
					{
						ClusterID: "cluster-1",
						Host:      "api.cluster1.example.com",
						Port:      "6443",
						TokenTag:  "token-1",
						CaDataTag: "ca-1",
						State:     ClusterStateDeployed,
					},
				},
				CurrentIndex: 0,
			},
			expectErr:   false,
			verifyRound: true,
		},
		{
			name: "sets multiple clusters with different states",
			metadata: &ClusterMetadata{
				BackendType: "triton",
				Clusters: []ClusterEntry{
					{ClusterID: "cluster-1", State: ClusterStateDeployed},
					{ClusterID: "cluster-2", State: ClusterStateCleanupInProgress},
					{ClusterID: "cluster-3", State: ClusterStateRolledBack},
				},
				CurrentIndex: 2,
			},
			expectErr:   false,
			verifyRound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition := &apipb.Condition{}

			err := SetClusterMetadata(condition, tt.metadata)

			if tt.expectErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, condition.Metadata)

			if tt.verifyRound {
				result := GetClusterMetadata(condition)
				require.NotNil(t, result)
				assert.Equal(t, tt.metadata.BackendType, result.BackendType)
				assert.Equal(t, tt.metadata.CurrentIndex, result.CurrentIndex)
				require.Len(t, result.Clusters, len(tt.metadata.Clusters))
				for i, expectedCluster := range tt.metadata.Clusters {
					assert.Equal(t, expectedCluster.ClusterID, result.Clusters[i].ClusterID)
					assert.Equal(t, expectedCluster.State, result.Clusters[i].State)
				}
			}
		})
	}
}
