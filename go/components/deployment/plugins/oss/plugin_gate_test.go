package oss

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/metricgate"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends/backendsmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory/clientfactorymocks"
	"github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// gateDeployment is a deployment mid-rollout of model-v2 over model-v1.
func gateDeployment() *v2pb.Deployment {
	return &v2pb.Deployment{
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
			Stage:             v2pb.DEPLOYMENT_STAGE_RESOURCE_ACQUISITION,
		},
	}
}

// prometheusReturning stands in for Prometheus and answers every query with one sample.
func prometheusReturning(t *testing.T, value string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1700000000,"%s"]}]}}`, value)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestHealthCheckGate(t *testing.T) {
	tests := []struct {
		name          string
		deployment    func(t *testing.T) *v2pb.Deployment
		setupMocks    func(*backendsmocks.MockBackend)
		metricValue   string // empty means no metric gate is configured
		wantHealthy   bool
		wantErr       bool
		wantReasonSub string
	}{
		{
			name: "no inference server target is never healthy",
			deployment: func(*testing.T) *v2pb.Deployment {
				dep := gateDeployment()
				dep.Spec.Target = nil
				return dep
			},
			setupMocks:  func(*backendsmocks.MockBackend) {},
			wantHealthy: false,
		},
		{
			name:        "no placed clusters yet has nothing to check",
			deployment:  func(*testing.T) *v2pb.Deployment { return gateDeployment() },
			setupMocks:  func(*backendsmocks.MockBackend) {},
			wantHealthy: true,
		},
		{
			name: "an unhealthy cluster fails the gate and records why",
			deployment: func(t *testing.T) *v2pb.Deployment {
				return withSingleClusterAnnotation(t, gateDeployment(), "test-cluster")
			},
			setupMocks: func(mb *backendsmocks.MockBackend) {
				mb.EXPECT().IsHealthy(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default").Return(false, nil)
			},
			wantHealthy:   false,
			wantReasonSub: "not healthy in cluster test-cluster",
		},
		{
			name: "a health probe error is surfaced",
			deployment: func(t *testing.T) *v2pb.Deployment {
				return withSingleClusterAnnotation(t, gateDeployment(), "test-cluster")
			},
			setupMocks: func(mb *backendsmocks.MockBackend) {
				mb.EXPECT().IsHealthy(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default").Return(false, errors.New("connection refused"))
			},
			wantErr: true,
		},
		{
			name: "healthy clusters with a breached metric fail the gate",
			deployment: func(t *testing.T) *v2pb.Deployment {
				return withSingleClusterAnnotation(t, gateDeployment(), "test-cluster")
			},
			setupMocks: func(mb *backendsmocks.MockBackend) {
				mb.EXPECT().IsHealthy(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default").Return(true, nil)
			},
			metricValue:   "0.5",
			wantHealthy:   false,
			wantReasonSub: "breached for model model-v2",
		},
		{
			name: "healthy clusters and healthy metrics pass",
			deployment: func(t *testing.T) *v2pb.Deployment {
				dep := withSingleClusterAnnotation(t, gateDeployment(), "test-cluster")
				dep.Annotations[common.HealthGateReasonAnnotation] = "stale reason from an earlier reconcile"
				return dep
			},
			setupMocks: func(mb *backendsmocks.MockBackend) {
				mb.EXPECT().IsHealthy(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default").Return(true, nil)
			},
			metricValue: "0.001",
			wantHealthy: true,
		},
		{
			name: "healthy clusters without a metric gate pass",
			deployment: func(t *testing.T) *v2pb.Deployment {
				return withSingleClusterAnnotation(t, gateDeployment(), "test-cluster")
			},
			setupMocks: func(mb *backendsmocks.MockBackend) {
				mb.EXPECT().IsHealthy(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default").Return(true, nil)
			},
			wantHealthy: true,
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

			var gate *metricgate.Gate
			if tt.metricValue != "" {
				server := prometheusReturning(t, tt.metricValue)
				gate = metricgate.New(maconfig.MetricGateConfig{PrometheusURL: server.URL}, zap.NewNop())
			}
			plugin := &Plugin{
				backendRegistry: createTestRegistry(mockBackend),
				clientFactory:   mockClientFactory,
				logger:          zap.NewNop(),
				metricGate:      gate,
			}
			deployment := tt.deployment(t)

			healthy, err := plugin.HealthCheckGate(context.Background(), plugins.ObservabilityContext{}, deployment)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantHealthy, healthy)
			reason := deployment.GetAnnotations()[common.HealthGateReasonAnnotation]
			if tt.wantReasonSub != "" {
				assert.Contains(t, reason, tt.wantReasonSub)
			} else {
				assert.Empty(t, reason, "a passing gate must clear any stale reason")
			}
		})
	}
}

func TestGetRollbackPlugin(t *testing.T) {
	plugin := &Plugin{logger: zap.NewNop()}

	t.Run("one rollback actor per placed cluster plus the completion marker", func(t *testing.T) {
		deployment := withSingleClusterAnnotation(t, gateDeployment(), "test-cluster")

		rollbackPlugin, err := plugin.GetRollbackPlugin(context.Background(), deployment)

		require.NoError(t, err)
		types := make([]string, 0)
		for _, actor := range rollbackPlugin.GetActors() {
			types = append(types, actor.GetType())
		}
		assert.Equal(t, []string{"RollbackComplete-test-cluster", common.ActorTypeRollback}, types)
	})

	t.Run("no placement leaves only the completion marker", func(t *testing.T) {
		rollbackPlugin, err := plugin.GetRollbackPlugin(context.Background(), gateDeployment())

		require.NoError(t, err)
		require.Len(t, rollbackPlugin.GetActors(), 1)
		assert.Equal(t, common.ActorTypeRollback, rollbackPlugin.GetActors()[0].GetType())
	})

	t.Run("an unreadable placement snapshot is an error", func(t *testing.T) {
		deployment := gateDeployment()
		deployment.Annotations = map[string]string{common.TargetClustersAnnotation: "{not json}"}

		_, err := plugin.GetRollbackPlugin(context.Background(), deployment)

		require.Error(t, err)
	})
}
