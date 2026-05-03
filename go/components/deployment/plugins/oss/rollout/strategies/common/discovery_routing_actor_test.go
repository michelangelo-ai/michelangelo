package common

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/routes/routesmocks"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
)

// discoveryMocks groups the mocks used by the DiscoveryRoutingActor tests.
// The actor uses the control-plane dynamic client directly from Params, so no
// ClientFactory is required.
type discoveryMocks struct {
	routeProvider *routesmocks.MockRouteProvider
}

// newDiscoveryFixture builds a Params + mocks for the DiscoveryRoutingActor.
func newDiscoveryFixture(t *testing.T) (Params, *discoveryMocks) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mocks := &discoveryMocks{
		routeProvider: routesmocks.NewMockRouteProvider(ctrl),
	}

	params := Params{
		RouteProvider: mocks.routeProvider,
		Logger:        zap.NewNop(),
	}
	return params, mocks
}

func TestDiscoveryRoutingActor_Retrieve(t *testing.T) {
	tests := []struct {
		name              string
		setupMocks        func(*discoveryMocks)
		expectedStatus    apipb.ConditionStatus
		expectedReasonSub string
	}{
		{
			name: "DeploymentDiscoveryRouteExists errors",
			setupMocks: func(m *discoveryMocks) {
				m.routeProvider.EXPECT().DeploymentDiscoveryRouteExists(gomock.Any(), gomock.Any(),
					testISName, testNamespace, testDeploymentName).
					Return(false, errors.New("api error"))
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "api error",
		},
		{
			name: "rule not present",
			setupMocks: func(m *discoveryMocks) {
				m.routeProvider.EXPECT().DeploymentDiscoveryRouteExists(gomock.Any(), gomock.Any(),
					testISName, testNamespace, testDeploymentName).
					Return(false, nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "discovery route is not configured for the deployment",
		},
		{
			name: "rule present",
			setupMocks: func(m *discoveryMocks) {
				m.routeProvider.EXPECT().DeploymentDiscoveryRouteExists(gomock.Any(), gomock.Any(),
					testISName, testNamespace, testDeploymentName).
					Return(true, nil)
			},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, mocks := newDiscoveryFixture(t)
			tt.setupMocks(mocks)

			actor := NewDiscoveryRoutingActor(params)
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
	tests := []struct {
		name              string
		setupMocks        func(*discoveryMocks)
		expectedStatus    apipb.ConditionStatus
		expectedReasonSub string
	}{
		{
			name: "UpsertDiscoveryRule errors",
			setupMocks: func(m *discoveryMocks) {
				m.routeProvider.EXPECT().UpsertDiscoveryRule(gomock.Any(), gomock.Any(),
					testISName, testNamespace, testDeploymentName, testModelName).
					Return(errors.New("update failed"))
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "update failed",
		},
		{
			name: "happy path",
			setupMocks: func(m *discoveryMocks) {
				m.routeProvider.EXPECT().UpsertDiscoveryRule(gomock.Any(), gomock.Any(),
					testISName, testNamespace, testDeploymentName, testModelName).
					Return(nil)
			},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, mocks := newDiscoveryFixture(t)
			tt.setupMocks(mocks)

			actor := NewDiscoveryRoutingActor(params)
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
	params, _ := newDiscoveryFixture(t)
	actor := NewDiscoveryRoutingActor(params)
	assert.Equal(t, "DiscoveryRoutingConfigured", actor.GetType())
}
