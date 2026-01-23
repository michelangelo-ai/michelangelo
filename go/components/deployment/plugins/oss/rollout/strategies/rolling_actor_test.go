package strategies

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways/gatewaysmocks"
	"github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

func TestRollingRolloutRetrieve(t *testing.T) {
	tests := []struct {
		name                    string
		deployment              *v2pb.Deployment
		metadata                *ClusterMetadata
		setupMocks              func(*gatewaysmocks.MockGateway)
		expectedConditionStatus api.ConditionStatus
		expectedReasonContains  string
	}{
		{
			name:                    "no metadata returns FALSE to trigger Run",
			deployment:              createDeployment("test-deployment", "default", "model-v1", "test-server"),
			metadata:                nil,
			setupMocks:              func(gw *gatewaysmocks.MockGateway) {},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedReasonContains:  "Rolling rollout has not started",
		},
		{
			name:       "all clusters deployed returns TRUE",
			deployment: createDeployment("test-deployment", "default", "model-v1", "test-server"),
			metadata: &ClusterMetadata{
				BackendType:  "BACKEND_TYPE_TRITON",
				CurrentIndex: 0,
				Clusters: []ClusterEntry{
					{ClusterID: "cluster-1", State: ClusterStateDeployed},
					{ClusterID: "cluster-2", State: ClusterStateDeployed},
				},
			},
			setupMocks:              func(gw *gatewaysmocks.MockGateway) {},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
			expectedReasonContains:  "",
		},
		{
			name:       "cluster pending returns FALSE to trigger Run",
			deployment: createDeployment("test-deployment", "default", "model-v1", "test-server"),
			metadata: &ClusterMetadata{
				BackendType:  "BACKEND_TYPE_TRITON",
				CurrentIndex: 0,
				Clusters: []ClusterEntry{
					{ClusterID: "cluster-1", Host: "1.2.3.4", Port: "6443", State: ClusterStatePending},
				},
			},
			setupMocks:              func(gw *gatewaysmocks.MockGateway) {},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedReasonContains:  "pending deployment",
		},
		{
			name:       "cluster in progress and model ready marks as deployed",
			deployment: createDeployment("test-deployment", "default", "model-v1", "test-server"),
			metadata: &ClusterMetadata{
				BackendType:  "BACKEND_TYPE_TRITON",
				CurrentIndex: 0,
				Clusters: []ClusterEntry{
					{ClusterID: "cluster-1", Host: "1.2.3.4", Port: "6443", TokenTag: "token", CaDataTag: "ca", State: ClusterStateInProgress},
				},
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().CheckModelStatus(
					gomock.Any(), gomock.Any(), "model-v1", "test-server", "default",
					gomock.Any(), v2pb.BACKEND_TYPE_TRITON,
				).Return(true, nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
			expectedReasonContains:  "",
		},
		{
			name:       "cluster in progress and model not ready returns UNKNOWN",
			deployment: createDeployment("test-deployment", "default", "model-v1", "test-server"),
			metadata: &ClusterMetadata{
				BackendType:  "BACKEND_TYPE_TRITON",
				CurrentIndex: 0,
				Clusters: []ClusterEntry{
					{ClusterID: "cluster-1", Host: "1.2.3.4", Port: "6443", TokenTag: "token", CaDataTag: "ca", State: ClusterStateInProgress},
				},
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().CheckModelStatus(
					gomock.Any(), gomock.Any(), "model-v1", "test-server", "default",
					gomock.Any(), v2pb.BACKEND_TYPE_TRITON,
				).Return(false, nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_UNKNOWN,
			expectedReasonContains:  "loading",
		},
		{
			name:       "cluster in progress with status check error returns UNKNOWN",
			deployment: createDeployment("test-deployment", "default", "model-v1", "test-server"),
			metadata: &ClusterMetadata{
				BackendType:  "BACKEND_TYPE_TRITON",
				CurrentIndex: 0,
				Clusters: []ClusterEntry{
					{ClusterID: "cluster-1", Host: "1.2.3.4", Port: "6443", TokenTag: "token", CaDataTag: "ca", State: ClusterStateInProgress},
				},
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().CheckModelStatus(
					gomock.Any(), gomock.Any(), "model-v1", "test-server", "default",
					gomock.Any(), v2pb.BACKEND_TYPE_TRITON,
				).Return(false, errors.New("connection error"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_UNKNOWN,
			expectedReasonContains:  "Failed to check model status",
		},
		{
			name:       "first cluster deployed moves to second cluster",
			deployment: createDeployment("test-deployment", "default", "model-v1", "test-server"),
			metadata: &ClusterMetadata{
				BackendType:  "BACKEND_TYPE_TRITON",
				CurrentIndex: 0,
				Clusters: []ClusterEntry{
					{ClusterID: "cluster-1", State: ClusterStateDeployed},
					{ClusterID: "cluster-2", Host: "5.6.7.8", Port: "6443", State: ClusterStatePending},
				},
			},
			setupMocks:              func(gw *gatewaysmocks.MockGateway) {},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedReasonContains:  "pending deployment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockGateway := gatewaysmocks.NewMockGateway(ctrl)
			tt.setupMocks(mockGateway)

			actor := &RollingRolloutActor{
				gateway: mockGateway,
				logger:  zap.NewNop(),
			}

			condition := createConditionWithMetadata(t, tt.metadata)
			result, err := actor.Retrieve(context.Background(), tt.deployment, condition)

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, tt.expectedConditionStatus, result.Status)
			if tt.expectedReasonContains != "" {
				assert.Contains(t, result.Reason, tt.expectedReasonContains)
			}
		})
	}
}

func TestRollingRolloutRun(t *testing.T) {
	tests := []struct {
		name                    string
		deployment              *v2pb.Deployment
		metadata                *ClusterMetadata
		setupMocks              func(*gatewaysmocks.MockGateway)
		expectedConditionStatus api.ConditionStatus
		expectedReasonContains  string
	}{
		{
			name:       "no metadata initializes from gateway and returns UNKNOWN",
			deployment: createDeployment("test-deployment", "default", "model-v1", "test-server"),
			metadata:   nil,
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().GetDeploymentTargetInfo(
					gomock.Any(), gomock.Any(), "test-server", "default",
				).Return(&gateways.DeploymentTargetInfo{
					BackendType: v2pb.BACKEND_TYPE_TRITON,
					ClusterTargets: []*v2pb.ClusterTarget{
						createClusterTarget("cluster-1", "1.2.3.4", "6443", "token1", "ca1"),
						createClusterTarget("cluster-2", "5.6.7.8", "6443", "token2", "ca2"),
					},
				}, nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_UNKNOWN,
			expectedReasonContains:  "initialized",
		},
		{
			name:       "no metadata with GetDeploymentTargetInfo error returns FALSE",
			deployment: createDeployment("test-deployment", "default", "model-v1", "test-server"),
			metadata:   nil,
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().GetDeploymentTargetInfo(
					gomock.Any(), gomock.Any(), "test-server", "default",
				).Return(nil, errors.New("failed to get target info"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedReasonContains:  "Failed to get deployment target info",
		},
		{
			name:       "no metadata with no clusters returns FALSE",
			deployment: createDeployment("test-deployment", "default", "model-v1", "test-server"),
			metadata:   nil,
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().GetDeploymentTargetInfo(
					gomock.Any(), gomock.Any(), "test-server", "default",
				).Return(&gateways.DeploymentTargetInfo{
					BackendType:    v2pb.BACKEND_TYPE_TRITON,
					ClusterTargets: []*v2pb.ClusterTarget{},
				}, nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedReasonContains:  "No target clusters found",
		},
		{
			name:       "pending cluster deploys model and returns UNKNOWN",
			deployment: createDeployment("test-deployment", "default", "model-v1", "test-server"),
			metadata: &ClusterMetadata{
				BackendType:  "BACKEND_TYPE_TRITON",
				CurrentIndex: 0,
				Clusters: []ClusterEntry{
					{ClusterID: "cluster-1", Host: "1.2.3.4", Port: "6443", TokenTag: "token", CaDataTag: "ca", State: ClusterStatePending},
				},
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().LoadModel(
					gomock.Any(), gomock.Any(), "model-v1", "s3://deploy-models/model-v1/",
					"test-server", "default", gomock.Any(),
				).Return(nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_UNKNOWN,
			expectedReasonContains:  "deployment started",
		},
		{
			name:       "in progress cluster returns UNKNOWN without deploying",
			deployment: createDeployment("test-deployment", "default", "model-v1", "test-server"),
			metadata: &ClusterMetadata{
				BackendType:  "BACKEND_TYPE_TRITON",
				CurrentIndex: 0,
				Clusters: []ClusterEntry{
					{ClusterID: "cluster-1", Host: "1.2.3.4", Port: "6443", State: ClusterStateInProgress},
				},
			},
			setupMocks:              func(gw *gatewaysmocks.MockGateway) {},
			expectedConditionStatus: api.CONDITION_STATUS_UNKNOWN,
			expectedReasonContains:  "in progress",
		},
		{
			name:       "model loading fails returns FALSE",
			deployment: createDeployment("test-deployment", "default", "model-v1", "test-server"),
			metadata: &ClusterMetadata{
				BackendType:  "BACKEND_TYPE_TRITON",
				CurrentIndex: 0,
				Clusters: []ClusterEntry{
					{ClusterID: "cluster-1", Host: "1.2.3.4", Port: "6443", TokenTag: "token", CaDataTag: "ca", State: ClusterStatePending},
				},
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().LoadModel(
					gomock.Any(), gomock.Any(), "model-v1", "s3://deploy-models/model-v1/",
					"test-server", "default", gomock.Any(),
				).Return(errors.New("model loading failed"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedReasonContains:  "Failed to load model",
		},
		{
			name: "no desired revision returns FALSE",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: nil,
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			metadata:                nil,
			setupMocks:              func(gw *gatewaysmocks.MockGateway) {},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedReasonContains:  "No desired revision",
		},
		{
			name:       "all clusters deployed returns TRUE",
			deployment: createDeployment("test-deployment", "default", "model-v1", "test-server"),
			metadata: &ClusterMetadata{
				BackendType:  "BACKEND_TYPE_TRITON",
				CurrentIndex: 2, // Past the last index
				Clusters: []ClusterEntry{
					{ClusterID: "cluster-1", State: ClusterStateDeployed},
					{ClusterID: "cluster-2", State: ClusterStateDeployed},
				},
			},
			setupMocks:              func(gw *gatewaysmocks.MockGateway) {},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
			expectedReasonContains:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockGateway := gatewaysmocks.NewMockGateway(ctrl)
			tt.setupMocks(mockGateway)

			actor := &RollingRolloutActor{
				gateway: mockGateway,
				logger:  zap.NewNop(),
			}

			condition := createConditionWithMetadata(t, tt.metadata)
			result, err := actor.Run(context.Background(), tt.deployment, condition)

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, tt.expectedConditionStatus, result.Status)
			if tt.expectedReasonContains != "" {
				assert.Contains(t, result.Reason, tt.expectedReasonContains)
			}
		})
	}
}

func TestClusterMetadataRoundTrip(t *testing.T) {
	// Test that metadata can be serialized and deserialized correctly
	original := &ClusterMetadata{
		BackendType:  "BACKEND_TYPE_TRITON",
		CurrentIndex: 1,
		Clusters: []ClusterEntry{
			{ClusterID: "cluster-1", Host: "1.2.3.4", Port: "6443", TokenTag: "token1", CaDataTag: "ca1", State: ClusterStateDeployed},
			{ClusterID: "cluster-2", Host: "5.6.7.8", Port: "6443", TokenTag: "token2", CaDataTag: "ca2", State: ClusterStateInProgress},
		},
	}

	condition := createConditionWithMetadata(t, original)

	retrieved := GetClusterMetadata(condition)

	require.NotNil(t, retrieved)
	assert.Equal(t, original.BackendType, retrieved.BackendType)
	assert.Equal(t, original.CurrentIndex, retrieved.CurrentIndex)
	assert.Len(t, retrieved.Clusters, 2)
	assert.Equal(t, original.Clusters[0].ClusterID, retrieved.Clusters[0].ClusterID)
	assert.Equal(t, original.Clusters[0].State, retrieved.Clusters[0].State)
	assert.Equal(t, original.Clusters[1].ClusterID, retrieved.Clusters[1].ClusterID)
	assert.Equal(t, original.Clusters[1].State, retrieved.Clusters[1].State)
}

// Helper to create a condition with ClusterMetadata
func createConditionWithMetadata(t *testing.T, metadata *ClusterMetadata) *api.Condition {
	if metadata == nil {
		return &api.Condition{}
	}

	jsonBytes, err := json.Marshal(metadata)
	require.NoError(t, err)

	var rawMap map[string]interface{}
	require.NoError(t, json.Unmarshal(jsonBytes, &rawMap))

	structVal := convertMapToStruct(rawMap)
	anyVal, err := types.MarshalAny(structVal)
	require.NoError(t, err)

	return &api.Condition{Metadata: anyVal}
}

// Helper to create a basic deployment
func createDeployment(name, namespace, modelName, serverName string) *v2pb.Deployment {
	return &v2pb.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: v2pb.DeploymentSpec{
			DesiredRevision: &api.ResourceIdentifier{Name: modelName},
			Target: &v2pb.DeploymentSpec_InferenceServer{
				InferenceServer: &api.ResourceIdentifier{Name: serverName},
			},
		},
	}
}

// Helper to create cluster targets
func createClusterTarget(clusterID, host, port, tokenTag, caDataTag string) *v2pb.ClusterTarget {
	return &v2pb.ClusterTarget{
		ClusterId: clusterID,
		Config: &v2pb.ClusterTarget_Kubernetes{
			Kubernetes: &v2pb.ConnectionSpec{
				Host:      host,
				Port:      port,
				TokenTag:  tokenTag,
				CaDataTag: caDataTag,
			},
		},
	}
}
