package rollback

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	goapi "github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/components/common/routing"
	"github.com/michelangelo-ai/michelangelo/go/components/common/routing/routingmocks"
	osscommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends/backendsmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory/clientfactorymocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/common/routenames"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig/modelconfigmocks"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	testCluster         = "c1"
	testDeploymentName  = "test-deployment"
	testNamespace       = "default"
	testISName          = "test-server"
	testCandidate       = "failed-model"
	testPrevious        = "model-v0"
	testPreviousStorage = "s3://custom-bucket/artifacts/model-v0/"
)

var (
	testNow      = time.Unix(1_700_000_000, 0)
	testSettings = osscommon.RolloutSettings{ModelLoadTimeout: 10 * time.Minute, RollbackTimeout: 5 * time.Minute}
)

type clientErrors struct {
	getClient        error
	getHTTPClient    error
	getDynamicClient error
}

type rollbackMocks struct {
	factory             *clientfactorymocks.MockClientFactory
	backend             *backendsmocks.MockBackend
	modelConfigProvider *modelconfigmocks.MockModelConfigProvider
	routeManager        *routingmocks.MockManager
	backendRegistry     *backends.Registry
	controlPlane        goapi.Handler
}

func (m *rollbackMocks) params() Params {
	return Params{
		ClientFactory:       m.factory,
		APIHandler:          m.controlPlane,
		BackendRegistry:     m.backendRegistry,
		ModelConfigProvider: m.modelConfigProvider,
		RouteManager:        m.routeManager,
		Logger:              zap.NewNop(),
		Settings:            testSettings,
		Now:                 func() time.Time { return testNow },
	}
}

func (m *rollbackMocks) expectModelStatus(model string, status *backends.ModelStatus, err error) {
	m.backend.EXPECT().GetModelStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		testISName, testNamespace, model).Return(status, err)
}

func (m *rollbackMocks) expectEntries(entries ...modelconfig.ModelConfigEntry) {
	m.modelConfigProvider.EXPECT().GetModelsFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), testISName, testNamespace).
		Return(entries, nil)
}

func newControlPlaneClient(t *testing.T, objects ...client.Object) goapi.Handler {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))
	return handler.NewFakeAPIHandler(fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build())
}

func previousModel() *v2pb.Model {
	return &v2pb.Model{
		ObjectMeta: metav1.ObjectMeta{Name: testPrevious, Namespace: testNamespace},
		Spec: v2pb.ModelSpec{
			PackageType:           v2pb.DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON,
			DeployableArtifactUri: []string{testPreviousStorage},
		},
	}
}

func newRollbackFixture(t *testing.T, clientErrs clientErrors) (*rollbackMocks, *v2pb.ClusterTarget) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mocks := &rollbackMocks{
		factory:             clientfactorymocks.NewMockClientFactory(ctrl),
		backend:             backendsmocks.NewMockBackend(ctrl),
		modelConfigProvider: modelconfigmocks.NewMockModelConfigProvider(ctrl),
		routeManager:        routingmocks.NewMockManager(ctrl),
		backendRegistry:     backends.NewRegistry(),
		controlPlane:        newControlPlaneClient(t, previousModel()),
	}
	mocks.factory.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(client.Client(nil), clientErrs.getClient).AnyTimes()
	mocks.factory.EXPECT().GetHTTPClient(gomock.Any(), gomock.Any()).Return((*http.Client)(nil), clientErrs.getHTTPClient).AnyTimes()
	mocks.factory.EXPECT().GetDynamicClient(gomock.Any(), gomock.Any()).Return(dynamic.Interface(nil), clientErrs.getDynamicClient).AnyTimes()
	mocks.backendRegistry.Register(v2pb.BACKEND_TYPE_TRITON, mocks.backend)

	return mocks, &v2pb.ClusterTarget{ClusterId: testCluster}
}

// rollbackDeployment builds a Deployment whose candidate failed. current="" models a failed
// first rollout, where there is nothing to fall back to.
func rollbackDeployment(current string) *v2pb.Deployment {
	dep := &v2pb.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: testDeploymentName, Namespace: testNamespace},
		Spec: v2pb.DeploymentSpec{
			DesiredRevision: &apipb.ResourceIdentifier{Name: testCandidate},
			Target: &v2pb.DeploymentSpec_InferenceServer{
				InferenceServer: &apipb.ResourceIdentifier{Name: testISName},
			},
		},
		Status: v2pb.DeploymentStatus{
			CandidateRevision: &apipb.ResourceIdentifier{Name: testCandidate},
		},
	}
	if current != "" {
		dep.Status.CurrentRevision = &apipb.ResourceIdentifier{Name: current}
	}
	return dep
}

func conditionWithProgress(t *testing.T, progress *osscommon.RolloutProgress) *apipb.Condition {
	t.Helper()
	condition := &apipb.Condition{}
	if progress != nil {
		require.NoError(t, osscommon.WriteRolloutProgress(condition, *progress))
	}
	return condition
}

func startedAgo(ago time.Duration) *osscommon.RolloutProgress {
	return &osscommon.RolloutProgress{StartedAt: testNow.Add(-ago).Unix()}
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

func statusOf(desired int32, replicas ...backends.ReplicaModelStatus) *backends.ModelStatus {
	return &backends.ModelStatus{Desired: desired, Replicas: replicas}
}

func candidateEntry() modelconfig.ModelConfigEntry {
	return modelconfig.ModelConfigEntry{Name: testCandidate, StoragePath: "s3://custom-bucket/artifacts/failed-model/", DeploymentName: testDeploymentName, Phase: modelconfig.ModelPhaseServing}
}

func previousEntry(phase modelconfig.ModelPhase) modelconfig.ModelConfigEntry {
	return modelconfig.ModelConfigEntry{Name: testPrevious, StoragePath: testPreviousStorage, DeploymentName: testDeploymentName, Phase: phase}
}

func TestClusterRollbackActor_Retrieve(t *testing.T) {
	trafficRoute := routenames.TrafficRouteName(testISName)
	matchPath := routenames.TrafficMatchPath(testISName, testDeploymentName)
	previousRule := routing.Rule{MatchPath: matchPath, RewritePath: routenames.TrafficRewritePath(testPrevious)}

	tests := []struct {
		name              string
		deployment        *v2pb.Deployment
		clientErrs        clientErrors
		setupMocks        func(*rollbackMocks)
		expectedStatus    apipb.ConditionStatus
		expectedMessage   string
		expectedReasonSub string
	}{
		{
			name: "no candidate means nothing to roll back",
			deployment: func() *v2pb.Deployment {
				dep := rollbackDeployment(testPrevious)
				dep.Status.CandidateRevision = nil
				return dep
			}(),
			setupMocks:     func(*rollbackMocks) {},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
		},
		{
			name:           "candidate equal to current means nothing to roll back",
			deployment:     rollbackDeployment(testCandidate),
			setupMocks:     func(*rollbackMocks) {},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
		},
		{
			name:              "GetClient errors",
			deployment:        rollbackDeployment(testPrevious),
			clientErrs:        clientErrors{getClient: errors.New("auth refused")},
			setupMocks:        func(*rollbackMocks) {},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "auth refused",
		},
		{
			name:       "GetModelsFromConfig errors",
			deployment: rollbackDeployment(testPrevious),
			setupMocks: func(m *rollbackMocks) {
				m.modelConfigProvider.EXPECT().GetModelsFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), testISName, testNamespace).
					Return(nil, errors.New("api error"))
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   ReasonModelConfigReadFailed,
			expectedReasonSub: "api error",
		},
		{
			name:       "candidate still in the model config",
			deployment: rollbackDeployment(testPrevious),
			setupMocks: func(m *rollbackMocks) {
				m.expectEntries(previousEntry(modelconfig.ModelPhaseServing), candidateEntry())
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   ReasonCandidateStillInModelConfig,
			expectedReasonSub: "candidate model failed-model is still in the model config of cluster c1",
		},
		{
			name:       "no previous model and the traffic rule is still there",
			deployment: rollbackDeployment(""),
			setupMocks: func(m *rollbackMocks) {
				m.expectEntries()
				m.routeManager.EXPECT().RuleExists(gomock.Any(), gomock.Any(), trafficRoute, testNamespace, routing.Rule{MatchPath: matchPath}).Return(true, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: ReasonTrafficRouteStillPresent,
		},
		{
			name:       "no previous model and the traffic rule is gone",
			deployment: rollbackDeployment(""),
			setupMocks: func(m *rollbackMocks) {
				m.expectEntries()
				m.routeManager.EXPECT().RuleExists(gomock.Any(), gomock.Any(), trafficRoute, testNamespace, routing.Rule{MatchPath: matchPath}).Return(false, nil)
			},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
		},
		{
			name:       "previous model missing from the model config",
			deployment: rollbackDeployment(testPrevious),
			setupMocks: func(m *rollbackMocks) {
				m.expectEntries()
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   ReasonPreviousModelNotServing,
			expectedReasonSub: "previous model model-v0 is not in the serving phase in cluster c1",
		},
		{
			name:       "previous model only staged",
			deployment: rollbackDeployment(testPrevious),
			setupMocks: func(m *rollbackMocks) {
				m.expectEntries(previousEntry(modelconfig.ModelPhaseStaged))
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: ReasonPreviousModelNotServing,
		},
		{
			name:       "traffic rule not pointing at the previous model",
			deployment: rollbackDeployment(testPrevious),
			setupMocks: func(m *rollbackMocks) {
				m.expectEntries(previousEntry(modelconfig.ModelPhaseServing))
				m.routeManager.EXPECT().RuleExists(gomock.Any(), gomock.Any(), trafficRoute, testNamespace, previousRule).Return(false, nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   ReasonTrafficRouteNotRestored,
			expectedReasonSub: "does not point at previous model model-v0",
		},
		{
			name:       "rolled back",
			deployment: rollbackDeployment(testPrevious),
			setupMocks: func(m *rollbackMocks) {
				m.expectEntries(previousEntry(modelconfig.ModelPhaseServing))
				m.routeManager.EXPECT().RuleExists(gomock.Any(), gomock.Any(), trafficRoute, testNamespace, previousRule).Return(true, nil)
			},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mocks, target := newRollbackFixture(t, tt.clientErrs)
			tt.setupMocks(mocks)

			actor := NewClusterRollbackActor(mocks.params(), target)
			got, err := actor.Retrieve(context.Background(), tt.deployment, &apipb.Condition{})

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

func TestClusterRollbackActor_Run(t *testing.T) {
	trafficRoute := routenames.TrafficRouteName(testISName)
	matchPath := routenames.TrafficMatchPath(testISName, testDeploymentName)
	previousRule := routing.Rule{
		MatchPath:   matchPath,
		MatchType:   routing.PathMatchPrefix,
		RewritePath: routenames.TrafficRewritePath(testPrevious),
		RewriteType: routing.RewritePrefix,
		BackendName: testISName + "-inference-service",
	}
	expectRestoreRoute := func(m *rollbackMocks) {
		m.routeManager.EXPECT().AddRules(gomock.Any(), gomock.Any(), trafficRoute, testNamespace, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ dynamic.Interface, _, _ string, rules ...routing.Rule) error {
				assert.Equal(t, []routing.Rule{previousRule}, rules)
				return nil
			})
	}
	expectRemoveCandidate := func(m *rollbackMocks, err error) {
		m.modelConfigProvider.EXPECT().RemoveModelFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), testISName, testNamespace, testDeploymentName, testCandidate).Return(err)
	}
	expectRestoreEntry := func(m *rollbackMocks) {
		m.modelConfigProvider.EXPECT().AddModelToConfig(gomock.Any(), gomock.Any(), gomock.Any(), testISName, testNamespace, previousEntry(modelconfig.ModelPhaseServing)).Return(nil)
	}

	tests := []struct {
		name              string
		deployment        *v2pb.Deployment
		condition         *apipb.Condition
		setupMocks        func(*rollbackMocks)
		expectedStatus    apipb.ConditionStatus
		expectedMessage   string
		expectedReasonSub string
		expectStarted     bool
	}{
		{
			name:            "terminal failure passes through untouched",
			deployment:      rollbackDeployment(testPrevious),
			condition:       &apipb.Condition{Status: apipb.CONDITION_STATUS_FALSE, Message: ReasonRollbackTimeout},
			setupMocks:      func(*rollbackMocks) {},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: ReasonRollbackTimeout,
		},
		{
			name:           "candidate equal to current is a no-op",
			deployment:     rollbackDeployment(testCandidate),
			setupMocks:     func(*rollbackMocks) {},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
		},
		{
			name:       "no previous model removes the route and the candidate",
			deployment: rollbackDeployment(""),
			setupMocks: func(m *rollbackMocks) {
				m.routeManager.EXPECT().RemoveRules(gomock.Any(), gomock.Any(), trafficRoute, testNamespace, matchPath).Return(nil)
				expectRemoveCandidate(m, nil)
			},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
			expectStarted:  true,
		},
		{
			name:       "previous model still serving: restore the route then drop the candidate",
			deployment: rollbackDeployment(testPrevious),
			setupMocks: func(m *rollbackMocks) {
				m.expectEntries(previousEntry(modelconfig.ModelPhaseServing), candidateEntry())
				m.expectModelStatus(testPrevious, statusOf(2, readyReplica("pod-a"), readyReplica("pod-b")), nil)
				expectRestoreRoute(m)
				expectRemoveCandidate(m, nil)
			},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
			expectStarted:  true,
		},
		{
			name:       "previous model entry was already removed: recreate it from the Model CR",
			deployment: rollbackDeployment(testPrevious),
			setupMocks: func(m *rollbackMocks) {
				m.expectEntries(candidateEntry())
				expectRestoreEntry(m)
				m.expectModelStatus(testPrevious, statusOf(1, readyReplica("pod-a")), nil)
				expectRestoreRoute(m)
				expectRemoveCandidate(m, nil)
			},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
		},
		{
			name:       "previous model only staged is promoted back to serving",
			deployment: rollbackDeployment(testPrevious),
			setupMocks: func(m *rollbackMocks) {
				m.expectEntries(previousEntry(modelconfig.ModelPhaseStaged), candidateEntry())
				expectRestoreEntry(m)
				m.expectModelStatus(testPrevious, statusOf(1, readyReplica("pod-a")), nil)
				expectRestoreRoute(m)
				expectRemoveCandidate(m, nil)
			},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
		},
		{
			name:       "waits while the previous model reloads; no route or config change",
			deployment: rollbackDeployment(testPrevious),
			setupMocks: func(m *rollbackMocks) {
				m.expectEntries(previousEntry(modelconfig.ModelPhaseServing), candidateEntry())
				m.expectModelStatus(testPrevious, statusOf(2, readyReplica("pod-a"), loadingReplica("pod-b")), nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_UNKNOWN,
			expectedMessage:   ReasonRestoringPreviousModel,
			expectedReasonSub: "waiting for previous model model-v0 on every replica in cluster c1",
			expectStarted:     true,
		},
		{
			name:       "previous model failed to reload is terminal",
			deployment: rollbackDeployment(testPrevious),
			setupMocks: func(m *rollbackMocks) {
				m.expectEntries(previousEntry(modelconfig.ModelPhaseServing), candidateEntry())
				m.expectModelStatus(testPrevious, statusOf(1, failedReplica("pod-a", "artifact missing")), nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   ReasonPreviousModelLoadFailed,
			expectedReasonSub: "pod-a: artifact missing",
		},
		{
			name:       "rollback budget exhausted is terminal",
			deployment: rollbackDeployment(testPrevious),
			condition:  conditionWithProgress(t, startedAgo(testSettings.RollbackTimeout+time.Second)),
			setupMocks: func(m *rollbackMocks) {
				m.expectEntries(previousEntry(modelconfig.ModelPhaseServing), candidateEntry())
				m.expectModelStatus(testPrevious, statusOf(2, readyReplica("pod-a"), loadingReplica("pod-b")), nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   ReasonRollbackTimeout,
			expectedReasonSub: "within 5m0s",
		},
		{
			name:       "RemoveModelFromConfig errors",
			deployment: rollbackDeployment(testPrevious),
			setupMocks: func(m *rollbackMocks) {
				m.expectEntries(previousEntry(modelconfig.ModelPhaseServing), candidateEntry())
				m.expectModelStatus(testPrevious, statusOf(1, readyReplica("pod-a")), nil)
				expectRestoreRoute(m)
				expectRemoveCandidate(m, errors.New("removal failed"))
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "removal failed",
		},
		{
			name:       "AddRules errors",
			deployment: rollbackDeployment(testPrevious),
			setupMocks: func(m *rollbackMocks) {
				m.expectEntries(previousEntry(modelconfig.ModelPhaseServing), candidateEntry())
				m.expectModelStatus(testPrevious, statusOf(1, readyReplica("pod-a")), nil)
				m.routeManager.EXPECT().AddRules(gomock.Any(), gomock.Any(), trafficRoute, testNamespace, gomock.Any()).Return(errors.New("update failed"))
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "update failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mocks, target := newRollbackFixture(t, clientErrors{})
			tt.setupMocks(mocks)
			condition := tt.condition
			if condition == nil {
				condition = &apipb.Condition{}
			}

			actor := NewClusterRollbackActor(mocks.params(), target)
			got, err := actor.Run(context.Background(), tt.deployment, condition)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, got.Status)
			if tt.expectedMessage != "" {
				assert.Equal(t, tt.expectedMessage, got.Message)
			}
			if tt.expectedReasonSub != "" {
				assert.Contains(t, got.Reason, tt.expectedReasonSub)
			}
			if tt.expectStarted {
				progress, err := osscommon.ReadRolloutProgress(got)
				require.NoError(t, err)
				assert.Equal(t, testNow.Unix(), progress.StartedAt)
			}
		})
	}
}

func TestClusterRollbackActor_PreviousModelUnresolvable(t *testing.T) {
	mocks, target := newRollbackFixture(t, clientErrors{})
	mocks.controlPlane = newControlPlaneClient(t) // the previous Model CR is gone
	mocks.expectEntries(candidateEntry())

	actor := NewClusterRollbackActor(mocks.params(), target)
	got, err := actor.Run(context.Background(), rollbackDeployment(testPrevious), &apipb.Condition{})

	require.NoError(t, err)
	assert.Equal(t, apipb.CONDITION_STATUS_FALSE, got.Status)
	assert.Contains(t, got.Reason, "model default/model-v0 not found")
}

func TestClusterRollbackActor_GetType(t *testing.T) {
	mocks, target := newRollbackFixture(t, clientErrors{})
	actor := NewClusterRollbackActor(mocks.params(), target)
	assert.Equal(t, "RollbackComplete-"+testCluster, actor.GetType())
}

func TestRollbackCompletionActor(t *testing.T) {
	discoveryRoute := routenames.DiscoveryRouteName(testISName)
	discoveryMatch := routenames.DiscoveryMatchPath(testISName, testDeploymentName)

	t.Run("previous model keeps the discovery route", func(t *testing.T) {
		mocks, _ := newRollbackFixture(t, clientErrors{})
		actor := NewRollbackCompletionActor(mocks.params())

		got, err := actor.Retrieve(context.Background(), rollbackDeployment(testPrevious), &apipb.Condition{})
		require.NoError(t, err)
		assert.Equal(t, apipb.CONDITION_STATUS_TRUE, got.Status)

		got, err = actor.Run(context.Background(), rollbackDeployment(testPrevious), &apipb.Condition{})
		require.NoError(t, err)
		assert.Equal(t, apipb.CONDITION_STATUS_TRUE, got.Status)
	})

	t.Run("no previous model removes the discovery route", func(t *testing.T) {
		mocks, _ := newRollbackFixture(t, clientErrors{})
		mocks.routeManager.EXPECT().RuleExists(gomock.Any(), gomock.Any(), discoveryRoute, testNamespace, routing.Rule{MatchPath: discoveryMatch}).Return(true, nil)
		mocks.routeManager.EXPECT().RemoveRules(gomock.Any(), gomock.Any(), discoveryRoute, testNamespace, discoveryMatch).Return(nil)
		actor := NewRollbackCompletionActor(mocks.params())

		got, err := actor.Retrieve(context.Background(), rollbackDeployment(""), &apipb.Condition{})
		require.NoError(t, err)
		assert.Equal(t, apipb.CONDITION_STATUS_FALSE, got.Status)
		assert.Equal(t, ReasonDiscoveryRouteStillPresent, got.Message)

		got, err = actor.Run(context.Background(), rollbackDeployment(""), &apipb.Condition{})
		require.NoError(t, err)
		assert.Equal(t, apipb.CONDITION_STATUS_TRUE, got.Status)
	})

	t.Run("no previous model and no discovery route is complete", func(t *testing.T) {
		mocks, _ := newRollbackFixture(t, clientErrors{})
		mocks.routeManager.EXPECT().RuleExists(gomock.Any(), gomock.Any(), discoveryRoute, testNamespace, routing.Rule{MatchPath: discoveryMatch}).Return(false, nil)
		actor := NewRollbackCompletionActor(mocks.params())

		got, err := actor.Retrieve(context.Background(), rollbackDeployment(""), &apipb.Condition{})
		require.NoError(t, err)
		assert.Equal(t, apipb.CONDITION_STATUS_TRUE, got.Status)
	})

	t.Run("RemoveRules errors", func(t *testing.T) {
		mocks, _ := newRollbackFixture(t, clientErrors{})
		mocks.routeManager.EXPECT().RemoveRules(gomock.Any(), gomock.Any(), discoveryRoute, testNamespace, discoveryMatch).Return(errors.New("update failed"))
		actor := NewRollbackCompletionActor(mocks.params())

		got, err := actor.Run(context.Background(), rollbackDeployment(""), &apipb.Condition{})
		require.NoError(t, err)
		assert.Equal(t, apipb.CONDITION_STATUS_FALSE, got.Status)
		assert.Contains(t, got.Reason, "update failed")
	})

	t.Run("type is the terminal rollback marker", func(t *testing.T) {
		mocks, _ := newRollbackFixture(t, clientErrors{})
		assert.Equal(t, osscommon.ActorTypeRollback, NewRollbackCompletionActor(mocks.params()).GetType())
	})
}

func TestNewRollbackPlugin(t *testing.T) {
	mocks, _ := newRollbackFixture(t, clientErrors{})

	t.Run("one actor per placed cluster followed by the completion actor", func(t *testing.T) {
		deployment := rollbackDeployment(testPrevious)
		require.NoError(t, osscommon.WriteTargetClustersAnnotation(deployment, []*v2pb.ClusterTarget{{ClusterId: "c1"}, {ClusterId: "c2"}}))

		plugin, err := NewRollbackPlugin(context.Background(), mocks.params(), deployment)
		require.NoError(t, err)

		types := make([]string, 0)
		for _, actor := range plugin.GetActors() {
			types = append(types, actor.GetType())
		}
		assert.Equal(t, []string{"RollbackComplete-c1", "RollbackComplete-c2", "RollbackComplete"}, types)
	})

	t.Run("no placement yet leaves only the completion actor", func(t *testing.T) {
		plugin, err := NewRollbackPlugin(context.Background(), mocks.params(), rollbackDeployment(testPrevious))
		require.NoError(t, err)
		require.Len(t, plugin.GetActors(), 1)
		assert.Equal(t, osscommon.ActorTypeRollback, plugin.GetActors()[0].GetType())
	})

	t.Run("unreadable placement is an error", func(t *testing.T) {
		deployment := rollbackDeployment(testPrevious)
		deployment.Annotations = map[string]string{osscommon.TargetClustersAnnotation: "{not json}"}

		_, err := NewRollbackPlugin(context.Background(), mocks.params(), deployment)
		require.Error(t, err)
	})
}
