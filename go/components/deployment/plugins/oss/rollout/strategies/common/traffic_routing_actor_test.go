package common

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory/clientfactorymocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/routes/routesmocks"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// trafficMocks groups the mocks used by traffic-routing tests. Distinct from
// rolloutMocks because this actor uses GetDynamicClient and the route provider
// rather than the typed kube client and the backend / model config providers.
type trafficMocks struct {
	factory       *clientfactorymocks.MockClientFactory
	routeProvider *routesmocks.MockRouteProvider
}

// newTrafficFixture builds a Params + target wired to the supplied mocks.
// dynamicClientErr lets a test inject a GetDynamicClient failure without
// re-mocking the factory each time.
func newTrafficFixture(t *testing.T, dynamicClientErr error) (Params, *v2pb.ClusterTarget, *trafficMocks) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mocks := &trafficMocks{
		factory:       clientfactorymocks.NewMockClientFactory(ctrl),
		routeProvider: routesmocks.NewMockRouteProvider(ctrl),
	}

	mocks.factory.EXPECT().GetDynamicClient(gomock.Any(), gomock.Any()).
		Return(dynamic.Interface(nil), dynamicClientErr).AnyTimes()

	params := Params{
		ClientFactory: mocks.factory,
		RouteProvider: mocks.routeProvider,
		Logger:        zap.NewNop(),
	}
	target := &v2pb.ClusterTarget{ClusterId: testCluster}
	return params, target, mocks
}

func TestTrafficRoutingActor_Retrieve(t *testing.T) {
	tests := []struct {
		name              string
		dynamicClientErr  error
		setupMocks        func(*trafficMocks)
		expectedStatus    apipb.ConditionStatus
		expectedReasonSub string
	}{
		{
			name:              "GetDynamicClient errors",
			dynamicClientErr:  errors.New("dial timeout"),
			setupMocks:        func(*trafficMocks) {},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "dial timeout",
		},
		{
			name: "DeploymentTrafficRouteExists errors",
			setupMocks: func(m *trafficMocks) {
				m.routeProvider.EXPECT().DeploymentTrafficRouteExists(gomock.Any(), gomock.Any(),
					testCluster, testISName, testNamespace, testDeploymentName, testModelName).
					Return(false, errors.New("api error"))
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "api error",
		},
		{
			name: "rule not present or model differs",
			setupMocks: func(m *trafficMocks) {
				m.routeProvider.EXPECT().DeploymentTrafficRouteExists(gomock.Any(), gomock.Any(),
					testCluster, testISName, testNamespace, testDeploymentName, testModelName).
					Return(false, nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "traffic route for deployment test-deployment is not configured for model model-v1 in cluster c1",
		},
		{
			name: "rule present and model matches",
			setupMocks: func(m *trafficMocks) {
				m.routeProvider.EXPECT().DeploymentTrafficRouteExists(gomock.Any(), gomock.Any(),
					testCluster, testISName, testNamespace, testDeploymentName, testModelName).
					Return(true, nil)
			},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, target, mocks := newTrafficFixture(t, tt.dynamicClientErr)
			tt.setupMocks(mocks)

			actor := NewTrafficRoutingActor(params, target)
			got, err := actor.Retrieve(context.Background(), rolloutDeployment(""), &apipb.Condition{})

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, got.Status)
			if tt.expectedReasonSub != "" {
				assert.Contains(t, got.Reason, tt.expectedReasonSub)
			}
		})
	}
}

func TestTrafficRoutingActor_Run(t *testing.T) {
	tests := []struct {
		name              string
		dynamicClientErr  error
		setupMocks        func(*trafficMocks)
		expectedStatus    apipb.ConditionStatus
		expectedReasonSub string
	}{
		{
			name:              "GetDynamicClient errors",
			dynamicClientErr:  errors.New("dial timeout"),
			setupMocks:        func(*trafficMocks) {},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "dial timeout",
		},
		{
			name: "UpsertTrafficRule errors",
			setupMocks: func(m *trafficMocks) {
				m.routeProvider.EXPECT().UpsertTrafficRule(gomock.Any(), gomock.Any(),
					testCluster, testISName, testNamespace, testDeploymentName, testModelName).
					Return(errors.New("update failed"))
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "update failed",
		},
		{
			name: "happy path",
			setupMocks: func(m *trafficMocks) {
				m.routeProvider.EXPECT().UpsertTrafficRule(gomock.Any(), gomock.Any(),
					testCluster, testISName, testNamespace, testDeploymentName, testModelName).
					Return(nil)
			},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, target, mocks := newTrafficFixture(t, tt.dynamicClientErr)
			tt.setupMocks(mocks)

			actor := NewTrafficRoutingActor(params, target)
			got, err := actor.Run(context.Background(), rolloutDeployment(""), &apipb.Condition{})

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, got.Status)
			if tt.expectedReasonSub != "" {
				assert.Contains(t, got.Reason, tt.expectedReasonSub)
			}
		})
	}
}

func TestTrafficRoutingActor_GetType(t *testing.T) {
	params, target, _ := newTrafficFixture(t, nil)
	actor := NewTrafficRoutingActor(params, target)
	assert.Equal(t, "TrafficRoutingConfigured-"+testCluster, actor.GetType())
}
