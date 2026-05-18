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

	"github.com/michelangelo-ai/michelangelo/go/components/common/routing/routingmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/common/routenames"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// newDiscoveryRouteIS builds the InferenceServer fixture used by the
// DiscoveryRouteProvisionActor tests. Name and namespace are the only fields
// the actor reads.
func newDiscoveryRouteIS() *v2pb.InferenceServer {
	return &v2pb.InferenceServer{
		ObjectMeta: metav1.ObjectMeta{Name: "test-server", Namespace: "test-namespace"},
	}
}

func TestDiscoveryRouteProvisionActor_Retrieve(t *testing.T) {
	routeName := routenames.DiscoveryRouteName("test-server")

	tests := []struct {
		name            string
		setupMocks      func(*routingmocks.MockManager)
		expectedStatus  apipb.ConditionStatus
		expectedMessage string
	}{
		{
			name: "Exists errors",
			setupMocks: func(rm *routingmocks.MockManager) {
				rm.EXPECT().Exists(gomock.Any(), gomock.Any(), routeName, "test-namespace").
					Return(false, errors.New("api error"))
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "GetFailed",
		},
		{
			name: "route not provisioned",
			setupMocks: func(rm *routingmocks.MockManager) {
				rm.EXPECT().Exists(gomock.Any(), gomock.Any(), routeName, "test-namespace").
					Return(false, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "DiscoveryRouteMissing",
		},
		{
			name: "route provisioned",
			setupMocks: func(rm *routingmocks.MockManager) {
				rm.EXPECT().Exists(gomock.Any(), gomock.Any(), routeName, "test-namespace").
					Return(true, nil)
			},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			rm := routingmocks.NewMockManager(ctrl)
			tt.setupMocks(rm)

			actor := NewDiscoveryRouteProvisionActor(nil, rm, "test-gateway", zap.NewNop())
			got, err := actor.Retrieve(context.Background(), newDiscoveryRouteIS(), &apipb.Condition{})

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, got.Status)
			if tt.expectedMessage != "" {
				assert.Equal(t, tt.expectedMessage, got.Message)
			}
		})
	}
}

func TestDiscoveryRouteProvisionActor_Run(t *testing.T) {
	routeName := routenames.DiscoveryRouteName("test-server")

	tests := []struct {
		name            string
		setupMocks      func(*routingmocks.MockManager)
		expectedStatus  apipb.ConditionStatus
		expectedMessage string
	}{
		{
			name: "Create errors",
			setupMocks: func(rm *routingmocks.MockManager) {
				rm.EXPECT().Create(gomock.Any(), gomock.Any(), routeName, "test-namespace", gomock.Any()).
					Return(errors.New("apply failed"))
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "EnsureFailed",
		},
		{
			name: "AddRules errors",
			setupMocks: func(rm *routingmocks.MockManager) {
				rm.EXPECT().Create(gomock.Any(), gomock.Any(), routeName, "test-namespace", gomock.Any()).
					Return(nil)
				rm.EXPECT().AddRules(gomock.Any(), gomock.Any(), routeName, "test-namespace", gomock.Any()).
					Return(errors.New("rules failed"))
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "EnsureFailed",
		},
		{
			name: "happy path",
			setupMocks: func(rm *routingmocks.MockManager) {
				rm.EXPECT().Create(gomock.Any(), gomock.Any(), routeName, "test-namespace", gomock.Any()).
					Return(nil)
				rm.EXPECT().AddRules(gomock.Any(), gomock.Any(), routeName, "test-namespace", gomock.Any()).
					Return(nil)
			},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			rm := routingmocks.NewMockManager(ctrl)
			tt.setupMocks(rm)

			actor := NewDiscoveryRouteProvisionActor(nil, rm, "test-gateway", zap.NewNop())
			got, err := actor.Run(context.Background(), newDiscoveryRouteIS(), &apipb.Condition{})

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, got.Status)
			if tt.expectedMessage != "" {
				assert.Equal(t, tt.expectedMessage, got.Message)
			}
		})
	}
}

func TestDiscoveryRouteProvisionActor_GetType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	actor := NewDiscoveryRouteProvisionActor(nil, routingmocks.NewMockManager(ctrl), "test-gateway", zap.NewNop())
	assert.Equal(t, common.DiscoveryRouteProvisionType, actor.GetType())
}
