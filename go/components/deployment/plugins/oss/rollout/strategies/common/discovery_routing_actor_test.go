package common

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/michelangelo-ai/michelangelo/go/components/common/routing"
	"github.com/michelangelo-ai/michelangelo/go/components/common/routing/routingmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/common/routenames"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
)

// discoveryMocks groups the mocks used by the DiscoveryRoutingActor tests.
type discoveryMocks struct {
	routeManager *routingmocks.MockManager
}

// newDiscoveryFixture builds the mocks for the DiscoveryRoutingActor.
func newDiscoveryFixture(t *testing.T) *discoveryMocks {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	return &discoveryMocks{
		routeManager: routingmocks.NewMockManager(ctrl),
	}
}

func TestDiscoveryRoutingActor_Retrieve(t *testing.T) {
	routeName := routenames.DiscoveryRouteName(testISName)
	matchPath := routenames.DiscoveryMatchPath(testISName, testDeploymentName)

	tests := []struct {
		name              string
		setupMocks        func(*discoveryMocks)
		expectedStatus    apipb.ConditionStatus
		expectedReasonSub string
	}{
		{
			name: "RuleExists errors",
			setupMocks: func(m *discoveryMocks) {
				m.routeManager.EXPECT().RuleExists(gomock.Any(), gomock.Any(), routeName, testNamespace,
					routing.Rule{MatchPath: matchPath}).
					Return(false, errors.New("api error"))
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "api error",
		},
		{
			name: "rule not present",
			setupMocks: func(m *discoveryMocks) {
				m.routeManager.EXPECT().RuleExists(gomock.Any(), gomock.Any(), routeName, testNamespace,
					routing.Rule{MatchPath: matchPath}).
					Return(false, nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "discovery route is not configured for the deployment",
		},
		{
			name: "rule present",
			setupMocks: func(m *discoveryMocks) {
				m.routeManager.EXPECT().RuleExists(gomock.Any(), gomock.Any(), routeName, testNamespace,
					routing.Rule{MatchPath: matchPath}).
					Return(true, nil)
			},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mocks := newDiscoveryFixture(t)
			tt.setupMocks(mocks)

			actor := NewDiscoveryRoutingActor(nil, mocks.routeManager)
			got, err := actor.Retrieve(context.Background(), rolloutDeployment(""), &apipb.Condition{})

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, got.Status)
			if tt.expectedReasonSub != "" {
				assert.Contains(t, got.Reason, tt.expectedReasonSub)
			}
		})
	}
}

func TestDiscoveryRoutingActor_Run(t *testing.T) {
	routeName := routenames.DiscoveryRouteName(testISName)

	tests := []struct {
		name              string
		setupMocks        func(*discoveryMocks)
		expectedStatus    apipb.ConditionStatus
		expectedReasonSub string
	}{
		{
			name: "AddRules errors",
			setupMocks: func(m *discoveryMocks) {
				m.routeManager.EXPECT().AddRules(gomock.Any(), gomock.Any(), routeName, testNamespace, gomock.Any()).
					Return(errors.New("update failed"))
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "update failed",
		},
		{
			name: "happy path",
			setupMocks: func(m *discoveryMocks) {
				m.routeManager.EXPECT().AddRules(gomock.Any(), gomock.Any(), routeName, testNamespace, gomock.Any()).
					Return(nil)
			},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mocks := newDiscoveryFixture(t)
			tt.setupMocks(mocks)

			actor := NewDiscoveryRoutingActor(nil, mocks.routeManager)
			got, err := actor.Run(context.Background(), rolloutDeployment(""), &apipb.Condition{})

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, got.Status)
			if tt.expectedReasonSub != "" {
				assert.Contains(t, got.Reason, tt.expectedReasonSub)
			}
		})
	}
}

func TestDiscoveryRoutingActor_GetType(t *testing.T) {
	mocks := newDiscoveryFixture(t)
	actor := NewDiscoveryRoutingActor(nil, mocks.routeManager)
	assert.Equal(t, "DiscoveryRoutingConfigured", actor.GetType())
}
