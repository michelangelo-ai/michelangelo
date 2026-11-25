package strategies

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	osscommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory/clientfactorymocks"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// deploymentWithTargets builds a Deployment carrying the cluster snapshot annotation that
// PlacementPrepActor writes, since that annotation is what the actor chain is built from.
func deploymentWithTargets(t *testing.T, clusterIDs ...string) *v2pb.Deployment {
	t.Helper()
	deployment := &v2pb.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "dep", Namespace: "default"}}
	if len(clusterIDs) == 0 {
		return deployment
	}

	targets := make([]*v2pb.ClusterTarget, 0, len(clusterIDs))
	for _, id := range clusterIDs {
		targets = append(targets, &v2pb.ClusterTarget{ClusterId: id})
	}
	require.NoError(t, osscommon.WriteTargetClustersAnnotation(deployment, targets))
	return deployment
}

func actorTypes(actors []conditionInterfaces.ConditionActor[*v2pb.Deployment]) []string {
	types := make([]string, 0, len(actors))
	for _, actor := range actors {
		types = append(types, actor.GetType())
	}
	return types
}

func TestGetActorsForStrategy(t *testing.T) {
	safeSettings := osscommon.RolloutSettings{Canary: true, SoakPeriod: 5 * time.Minute}

	tests := []struct {
		name       string
		deployment *v2pb.Deployment
		settings   osscommon.RolloutSettings
		want       []string
	}{
		{
			// PlacementPrep has not written the snapshot yet, so no cluster is known and
			// the per-cluster actors only appear on a later reconcile.
			name:       "no cluster snapshot yet",
			deployment: deploymentWithTargets(t),
			settings:   safeSettings,
			want:       []string{},
		},
		{
			name:       "rollout and traffic are interleaved per cluster",
			deployment: deploymentWithTargets(t, "c1"),
			want: []string{
				"RollingRolloutComplete-c1",
				"TrafficRoutingConfigured-c1",
				"DiscoveryRoutingConfigured",
				"ModelCleanupComplete-c1",
			},
		},
		{
			// Cleanup trails every cluster's rollout so the old model stays loaded until
			// the last cluster has flipped to the new one.
			name:       "cleanup for all clusters follows the last rollout",
			deployment: deploymentWithTargets(t, "c1", "c2"),
			want: []string{
				"RollingRolloutComplete-c1",
				"TrafficRoutingConfigured-c1",
				"RollingRolloutComplete-c2",
				"TrafficRoutingConfigured-c2",
				"DiscoveryRoutingConfigured",
				"ModelCleanupComplete-c1",
				"ModelCleanupComplete-c2",
			},
		},
		{
			// With a canary and a soak, each cluster is fully validated, flipped and
			// observed under traffic before the next cluster starts loading.
			name:       "canary and soak wrap every cluster",
			deployment: deploymentWithTargets(t, "c1", "c2"),
			settings:   safeSettings,
			want: []string{
				"CanaryRolloutComplete-c1",
				"RollingRolloutComplete-c1",
				"TrafficRoutingConfigured-c1",
				"SoakComplete-c1",
				"CanaryRolloutComplete-c2",
				"RollingRolloutComplete-c2",
				"TrafficRoutingConfigured-c2",
				"SoakComplete-c2",
				"DiscoveryRoutingConfigured",
				"ModelCleanupComplete-c1",
				"ModelCleanupComplete-c2",
			},
		},
		{
			name:       "canary without soak",
			deployment: deploymentWithTargets(t, "c1"),
			settings:   osscommon.RolloutSettings{Canary: true},
			want: []string{
				"CanaryRolloutComplete-c1",
				"RollingRolloutComplete-c1",
				"TrafficRoutingConfigured-c1",
				"DiscoveryRoutingConfigured",
				"ModelCleanupComplete-c1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actors, err := GetActorsForStrategy(context.Background(), Params{Logger: zap.NewNop(), Settings: tt.settings}, tt.deployment)

			require.NoError(t, err)
			assert.Equal(t, tt.want, actorTypes(actors))
		})
	}
}

func TestGetActorsForStrategy_SecretsRenamedSinceSnapshot_UsesInferenceServerConnection(t *testing.T) {
	connection := func(tokenTag string) *v2pb.ClusterTarget {
		return &v2pb.ClusterTarget{
			ClusterId: "c1",
			Connection: &v2pb.ClusterTarget_Kubernetes{Kubernetes: &v2pb.ConnectionSpec{
				Host:     "https://c1.example.com",
				Port:     "443",
				TokenTag: tokenTag,
			}},
		}
	}
	deployment := &v2pb.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "dep", Namespace: "default"},
		Spec: v2pb.DeploymentSpec{
			DesiredRevision: &apipb.ResourceIdentifier{Name: "model-v2"},
			Target: &v2pb.DeploymentSpec_InferenceServer{
				InferenceServer: &apipb.ResourceIdentifier{Name: "test-server"},
			},
		},
	}
	require.NoError(t, osscommon.WriteTargetClustersAnnotation(deployment, []*v2pb.ClusterTarget{connection("token-v1")}))
	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))
	kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&v2pb.InferenceServer{
		ObjectMeta: metav1.ObjectMeta{Name: "test-server", Namespace: "default"},
		Spec:       v2pb.InferenceServerSpec{ClusterTargets: []*v2pb.ClusterTarget{connection("token-v2")}},
	}).Build()

	ctrl := gomock.NewController(t)
	mockClientFactory := clientfactorymocks.NewMockClientFactory(ctrl)
	mockClientFactory.EXPECT().
		GetDynamicClient(gomock.Any(), tokenTagMatcher("token-v2")).
		Return(nil, errors.New("stop after the client lookup"))

	actors, err := GetActorsForStrategy(context.Background(),
		Params{Client: kubeClient, ClientFactory: mockClientFactory, Logger: zap.NewNop()}, deployment)
	require.NoError(t, err)
	require.Equal(t, "TrafficRoutingConfigured-c1", actors[1].GetType())

	condition, err := actors[1].Retrieve(context.Background(), deployment, &apipb.Condition{})

	require.NoError(t, err)
	assert.Equal(t, apipb.CONDITION_STATUS_FALSE, condition.Status)
}

func TestGetActorsForStrategy_InferenceServerReadFails_ReturnsError(t *testing.T) {
	deployment := &v2pb.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "dep", Namespace: "default"},
		Spec: v2pb.DeploymentSpec{
			DesiredRevision: &apipb.ResourceIdentifier{Name: "model-v2"},
			Target: &v2pb.DeploymentSpec_InferenceServer{
				InferenceServer: &apipb.ResourceIdentifier{Name: "test-server"},
			},
		},
	}
	require.NoError(t, osscommon.WriteTargetClustersAnnotation(deployment, []*v2pb.ClusterTarget{{ClusterId: "c1"}}))
	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))
	kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
			return errors.New("cache not synced")
		},
	}).Build()

	_, err := GetActorsForStrategy(context.Background(), Params{Client: kubeClient, Logger: zap.NewNop()}, deployment)

	require.ErrorContains(t, err, "resolve target clusters: get inference server default/test-server: cache not synced")
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

func TestGetActorsForStrategyUnreadableSnapshot(t *testing.T) {
	deployment := &v2pb.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dep",
			Namespace:   "default",
			Annotations: map[string]string{osscommon.TargetClustersAnnotation: "{not json}"},
		},
	}

	_, err := GetActorsForStrategy(context.Background(), Params{Logger: zap.NewNop()}, deployment)

	require.Error(t, err)
}
