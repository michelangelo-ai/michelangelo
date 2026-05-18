package common

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/dynamic"

	"github.com/michelangelo-ai/michelangelo/go/components/common/routing"
	"github.com/michelangelo-ai/michelangelo/go/components/common/routing/routingmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory/clientfactorymocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/common/routenames"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// trafficMocks groups the mocks used by traffic-routing tests. Distinct from
// rolloutMocks because this actor uses GetDynamicClient and the route manager
// rather than the typed kube client and the backend / model config providers.
type trafficMocks struct {
	factory      *clientfactorymocks.MockClientFactory
	routeManager *routingmocks.MockManager
}

// newTrafficFixture builds a target wired to the supplied mocks. dynamicClientErr lets a
// test inject a GetDynamicClient failure without re-mocking the factory each time.
func newTrafficFixture(t *testing.T, dynamicClientErr error) (*trafficMocks, *v2pb.ClusterTarget) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mocks := &trafficMocks{
		factory:      clientfactorymocks.NewMockClientFactory(ctrl),
		routeManager: routingmocks.NewMockManager(ctrl),
	}

	mocks.factory.EXPECT().GetDynamicClient(gomock.Any(), gomock.Any()).
		Return(dynamic.Interface(nil), dynamicClientErr).AnyTimes()

	target := &v2pb.ClusterTarget{ClusterId: testCluster}
	return mocks, target
}

func TestTrafficRoutingActor_Retrieve(t *testing.T) {
	routeName := routenames.TrafficRouteName(testISName)
	matchPath := routenames.TrafficMatchPath(testISName, testDeploymentName)
	rewritePath := routenames.TrafficRewritePath(testModelName)

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
			name: "RuleExists errors",
			setupMocks: func(m *trafficMocks) {
				m.routeManager.EXPECT().RuleExists(gomock.Any(), gomock.Any(), routeName, testNamespace,
					routing.Rule{MatchPath: matchPath, RewritePath: rewritePath}).
					Return(false, errors.New("api error"))
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "api error",
		},
		{
			name: "rule not present or model differs",
			setupMocks: func(m *trafficMocks) {
				m.routeManager.EXPECT().RuleExists(gomock.Any(), gomock.Any(), routeName, testNamespace,
					routing.Rule{MatchPath: matchPath, RewritePath: rewritePath}).
					Return(false, nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "traffic route for deployment test-deployment is not configured for model model-v1 in cluster c1",
		},
		{
			name: "rule present and model matches",
			setupMocks: func(m *trafficMocks) {
				m.routeManager.EXPECT().RuleExists(gomock.Any(), gomock.Any(), routeName, testNamespace,
					routing.Rule{MatchPath: matchPath, RewritePath: rewritePath}).
					Return(true, nil)
			},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mocks, target := newTrafficFixture(t, tt.dynamicClientErr)
			tt.setupMocks(mocks)

			actor := NewTrafficRoutingActor(mocks.factory, mocks.routeManager, target)
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
	routeName := routenames.TrafficRouteName(testISName)

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
			name: "AddRules errors",
			setupMocks: func(m *trafficMocks) {
				m.routeManager.EXPECT().AddRules(gomock.Any(), gomock.Any(), routeName, testNamespace, gomock.Any()).
					Return(errors.New("update failed"))
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "update failed",
		},
		{
			name: "happy path",
			setupMocks: func(m *trafficMocks) {
				m.routeManager.EXPECT().AddRules(gomock.Any(), gomock.Any(), routeName, testNamespace, gomock.Any()).
					Return(nil)
			},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mocks, target := newTrafficFixture(t, tt.dynamicClientErr)
			tt.setupMocks(mocks)

			actor := NewTrafficRoutingActor(mocks.factory, mocks.routeManager, target)
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
	mocks, target := newTrafficFixture(t, nil)
	actor := NewTrafficRoutingActor(mocks.factory, mocks.routeManager, target)
	assert.Equal(t, "TrafficRoutingConfigured-"+testCluster, actor.GetType())
}
