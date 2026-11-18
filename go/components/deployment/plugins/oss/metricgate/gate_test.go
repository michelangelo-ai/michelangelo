package metricgate

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
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
	result, err := gate.Evaluate(context.Background(), testDeployment())
	require.NoError(t, err)
	assert.Equal(t, Result{Healthy: true, AllQueriesAnswered: true}, result)
}

func TestEvaluate_DefaultQueryTargetsTheCandidate(t *testing.T) {
	var seen string
	server := promServer(t, func(expr string) (int, string) {
		seen = expr
		return http.StatusOK, vectorResponse("0.01")
	})
	gate := New(maconfig.MetricGateConfig{PrometheusURL: server.URL}, zap.NewNop())

	result, err := gate.Evaluate(context.Background(), testDeployment())

	require.NoError(t, err)
	assert.True(t, result.Healthy, result.Reason)
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
			name:        "a breaching sample beside NaN breaches",
			query:       maconfig.MetricGateQuery{Name: "errors", Expr: "up", Threshold: 0.05},
			body:        vectorResponse("1", "NaN"),
			wantHealthy: false,
		},
		{
			name:        "+Inf above a gt threshold breaches",
			query:       maconfig.MetricGateQuery{Name: "errors", Expr: "up", Threshold: 0.05},
			body:        vectorResponse("+Inf"),
			wantHealthy: false,
		},
		{
			name:        "-Inf below a gt threshold passes",
			query:       maconfig.MetricGateQuery{Name: "errors", Expr: "up", Threshold: 0.05},
			body:        vectorResponse("-Inf"),
			wantHealthy: true,
		},
		{
			name:        "-Inf below an lt threshold breaches",
			query:       maconfig.MetricGateQuery{Name: "success", Expr: "up", Threshold: 0.99, Comparison: "lt"},
			body:        vectorResponse("-Inf"),
			wantHealthy: false,
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

			result, err := gate.Evaluate(context.Background(), testDeployment())

			require.NoError(t, err)
			assert.Equal(t, tt.wantHealthy, result.Healthy, result.Reason)
			assert.Empty(t, result.NoData, "a result with a number in it has samples")
			if tt.wantReason != "" {
				assert.Contains(t, result.Reason, tt.wantReason)
			}
		})
	}
}

func TestEvaluate_NoData(t *testing.T) {
	emptyVector := vectorResponse()
	query := func(name string) maconfig.MetricGateQuery {
		return maconfig.MetricGateQuery{Name: name, Expr: name, Threshold: 0.5}
	}
	unnamed := func(expr string) maconfig.MetricGateQuery {
		return maconfig.MetricGateQuery{Expr: expr, Threshold: 0.5}
	}
	below := func(name string) maconfig.MetricGateQuery {
		return maconfig.MetricGateQuery{Name: name, Expr: name, Threshold: 0.5, Comparison: ComparisonLess}
	}

	tests := []struct {
		name            string
		queries         []maconfig.MetricGateQuery
		bodies          map[string]string // response per expression; an unlisted expression returns one sample of 0
		failing         []string          // expressions answered with HTTP 500
		failClosed      bool
		wantHealthy     bool
		wantNoData      []string
		wantAllAnswered bool
	}{
		{
			name:            "an empty vector passes and is reported",
			queries:         []maconfig.MetricGateQuery{query("errors")},
			bodies:          map[string]string{"errors": emptyVector},
			wantHealthy:     true,
			wantNoData:      []string{"errors"},
			wantAllAnswered: true,
		},
		{
			name:            "a null result passes and is reported",
			queries:         []maconfig.MetricGateQuery{query("errors")},
			bodies:          map[string]string{"errors": `{"status":"success","data":{"resultType":"vector","result":null}}`},
			wantHealthy:     true,
			wantNoData:      []string{"errors"},
			wantAllAnswered: true,
		},
		{
			name:            "a query that returns a sample is not reported",
			queries:         []maconfig.MetricGateQuery{query("errors")},
			wantHealthy:     true,
			wantAllAnswered: true,
		},
		{
			name:            "a result of only NaN passes and is reported",
			queries:         []maconfig.MetricGateQuery{query("errors")},
			bodies:          map[string]string{"errors": vectorResponse("NaN", "NaN")},
			wantHealthy:     true,
			wantNoData:      []string{"errors"},
			wantAllAnswered: true,
		},
		{
			name:            "NaN before a number is not reported",
			queries:         []maconfig.MetricGateQuery{query("errors")},
			bodies:          map[string]string{"errors": vectorResponse("NaN", "0.1")},
			wantHealthy:     true,
			wantAllAnswered: true,
		},
		{
			name:            "NaN after a number is not reported",
			queries:         []maconfig.MetricGateQuery{query("errors")},
			bodies:          map[string]string{"errors": vectorResponse("0.1", "NaN")},
			wantHealthy:     true,
			wantAllAnswered: true,
		},
		{
			name:            "an lt query that returns no samples is reported too",
			queries:         []maconfig.MetricGateQuery{below("empty"), below("nan")},
			bodies:          map[string]string{"empty": emptyVector, "nan": vectorResponse("NaN")},
			wantHealthy:     true,
			wantNoData:      []string{"empty", "nan"},
			wantAllAnswered: true,
		},
		{
			name:            "every query that returns no samples is named, in order",
			queries:         []maconfig.MetricGateQuery{query("first"), query("second"), query("third")},
			bodies:          map[string]string{"first": emptyVector, "third": emptyVector},
			wantHealthy:     true,
			wantNoData:      []string{"first", "third"},
			wantAllAnswered: true,
		},
		{
			name:            "a query that returns no samples does not breach when the gate fails closed",
			queries:         []maconfig.MetricGateQuery{query("errors")},
			bodies:          map[string]string{"errors": emptyVector},
			failClosed:      true,
			wantHealthy:     true,
			wantNoData:      []string{"errors"},
			wantAllAnswered: true,
		},
		{
			name:            "an unnamed query is reported by its position among all queries",
			queries:         []maconfig.MetricGateQuery{unnamed("a"), query("named"), unnamed("c")},
			bodies:          map[string]string{"a": emptyVector, "named": emptyVector, "c": emptyVector},
			wantHealthy:     true,
			wantNoData:      []string{"unnamed query 1", "named", "unnamed query 3"},
			wantAllAnswered: true,
		},
		{
			name:            "a breach does not stop later queries from being reported",
			queries:         []maconfig.MetricGateQuery{query("first"), query("second"), query("third")},
			bodies:          map[string]string{"first": emptyVector, "second": vectorResponse("1"), "third": emptyVector},
			wantHealthy:     false,
			wantNoData:      []string{"first", "third"},
			wantAllAnswered: true,
		},
		{
			name:        "a failed query that is skipped leaves a query unanswered",
			queries:     []maconfig.MetricGateQuery{query("first"), query("second")},
			bodies:      map[string]string{"first": emptyVector},
			failing:     []string{"second"},
			wantHealthy: true,
			wantNoData:  []string{"first"},
		},
		{
			name:        "a failed query that is skipped does not stop later queries",
			queries:     []maconfig.MetricGateQuery{query("first"), query("second")},
			bodies:      map[string]string{"second": vectorResponse("1")},
			failing:     []string{"first"},
			wantHealthy: false,
		},
		{
			name:        "a failed query leaves a query unanswered when the gate fails closed",
			queries:     []maconfig.MetricGateQuery{query("first"), query("second")},
			bodies:      map[string]string{"first": emptyVector},
			failing:     []string{"second"},
			failClosed:  true,
			wantHealthy: false,
			wantNoData:  []string{"first"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := promServer(t, func(expr string) (int, string) {
				if slices.Contains(tt.failing, expr) {
					return http.StatusInternalServerError, "boom"
				}
				if body, ok := tt.bodies[expr]; ok {
					return http.StatusOK, body
				}
				return http.StatusOK, vectorResponse("0")
			})
			gate := New(maconfig.MetricGateConfig{
				PrometheusURL: server.URL,
				FailClosed:    tt.failClosed,
				Queries:       tt.queries,
			}, zap.NewNop())

			result, err := gate.Evaluate(context.Background(), testDeployment())

			require.NoError(t, err)
			assert.Equal(t, tt.wantHealthy, result.Healthy, result.Reason)
			assert.Equal(t, tt.wantNoData, result.NoData)
			assert.Equal(t, tt.wantAllAnswered, result.AllQueriesAnswered)
			assert.Equal(t, "model-v2", result.Model)
		})
	}
}

func TestEvaluate_QueryWithoutSamples_IsLoggedAtDebugWithItsExpression(t *testing.T) {
	server := promServer(t, func(string) (int, string) { return http.StatusOK, vectorResponse("NaN") })
	core, logs := observer.New(zap.DebugLevel)
	gate := New(maconfig.MetricGateConfig{
		PrometheusURL: server.URL,
		Queries:       []maconfig.MetricGateQuery{{Name: "errors", Expr: `up{model="{{.Model}}"}`}},
	}, zap.New(core))

	_, err := gate.Evaluate(context.Background(), testDeployment())

	require.NoError(t, err)
	require.Equal(t, 1, logs.Len())
	entry := logs.All()[0]
	assert.Equal(t, zap.DebugLevel, entry.Level)
	assert.Equal(t, map[string]interface{}{
		"query":       "errors",
		"namespace":   "ns",
		"deployment":  "dep",
		"model":       "model-v2",
		"expr":        `up{model="model-v2"}`,
		"nan_samples": int64(1),
	}, entry.ContextMap())
}

func TestEvaluate_Model(t *testing.T) {
	tests := []struct {
		name      string
		candidate *apipb.ResourceIdentifier
		want      string
	}{
		{name: "the candidate is the model evaluated", candidate: &apipb.ResourceIdentifier{Name: "candidate"}, want: "candidate"},
		{name: "the desired revision is evaluated before a candidate is recorded", want: "desired"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seen string
			server := promServer(t, func(expr string) (int, string) {
				seen = expr
				return http.StatusOK, vectorResponse("0")
			})
			gate := New(maconfig.MetricGateConfig{
				PrometheusURL: server.URL,
				Queries:       []maconfig.MetricGateQuery{{Name: "errors", Expr: "{{.Model}}"}},
			}, zap.NewNop())
			deployment := testDeployment()
			deployment.Spec.DesiredRevision = &apipb.ResourceIdentifier{Name: "desired"}
			deployment.Status.CandidateRevision = tt.candidate

			result, err := gate.Evaluate(context.Background(), deployment)

			require.NoError(t, err)
			assert.Equal(t, tt.want, result.Model)
			assert.Equal(t, tt.want, seen)
		})
	}
}

func TestEvaluate_UnnamedQuery_IsNamedByPositionInReasons(t *testing.T) {
	server := promServer(t, func(string) (int, string) { return http.StatusOK, vectorResponse("1") })
	queries := []maconfig.MetricGateQuery{{Expr: "up", Threshold: 0.5}}
	gate := New(maconfig.MetricGateConfig{PrometheusURL: server.URL, Queries: queries}, zap.NewNop())

	result, err := gate.Evaluate(context.Background(), testDeployment())

	require.NoError(t, err)
	assert.Contains(t, result.Reason, `metric gate "unnamed query 1" breached`)
	assert.Empty(t, queries[0].Name, "the caller's configuration must not be modified")
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

			result, err := gate.Evaluate(context.Background(), testDeployment())

			require.NoError(t, err)
			assert.Equal(t, tt.wantHealthy, result.Healthy)
			assert.Empty(t, result.NoData, "a failed query is not one that returned no samples")
			assert.False(t, result.AllQueriesAnswered)
			if !tt.wantHealthy {
				assert.Contains(t, result.Reason, "fails closed")
			}
		})
	}
}

func TestEvaluate_UnreachablePrometheus(t *testing.T) {
	server := promServer(t, func(string) (int, string) { return http.StatusOK, vectorResponse("0") })
	url := server.URL
	server.Close()

	open := New(maconfig.MetricGateConfig{PrometheusURL: url, Timeout: time.Second}, zap.NewNop())
	result, err := open.Evaluate(context.Background(), testDeployment())
	require.NoError(t, err)
	assert.True(t, result.Healthy)

	closed := New(maconfig.MetricGateConfig{PrometheusURL: url, Timeout: time.Second, FailClosed: true}, zap.NewNop())
	result, err = closed.Evaluate(context.Background(), testDeployment())
	require.NoError(t, err)
	assert.False(t, result.Healthy)
	assert.Contains(t, result.Reason, "could not be evaluated")
}

func TestEvaluate_TemplateErrors(t *testing.T) {
	server := promServer(t, func(string) (int, string) { return http.StatusOK, vectorResponse("0") })
	gate := New(maconfig.MetricGateConfig{
		PrometheusURL: server.URL,
		Queries:       []maconfig.MetricGateQuery{{Name: "broken", Expr: "up{model={{.Nope}}}"}},
	}, zap.NewNop())

	_, err := gate.Evaluate(context.Background(), testDeployment())

	require.Error(t, err)
	assert.Contains(t, err.Error(), `"broken"`)
}

func TestEvaluate_TemplateErrorAfterAQueryWithoutSamples_IsReturned(t *testing.T) {
	server := promServer(t, func(string) (int, string) { return http.StatusOK, vectorResponse() })
	gate := New(maconfig.MetricGateConfig{
		PrometheusURL: server.URL,
		Queries: []maconfig.MetricGateQuery{
			{Name: "first", Expr: "up"},
			{Name: "broken", Expr: "up{model={{.Nope}}}"},
		},
	}, zap.NewNop())

	_, err := gate.Evaluate(context.Background(), testDeployment())

	require.ErrorContains(t, err, `"broken"`)
}

func TestEvaluate_TemplateErrorAfterABreach_KeepsTheBreach(t *testing.T) {
	server := promServer(t, func(string) (int, string) { return http.StatusOK, vectorResponse("1") })
	gate := New(maconfig.MetricGateConfig{
		PrometheusURL: server.URL,
		Queries: []maconfig.MetricGateQuery{
			{Name: "first", Expr: "up", Threshold: 0.5},
			{Name: "broken", Expr: "up{model={{.Nope}}}"},
		},
	}, zap.NewNop())

	result, err := gate.Evaluate(context.Background(), testDeployment())

	require.NoError(t, err, "an error would keep the controller from rolling the candidate back")
	assert.False(t, result.Healthy)
	assert.Contains(t, result.Reason, `"first" breached`)
	assert.False(t, result.AllQueriesAnswered)
}

func TestEvaluate_NoModelSkipsQueries(t *testing.T) {
	server := promServer(t, func(string) (int, string) {
		t.Fatal("prometheus must not be queried without a model")
		return http.StatusOK, ""
	})
	gate := New(maconfig.MetricGateConfig{PrometheusURL: server.URL}, zap.NewNop())

	result, err := gate.Evaluate(context.Background(), &v2pb.Deployment{})

	require.NoError(t, err)
	assert.Equal(t, Result{Healthy: true, AllQueriesAnswered: true}, result)
}

func TestEvaluate_EveryQueryRuns_AndTheReasonNamesTheFirstBreach(t *testing.T) {
	var exprs []string
	server := promServer(t, func(expr string) (int, string) {
		exprs = append(exprs, expr)
		if expr == "first" {
			return http.StatusOK, vectorResponse("0")
		}
		return http.StatusOK, vectorResponse("1")
	})
	gate := New(maconfig.MetricGateConfig{
		PrometheusURL: server.URL,
		Queries: []maconfig.MetricGateQuery{
			{Name: "first", Expr: "first", Threshold: 0.5},
			{Name: "second", Expr: "second", Threshold: 0.5},
			{Name: "third", Expr: "third", Threshold: 0.5},
		},
	}, zap.NewNop())

	result, err := gate.Evaluate(context.Background(), testDeployment())

	require.NoError(t, err)
	assert.False(t, result.Healthy)
	assert.Contains(t, result.Reason, `"second"`)
	assert.Equal(t, []string{"first", "second", "third"}, exprs)
}

func TestEvaluate_FailedQuery_EndsTheEvaluationWhenTheGateFailsClosed(t *testing.T) {
	var exprs []string
	server := promServer(t, func(expr string) (int, string) {
		exprs = append(exprs, expr)
		return http.StatusInternalServerError, "boom"
	})
	gate := New(maconfig.MetricGateConfig{
		PrometheusURL: server.URL,
		FailClosed:    true,
		Queries: []maconfig.MetricGateQuery{
			{Name: "first", Expr: "first", Threshold: 0.5},
			{Name: "second", Expr: "second", Threshold: 0.5},
		},
	}, zap.NewNop())

	result, err := gate.Evaluate(context.Background(), testDeployment())

	require.NoError(t, err)
	assert.False(t, result.Healthy)
	assert.Contains(t, result.Reason, `"first"`)
	assert.Equal(t, []string{"first"}, exprs)
}
