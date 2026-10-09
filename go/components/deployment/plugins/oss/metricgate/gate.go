// Package metricgate evaluates Prometheus queries against the model being rolled out and
// reports whether any of them breaches its threshold. The deployment plugin folds the result
// into its health check gate, so a model that loads fine but misbehaves under traffic is
// rolled back while it is still confined to the clusters the rollout has reached.
package metricgate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"text/template"
	"time"

	"go.uber.org/zap"

	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	defaultTimeout = 10 * time.Second

	// ComparisonGreater marks a sample above the threshold as a breach.
	ComparisonGreater = "gt"
	// ComparisonLess marks a sample below the threshold as a breach.
	ComparisonLess = "lt"

	defaultQueryName = "triton-inference-failure-ratio"
	// defaultQueryExpr is the share of Triton inference requests for the model that failed
	// over the last five minutes. clamp_min keeps the ratio at zero when there is no traffic.
	defaultQueryExpr = `sum(rate(nv_inference_request_failure{model="{{.Model}}"}[5m]))` +
		` / clamp_min(sum(rate(nv_inference_request_success{model="{{.Model}}"}[5m]))` +
		` + sum(rate(nv_inference_request_failure{model="{{.Model}}"}[5m])), 1)`
	defaultThreshold = 0.05

	maxResponseBytes = 1 << 20
)

// QueryScope is the data available to query templates.
type QueryScope struct {
	Model           string
	Deployment      string
	Namespace       string
	InferenceServer string
}

// Gate evaluates the configured queries against Prometheus.
type Gate struct {
	prometheusURL string
	failClosed    bool
	queries       []maconfig.MetricGateQuery
	client        *http.Client
	logger        *zap.Logger
}

// New builds a gate from configuration. It returns nil, meaning "no gate", when no Prometheus
// URL is configured; a nil *Gate is safe to call.
func New(cfg maconfig.MetricGateConfig, logger *zap.Logger) *Gate {
	if strings.TrimSpace(cfg.PrometheusURL) == "" {
		return nil
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	queries := cfg.Queries
	if len(queries) == 0 {
		queries = []maconfig.MetricGateQuery{DefaultQuery()}
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Gate{
		prometheusURL: strings.TrimRight(cfg.PrometheusURL, "/"),
		failClosed:    cfg.FailClosed,
		queries:       queries,
		client:        &http.Client{Timeout: timeout},
		logger:        logger,
	}
}

// DefaultQuery is the query used when none is configured: the model's inference failure
// ratio over five minutes, breaching above five percent.
func DefaultQuery() maconfig.MetricGateQuery {
	return maconfig.MetricGateQuery{
		Name:       defaultQueryName,
		Expr:       defaultQueryExpr,
		Threshold:  defaultThreshold,
		Comparison: ComparisonGreater,
	}
}

// Evaluate runs every query for the model the deployment is rolling out. It returns healthy
// when no sample breaches its threshold. reason describes the first breach. An unreachable
// Prometheus counts as a breach only when the gate is configured to fail closed; otherwise it
// is logged and skipped. A malformed query template is returned as err.
func (g *Gate) Evaluate(ctx context.Context, deployment *v2pb.Deployment) (healthy bool, reason string, err error) {
	if g == nil {
		return true, "", nil
	}
	scope := scopeFor(deployment)
	if scope.Model == "" {
		return true, "", nil
	}

	for _, query := range g.queries {
		expr, err := render(query, scope)
		if err != nil {
			return false, "", err
		}
		samples, err := g.query(ctx, expr)
		if err != nil {
			if g.failClosed {
				return false, fmt.Sprintf("metric gate %q could not be evaluated and the gate fails closed: %v", query.Name, err), nil
			}
			g.logger.Warn("metric gate query failed; skipping it because the gate fails open",
				zap.String("query", query.Name), zap.String("deployment", deployment.GetName()), zap.Error(err))
			continue
		}
		for _, sample := range samples {
			if breaches(sample, query) {
				return false, fmt.Sprintf("metric gate %q breached for model %s: %g %s %g", query.Name, scope.Model, sample, comparisonOf(query), query.Threshold), nil
			}
		}
	}
	return true, "", nil
}

// scopeFor names the model under evaluation: the candidate being rolled out, or the desired
// revision before a candidate is recorded.
func scopeFor(deployment *v2pb.Deployment) QueryScope {
	model := deployment.Status.GetCandidateRevision().GetName()
	if model == "" {
		model = deployment.Spec.GetDesiredRevision().GetName()
	}
	return QueryScope{
		Model:           model,
		Deployment:      deployment.GetName(),
		Namespace:       deployment.GetNamespace(),
		InferenceServer: deployment.Spec.GetInferenceServer().GetName(),
	}
}

func render(query maconfig.MetricGateQuery, scope QueryScope) (string, error) {
	tmpl, err := template.New(query.Name).Option("missingkey=error").Parse(query.Expr)
	if err != nil {
		return "", fmt.Errorf("parse metric gate query %q: %w", query.Name, err)
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, scope); err != nil {
		return "", fmt.Errorf("render metric gate query %q: %w", query.Name, err)
	}
	return out.String(), nil
}

func breaches(sample float64, query maconfig.MetricGateQuery) bool {
	if math.IsNaN(sample) {
		return false
	}
	if comparisonOf(query) == ComparisonLess {
		return sample < query.Threshold
	}
	return sample > query.Threshold
}

func comparisonOf(query maconfig.MetricGateQuery) string {
	if strings.EqualFold(query.Comparison, ComparisonLess) {
		return ComparisonLess
	}
	return ComparisonGreater
}

type promResponse struct {
	Status    string `json:"status"`
	ErrorType string `json:"errorType"`
	Error     string `json:"error"`
	Data      struct {
		ResultType string          `json:"resultType"`
		Result     json.RawMessage `json:"result"`
	} `json:"data"`
}

// query runs an instant query and returns every sample value in the result.
func (g *Gate) query(ctx context.Context, expr string) ([]float64, error) {
	endpoint := g.prometheusURL + "/api/v1/query?query=" + url.QueryEscape(expr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build prometheus request: %w", err)
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query prometheus: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read prometheus response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prometheus returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed promResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode prometheus response: %w", err)
	}
	if parsed.Status != "success" {
		return nil, fmt.Errorf("prometheus query failed: %s %s", parsed.ErrorType, parsed.Error)
	}
	return parseSamples(parsed.Data.ResultType, parsed.Data.Result)
}

// parseSamples extracts the numeric values of a vector or scalar result. Values Prometheus
// renders as NaN or Inf are kept as such; breaches ignores NaN.
func parseSamples(resultType string, raw json.RawMessage) ([]float64, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if resultType == "scalar" {
		var scalar []json.RawMessage
		if err := json.Unmarshal(raw, &scalar); err != nil {
			return nil, fmt.Errorf("decode scalar result: %w", err)
		}
		value, err := sampleValue(scalar)
		if err != nil {
			return nil, err
		}
		return []float64{value}, nil
	}
	var vector []struct {
		Value []json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(raw, &vector); err != nil {
		return nil, fmt.Errorf("decode %s result: %w", resultType, err)
	}
	samples := make([]float64, 0, len(vector))
	for _, entry := range vector {
		value, err := sampleValue(entry.Value)
		if err != nil {
			return nil, err
		}
		samples = append(samples, value)
	}
	return samples, nil
}

// sampleValue decodes the [timestamp, "value"] pair Prometheus uses for samples.
func sampleValue(pair []json.RawMessage) (float64, error) {
	if len(pair) != 2 {
		return 0, fmt.Errorf("malformed sample: expected [timestamp, value], got %d elements", len(pair))
	}
	var text string
	if err := json.Unmarshal(pair[1], &text); err != nil {
		return 0, fmt.Errorf("decode sample value %s: %w", string(pair[1]), err)
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, fmt.Errorf("parse sample value %q: %w", text, err)
	}
	return value, nil
}
