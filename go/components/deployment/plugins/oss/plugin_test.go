package oss

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends/backendsmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory/clientfactorymocks"
	"github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// newKubeClient returns a fake control-plane client holding the given objects.
func newKubeClient(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
}

func createTestRegistry(mockBackend *backendsmocks.MockBackend) *backends.Registry {
	registry := backends.NewRegistry()
	registry.Register(v2pb.BACKEND_TYPE_TRITON, mockBackend)
	return registry
}

// readyOnAllReplicas is a model status where every desired replica serves the model.
func readyOnAllReplicas() *backends.ModelStatus {
	return &backends.ModelStatus{Desired: 2, Replicas: []backends.ReplicaModelStatus{
		{Replica: "pod-a", Running: true, State: backends.ModelLoadStateReady},
		{Replica: "pod-b", Running: true, State: backends.ModelLoadStateReady},
	}}
}

// loadingOnOneReplica is a model status where one desired replica still lacks the model.
func loadingOnOneReplica() *backends.ModelStatus {
	return &backends.ModelStatus{Desired: 2, Replicas: []backends.ReplicaModelStatus{
		{Replica: "pod-a", Running: true, State: backends.ModelLoadStateReady},
		{Replica: "pod-b", Running: true, State: backends.ModelLoadStateLoading, Reason: "model not yet loaded"},
	}}
}

// withSingleClusterAnnotation adds a target-clusters snapshot annotation to the deployment
// so GetState's CheckModelStatusAllClusters helper has a cluster to iterate.
func withSingleClusterAnnotation(t *testing.T, deployment *v2pb.Deployment, clusterID string) *v2pb.Deployment {
	t.Helper()
	target := &v2pb.ClusterTarget{
		ClusterId: clusterID,
		Connection: &v2pb.ClusterTarget_Kubernetes{
			Kubernetes: &v2pb.ConnectionSpec{
				Host: "https://kubernetes.default.svc",
				Port: "443",
			},
		},
	}
	if err := common.WriteTargetClustersAnnotation(deployment, []*v2pb.ClusterTarget{target}); err != nil {
		t.Fatalf("seed target-clusters annotation: %v", err)
	}
	return deployment
}

func TestParseStage(t *testing.T) {
	tests := []struct {
		name          string
		deployment    *v2pb.Deployment
		expectedStage v2pb.DeploymentStage
	}{
		{
			name: "no conditions, returns current stage",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_PLACEMENT,
					Conditions:        []*api.Condition{},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_PLACEMENT,
		},
		{
			name: "validated condition is true, returns current stage",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_VALIDATION,
					Conditions: []*api.Condition{
						{Type: common.ActorTypeValidation, Status: api.CONDITION_STATUS_TRUE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_VALIDATION,
		},
		{
			name: "validated condition is false, validation stage",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_PLACEMENT,
					Conditions: []*api.Condition{
						{Type: common.ActorTypeValidation, Status: api.CONDITION_STATUS_FALSE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_VALIDATION,
		},
		{
			name: "asset preparation condition is false, validation stage",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_PLACEMENT,
					Conditions: []*api.Condition{
						{Type: common.ActorTypeAssetPreparation, Status: api.CONDITION_STATUS_FALSE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_VALIDATION,
		},
		{
			name: "resource acquisition condition is false, resource acquisition stage",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_VALIDATION,
					Conditions: []*api.Condition{
						{Type: common.ActorTypePlacementPrep, Status: api.CONDITION_STATUS_FALSE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_PLACEMENT,
		},
		{
			name: "rollout complete condition true, rollout complete stage",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_PLACEMENT,
					Conditions: []*api.Condition{
						{Type: common.ActorTypeRolloutComplete, Status: api.CONDITION_STATUS_TRUE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
		},
		{
			name: "cleanup complete condition true, cleanup complete stage",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_CLEAN_UP_IN_PROGRESS,
					Conditions: []*api.Condition{
						{Type: common.ActorTypeCleanup, Status: api.CONDITION_STATUS_TRUE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE,
		},
		{
			name: "cleanup complete condition false, cleanup in progress stage",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					Conditions: []*api.Condition{
						{Type: common.ActorTypeCleanup, Status: api.CONDITION_STATUS_FALSE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_CLEAN_UP_IN_PROGRESS,
		},
		{
			name: "rollback complete condition true, rollback complete stage",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS,
					Conditions: []*api.Condition{
						{Type: common.ActorTypeRollback, Status: api.CONDITION_STATUS_TRUE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE,
		},
		{
			name: "rollback complete condition false, rollback in progress stage",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					Conditions: []*api.Condition{
						{Type: common.ActorTypeRollback, Status: api.CONDITION_STATUS_FALSE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS,
		},
		{
			name: "unknown condition with false status, falls through to placement",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_VALIDATION,
					Conditions: []*api.Condition{
						{Type: "SomeOtherCondition", Status: api.CONDITION_STATUS_FALSE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_PLACEMENT,
		},
		{
			name: "multiple conditions, rollout complete has priority when true",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_PLACEMENT,
					Conditions: []*api.Condition{
						{Type: common.ActorTypeRolloutComplete, Status: api.CONDITION_STATUS_TRUE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
		},
		{
			name: "desired and candidate both nil, no conditions",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{},
				Status: v2pb.DeploymentStatus{
					Stage:      v2pb.DEPLOYMENT_STAGE_INVALID,
					Conditions: []*api.Condition{},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_INVALID,
		},
		{
			name: "desired nil, candidate exists with rollout complete",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					Conditions: []*api.Condition{
						{Type: common.ActorTypeRolloutComplete, Status: api.CONDITION_STATUS_TRUE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
		},
		{
			name: "first false condition determines stage",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Stage:             v2pb.DEPLOYMENT_STAGE_VALIDATION,
					Conditions: []*api.Condition{
						{Type: common.ActorTypeValidation, Status: api.CONDITION_STATUS_TRUE},
						{Type: common.ActorTypePlacementPrep, Status: api.CONDITION_STATUS_FALSE},
						{Type: common.ActorTypeRolloutComplete, Status: api.CONDITION_STATUS_TRUE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_PLACEMENT,
		},
		{
			name: "per-cluster rollback still running",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v2"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v2"},
					Stage:             v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS,
					Conditions: []*api.Condition{
						{Type: common.ActorTypeRollback + "-c1", Status: api.CONDITION_STATUS_TRUE},
						{Type: common.ActorTypeRollback + "-c2", Status: api.CONDITION_STATUS_UNKNOWN},
						{Type: common.ActorTypeRollback, Status: api.CONDITION_STATUS_FALSE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS,
		},
		{
			name: "per-cluster rollback failed in the first cluster",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v2"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v2"},
					Stage:             v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS,
					Conditions: []*api.Condition{
						{Type: common.ActorTypeRollback + "-c1", Status: api.CONDITION_STATUS_FALSE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS,
		},
		{
			// Only the completion marker ends the rollback; a cluster finishing on its own
			// does not.
			name: "per-cluster rollback done but completion pending keeps the stage",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v2"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v2"},
					Stage:             v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS,
					Conditions: []*api.Condition{
						{Type: common.ActorTypeRollback + "-c1", Status: api.CONDITION_STATUS_TRUE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS,
		},
		{
			name: "rollback complete once every cluster and the completion marker are true",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v2"},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v2"},
					Stage:             v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS,
					Conditions: []*api.Condition{
						{Type: common.ActorTypeRollback + "-c1", Status: api.CONDITION_STATUS_TRUE},
						{Type: common.ActorTypeRollback, Status: api.CONDITION_STATUS_TRUE},
					},
				},
			},
			expectedStage: v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := &Plugin{}
			actualStage := plugin.ParseStage(tt.deployment)
			assert.Equal(t, tt.expectedStage, actualStage, "Stage mismatch for test case: %s", tt.name)
		})
	}
}

// withRenamedSecrets records a snapshot that names the remote cluster's v1 Secrets on the
// deployment, and returns its InferenceServer, which now names the v2 Secrets.
func withRenamedSecrets(t *testing.T, deployment *v2pb.Deployment) *v2pb.InferenceServer {
	t.Helper()
	remoteTarget := func(version string) *v2pb.ClusterTarget {
		return &v2pb.ClusterTarget{
			ClusterId: "remote",
			Connection: &v2pb.ClusterTarget_Kubernetes{Kubernetes: &v2pb.ConnectionSpec{
				Host:      "https://remote.example.com",
				Port:      "443",
				TokenTag:  "remote-token-" + version,
				CaDataTag: "remote-ca-" + version,
			}},
		}
	}
	require.NoError(t, common.WriteTargetClustersAnnotation(deployment,
		[]*v2pb.ClusterTarget{remoteTarget("v1")}))
	return &v2pb.InferenceServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deployment.Spec.GetInferenceServer().GetName(),
			Namespace: deployment.Namespace,
		},
		Spec: v2pb.InferenceServerSpec{ClusterTargets: []*v2pb.ClusterTarget{remoteTarget("v2")}},
	}
}

func TestGetState_SecretsRenamedSinceRollout_UsesInferenceServerConnection(t *testing.T) {
	deployment := &v2pb.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
		Spec: v2pb.DeploymentSpec{
			DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
			Target: &v2pb.DeploymentSpec_InferenceServer{
				InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
			},
		},
		Status: v2pb.DeploymentStatus{CurrentRevision: &api.ResourceIdentifier{Name: "model-v1"}},
	}
	inferenceServer := withRenamedSecrets(t, deployment)

	ctrl := gomock.NewController(t)
	mockBackend := backendsmocks.NewMockBackend(ctrl)
	mockBackend.EXPECT().GetModelStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default", "model-v1").
		Return(readyOnAllReplicas(), nil)
	mockClientFactory := clientfactorymocks.NewMockClientFactory(ctrl)
	mockClientFactory.EXPECT().GetClient(gomock.Any(), tokenTagMatcher("remote-token-v2")).Return(nil, nil)
	mockClientFactory.EXPECT().GetHTTPClient(gomock.Any(), tokenTagMatcher("remote-token-v2")).Return(nil, nil)

	plugin := &Plugin{
		client:          newKubeClient(t, inferenceServer),
		backendRegistry: createTestRegistry(mockBackend),
		clientFactory:   mockClientFactory,
		logger:          zap.NewNop(),
	}

	status, err := plugin.GetState(context.Background(), plugins.ObservabilityContext{}, deployment)

	require.NoError(t, err)
	assert.Equal(t, v2pb.DEPLOYMENT_STATE_HEALTHY, status.State)
}

// tokenTagMatcher matches a ClusterTarget argument by the name of its token Secret.
type tokenTagMatcher string

func (m tokenTagMatcher) Matches(x interface{}) bool {
	target, ok := x.(*v2pb.ClusterTarget)
	return ok && target.GetKubernetes().GetTokenTag() == string(m)
}

func (m tokenTagMatcher) String() string {
	return "cluster target with token Secret " + string(m)
}

func TestGetState(t *testing.T) {
	tests := []struct {
		name          string
		deployment    *v2pb.Deployment
		setupMocks    func(*backendsmocks.MockBackend)
		expectedState v2pb.DeploymentState
		expectError   bool
	}{
		{
			name: "returns initializing when current revision is nil",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: nil,
					Stage:           v2pb.DEPLOYMENT_STAGE_PLACEMENT,
				},
			},
			setupMocks:    func(mb *backendsmocks.MockBackend) {},
			expectedState: v2pb.DEPLOYMENT_STATE_INITIALIZING,
			expectError:   false,
		},
		{
			name: "returns empty when current revision is nil and stage is cleanup complete",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: nil,
					Stage:           v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE,
				},
			},
			setupMocks:    func(mb *backendsmocks.MockBackend) {},
			expectedState: v2pb.DEPLOYMENT_STATE_EMPTY,
			expectError:   false,
		},
		{
			name: "returns invalid when inference server is nil",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target:          nil,
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
			},
			setupMocks:    func(mb *backendsmocks.MockBackend) {},
			expectedState: v2pb.DEPLOYMENT_STATE_INVALID,
			expectError:   false,
		},
		{
			name: "returns invalid when inference server name is empty",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: ""},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
			},
			setupMocks:    func(mb *backendsmocks.MockBackend) {},
			expectedState: v2pb.DEPLOYMENT_STATE_INVALID,
			expectError:   false,
		},
		{
			name: "returns healthy when model status check succeeds",
			deployment: withSingleClusterAnnotation(t, &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
			}, "test-cluster"),
			setupMocks: func(mb *backendsmocks.MockBackend) {
				mb.EXPECT().GetModelStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default", "model-v1").
					Return(readyOnAllReplicas(), nil)
			},
			expectedState: v2pb.DEPLOYMENT_STATE_HEALTHY,
			expectError:   false,
		},
		{
			name: "returns unhealthy when model status check returns not healthy",
			deployment: withSingleClusterAnnotation(t, &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
			}, "test-cluster"),
			setupMocks: func(mb *backendsmocks.MockBackend) {
				mb.EXPECT().GetModelStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default", "model-v1").
					Return(loadingOnOneReplica(), nil)
			},
			expectedState: v2pb.DEPLOYMENT_STATE_UNHEALTHY,
			expectError:   false,
		},
		{
			name: "returns error when model status check fails",
			deployment: withSingleClusterAnnotation(t, &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
			}, "test-cluster"),
			setupMocks: func(mb *backendsmocks.MockBackend) {
				mb.EXPECT().GetModelStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default", "model-v1").
					Return(nil, errors.New("connection error"))
			},
			expectedState: v2pb.DEPLOYMENT_STATE_INVALID,
			expectError:   true,
		},
		{
			// At the end of a rollout the old model is being unloaded while the new one
			// already serves traffic; the deployment must not flip to UNHEALTHY in between.
			name: "returns healthy when the candidate is ready even if the current revision is gone",
			deployment: withSingleClusterAnnotation(t, &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v2"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision:   &api.ResourceIdentifier{Name: "model-v1"},
					CandidateRevision: &api.ResourceIdentifier{Name: "model-v2"},
				},
			}, "test-cluster"),
			setupMocks: func(mb *backendsmocks.MockBackend) {
				mb.EXPECT().GetModelStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default", "model-v1").
					Return(loadingOnOneReplica(), nil)
				mb.EXPECT().GetModelStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default", "model-v2").
					Return(readyOnAllReplicas(), nil)
			},
			expectedState: v2pb.DEPLOYMENT_STATE_HEALTHY,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockBackend := backendsmocks.NewMockBackend(ctrl)
			tt.setupMocks(mockBackend)

			mockClientFactory := clientfactorymocks.NewMockClientFactory(ctrl)
			mockClientFactory.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
			mockClientFactory.EXPECT().GetHTTPClient(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

			plugin := &Plugin{
				client:          newKubeClient(t),
				backendRegistry: createTestRegistry(mockBackend),
				clientFactory:   mockClientFactory,
				logger:          zap.NewNop(),
			}

			status, err := plugin.GetState(context.Background(), plugins.ObservabilityContext{}, tt.deployment)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedState, status.State)
			}
		})
	}
}
