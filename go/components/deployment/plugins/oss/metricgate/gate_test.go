package metricgate

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// promServer stands in for the Prometheus HTTP API. handler receives the PromQL expression
// and returns the status code and body to send.
func promServer(t *testing.T, handler func(expr string) (int, string)) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/query", r.URL.Path)
		code, body := handler(r.URL.Query().Get("query"))
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

func vectorResponse(values ...string) string {
	entries := make([]string, 0, len(values))
	for _, value := range values {
		entries = append(entries, fmt.Sprintf(`{"metric":{"model":"m"},"value":[1700000000,"%s"]}`, value))
	}
	return fmt.Sprintf(`{"status":"success","data":{"resultType":"vector","result":[%s]}}`, strings.Join(entries, ","))
}

func testDeployment() *v2pb.Deployment {
	return &v2pb.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "dep", Namespace: "ns"},
		Spec: v2pb.DeploymentSpec{
			DesiredRevision: &apipb.ResourceIdentifier{Name: "model-v2"},
			Target: &v2pb.DeploymentSpec_InferenceServer{
				InferenceServer: &apipb.ResourceIdentifier{Name: "is"},
			},
		},
		Status: v2pb.DeploymentStatus{
			CurrentRevision:   &apipb.ResourceIdentifier{Name: "model-v1"},
			CandidateRevision: &apipb.ResourceIdentifier{Name: "model-v2"},
		},
	}
}

func TestNew(t *testing.T) {
	assert.Nil(t, New(maconfig.MetricGateConfig{}, zap.NewNop()), "no URL means no gate")

	var gate *Gate
	healthy, reason, err := gate.Evaluate(context.Background(), testDeployment())
	require.NoError(t, err)
	assert.True(t, healthy)
	assert.Empty(t, reason)
}

func TestEvaluate_DefaultQueryTargetsTheCandidate(t *testing.T) {
	var seen string
	server := promServer(t, func(expr string) (int, string) {
		seen = expr
		return http.StatusOK, vectorResponse("0.01")
	})
	gate := New(maconfig.MetricGateConfig{PrometheusURL: server.URL}, zap.NewNop())

	healthy, reason, err := gate.Evaluate(context.Background(), testDeployment())

	require.NoError(t, err)
	assert.True(t, healthy, reason)
	assert.Contains(t, seen, `nv_inference_request_failure{model="model-v2"}`)
}

func TestEvaluate_Thresholds(t *testing.T) {
	tests := []struct {
		name        string
		query       maconfig.MetricGateQuery
		body        string
		wantHealthy bool
		wantReason  string
	}{
		{
			name:        "sample above a gt threshold breaches",
			query:       maconfig.MetricGateQuery{Name: "errors", Expr: "up", Threshold: 0.05, Comparison: "gt"},
			body:        vectorResponse("0.01", "0.5"),
			wantHealthy: false,
			wantReason:  `metric gate "errors" breached for model model-v2: 0.5 gt 0.05`,
		},
		{
			name:        "samples at or below a gt threshold pass",
			query:       maconfig.MetricGateQuery{Name: "errors", Expr: "up", Threshold: 0.05},
			body:        vectorResponse("0.05", "0"),
			wantHealthy: true,
		},
		{
			name:        "sample below an lt threshold breaches",
			query:       maconfig.MetricGateQuery{Name: "success", Expr: "up", Threshold: 0.99, Comparison: "lt"},
			body:        vectorResponse("0.9"),
			wantHealthy: false,
			wantReason:  `"success" breached`,
		},
		{
			name:        "empty result passes",
			query:       maconfig.MetricGateQuery{Name: "errors", Expr: "up", Threshold: 0.05},
			body:        `{"status":"success","data":{"resultType":"vector","result":[]}}`,
			wantHealthy: true,
		},
		{
			name:        "NaN samples are ignored",
			query:       maconfig.MetricGateQuery{Name: "errors", Expr: "up", Threshold: 0.05},
			body:        vectorResponse("NaN"),
			wantHealthy: true,
		},
		{
			name:        "scalar results are read too",
			query:       maconfig.MetricGateQuery{Name: "errors", Expr: "scalar(up)", Threshold: 0.05},
			body:        `{"status":"success","data":{"resultType":"scalar","result":[1700000000,"0.2"]}}`,
			wantHealthy: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := promServer(t, func(string) (int, string) { return http.StatusOK, tt.body })
			gate := New(maconfig.MetricGateConfig{PrometheusURL: server.URL, Queries: []maconfig.MetricGateQuery{tt.query}}, zap.NewNop())

			healthy, reason, err := gate.Evaluate(context.Background(), testDeployment())

			require.NoError(t, err)
			assert.Equal(t, tt.wantHealthy, healthy, reason)
			if tt.wantReason != "" {
				assert.Contains(t, reason, tt.wantReason)
			}
		})
	}
}

func TestEvaluate_QueryFailures(t *testing.T) {
	tests := []struct {
		name        string
		code        int
		body        string
		failClosed  bool
		wantHealthy bool
	}{
		{name: "HTTP error fails open by default", code: http.StatusInternalServerError, body: "boom", wantHealthy: true},
		{name: "HTTP error fails closed when configured", code: http.StatusInternalServerError, body: "boom", failClosed: true, wantHealthy: false},
		{name: "prometheus error status fails open", code: http.StatusOK, body: `{"status":"error","errorType":"bad_data","error":"parse error"}`, wantHealthy: true},
		{name: "prometheus error status fails closed when configured", code: http.StatusOK, body: `{"status":"error","errorType":"bad_data","error":"parse error"}`, failClosed: true, wantHealthy: false},
		{name: "malformed body fails closed when configured", code: http.StatusOK, body: `not json`, failClosed: true, wantHealthy: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := promServer(t, func(string) (int, string) { return tt.code, tt.body })
			gate := New(maconfig.MetricGateConfig{PrometheusURL: server.URL, FailClosed: tt.failClosed}, zap.NewNop())

			healthy, reason, err := gate.Evaluate(context.Background(), testDeployment())

			require.NoError(t, err)
			assert.Equal(t, tt.wantHealthy, healthy)
			if !tt.wantHealthy {
				assert.Contains(t, reason, "fails closed")
			}
		})
	}
}

func TestEvaluate_UnreachablePrometheus(t *testing.T) {
	server := promServer(t, func(string) (int, string) { return http.StatusOK, vectorResponse("0") })
	url := server.URL
	server.Close()

	open := New(maconfig.MetricGateConfig{PrometheusURL: url, Timeout: time.Second}, zap.NewNop())
	healthy, _, err := open.Evaluate(context.Background(), testDeployment())
	require.NoError(t, err)
	assert.True(t, healthy)

	closed := New(maconfig.MetricGateConfig{PrometheusURL: url, Timeout: time.Second, FailClosed: true}, zap.NewNop())
	healthy, reason, err := closed.Evaluate(context.Background(), testDeployment())
	require.NoError(t, err)
	assert.False(t, healthy)
	assert.Contains(t, reason, "could not be evaluated")
}

func TestEvaluate_TemplateErrors(t *testing.T) {
	server := promServer(t, func(string) (int, string) { return http.StatusOK, vectorResponse("0") })
	gate := New(maconfig.MetricGateConfig{
		PrometheusURL: server.URL,
		Queries:       []maconfig.MetricGateQuery{{Name: "broken", Expr: "up{model={{.Nope}}}"}},
	}, zap.NewNop())

	_, _, err := gate.Evaluate(context.Background(), testDeployment())

	require.Error(t, err)
	assert.Contains(t, err.Error(), `"broken"`)
}

func TestEvaluate_NoModelSkipsQueries(t *testing.T) {
	server := promServer(t, func(string) (int, string) {
		t.Fatal("prometheus must not be queried without a model")
		return http.StatusOK, ""
	})
	gate := New(maconfig.MetricGateConfig{PrometheusURL: server.URL}, zap.NewNop())

	healthy, _, err := gate.Evaluate(context.Background(), &v2pb.Deployment{})

	require.NoError(t, err)
	assert.True(t, healthy)
}

func TestEvaluate_QueriesRunInOrderUntilTheFirstBreach(t *testing.T) {
	var exprs []string
	server := promServer(t, func(expr string) (int, string) {
		exprs = append(exprs, expr)
		if expr == "second" {
			return http.StatusOK, vectorResponse("1")
		}
		return http.StatusOK, vectorResponse("0")
	})
	gate := New(maconfig.MetricGateConfig{
		PrometheusURL: server.URL,
		Queries: []maconfig.MetricGateQuery{
			{Name: "first", Expr: "first", Threshold: 0.5},
			{Name: "second", Expr: "second", Threshold: 0.5},
			{Name: "third", Expr: "third", Threshold: 0.5},
		},
	}, zap.NewNop())

	healthy, reason, err := gate.Evaluate(context.Background(), testDeployment())

	require.NoError(t, err)
	assert.False(t, healthy)
	assert.Contains(t, reason, `"second"`)
	assert.Equal(t, []string{"first", "second"}, exprs)
}
