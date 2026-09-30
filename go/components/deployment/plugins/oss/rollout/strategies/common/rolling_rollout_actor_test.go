package common

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	goapi "github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/api/apimocks"
	"github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/components/common/routing/routingmocks"
	osscommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends/backendsmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory/clientfactorymocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig/modelconfigmocks"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	testCluster        = "c1"
	testDeploymentName = "test-deployment"
	testNamespace      = "default"
	testISName         = "test-server"
	testModelName      = "model-v1"
	// Deliberately not the legacy s3://deploy-models/{name}/ layout, so the tests fail
	// if the actor ever falls back to deriving the path from the model name.
	testModelStoragePath = "s3://custom-bucket/artifacts/model-v1/"
)

// testNow is the fixed clock every actor under test reads. Timeouts are exercised by
// writing an older start time into the condition, never by sleeping.
var testNow = time.Unix(1_700_000_000, 0)

// testSettings are the rollout knobs used unless a test overrides them.
var testSettings = osscommon.RolloutSettings{
	Canary:           true,
	ModelLoadTimeout: 10 * time.Minute,
	RollbackTimeout:  5 * time.Minute,
}

type clientErrors struct {
	getClient        error
	getHTTPClient    error
	getDynamicClient error
}

// rolloutMocks groups every mock used by the per-cluster actor tests so per-test setup
// callbacks can program them in one place.
type rolloutMocks struct {
	factory             *clientfactorymocks.MockClientFactory
	backend             *backendsmocks.MockBackend
	modelConfigProvider *modelconfigmocks.MockModelConfigProvider
	routeManager        *routingmocks.MockManager
	backendRegistry     *backends.Registry
	// controlPlane serves the Model the actor resolves the storage path from.
	controlPlane goapi.Handler
}

// deps wires the mocks into the dependency bundle the actors take.
func (m *rolloutMocks) deps(settings osscommon.RolloutSettings) ClusterActorDeps {
	return ClusterActorDeps{
		ClientFactory:       m.factory,
		APIHandler:          m.controlPlane,
		BackendRegistry:     m.backendRegistry,
		ModelConfigProvider: m.modelConfigProvider,
		RouteManager:        m.routeManager,
		Logger:              zap.NewNop(),
		Settings:            settings,
		Now:                 func() time.Time { return testNow },
	}
}

// expectModelStatus programs one GetModelStatus probe for the model.
func (m *rolloutMocks) expectModelStatus(model string, status *backends.ModelStatus, err error) {
	m.backend.EXPECT().GetModelStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		testISName, testNamespace, model).Return(status, err)
}

// newControlPlaneClient builds a control-plane API handler seeded with the supplied objects.
func newControlPlaneClient(t *testing.T, objects ...client.Object) goapi.Handler {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))
	return handler.NewFakeAPIHandler(fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build())
}

// newUnreachableControlPlane builds a control-plane handler whose reads fail outright,
// standing in for metadata storage being unavailable rather than the Model being absent.
func newUnreachableControlPlane(t *testing.T) goapi.Handler {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)
	apiHandler := apimocks.NewMockHandler(ctrl)
	apiHandler.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(status.Error(codes.Unavailable, "metadata storage unreachable"))
	return apiHandler
}

// testModel is the Triton-packaged Model CR the canonical test Deployment points at.
func testModel() *v2pb.Model {
	return &v2pb.Model{
		ObjectMeta: metav1.ObjectMeta{Name: testModelName, Namespace: testNamespace},
		Spec: v2pb.ModelSpec{
			PackageType:           v2pb.DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON,
			DeployableArtifactUri: []string{testModelStoragePath},
		},
	}
}

// newRolloutFixture builds a target wired to the supplied mocks. clientErrs lets a test
// inject client factory failures without re-mocking the factory each time; when all are nil
// the factory returns nil clients without error.
//
// registerBackend controls whether the BackendRegistry has a backend registered for Triton;
// when false, GetBackend returns an error so the actor's BackendUnavailable branch fires.
func newRolloutFixture(t *testing.T, clientErrs clientErrors, registerBackend bool) (*rolloutMocks, *v2pb.ClusterTarget) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mocks := &rolloutMocks{
		factory:             clientfactorymocks.NewMockClientFactory(ctrl),
		backend:             backendsmocks.NewMockBackend(ctrl),
		modelConfigProvider: modelconfigmocks.NewMockModelConfigProvider(ctrl),
		routeManager:        routingmocks.NewMockManager(ctrl),
		controlPlane:        newControlPlaneClient(t, testModel()),
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

// conditionWithProgress returns a condition carrying the given rollout progress, or an empty
// condition when progress is nil.
func conditionWithProgress(t *testing.T, progress *osscommon.RolloutProgress) *apipb.Condition {
	t.Helper()
	condition := &apipb.Condition{}
	if progress != nil {
		require.NoError(t, osscommon.WriteRolloutProgress(condition, *progress))
	}
	return condition
}

// startedAgo is a progress record whose clock started the given duration before testNow.
func startedAgo(ago time.Duration, replica string) *osscommon.RolloutProgress {
	return &osscommon.RolloutProgress{StartedAt: testNow.Add(-ago).Unix(), Replica: replica}
}

// terminalCondition is what Retrieve hands Run after a terminal failure.
func terminalCondition(reason string) *apipb.Condition {
	return &apipb.Condition{Status: apipb.CONDITION_STATUS_FALSE, Message: reason, Reason: "details"}
}

func readyReplica(name string) backends.ReplicaModelStatus {
	return backends.ReplicaModelStatus{Replica: name, Running: true, State: backends.ModelLoadStateReady}
}

func loadingReplica(name string) backends.ReplicaModelStatus {
	return backends.ReplicaModelStatus{Replica: name, Running: true, State: backends.ModelLoadStateLoading, Reason: "model not yet loaded"}
}

func failedReplica(name, reason string) backends.ReplicaModelStatus {
	return backends.ReplicaModelStatus{Replica: name, Running: true, State: backends.ModelLoadStateFailed, Reason: reason}
}

func pendingReplica(name string) backends.ReplicaModelStatus {
	return backends.ReplicaModelStatus{Replica: name, Running: false, State: backends.ModelLoadStateLoading, Reason: "pod is Pending"}
}

func statusOf(desired int32, replicas ...backends.ReplicaModelStatus) *backends.ModelStatus {
	return &backends.ModelStatus{Desired: desired, Replicas: replicas}
}

func TestRollingRolloutActor_Retrieve(t *testing.T) {
	tests := []struct {
		name              string
		clientErrs        clientErrors
		registerBackend   bool
		progress          *osscommon.RolloutProgress
		setupMocks        func(*rolloutMocks)
		expectedStatus    apipb.ConditionStatus
		expectedMessage   string
		expectedReasonSub string
		expectDone        bool
	}{
		{
			name:            "short-circuit once the load is recorded as done",
			registerBackend: true,
			progress:        &osscommon.RolloutProgress{StartedAt: testNow.Unix(), Done: true},
			setupMocks:      func(*rolloutMocks) {}, // no calls expected
			expectedStatus:  apipb.CONDITION_STATUS_TRUE,
			expectDone:      true,
		},
		{
			name:              "not started yet means no probe",
			registerBackend:   true,
			setupMocks:        func(*rolloutMocks) {}, // no calls expected
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   ReasonModelLoadNotStarted,
			expectedReasonSub: "load of model model-v1 not started in cluster c1",
		},
		{
			name:              "GetClient errors",
			clientErrs:        clientErrors{getClient: errors.New("auth refused")},
			registerBackend:   true,
			progress:          startedAgo(time.Minute, ""),
			setupMocks:        func(*rolloutMocks) {},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   osscommon.ReasonClientUnavailable,
			expectedReasonSub: "auth refused",
		},
		{
			name:              "GetHTTPClient errors",
			clientErrs:        clientErrors{getHTTPClient: errors.New("dial timeout")},
			registerBackend:   true,
			progress:          startedAgo(time.Minute, ""),
			setupMocks:        func(*rolloutMocks) {},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   osscommon.ReasonHTTPClientUnavailable,
			expectedReasonSub: "dial timeout",
		},
		{
			name:              "backend not in registry",
			registerBackend:   false,
			progress:          startedAgo(time.Minute, ""),
			setupMocks:        func(*rolloutMocks) {},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   osscommon.ReasonBackendUnavailable,
			expectedReasonSub: "backend not found",
		},
		{
			name:            "GetModelStatus errors",
			registerBackend: true,
			progress:        startedAgo(time.Minute, ""),
			setupMocks: func(m *rolloutMocks) {
				m.expectModelStatus(testModelName, nil, errors.New("api error"))
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   osscommon.ReasonModelStatusCheckFailed,
			expectedReasonSub: "api error",
		},
		{
			name:            "one replica still loading keeps waiting",
			registerBackend: true,
			progress:        startedAgo(time.Minute, ""),
			setupMocks: func(m *rolloutMocks) {
				m.expectModelStatus(testModelName, statusOf(2, readyReplica("pod-a"), loadingReplica("pod-b")), nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   ReasonModelNotReady,
			expectedReasonSub: "model model-v1 not yet loaded in cluster c1",
		},
		{
			name:            "fewer replicas than desired keeps waiting",
			registerBackend: true,
			progress:        startedAgo(time.Minute, ""),
			setupMocks: func(m *rolloutMocks) {
				m.expectModelStatus(testModelName, statusOf(2, readyReplica("pod-a")), nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   ReasonModelNotReady,
			expectedReasonSub: "1/2",
		},
		{
			name:            "a failed replica is terminal",
			registerBackend: true,
			progress:        startedAgo(time.Minute, ""),
			setupMocks: func(m *rolloutMocks) {
				m.expectModelStatus(testModelName, statusOf(2, readyReplica("pod-a"), failedReplica("pod-b", "bad config.pbtxt")), nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   ReasonModelLoadFailed,
			expectedReasonSub: "pod-b: bad config.pbtxt",
		},
		{
			name:            "load budget exhausted is terminal",
			registerBackend: true,
			progress:        startedAgo(testSettings.ModelLoadTimeout+time.Second, ""),
			setupMocks: func(m *rolloutMocks) {
				m.expectModelStatus(testModelName, statusOf(2, readyReplica("pod-a"), loadingReplica("pod-b")), nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   ReasonModelLoadTimeout,
			expectedReasonSub: "within 10m0s",
		},
		{
			name:            "all replicas ready",
			registerBackend: true,
			progress:        startedAgo(time.Minute, ""),
			setupMocks: func(m *rolloutMocks) {
				m.expectModelStatus(testModelName, statusOf(2, readyReplica("pod-a"), readyReplica("pod-b")), nil)
			},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
			expectDone:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mocks, target := newRolloutFixture(t, tt.clientErrs, tt.registerBackend)
			tt.setupMocks(mocks)

			actor := NewRollingRolloutActor(mocks.deps(testSettings), target)
			got, err := actor.Retrieve(context.Background(), rolloutDeployment(""), conditionWithProgress(t, tt.progress))

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, got.Status)
			if tt.expectedMessage != "" {
				assert.Equal(t, tt.expectedMessage, got.Message)
			}
			if tt.expectedReasonSub != "" {
				assert.Contains(t, got.Reason, tt.expectedReasonSub)
			}
			progress, err := osscommon.ReadRolloutProgress(got)
			require.NoError(t, err)
			assert.Equal(t, tt.expectDone, progress.Done, "done flag recorded on the condition")
		})
	}
}

func TestRollingRolloutActor_Run(t *testing.T) {
	tests := []struct {
		name              string
		clientErrs        clientErrors
		condition         *apipb.Condition
		setupMocks        func(*rolloutMocks)
		controlPlane      goapi.Handler // when set, replaces the default model-seeded handler
		expectedStatus    apipb.ConditionStatus
		expectedMessage   string
		expectedReasonSub string
		expectedStartedAt int64 // when non-zero, asserted against the recorded progress
	}{
		{
			name:            "terminal failure from Retrieve passes through untouched",
			condition:       terminalCondition(ReasonModelLoadFailed),
			setupMocks:      func(*rolloutMocks) {}, // no calls expected
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: ReasonModelLoadFailed,
		},
		{
			name:              "GetClient errors",
			clientErrs:        clientErrors{getClient: errors.New("auth refused")},
			setupMocks:        func(*rolloutMocks) {},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "auth refused",
		},
		{
			name: "AddModelToConfig errors",
			setupMocks: func(m *rolloutMocks) {
				m.modelConfigProvider.EXPECT().AddModelToConfig(gomock.Any(), gomock.Any(), gomock.Any(),
					testISName, testNamespace, gomock.Any()).Return(errors.New("apply failed"))
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "apply failed",
		},
		{
			name:              "model CR missing",
			setupMocks:        func(*rolloutMocks) {}, // AddModelToConfig must not be reached
			controlPlane:      newControlPlaneClient(t),
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "model default/model-v1 not found",
		},
		{
			name:       "model CR is not Triton-packaged",
			setupMocks: func(*rolloutMocks) {},
			controlPlane: newControlPlaneClient(t, &v2pb.Model{
				ObjectMeta: metav1.ObjectMeta{Name: testModelName, Namespace: testNamespace},
				Spec: v2pb.ModelSpec{
					PackageType:           v2pb.DEPLOYABLE_MODEL_PACKAGE_TYPE_SPARK_PIPELINE,
					DeployableArtifactUri: []string{testModelStoragePath},
				},
			}),
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "want DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON",
		},
		{
			name:              "model read fails outright",
			setupMocks:        func(*rolloutMocks) {}, // AddModelToConfig must not be reached
			controlPlane:      newUnreachableControlPlane(t),
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "metadata storage unreachable",
		},
		{
			name: "happy path stages the model with the storage path from the Model CR",
			setupMocks: func(m *rolloutMocks) {
				m.modelConfigProvider.EXPECT().AddModelToConfig(gomock.Any(), gomock.Any(), gomock.Any(),
					testISName, testNamespace, gomock.Any()).
					DoAndReturn(func(_ context.Context, _ *zap.Logger, _ client.Client, _, _ string, entry modelconfig.ModelConfigEntry) error {
						assert.Equal(t, modelconfig.ModelConfigEntry{
							Name:           testModelName,
							StoragePath:    testModelStoragePath,
							DeploymentName: testDeploymentName,
							Phase:          modelconfig.ModelPhaseStaged,
						}, entry)
						return nil
					})
			},
			expectedStatus:    apipb.CONDITION_STATUS_UNKNOWN,
			expectedMessage:   ReasonModelLoading,
			expectedReasonSub: "model model-v1 loading in cluster c1",
			expectedStartedAt: testNow.Unix(),
		},
		{
			name:      "a later run keeps the original start time",
			condition: conditionWithProgress(t, startedAgo(5*time.Minute, "")),
			setupMocks: func(m *rolloutMocks) {
				m.modelConfigProvider.EXPECT().AddModelToConfig(gomock.Any(), gomock.Any(), gomock.Any(),
					testISName, testNamespace, gomock.Any()).Return(nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_UNKNOWN,
			expectedStartedAt: testNow.Add(-5 * time.Minute).Unix(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mocks, target := newRolloutFixture(t, tt.clientErrs, true)
			if tt.controlPlane != nil {
				mocks.controlPlane = tt.controlPlane
			}
			tt.setupMocks(mocks)
			condition := tt.condition
			if condition == nil {
				condition = &apipb.Condition{}
			}

			actor := NewRollingRolloutActor(mocks.deps(testSettings), target)
			got, err := actor.Run(context.Background(), rolloutDeployment(""), condition)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, got.Status)
			if tt.expectedMessage != "" {
				assert.Equal(t, tt.expectedMessage, got.Message)
			}
			if tt.expectedReasonSub != "" {
				assert.Contains(t, got.Reason, tt.expectedReasonSub)
			}
			if tt.expectedStartedAt != 0 {
				progress, err := osscommon.ReadRolloutProgress(got)
				require.NoError(t, err)
				assert.Equal(t, tt.expectedStartedAt, progress.StartedAt)
			}
		})
	}
}

func TestRollingRolloutActor_GetType(t *testing.T) {
	mocks, target := newRolloutFixture(t, clientErrors{}, true)
	actor := NewRollingRolloutActor(mocks.deps(testSettings), target)
	assert.Equal(t, "RollingRolloutComplete-"+testCluster, actor.GetType())
}
