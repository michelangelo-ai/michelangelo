package creation

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/endpointregistry"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/endpointregistry/endpointregistrymocks"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

func TestControlPlaneDiscoveryActor_Retrieve(t *testing.T) {
	tests := []struct {
		name            string
		resource        *v2pb.InferenceServer
		setupMocks      func(*endpointregistrymocks.MockEndpointRegistry)
		expectedStatus  apipb.ConditionStatus
		expectedMessage string
		expectedErr     bool
	}{
		{
			name: "single cluster setup",
			resource: &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "test-namespace",
				},
				Spec: v2pb.InferenceServerSpec{
					ClusterTargets: []*v2pb.ClusterTarget{
						{ClusterId: "control-plane"}, // no kubernetes config
					},
				},
			},
			setupMocks:      func(mockRegistry *endpointregistrymocks.MockEndpointRegistry) {},
			expectedStatus:  apipb.CONDITION_STATUS_TRUE,
			expectedMessage: "",
			expectedErr:     false,
		},
		{
			name: "all remote clusters registered in sync",
			resource: &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "test-namespace",
				},
				Spec: v2pb.InferenceServerSpec{
					ClusterTargets: []*v2pb.ClusterTarget{
						{ClusterId: "cluster-1", Config: &v2pb.ClusterTarget_Kubernetes{Kubernetes: &v2pb.ConnectionSpec{Host: "host1"}}},
						{ClusterId: "cluster-2", Config: &v2pb.ClusterTarget_Kubernetes{Kubernetes: &v2pb.ConnectionSpec{Host: "host2"}}},
					},
				},
			},
			setupMocks: func(mockRegistry *endpointregistrymocks.MockEndpointRegistry) {
				mockRegistry.EXPECT().
					ListRegisteredEndpoints(gomock.Any(), gomock.Any(), "test-server", "test-namespace").
					Return([]endpointregistry.ClusterEndpoint{
						{ClusterID: "cluster-1"},
						{ClusterID: "cluster-2"},
					}, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_TRUE,
			expectedMessage: "",
			expectedErr:     false,
		},
		{
			name: "out of sync: missing cluster",
			resource: &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "test-namespace",
				},
				Spec: v2pb.InferenceServerSpec{
					ClusterTargets: []*v2pb.ClusterTarget{
						{ClusterId: "cluster-1", Config: &v2pb.ClusterTarget_Kubernetes{Kubernetes: &v2pb.ConnectionSpec{Host: "host1"}}},
						{ClusterId: "cluster-2", Config: &v2pb.ClusterTarget_Kubernetes{Kubernetes: &v2pb.ConnectionSpec{Host: "host2"}}},
					},
				},
			},
			setupMocks: func(mockRegistry *endpointregistrymocks.MockEndpointRegistry) {
				mockRegistry.EXPECT().
					ListRegisteredEndpoints(gomock.Any(), gomock.Any(), "test-server", "test-namespace").
					Return([]endpointregistry.ClusterEndpoint{
						{ClusterID: "cluster-1"},
					}, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_UNKNOWN,
			expectedMessage: "DiscoveryOutOfSync",
			expectedErr:     false,
		},
		{
			name: "out of sync: stale cluster",
			resource: &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "test-namespace",
				},
				Spec: v2pb.InferenceServerSpec{
					ClusterTargets: []*v2pb.ClusterTarget{
						{ClusterId: "cluster-1", Config: &v2pb.ClusterTarget_Kubernetes{Kubernetes: &v2pb.ConnectionSpec{Host: "host1"}}},
					},
				},
			},
			setupMocks: func(mockRegistry *endpointregistrymocks.MockEndpointRegistry) {
				mockRegistry.EXPECT().
					ListRegisteredEndpoints(gomock.Any(), gomock.Any(), "test-server", "test-namespace").
					Return([]endpointregistry.ClusterEndpoint{
						{ClusterID: "cluster-1"},
						{ClusterID: "cluster-stale"},
					}, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_UNKNOWN,
			expectedMessage: "DiscoveryOutOfSync",
			expectedErr:     false,
		},
		{
			name: "list endpoints fails",
			resource: &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "test-namespace",
				},
				Spec: v2pb.InferenceServerSpec{
					ClusterTargets: []*v2pb.ClusterTarget{
						{ClusterId: "cluster-1", Config: &v2pb.ClusterTarget_Kubernetes{Kubernetes: &v2pb.ConnectionSpec{Host: "host1"}}},
					},
				},
			},
			setupMocks: func(mockRegistry *endpointregistrymocks.MockEndpointRegistry) {
				mockRegistry.EXPECT().
					ListRegisteredEndpoints(gomock.Any(), gomock.Any(), "test-server", "test-namespace").
					Return(nil, errors.New("API error"))
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "ListEndpointsFailed",
			expectedErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRegistry := endpointregistrymocks.NewMockEndpointRegistry(ctrl)
			tt.setupMocks(mockRegistry)

			actor := NewControlPlaneDiscoveryActor(mockRegistry, zap.NewNop())

			condition := &apipb.Condition{
				Type: "TritonControlPlaneDiscovery",
			}

			result, err := actor.Retrieve(context.Background(), tt.resource, condition)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, result.Status)
				if tt.expectedMessage != "" {
					assert.Equal(t, tt.expectedMessage, result.Message)
				}
			}
		})
	}
}

func TestControlPlaneDiscoveryActor_Run(t *testing.T) {
	tests := []struct {
		name            string
		resource        *v2pb.InferenceServer
		setupMocks      func(*endpointregistrymocks.MockEndpointRegistry)
		expectedStatus  apipb.ConditionStatus
		expectedMessage string
		expectedErr     bool
	}{
		{
			name: "registers missing endpoint",
			resource: &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "test-namespace",
				},
				Spec: v2pb.InferenceServerSpec{
					ClusterTargets: []*v2pb.ClusterTarget{
						{ClusterId: "cluster-1", Config: &v2pb.ClusterTarget_Kubernetes{Kubernetes: &v2pb.ConnectionSpec{Host: "host1"}}},
					},
				},
			},
			setupMocks: func(mockRegistry *endpointregistrymocks.MockEndpointRegistry) {
				mockRegistry.EXPECT().
					ListRegisteredEndpoints(gomock.Any(), gomock.Any(), "test-server", "test-namespace").
					Return([]endpointregistry.ClusterEndpoint{}, nil)
				mockRegistry.EXPECT().
					EnsureRegisteredEndpoint(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_UNKNOWN,
			expectedMessage: "DiscoveryReconciled",
			expectedErr:     false,
		},
		{
			name: "deletes stale endpoint",
			resource: &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "test-namespace",
				},
				Spec: v2pb.InferenceServerSpec{
					ClusterTargets: []*v2pb.ClusterTarget{},
				},
			},
			setupMocks: func(mockRegistry *endpointregistrymocks.MockEndpointRegistry) {
				mockRegistry.EXPECT().
					ListRegisteredEndpoints(gomock.Any(), gomock.Any(), "test-server", "test-namespace").
					Return([]endpointregistry.ClusterEndpoint{
						{ClusterID: "stale-cluster"},
					}, nil)
				mockRegistry.EXPECT().
					DeleteRegisteredEndpoint(gomock.Any(), gomock.Any(), "test-server", "test-namespace", "stale-cluster").
					Return(nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_UNKNOWN,
			expectedMessage: "DiscoveryReconciled",
			expectedErr:     false,
		},
		{
			name: "registers missing and deletes stale",
			resource: &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "test-namespace",
				},
				Spec: v2pb.InferenceServerSpec{
					ClusterTargets: []*v2pb.ClusterTarget{
						{ClusterId: "cluster-new", Config: &v2pb.ClusterTarget_Kubernetes{Kubernetes: &v2pb.ConnectionSpec{Host: "host1"}}},
					},
				},
			},
			setupMocks: func(mockRegistry *endpointregistrymocks.MockEndpointRegistry) {
				mockRegistry.EXPECT().
					ListRegisteredEndpoints(gomock.Any(), gomock.Any(), "test-server", "test-namespace").
					Return([]endpointregistry.ClusterEndpoint{
						{ClusterID: "cluster-stale"},
					}, nil)
				mockRegistry.EXPECT().
					EnsureRegisteredEndpoint(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
				mockRegistry.EXPECT().
					DeleteRegisteredEndpoint(gomock.Any(), gomock.Any(), "test-server", "test-namespace", "cluster-stale").
					Return(nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_UNKNOWN,
			expectedMessage: "DiscoveryReconciled",
			expectedErr:     false,
		},
		{
			name: "register endpoint fails",
			resource: &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "test-namespace",
				},
				Spec: v2pb.InferenceServerSpec{
					ClusterTargets: []*v2pb.ClusterTarget{
						{ClusterId: "cluster-1", Config: &v2pb.ClusterTarget_Kubernetes{Kubernetes: &v2pb.ConnectionSpec{Host: "host1"}}},
					},
				},
			},
			setupMocks: func(mockRegistry *endpointregistrymocks.MockEndpointRegistry) {
				mockRegistry.EXPECT().
					ListRegisteredEndpoints(gomock.Any(), gomock.Any(), "test-server", "test-namespace").
					Return([]endpointregistry.ClusterEndpoint{}, nil)
				mockRegistry.EXPECT().
					EnsureRegisteredEndpoint(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("register failed"))
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "RegisterEndpointFailed",
			expectedErr:     false,
		},
		{
			name: "delete endpoint fails",
			resource: &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "test-namespace",
				},
				Spec: v2pb.InferenceServerSpec{
					ClusterTargets: []*v2pb.ClusterTarget{},
				},
			},
			setupMocks: func(mockRegistry *endpointregistrymocks.MockEndpointRegistry) {
				mockRegistry.EXPECT().
					ListRegisteredEndpoints(gomock.Any(), gomock.Any(), "test-server", "test-namespace").
					Return([]endpointregistry.ClusterEndpoint{
						{ClusterID: "stale-cluster"},
					}, nil)
				mockRegistry.EXPECT().
					DeleteRegisteredEndpoint(gomock.Any(), gomock.Any(), "test-server", "test-namespace", "stale-cluster").
					Return(errors.New("delete failed"))
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "DeleteEndpointFailed",
			expectedErr:     false,
		},
		{
			name: "list endpoints fails",
			resource: &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "test-namespace",
				},
				Spec: v2pb.InferenceServerSpec{
					ClusterTargets: []*v2pb.ClusterTarget{
						{ClusterId: "cluster-1", Config: &v2pb.ClusterTarget_Kubernetes{Kubernetes: &v2pb.ConnectionSpec{Host: "host1"}}},
					},
				},
			},
			setupMocks: func(mockRegistry *endpointregistrymocks.MockEndpointRegistry) {
				mockRegistry.EXPECT().
					ListRegisteredEndpoints(gomock.Any(), gomock.Any(), "test-server", "test-namespace").
					Return(nil, errors.New("API error"))
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "ListEndpointsFailed",
			expectedErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRegistry := endpointregistrymocks.NewMockEndpointRegistry(ctrl)
			tt.setupMocks(mockRegistry)

			actor := NewControlPlaneDiscoveryActor(mockRegistry, zap.NewNop())

			condition := &apipb.Condition{
				Type: "TritonControlPlaneDiscovery",
			}

			result, err := actor.Run(context.Background(), tt.resource, condition)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedStatus, result.Status)
				assert.Equal(t, tt.expectedMessage, result.Message)
			}
		})
	}
}
