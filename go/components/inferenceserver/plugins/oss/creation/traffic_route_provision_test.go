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
// an entry get (nil, nil). Mirrors the GetClient dispatcher in
// backend_provision_test.go.
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

// newTrafficRouteIS builds the InferenceServer fixture used by the
// TrafficRouteProvisionActor tests, with two cluster targets to exercise the
// per-cluster fan-out.
func newTrafficRouteIS() *v2pb.InferenceServer {
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

func TestTrafficRouteProvisionActor_Retrieve(t *testing.T) {
	routeName := routenames.TrafficRouteName("test-server")

	tests := []struct {
		name                string
		clientFactoryErrors map[string]error
		setupMocks          func(*routingmocks.MockManager)
		expectedStatus      apipb.ConditionStatus
		expectedMessage     string
		expectedReasonSub   string
	}{
		{
			name: "all clusters provisioned",
			setupMocks: func(rm *routingmocks.MockManager) {
				rm.EXPECT().Exists(gomock.Any(), gomock.Any(), routeName, "test-namespace").Return(true, nil).Times(2)
			},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
		},
		{
			name: "one cluster missing the route",
			setupMocks: func(rm *routingmocks.MockManager) {
				rm.EXPECT().Exists(gomock.Any(), gomock.Any(), routeName, "test-namespace").Return(true, nil)
				rm.EXPECT().Exists(gomock.Any(), gomock.Any(), routeName, "test-namespace").Return(false, nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   "TrafficRouteMissing",
			expectedReasonSub: "c-2",
		},
		{
			name:                "GetDynamicClient fails for one cluster",
			clientFactoryErrors: map[string]error{"c-2": errors.New("auth refused")},
			setupMocks: func(rm *routingmocks.MockManager) {
				rm.EXPECT().Exists(gomock.Any(), gomock.Any(), routeName, "test-namespace").Return(true, nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   "TrafficRouteMissing",
			expectedReasonSub: "c-2: auth refused",
		},
		{
			name: "Exists errors for one cluster",
			setupMocks: func(rm *routingmocks.MockManager) {
				rm.EXPECT().Exists(gomock.Any(), gomock.Any(), routeName, "test-namespace").Return(true, nil)
				rm.EXPECT().Exists(gomock.Any(), gomock.Any(), routeName, "test-namespace").Return(false, errors.New("api timeout"))
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   "TrafficRouteMissing",
			expectedReasonSub: "c-2: api timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			rm := routingmocks.NewMockManager(ctrl)
			tt.setupMocks(rm)
			factory := newClientFactoryDispatchingDynamic(ctrl, tt.clientFactoryErrors)

			actor := NewTrafficRouteProvisionActor(factory, rm, "test-gateway", zap.NewNop())
			got, err := actor.Retrieve(context.Background(), newTrafficRouteIS(), &apipb.Condition{})

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

func TestTrafficRouteProvisionActor_Run(t *testing.T) {
	routeName := routenames.TrafficRouteName("test-server")

	tests := []struct {
		name                string
		clientFactoryErrors map[string]error
		setupMocks          func(*routingmocks.MockManager)
		expectedStatus      apipb.ConditionStatus
		expectedMessage     string
		expectedReasonSub   string
	}{
		{
			name: "all ensures succeed",
			setupMocks: func(rm *routingmocks.MockManager) {
				rm.EXPECT().Create(gomock.Any(), gomock.Any(), routeName, "test-namespace", gomock.Any()).Return(nil).Times(2)
				rm.EXPECT().AddRules(gomock.Any(), gomock.Any(), routeName, "test-namespace", gomock.Any()).Return(nil).Times(2)
			},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
		},
		{
			name: "one Create fails",
			setupMocks: func(rm *routingmocks.MockManager) {
				rm.EXPECT().Create(gomock.Any(), gomock.Any(), routeName, "test-namespace", gomock.Any()).Return(nil)
				rm.EXPECT().AddRules(gomock.Any(), gomock.Any(), routeName, "test-namespace", gomock.Any()).Return(nil)
				rm.EXPECT().Create(gomock.Any(), gomock.Any(), routeName, "test-namespace", gomock.Any()).Return(errors.New("apply failed"))
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   "EnsureFailed",
			expectedReasonSub: "c-2: apply failed",
		},
		{
			name:                "GetDynamicClient fails for one cluster",
			clientFactoryErrors: map[string]error{"c-1": errors.New("no token")},
			setupMocks: func(rm *routingmocks.MockManager) {
				rm.EXPECT().Create(gomock.Any(), gomock.Any(), routeName, "test-namespace", gomock.Any()).Return(nil)
				rm.EXPECT().AddRules(gomock.Any(), gomock.Any(), routeName, "test-namespace", gomock.Any()).Return(nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   "EnsureFailed",
			expectedReasonSub: "c-1: no token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			rm := routingmocks.NewMockManager(ctrl)
			tt.setupMocks(rm)
			factory := newClientFactoryDispatchingDynamic(ctrl, tt.clientFactoryErrors)

			actor := NewTrafficRouteProvisionActor(factory, rm, "test-gateway", zap.NewNop())
			got, err := actor.Run(context.Background(), newTrafficRouteIS(), &apipb.Condition{})

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

func TestTrafficRouteProvisionActor_GetType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	actor := NewTrafficRouteProvisionActor(
		newClientFactoryDispatchingDynamic(ctrl, nil),
		routingmocks.NewMockManager(ctrl),
		"test-gateway",
		zap.NewNop(),
	)
	assert.Equal(t, common.TrafficRouteProvisionType, actor.GetType())
}
