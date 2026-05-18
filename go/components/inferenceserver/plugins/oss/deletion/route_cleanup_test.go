package deletion

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/michelangelo-ai/michelangelo/go/components/common/routing/routingmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory/clientfactorymocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/common/routenames"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// newClientFactoryDispatchingDynamic returns a MockClientFactory whose
// GetDynamicClient looks up the per-target error in clientErr. Targets without
// an entry get (nil, nil).
func newClientFactoryDispatchingDynamic(ctrl *gomock.Controller, clientErr map[string]error) *clientfactorymocks.MockClientFactory {
	m := clientfactorymocks.NewMockClientFactory(ctrl)
	m.EXPECT().GetDynamicClient(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, target *v2pb.ClusterTarget) (dynamic.Interface, error) {
			if err, ok := clientErr[target.GetClusterId()]; ok {
				return nil, err
			}
			return nil, nil
		},
	).AnyTimes()
	return m
}

// newRouteCleanupIS builds the InferenceServer fixture used by the
// RouteCleanupActor tests, with two cluster targets so the per-cluster loop
// executes more than once.
func newRouteCleanupIS() *v2pb.InferenceServer {
	return &v2pb.InferenceServer{
		ObjectMeta: metav1.ObjectMeta{Name: "test-server", Namespace: "test-namespace"},
		Spec: v2pb.InferenceServerSpec{
			ClusterTargets: []*v2pb.ClusterTarget{
				{ClusterId: "c-1"},
				{ClusterId: "c-2"},
			},
		},
	}
}

func TestRouteCleanupActor_Retrieve(t *testing.T) {
	discoveryName := routenames.DiscoveryRouteName("test-server")
	trafficName := routenames.TrafficRouteName("test-server")

	tests := []struct {
		name                string
		clientFactoryErrors map[string]error
		setupMocks          func(rm *routingmocks.MockManager)
		expectedStatus      apipb.ConditionStatus
		expectedMessage     string
		expectedReasonSub   string
	}{
		{
			name: "routeManager.Exists errors for discovery",
			setupMocks: func(rm *routingmocks.MockManager) {
				rm.EXPECT().Exists(gomock.Any(), gomock.Any(), discoveryName, "test-namespace").
					Return(false, errors.New("api error"))
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   "GetFailed",
			expectedReasonSub: "discovery: api error",
		},
		{
			name: "discovery route still exists",
			setupMocks: func(rm *routingmocks.MockManager) {
				rm.EXPECT().Exists(gomock.Any(), gomock.Any(), discoveryName, "test-namespace").
					Return(true, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "DiscoveryRouteStillExists",
		},
		{
			name: "traffic route still exists in one cluster",
			setupMocks: func(rm *routingmocks.MockManager) {
				rm.EXPECT().Exists(gomock.Any(), gomock.Any(), discoveryName, "test-namespace").Return(false, nil)
				rm.EXPECT().Exists(gomock.Any(), gomock.Any(), trafficName, "test-namespace").Return(false, nil)
				rm.EXPECT().Exists(gomock.Any(), gomock.Any(), trafficName, "test-namespace").Return(true, nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   "TrafficRouteStillExists",
			expectedReasonSub: "c-2",
		},
		{
			name: "traffic route still exists in multiple clusters - sorted",
			setupMocks: func(rm *routingmocks.MockManager) {
				rm.EXPECT().Exists(gomock.Any(), gomock.Any(), discoveryName, "test-namespace").Return(false, nil)
				rm.EXPECT().Exists(gomock.Any(), gomock.Any(), trafficName, "test-namespace").Return(true, nil)
				rm.EXPECT().Exists(gomock.Any(), gomock.Any(), trafficName, "test-namespace").Return(true, nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   "TrafficRouteStillExists",
			expectedReasonSub: "c-1,c-2",
		},
		{
			name:                "GetDynamicClient errors for one cluster",
			clientFactoryErrors: map[string]error{"c-1": errors.New("auth refused")},
			setupMocks: func(rm *routingmocks.MockManager) {
				rm.EXPECT().Exists(gomock.Any(), gomock.Any(), discoveryName, "test-namespace").Return(false, nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   "GetFailed",
			expectedReasonSub: "c-1: auth refused",
		},
		{
			name: "all routes gone",
			setupMocks: func(rm *routingmocks.MockManager) {
				rm.EXPECT().Exists(gomock.Any(), gomock.Any(), discoveryName, "test-namespace").Return(false, nil)
				rm.EXPECT().Exists(gomock.Any(), gomock.Any(), trafficName, "test-namespace").Return(false, nil).Times(2)
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
			factory := newClientFactoryDispatchingDynamic(ctrl, tt.clientFactoryErrors)

			actor := NewRouteCleanupActor(nil, factory, rm, zap.NewNop())
			got, err := actor.Retrieve(context.Background(), newRouteCleanupIS(), &apipb.Condition{})

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, got.Status)
			if tt.expectedMessage != "" {
				assert.Equal(t, tt.expectedMessage, got.Message)
			}
			if tt.expectedReasonSub != "" {
				assert.Contains(t, got.Reason, tt.expectedReasonSub)
			}
		})
	}
}

func TestRouteCleanupActor_Run(t *testing.T) {
	discoveryName := routenames.DiscoveryRouteName("test-server")
	trafficName := routenames.TrafficRouteName("test-server")

	tests := []struct {
		name                string
		clientFactoryErrors map[string]error
		setupMocks          func(rm *routingmocks.MockManager)
		expectedStatus      apipb.ConditionStatus
		expectedMessage     string
		expectedReasonSub   string
	}{
		{
			name: "routeManager.Delete fails for discovery",
			setupMocks: func(rm *routingmocks.MockManager) {
				rm.EXPECT().Delete(gomock.Any(), gomock.Any(), discoveryName, "test-namespace").
					Return(errors.New("api error"))
				rm.EXPECT().Delete(gomock.Any(), gomock.Any(), trafficName, "test-namespace").Return(nil).Times(2)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   "DeleteFailed",
			expectedReasonSub: "discovery:",
		},
		{
			name: "routeManager.Delete fails for one cluster traffic",
			setupMocks: func(rm *routingmocks.MockManager) {
				rm.EXPECT().Delete(gomock.Any(), gomock.Any(), discoveryName, "test-namespace").Return(nil)
				rm.EXPECT().Delete(gomock.Any(), gomock.Any(), trafficName, "test-namespace").Return(nil)
				rm.EXPECT().Delete(gomock.Any(), gomock.Any(), trafficName, "test-namespace").Return(errors.New("delete failed"))
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   "DeleteFailed",
			expectedReasonSub: "c-2: delete failed",
		},
		{
			name:                "GetDynamicClient fails for one cluster",
			clientFactoryErrors: map[string]error{"c-1": errors.New("no token")},
			setupMocks: func(rm *routingmocks.MockManager) {
				rm.EXPECT().Delete(gomock.Any(), gomock.Any(), discoveryName, "test-namespace").Return(nil)
				rm.EXPECT().Delete(gomock.Any(), gomock.Any(), trafficName, "test-namespace").Return(nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   "DeleteFailed",
			expectedReasonSub: "c-1: no token",
		},
		{
			name: "multiple deletes fail - sorted",
			setupMocks: func(rm *routingmocks.MockManager) {
				rm.EXPECT().Delete(gomock.Any(), gomock.Any(), discoveryName, "test-namespace").
					Return(errors.New("a-err"))
				rm.EXPECT().Delete(gomock.Any(), gomock.Any(), trafficName, "test-namespace").
					Return(errors.New("b-err"))
				rm.EXPECT().Delete(gomock.Any(), gomock.Any(), trafficName, "test-namespace").Return(nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   "DeleteFailed",
			expectedReasonSub: "c-1: b-err; discovery: a-err",
		},
		{
			name: "all deletes succeed",
			setupMocks: func(rm *routingmocks.MockManager) {
				rm.EXPECT().Delete(gomock.Any(), gomock.Any(), discoveryName, "test-namespace").Return(nil)
				rm.EXPECT().Delete(gomock.Any(), gomock.Any(), trafficName, "test-namespace").Return(nil).Times(2)
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
			factory := newClientFactoryDispatchingDynamic(ctrl, tt.clientFactoryErrors)

			actor := NewRouteCleanupActor(nil, factory, rm, zap.NewNop())
			got, err := actor.Run(context.Background(), newRouteCleanupIS(), &apipb.Condition{})

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, got.Status)
			if tt.expectedMessage != "" {
				assert.Equal(t, tt.expectedMessage, got.Message)
			}
			if tt.expectedReasonSub != "" {
				assert.Contains(t, got.Reason, tt.expectedReasonSub)
			}
		})
	}
}

func TestRouteCleanupActor_GetType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	actor := NewRouteCleanupActor(
		nil,
		newClientFactoryDispatchingDynamic(ctrl, nil),
		routingmocks.NewMockManager(ctrl),
		zap.NewNop(),
	)
	assert.Equal(t, common.RouteCleanupConditionType, actor.GetType())
}
