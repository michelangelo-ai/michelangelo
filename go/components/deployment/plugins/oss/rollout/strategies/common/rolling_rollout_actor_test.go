package common

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	osscommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends/backendsmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory/clientfactorymocks"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	testCluster        = "c1"
	testDeploymentName = "test-deployment"
	testNamespace      = "default"
	testISName         = "test-server"
	testModelName      = "model-v1"
)

type clientErrors struct {
	getClient        error
	getHTTPClient    error
	getDynamicClient error
}

// rolloutMocks groups every mock used by rolling-rollout and model-cleanup tests.
type rolloutMocks struct {
	factory         *clientfactorymocks.MockClientFactory
	backend         *backendsmocks.MockBackend
	backendRegistry *backends.Registry
}

// newRolloutFixture builds a target wired to the supplied mocks.
// registerBackend controls whether BACKEND_TYPE_TRITON is in the registry.
func newRolloutFixture(t *testing.T, clientErrs clientErrors, registerBackend bool) (*rolloutMocks, *v2pb.ClusterTarget) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mocks := &rolloutMocks{
		factory: clientfactorymocks.NewMockClientFactory(ctrl),
		backend: backendsmocks.NewMockBackend(ctrl),
	}

	mocks.factory.EXPECT().GetClient(gomock.Any(), gomock.Any()).
		Return(client.Client(nil), clientErrs.getClient).AnyTimes()
	mocks.factory.EXPECT().GetHTTPClient(gomock.Any(), gomock.Any()).
		Return((*http.Client)(nil), clientErrs.getHTTPClient).AnyTimes()
	mocks.factory.EXPECT().GetDynamicClient(gomock.Any(), gomock.Any()).
		Return(dynamic.Interface(nil), clientErrs.getDynamicClient).AnyTimes()

	mocks.backendRegistry = backends.NewRegistry()
	if registerBackend {
		mocks.backendRegistry.Register(v2pb.BACKEND_TYPE_TRITON, mocks.backend)
	}

	target := &v2pb.ClusterTarget{ClusterId: testCluster}
	return mocks, target
}

// rolloutDeployment builds a Deployment with the canonical IS reference + desired revision.
// pass currentRevision="" to leave Status.CurrentRevision nil (first rollout case).
func rolloutDeployment(currentRevision string) *v2pb.Deployment {
	dep := &v2pb.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: testDeploymentName, Namespace: testNamespace},
		Spec: v2pb.DeploymentSpec{
			DesiredRevision: &apipb.ResourceIdentifier{Name: testModelName},
			Target: &v2pb.DeploymentSpec_InferenceServer{
				InferenceServer: &apipb.ResourceIdentifier{Name: testISName},
			},
		},
	}
	if currentRevision != "" {
		dep.Status = v2pb.DeploymentStatus{
			CurrentRevision: &apipb.ResourceIdentifier{Name: currentRevision},
		}
	}
	return dep
}

func TestRollingRolloutActor_Retrieve(t *testing.T) {
	tests := []struct {
		name              string
		clientErrs        clientErrors
		registerBackend   bool
		setupMocks        func(*rolloutMocks)
		preWriteFlag      bool
		expectedStatus    apipb.ConditionStatus
		expectedReasonSub string
	}{
		{
			name:            "short-circuit via cached loaded flag",
			registerBackend: true,
			setupMocks:      func(*rolloutMocks) {},
			preWriteFlag:    true,
			expectedStatus:  apipb.CONDITION_STATUS_TRUE,
		},
		{
			name:              "GetClient errors",
			clientErrs:        clientErrors{getClient: errors.New("auth refused")},
			registerBackend:   true,
			setupMocks:        func(*rolloutMocks) {},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "auth refused",
		},
		{
			name:              "GetHTTPClient errors",
			clientErrs:        clientErrors{getHTTPClient: errors.New("dial timeout")},
			registerBackend:   true,
			setupMocks:        func(*rolloutMocks) {},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "dial timeout",
		},
		{
			name:              "backend not in registry",
			registerBackend:   false,
			setupMocks:        func(*rolloutMocks) {},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "backend not found",
		},
		{
			name:            "CheckModelStatus errors",
			registerBackend: true,
			setupMocks: func(m *rolloutMocks) {
				m.backend.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					testISName, testNamespace, testModelName).Return(false, errors.New("api error"))
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "api error",
		},
		{
			name:            "model not ready",
			registerBackend: true,
			setupMocks: func(m *rolloutMocks) {
				m.backend.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					testISName, testNamespace, testModelName).Return(false, nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "model model-v1 not yet loaded in cluster c1",
		},
		{
			name:            "model ready",
			registerBackend: true,
			setupMocks: func(m *rolloutMocks) {
				m.backend.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					testISName, testNamespace, testModelName).Return(true, nil)
			},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mocks, target := newRolloutFixture(t, tt.clientErrs, tt.registerBackend)
			tt.setupMocks(mocks)

			condition := &apipb.Condition{}
			if tt.preWriteFlag {
				require.NoError(t, osscommon.WriteModelLoadedFlag(condition))
			}

			actor := NewRollingRolloutActor(mocks.factory, mocks.backendRegistry, v2pb.BACKEND_TYPE_TRITON, zap.NewNop(), target)
			got, err := actor.Retrieve(context.Background(), rolloutDeployment(""), condition)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, got.Status)
			if tt.expectedReasonSub != "" {
				assert.Contains(t, got.Reason, tt.expectedReasonSub)
			}
			if got.Status == apipb.CONDITION_STATUS_TRUE && !tt.preWriteFlag {
				loaded, err := osscommon.ReadModelLoadedFlag(got)
				require.NoError(t, err)
				assert.True(t, loaded, "loaded flag should be set after model is ready")
			}
		})
	}
}

func TestRollingRolloutActor_Run(t *testing.T) {
	tests := []struct {
		name              string
		clientErrs        clientErrors
		setupMocks        func(*rolloutMocks)
		expectedStatus    apipb.ConditionStatus
		expectedReasonSub string
	}{
		{
			name:              "GetClient errors",
			clientErrs:        clientErrors{getClient: errors.New("auth refused")},
			setupMocks:        func(*rolloutMocks) {},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "auth refused",
		},
		{
			name:              "GetDynamicClient errors",
			clientErrs:        clientErrors{getDynamicClient: errors.New("no dynamic client")},
			setupMocks:        func(*rolloutMocks) {},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "no dynamic client",
		},
		{
			name: "LoadModel errors",
			setupMocks: func(m *rolloutMocks) {
				m.backend.EXPECT().LoadModel(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					testISName, testNamespace, testModelName, gomock.Any()).Return(errors.New("apply failed"))
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "apply failed",
		},
		{
			name: "happy path",
			setupMocks: func(m *rolloutMocks) {
				m.backend.EXPECT().LoadModel(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					testISName, testNamespace, testModelName, "s3://deploy-models/"+testModelName+"/").Return(nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_UNKNOWN,
			expectedReasonSub: "model model-v1 loading in cluster c1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mocks, target := newRolloutFixture(t, tt.clientErrs, true)
			tt.setupMocks(mocks)

			actor := NewRollingRolloutActor(mocks.factory, mocks.backendRegistry, v2pb.BACKEND_TYPE_TRITON, zap.NewNop(), target)
			got, err := actor.Run(context.Background(), rolloutDeployment(""), &apipb.Condition{})

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, got.Status)
			if tt.expectedReasonSub != "" {
				assert.Contains(t, got.Reason, tt.expectedReasonSub)
			}
		})
	}
}

func TestRollingRolloutActor_GetType(t *testing.T) {
	mocks, target := newRolloutFixture(t, clientErrors{}, true)
	actor := NewRollingRolloutActor(mocks.factory, mocks.backendRegistry, v2pb.BACKEND_TYPE_TRITON, zap.NewNop(), target)
	assert.Equal(t, "RollingRolloutComplete-"+testCluster, actor.GetType())
}
