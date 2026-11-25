package oss

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/metricgate"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends/backendsmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory/clientfactorymocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig/modelconfigmocks"
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

// prometheusServer stands in for Prometheus. answer gives, for a PromQL expression, the HTTP
// status and the sample values to return; no values is an empty vector.
func prometheusServer(t *testing.T, answer func(expr string) (int, []string)) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status, values := answer(r.URL.Query().Get("query"))
		entries := make([]string, 0, len(values))
		for _, value := range values {
			entries = append(entries, fmt.Sprintf(`{"metric":{},"value":[1700000000,"%s"]}`, value))
		}
		w.WriteHeader(status)
		if status != http.StatusOK {
			_, _ = fmt.Fprint(w, `{"status":"error","errorType":"internal","error":"boom"}`)
			return
		}
		_, _ = fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":[%s]}}`, strings.Join(entries, ","))
	}))
	t.Cleanup(server.Close)
	return server
}

// everyQueryReturns answers every query with the given sample values.
func everyQueryReturns(values ...string) func(string) (int, []string) {
	return func(string) (int, []string) { return http.StatusOK, values }
}

// queriesReturn answers each query with the sample values listed for its expression, and a
// query that is not listed with HTTP 500.
func queriesReturn(values map[string][]string) func(string) (int, []string) {
	return func(expr string) (int, []string) {
		if answer, ok := values[expr]; ok {
			return http.StatusOK, answer
		}
		return http.StatusInternalServerError, nil
	}
}

// gateQuery is a metric gate query whose expression is its name, so that a test's Prometheus
// can answer it by name.
func gateQuery(name string) maconfig.MetricGateQuery {
	return maconfig.MetricGateQuery{Name: name, Expr: name, Threshold: 0.5}
}

// servingEntry is a model config entry the named deployment serves.
func servingEntry(deploymentName, model string) modelconfig.ModelConfigEntry {
	return modelconfig.ModelConfigEntry{Name: model, DeploymentName: deploymentName, Phase: modelconfig.ModelPhaseServing}
}

// missingOnOneReplica is a model status where one of two replicas has not loaded the model.
func missingOnOneReplica() *backends.ModelStatus {
	return &backends.ModelStatus{Desired: 2, Replicas: []backends.ReplicaModelStatus{
		{Replica: "pod-a", Running: true, State: backends.ModelLoadStateReady},
		{Replica: "pod-b", Running: true, State: backends.ModelLoadStateLoading, Reason: "model not yet loaded"},
	}}
}

func TestHealthCheckGate(t *testing.T) {
	const deploymentName = "test-deployment"
	expectStatus := func(mb *backendsmocks.MockBackend, model string, status *backends.ModelStatus, err error) {
		mb.EXPECT().GetModelStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			"test-server", "default", model).Return(status, err)
	}
	expectConfig := func(mc *modelconfigmocks.MockModelConfigProvider, entries []modelconfig.ModelConfigEntry, err error) {
		mc.EXPECT().GetModelsFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default").Return(entries, err)
	}

	readyModel := func(mb *backendsmocks.MockBackend, mc *modelconfigmocks.MockModelConfigProvider) {
		expectConfig(mc, []modelconfig.ModelConfigEntry{servingEntry(deploymentName, "model-v1")}, nil)
		expectStatus(mb, "model-v1", readyOnAllReplicas(), nil)
	}
	mustNotBeQueried := func(expr string) (int, []string) {
		t.Errorf("prometheus must not be queried, got %q", expr)
		return http.StatusOK, nil
	}
	withNoDataAnnotation := func(annotation string) func(*testing.T) *v2pb.Deployment {
		return func(t *testing.T) *v2pb.Deployment {
			dep := withSingleClusterAnnotation(t, gateDeployment(), "test-cluster")
			dep.Annotations[common.MetricGateNoDataAnnotation] = annotation
			return dep
		}
	}

	tests := []struct {
		name          string
		deployment    func(t *testing.T) *v2pb.Deployment
		setupMocks    func(*backendsmocks.MockBackend, *modelconfigmocks.MockModelConfigProvider)
		prometheus    func(expr string) (int, []string) // nil means no metric gate is configured
		queries       []maconfig.MetricGateQuery        // empty uses the gate's default query
		failClosed    bool
		wantHealthy   bool
		wantErr       bool
		wantReasonSub string
		wantNoData    string
	}{
		{
			name: "no inference server target is never healthy",
			deployment: func(*testing.T) *v2pb.Deployment {
				dep := gateDeployment()
				dep.Spec.Target = nil
				return dep
			},
			setupMocks:  func(*backendsmocks.MockBackend, *modelconfigmocks.MockModelConfigProvider) {},
			wantHealthy: false,
		},
		{
			name:        "no placed clusters yet has nothing to check",
			deployment:  func(*testing.T) *v2pb.Deployment { return gateDeployment() },
			setupMocks:  func(*backendsmocks.MockBackend, *modelconfigmocks.MockModelConfigProvider) {},
			wantHealthy: true,
		},
		{
			name: "a serving model missing on a replica fails the gate and records why",
			deployment: func(t *testing.T) *v2pb.Deployment {
				return withSingleClusterAnnotation(t, gateDeployment(), "test-cluster")
			},
			setupMocks: func(mb *backendsmocks.MockBackend, mc *modelconfigmocks.MockModelConfigProvider) {
				expectConfig(mc, []modelconfig.ModelConfigEntry{servingEntry(deploymentName, "model-v1")}, nil)
				expectStatus(mb, "model-v1", missingOnOneReplica(), nil)
			},
			wantHealthy:   false,
			wantReasonSub: "model model-v1 is not ready in cluster test-cluster: 1/2 replicas ready (pod-b: LOADING model not yet loaded)",
		},
		{
			name: "every serving model of the deployment is checked",
			deployment: func(t *testing.T) *v2pb.Deployment {
				return withSingleClusterAnnotation(t, gateDeployment(), "test-cluster")
			},
			setupMocks: func(mb *backendsmocks.MockBackend, mc *modelconfigmocks.MockModelConfigProvider) {
				expectConfig(mc, []modelconfig.ModelConfigEntry{
					servingEntry(deploymentName, "model-v1"),
					servingEntry(deploymentName, "model-v2"),
				}, nil)
				expectStatus(mb, "model-v1", readyOnAllReplicas(), nil)
				expectStatus(mb, "model-v2", missingOnOneReplica(), nil)
			},
			wantHealthy:   false,
			wantReasonSub: "model model-v2 is not ready",
		},
		{
			// Regression: the server's pods are not ready while another deployment's model is
			// still loading, but this deployment's own model is fine and must stay healthy.
			name: "another deployment's model is never probed",
			deployment: func(t *testing.T) *v2pb.Deployment {
				return withSingleClusterAnnotation(t, gateDeployment(), "test-cluster")
			},
			setupMocks: func(mb *backendsmocks.MockBackend, mc *modelconfigmocks.MockModelConfigProvider) {
				expectConfig(mc, []modelconfig.ModelConfigEntry{
					servingEntry(deploymentName, "model-v1"),
					servingEntry("deployment-b", "model-b"),
				}, nil)
				expectStatus(mb, "model-v1", readyOnAllReplicas(), nil)
			},
			wantHealthy: true,
		},
		{
			name: "canary and staged entries are not judged yet",
			deployment: func(t *testing.T) *v2pb.Deployment {
				return withSingleClusterAnnotation(t, gateDeployment(), "test-cluster")
			},
			setupMocks: func(mb *backendsmocks.MockBackend, mc *modelconfigmocks.MockModelConfigProvider) {
				expectConfig(mc, []modelconfig.ModelConfigEntry{
					{Name: "model-v2", DeploymentName: deploymentName, Phase: modelconfig.ModelPhaseCanary, CanaryPod: "pod-a"},
					{Name: "model-v3", DeploymentName: deploymentName, Phase: modelconfig.ModelPhaseStaged},
				}, nil)
			},
			wantHealthy: true,
		},
		{
			name: "a deployment with no entries yet has nothing to check",
			deployment: func(t *testing.T) *v2pb.Deployment {
				return withSingleClusterAnnotation(t, gateDeployment(), "test-cluster")
			},
			setupMocks: func(mb *backendsmocks.MockBackend, mc *modelconfigmocks.MockModelConfigProvider) {
				expectConfig(mc, nil, nil)
			},
			wantHealthy: true,
		},
		{
			name: "a model status probe error is surfaced",
			deployment: func(t *testing.T) *v2pb.Deployment {
				return withSingleClusterAnnotation(t, gateDeployment(), "test-cluster")
			},
			setupMocks: func(mb *backendsmocks.MockBackend, mc *modelconfigmocks.MockModelConfigProvider) {
				expectConfig(mc, []modelconfig.ModelConfigEntry{servingEntry(deploymentName, "model-v1")}, nil)
				expectStatus(mb, "model-v1", nil, errors.New("connection refused"))
			},
			wantErr: true,
		},
		{
			name: "a model config read error is surfaced",
			deployment: func(t *testing.T) *v2pb.Deployment {
				return withSingleClusterAnnotation(t, gateDeployment(), "test-cluster")
			},
			setupMocks: func(mb *backendsmocks.MockBackend, mc *modelconfigmocks.MockModelConfigProvider) {
				expectConfig(mc, nil, errors.New("configmap not found"))
			},
			wantErr: true,
		},
		{
			name: "ready models with a breached metric fail the gate",
			deployment: func(t *testing.T) *v2pb.Deployment {
				return withSingleClusterAnnotation(t, gateDeployment(), "test-cluster")
			},
			setupMocks:    readyModel,
			prometheus:    everyQueryReturns("0.5"),
			wantHealthy:   false,
			wantReasonSub: "breached for model model-v2",
		},
		{
			name: "ready models and healthy metrics pass and clear a stale reason",
			deployment: func(t *testing.T) *v2pb.Deployment {
				dep := withSingleClusterAnnotation(t, gateDeployment(), "test-cluster")
				dep.Annotations[common.HealthGateReasonAnnotation] = "stale reason from an earlier reconcile"
				return dep
			},
			setupMocks:  readyModel,
			prometheus:  everyQueryReturns("0.001"),
			wantHealthy: true,
		},
		{
			name: "a metric gate query that returns no samples passes and is listed on the deployment",
			deployment: func(t *testing.T) *v2pb.Deployment {
				return withSingleClusterAnnotation(t, gateDeployment(), "test-cluster")
			},
			setupMocks:  readyModel,
			prometheus:  everyQueryReturns(),
			wantHealthy: true,
			wantNoData:  "model-v2: triton-inference-failure-ratio",
		},
		{
			name: "the no-data annotation names the desired revision before a candidate is recorded",
			deployment: func(t *testing.T) *v2pb.Deployment {
				dep := withSingleClusterAnnotation(t, gateDeployment(), "test-cluster")
				dep.Spec.DesiredRevision = &api.ResourceIdentifier{Name: "model-desired"}
				dep.Status.CandidateRevision = nil
				return dep
			},
			setupMocks:  readyModel,
			prometheus:  everyQueryReturns(),
			wantHealthy: true,
			wantNoData:  "model-desired: triton-inference-failure-ratio",
		},
		{
			name: "every metric gate query that returns no samples is listed",
			deployment: func(t *testing.T) *v2pb.Deployment {
				return withSingleClusterAnnotation(t, gateDeployment(), "test-cluster")
			},
			setupMocks:  readyModel,
			prometheus:  everyQueryReturns(),
			queries:     []maconfig.MetricGateQuery{gateQuery("first"), gateQuery("second")},
			wantHealthy: true,
			wantNoData:  "model-v2: first, second",
		},
		{
			name:        "samples clear a stale no-data annotation",
			deployment:  withNoDataAnnotation("model-v2: triton-inference-failure-ratio"),
			setupMocks:  readyModel,
			prometheus:  everyQueryReturns("0.001"),
			wantHealthy: true,
		},
		{
			name: "a rollback checks nothing and keeps both annotations",
			deployment: func(t *testing.T) *v2pb.Deployment {
				dep := withNoDataAnnotation("model-v2: left by the rollout")(t)
				dep.Annotations[common.HealthGateReasonAnnotation] = "the breach that started the rollback"
				dep.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS
				return dep
			},
			setupMocks:    func(*backendsmocks.MockBackend, *modelconfigmocks.MockModelConfigProvider) {},
			prometheus:    mustNotBeQueried,
			wantHealthy:   true,
			wantReasonSub: "the breach that started the rollback",
			wantNoData:    "model-v2: left by the rollout",
		},
		{
			name: "a rollback is not held up by a malformed query template",
			deployment: func(t *testing.T) *v2pb.Deployment {
				dep := withSingleClusterAnnotation(t, gateDeployment(), "test-cluster")
				dep.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS
				return dep
			},
			setupMocks:  func(*backendsmocks.MockBackend, *modelconfigmocks.MockModelConfigProvider) {},
			prometheus:  mustNotBeQueried,
			queries:     []maconfig.MetricGateQuery{{Name: "broken", Expr: "{{.Nope}}"}},
			wantHealthy: true,
		},
		{
			name:          "a breach on a query listed in the no-data annotation removes it from the list",
			deployment:    withNoDataAnnotation("model-v2: first"),
			setupMocks:    readyModel,
			prometheus:    everyQueryReturns("1"),
			queries:       []maconfig.MetricGateQuery{gateQuery("first")},
			wantHealthy:   false,
			wantReasonSub: `"first" breached`,
		},
		{
			name: "a breach still lists the later queries that return no samples",
			deployment: func(t *testing.T) *v2pb.Deployment {
				return withSingleClusterAnnotation(t, gateDeployment(), "test-cluster")
			},
			setupMocks:    readyModel,
			prometheus:    queriesReturn(map[string][]string{"first": {"1"}, "second": nil}),
			queries:       []maconfig.MetricGateQuery{gateQuery("first"), gateQuery("second")},
			wantHealthy:   false,
			wantReasonSub: `"first" breached`,
			wantNoData:    "model-v2: second",
		},
		{
			name:        "a failed query that is skipped leaves the no-data annotation unchanged",
			deployment:  withNoDataAnnotation("model-v2: left by an earlier evaluation"),
			setupMocks:  readyModel,
			prometheus:  queriesReturn(nil),
			wantHealthy: true,
			wantNoData:  "model-v2: left by an earlier evaluation",
		},
		{
			name:          "a failed query leaves the no-data annotation unchanged when the gate fails closed",
			deployment:    withNoDataAnnotation("model-v2: left by an earlier evaluation"),
			setupMocks:    readyModel,
			prometheus:    queriesReturn(map[string][]string{"first": nil}),
			queries:       []maconfig.MetricGateQuery{gateQuery("first"), gateQuery("second")},
			failClosed:    true,
			wantHealthy:   false,
			wantReasonSub: `"second" could not be evaluated`,
			wantNoData:    "model-v2: left by an earlier evaluation",
		},
		{
			name:        "a failed query removes a no-data annotation that names another model",
			deployment:  withNoDataAnnotation("model-v1: left by an earlier rollout"),
			setupMocks:  readyModel,
			prometheus:  queriesReturn(nil),
			wantHealthy: true,
		},
		{
			name:        "a failed query removes a no-data annotation for a model whose name only starts the same",
			deployment:  withNoDataAnnotation("model-v20: left by an earlier rollout"),
			setupMocks:  readyModel,
			prometheus:  queriesReturn(nil),
			wantHealthy: true,
		},
		{
			name:        "a failed query keeps the other queries that return no samples off the annotation",
			deployment:  withNoDataAnnotation("model-v2: left by an earlier evaluation"),
			setupMocks:  readyModel,
			prometheus:  queriesReturn(map[string][]string{"first": nil}),
			queries:     []maconfig.MetricGateQuery{gateQuery("first"), gateQuery("second")},
			wantHealthy: true,
			wantNoData:  "model-v2: left by an earlier evaluation",
		},
		{
			name:        "ready models without a metric gate pass and clear a stale no-data annotation",
			deployment:  withNoDataAnnotation("model-v2: triton-inference-failure-ratio"),
			setupMocks:  readyModel,
			wantHealthy: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockBackend := backendsmocks.NewMockBackend(ctrl)
			mockConfig := modelconfigmocks.NewMockModelConfigProvider(ctrl)
			tt.setupMocks(mockBackend, mockConfig)
			mockClientFactory := clientfactorymocks.NewMockClientFactory(ctrl)
			mockClientFactory.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
			mockClientFactory.EXPECT().GetHTTPClient(gomock.Any(), gomock.Any()).Return(&http.Client{}, nil).AnyTimes()

			var gate *metricgate.Gate
			if tt.prometheus != nil {
				server := prometheusServer(t, tt.prometheus)
				gate = metricgate.New(maconfig.MetricGateConfig{
					PrometheusURL: server.URL,
					FailClosed:    tt.failClosed,
					Queries:       tt.queries,
				}, zap.NewNop())
			}
			plugin := &Plugin{
				client:              newKubeClient(t),
				backendRegistry:     createTestRegistry(mockBackend),
				clientFactory:       mockClientFactory,
				modelConfigProvider: mockConfig,
				logger:              zap.NewNop(),
				metricGate:          gate,
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
			assert.Equal(t, tt.wantNoData, deployment.GetAnnotations()[common.MetricGateNoDataAnnotation])
		})
	}
}

func TestHealthCheckGate_MetricGateNoData_LogsOnlyWhenTheAnnotationChanges(t *testing.T) {
	const (
		reportedMessage = "metric gate queries returned no samples; the candidate was not judged on them"
		removedMessage  = "removed the metric gate no-data annotation"
	)
	missing := []string{"first", "second"}
	server := prometheusServer(t, func(expr string) (int, []string) {
		if slices.Contains(missing, expr) {
			return http.StatusOK, nil
		}
		return http.StatusOK, []string{"0"}
	})
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	core, logs := observer.New(zap.InfoLevel)
	plugin := &Plugin{
		client:          newKubeClient(t),
		backendRegistry: createTestRegistry(backendsmocks.NewMockBackend(ctrl)),
		logger:          zap.New(core),
		metricGate: metricgate.New(maconfig.MetricGateConfig{
			PrometheusURL: server.URL,
			Queries:       []maconfig.MetricGateQuery{gateQuery("first"), gateQuery("second")},
		}, zap.New(core)),
	}
	deployment := gateDeployment()
	evaluate := func() {
		t.Helper()
		healthy, err := plugin.HealthCheckGate(context.Background(), plugins.ObservabilityContext{}, deployment)
		require.NoError(t, err)
		require.True(t, healthy)
	}
	reported := func() []observer.LoggedEntry { return logs.FilterMessage(reportedMessage).All() }

	evaluate()
	evaluate()
	require.Len(t, reported(), 1, "the gate runs on every reconcile and must not log each time")
	assert.Equal(t, "model-v2: first, second", deployment.Annotations[common.MetricGateNoDataAnnotation])
	assert.Equal(t, zap.InfoLevel, reported()[0].Level)
	assert.Equal(t, map[string]interface{}{
		"operation":           "health_check_gate",
		"namespace":           "default",
		"deployment":          "test-deployment",
		"inference_server":    "test-server",
		"model":               "model-v2",
		"previous_annotation": "",
		"queries":             []interface{}{"first", "second"},
	}, reported()[0].ContextMap())

	missing = []string{"second"}
	evaluate()
	assert.Equal(t, "model-v2: second", deployment.Annotations[common.MetricGateNoDataAnnotation])
	assert.Len(t, reported(), 2, "a different set of queries is reported again")

	deployment.Status.CandidateRevision.Name = "model-v3"
	evaluate()
	assert.Equal(t, "model-v3: second", deployment.Annotations[common.MetricGateNoDataAnnotation])
	assert.Len(t, reported(), 3, "the same queries are reported again for a new candidate")

	missing = nil
	evaluate()
	evaluate()
	assert.NotContains(t, deployment.Annotations, common.MetricGateNoDataAnnotation)
	removed := logs.FilterMessage(removedMessage).All()
	require.Len(t, removed, 1)
	assert.Equal(t, zap.InfoLevel, removed[0].Level)
	assert.Equal(t, map[string]interface{}{
		"operation":           "health_check_gate",
		"namespace":           "default",
		"deployment":          "test-deployment",
		"inference_server":    "test-server",
		"model":               "model-v3",
		"previous_annotation": "model-v3: second",
	}, removed[0].ContextMap())
	assert.Equal(t, 4, logs.Len(), "the gate itself logs nothing at Info")
}

// TestHealthCheckGate_BreachThenRollback follows one rollout through the reconciles the
// controller would run: the candidate has no samples yet, then breaches, then is rolled back.
func TestHealthCheckGate_BreachThenRollback(t *testing.T) {
	var mu sync.Mutex
	answers := map[string][]string{"errors": nil, "latency": nil}
	requests := 0
	server := prometheusServer(t, func(expr string) (int, []string) {
		mu.Lock()
		defer mu.Unlock()
		requests++
		return http.StatusOK, answers[expr]
	})
	set := func(expr string, values ...string) {
		mu.Lock()
		defer mu.Unlock()
		answers[expr] = values
	}
	sent := func() int {
		mu.Lock()
		defer mu.Unlock()
		return requests
	}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	plugin := &Plugin{
		client:          newKubeClient(t),
		backendRegistry: createTestRegistry(backendsmocks.NewMockBackend(ctrl)),
		logger:          zap.NewNop(),
		metricGate: metricgate.New(maconfig.MetricGateConfig{
			PrometheusURL: server.URL,
			Queries:       []maconfig.MetricGateQuery{gateQuery("errors"), gateQuery("latency")},
		}, zap.NewNop()),
	}
	deployment := gateDeployment()
	gate := func() bool {
		t.Helper()
		healthy, err := plugin.HealthCheckGate(context.Background(), plugins.ObservabilityContext{}, deployment)
		require.NoError(t, err)
		return healthy
	}

	require.True(t, gate(), "a candidate without samples is not rolled back")
	assert.Equal(t, "model-v2: errors, latency", deployment.Annotations[common.MetricGateNoDataAnnotation])

	set("errors", "0.9")
	require.False(t, gate(), "the breach fails the gate")
	reason := deployment.Annotations[common.HealthGateReasonAnnotation]
	assert.Contains(t, reason, `metric gate "errors" breached for model model-v2: 0.9 gt 0.5`)
	assert.Equal(t, "model-v2: latency", deployment.Annotations[common.MetricGateNoDataAnnotation],
		"the breaching query has samples, so only the other one is still listed")

	// The controller starts the rollback, which unloads the candidate: its series disappear.
	deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS
	set("errors")
	before := sent()
	require.True(t, gate())
	require.True(t, gate())
	assert.Equal(t, before, sent(), "the gate is not evaluated during a rollback")
	assert.Equal(t, reason, deployment.Annotations[common.HealthGateReasonAnnotation],
		"the reason for the rollback stays readable")
	assert.Equal(t, "model-v2: latency", deployment.Annotations[common.MetricGateNoDataAnnotation])
}

func TestHealthCheckGate_MetricGateNoData_LogNamesTheModelInTheAnnotation(t *testing.T) {
	server := prometheusServer(t, everyQueryReturns())
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	core, logs := observer.New(zap.InfoLevel)
	plugin := &Plugin{
		client:          newKubeClient(t),
		backendRegistry: createTestRegistry(backendsmocks.NewMockBackend(ctrl)),
		logger:          zap.New(core),
		metricGate:      metricgate.New(maconfig.MetricGateConfig{PrometheusURL: server.URL}, zap.NewNop()),
	}
	deployment := gateDeployment()
	deployment.Spec.DesiredRevision = &api.ResourceIdentifier{Name: "model-desired"}
	deployment.Status.CandidateRevision = nil

	_, err := plugin.HealthCheckGate(context.Background(), plugins.ObservabilityContext{}, deployment)

	require.NoError(t, err)
	require.Equal(t, 1, logs.Len())
	assert.Equal(t, "model-desired", logs.All()[0].ContextMap()["model"])
	assert.Contains(t, deployment.Annotations[common.MetricGateNoDataAnnotation], "model-desired: ")
}

func TestNoDataAnnotation(t *testing.T) {
	atCap := strings.Repeat("q", maxNoDataAnnotationBytes-len("m: "))
	// 1024 two-byte characters: the cut lands in the middle of one.
	wide := strings.Repeat("é", maxNoDataAnnotationBytes)

	tests := []struct {
		name    string
		model   string
		queries []string
		want    string
	}{
		{name: "no listed query means no annotation", model: "model-v2"},
		{name: "queries are listed after the model", model: "model-v2", queries: []string{"first", "second"}, want: "model-v2: first, second"},
		{name: "a value at the cap is kept whole", model: "m", queries: []string{atCap}, want: "m: " + atCap},
		{
			name:    "a value over the cap is cut to the cap and ends in an ellipsis",
			model:   "m",
			queries: []string{atCap + "q"},
			want:    "m: " + atCap[:len(atCap)-len("...")] + "...",
		},
		{
			name:    "the cut does not split a character",
			model:   "model-v2",
			queries: []string{wide, "second"},
			want:    "model-v2: " + strings.Repeat("é", 505) + "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := noDataAnnotation(tt.model, tt.queries)

			assert.Equal(t, tt.want, got)
			assert.LessOrEqual(t, len(got), maxNoDataAnnotationBytes)
			assert.True(t, utf8.ValidString(got))
		})
	}
}

func TestHealthCheckGate_MetricGateTemplateError_ReturnsError(t *testing.T) {
	server := prometheusServer(t, everyQueryReturns("0"))
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	plugin := &Plugin{
		client:          newKubeClient(t),
		backendRegistry: createTestRegistry(backendsmocks.NewMockBackend(ctrl)),
		logger:          zap.NewNop(),
		metricGate: metricgate.New(maconfig.MetricGateConfig{
			PrometheusURL: server.URL,
			Queries:       []maconfig.MetricGateQuery{{Name: "broken", Expr: "{{.Nope}}"}},
		}, zap.NewNop()),
	}

	healthy, err := plugin.HealthCheckGate(context.Background(), plugins.ObservabilityContext{}, gateDeployment())

	require.ErrorContains(t, err, "evaluate metric gate for deployment default/test-deployment")
	assert.False(t, healthy)
}

func TestHealthCheckGate_SecretsRenamedSinceRollout_UsesInferenceServerConnection(t *testing.T) {
	deployment := gateDeployment()
	inferenceServer := withRenamedSecrets(t, deployment)

	ctrl := gomock.NewController(t)
	mockConfig := modelconfigmocks.NewMockModelConfigProvider(ctrl)
	mockConfig.EXPECT().GetModelsFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default").
		Return(nil, nil)
	mockClientFactory := clientfactorymocks.NewMockClientFactory(ctrl)
	mockClientFactory.EXPECT().GetClient(gomock.Any(), tokenTagMatcher("remote-token-v2")).Return(nil, nil)

	plugin := &Plugin{
		client:              newKubeClient(t, inferenceServer),
		backendRegistry:     createTestRegistry(backendsmocks.NewMockBackend(ctrl)),
		clientFactory:       mockClientFactory,
		modelConfigProvider: mockConfig,
		logger:              zap.NewNop(),
	}

	healthy, err := plugin.HealthCheckGate(context.Background(), plugins.ObservabilityContext{}, deployment)

	require.NoError(t, err)
	assert.True(t, healthy)
}

func TestHealthCheckGate_InferenceServerReadFails_ReturnsError(t *testing.T) {
	deployment := withSingleClusterAnnotation(t, gateDeployment(), "test-cluster")
	ctrl := gomock.NewController(t)
	plugin := &Plugin{
		client:          newFailingKubeClient(t),
		backendRegistry: createTestRegistry(backendsmocks.NewMockBackend(ctrl)),
		logger:          zap.NewNop(),
	}

	healthy, err := plugin.HealthCheckGate(context.Background(), plugins.ObservabilityContext{}, deployment)

	require.ErrorContains(t, err, "get inference server default/test-server: cache not synced")
	assert.False(t, healthy)
}

func TestGetRolloutPlugin_SecretsRenamedSinceRollout_UsesInferenceServerConnection(t *testing.T) {
	deployment := gateDeployment()
	inferenceServer := withRenamedSecrets(t, deployment)
	ctrl := gomock.NewController(t)
	mockClientFactory := clientfactorymocks.NewMockClientFactory(ctrl)
	mockClientFactory.EXPECT().GetDynamicClient(gomock.Any(), tokenTagMatcher("remote-token-v2")).
		Return(nil, errors.New("stop after the client lookup"))
	plugin := &Plugin{
		client:        newKubeClient(t, inferenceServer),
		clientFactory: mockClientFactory,
		logger:        zap.NewNop(),
	}

	rolloutPlugin, err := plugin.GetRolloutPlugin(context.Background(), deployment)
	require.NoError(t, err)
	var trafficRouting conditionInterfaces.ConditionActor[*v2pb.Deployment]
	for _, actor := range rolloutPlugin.GetActors() {
		if actor.GetType() == "TrafficRoutingConfigured-remote" {
			trafficRouting = actor
		}
	}
	require.NotNil(t, trafficRouting)
	condition, err := trafficRouting.Retrieve(context.Background(), deployment, &api.Condition{})

	require.NoError(t, err)
	assert.Equal(t, api.CONDITION_STATUS_FALSE, condition.Status)
}

func TestNewPlugin_CleanupUsesInferenceServerConnection(t *testing.T) {
	deployment := gateDeployment()
	inferenceServer := withRenamedSecrets(t, deployment)
	ctrl := gomock.NewController(t)
	mockClientFactory := clientfactorymocks.NewMockClientFactory(ctrl)
	mockClientFactory.EXPECT().GetClient(gomock.Any(), tokenTagMatcher("remote-token-v2")).
		Return(nil, errors.New("stop after the client lookup"))
	plugin := NewPlugin(Params{
		Client:        newKubeClient(t, inferenceServer),
		ClientFactory: mockClientFactory,
		Logger:        zap.NewNop(),
	})

	condition, err := plugin.GetCleanupPlugin().GetActors()[0].Retrieve(context.Background(), deployment, &api.Condition{})

	require.NoError(t, err)
	assert.Equal(t, api.CONDITION_STATUS_FALSE, condition.Status)
}

func TestGetRollbackPlugin(t *testing.T) {
	plugin := &Plugin{client: newKubeClient(t), logger: zap.NewNop()}

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

	t.Run("cluster actors use the inference server's current connection", func(t *testing.T) {
		deployment := gateDeployment()
		inferenceServer := withRenamedSecrets(t, deployment)
		ctrl := gomock.NewController(t)
		mockClientFactory := clientfactorymocks.NewMockClientFactory(ctrl)
		mockClientFactory.EXPECT().GetClient(gomock.Any(), tokenTagMatcher("remote-token-v2")).
			Return(nil, errors.New("stop after the client lookup"))
		plugin := &Plugin{
			client:        newKubeClient(t, inferenceServer),
			clientFactory: mockClientFactory,
			logger:        zap.NewNop(),
		}

		rollbackPlugin, err := plugin.GetRollbackPlugin(context.Background(), deployment)
		require.NoError(t, err)
		condition, err := rollbackPlugin.GetActors()[0].Retrieve(context.Background(), deployment, &api.Condition{})

		require.NoError(t, err)
		assert.Equal(t, api.CONDITION_STATUS_FALSE, condition.Status)
	})

	t.Run("an unreadable inference server is an error", func(t *testing.T) {
		deployment := withSingleClusterAnnotation(t, gateDeployment(), "test-cluster")
		plugin := &Plugin{client: newFailingKubeClient(t), logger: zap.NewNop()}

		_, err := plugin.GetRollbackPlugin(context.Background(), deployment)

		require.ErrorContains(t, err, "cache not synced")
	})

	t.Run("an unreadable placement snapshot is an error", func(t *testing.T) {
		deployment := gateDeployment()
		deployment.Annotations = map[string]string{common.TargetClustersAnnotation: "{not json}"}

		_, err := plugin.GetRollbackPlugin(context.Background(), deployment)

		require.Error(t, err)
	})
}
