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
	"slices"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"go.uber.org/zap"

	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	defaultTimeout = 10 * time.Second
	// longTimeout is the per-query timeout above which New warns. An evaluation can take one
	// timeout, and it shares the controller's 60s reconcile deadline with the rollout or
	// rollback step that acts on the verdict.
	longTimeout = 30 * time.Second

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
	queries := namedQueries(cfg.Queries)
	if len(queries) == 0 {
		queries = []maconfig.MetricGateQuery{DefaultQuery()}
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	reportMisconfiguration(logger, timeout, queries)
	return &Gate{
		prometheusURL: strings.TrimRight(cfg.PrometheusURL, "/"),
		failClosed:    cfg.FailClosed,
		queries:       queries,
		client:        &http.Client{Timeout: timeout, Transport: transportFor(len(queries))},
		logger:        logger,
	}
}

// transportFor returns a transport that keeps one idle connection per query. The queries of
// an evaluation are sent at the same time, and the default transport keeps only two idle
// connections per host, so every further query would open a new connection on each
// evaluation.
func transportFor(queries int) http.RoundTripper {
	defaults, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return http.DefaultTransport
	}
	transport := defaults.Clone()
	transport.MaxIdleConnsPerHost = max(http.DefaultMaxIdleConnsPerHost, queries)
	return transport
}

// reportMisconfiguration logs, once at startup, the configuration mistakes that would
// otherwise only show up during a rollout.
func reportMisconfiguration(logger *zap.Logger, timeout time.Duration, queries []maconfig.MetricGateQuery) {
	if timeout > longTimeout {
		logger.Warn("metric gate timeout can take most of the reconcile budget; lower it",
			zap.Duration("timeout", timeout), zap.Duration("recommended_max", longTimeout))
	}
	sample := QueryScope{
		Model:           "model",
		Deployment:      "deployment",
		Namespace:       "namespace",
		InferenceServer: "server",
	}
	seen := make(map[string]bool, len(queries))
	for _, query := range queries {
		if seen[query.Name] {
			logger.Warn("metric gate query names are not unique; "+
				"the no-data annotation and rollback reasons will be ambiguous",
				zap.String("query", query.Name))
		}
		seen[query.Name] = true
		if _, err := render(query, sample); err != nil {
			logger.Error("metric gate query template does not render with sample values; "+
				"evaluations will fail until it is fixed",
				zap.String("query", query.Name), zap.Error(err))
		}
	}
}

// namedQueries copies the configured queries and names each unnamed one after its 1-based
// position, so that logs, rollback reasons and Result.NoData identify it the same way.
func namedQueries(configured []maconfig.MetricGateQuery) []maconfig.MetricGateQuery {
	queries := slices.Clone(configured)
	for i := range queries {
		if queries[i].Name == "" {
			queries[i].Name = fmt.Sprintf("unnamed query %d", i+1)
		}
	}
	return queries
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

// Result is the outcome of one evaluation of the gate.
type Result struct {
	// Healthy is false when a sample breached its threshold or, if the gate fails closed, a
	// query failed.
	Healthy bool
	// Reason describes the first thing that made the gate unhealthy. It is empty when Healthy
	// is true.
	Reason string
	// Model is the model the queries were evaluated for. It is empty when the gate is
	// disabled or the deployment names no model.
	Model string
	// NoData names the queries that returned no samples; a result of only NaN counts as no
	// samples. Such a query cannot breach, so the model was not judged on it. When
	// AllQueriesAnswered is false, the list may omit queries that were not answered.
	NoData []string
	// AllQueriesAnswered is false when a query failed, and true otherwise, including when
	// the gate is disabled or the deployment names no model and so no query runs.
	AllQueriesAnswered bool
}

// fail marks the result unhealthy. Only the first reason is kept.
func (r *Result) fail(reason string) {
	if !r.Healthy {
		return
	}
	r.Healthy = false
	r.Reason = reason
}

// Evaluate runs every query for the model the deployment is rolling out. The queries are sent
// to Prometheus at the same time and judged in configuration order. The result is healthy
// when no sample breaches its threshold. A breach does not hide the other queries, so
// Result.NoData still covers them. A query that returns no samples, or only NaN, does not
// breach; it is named in Result.NoData. A query that fails is logged and skipped, unless the
// gate is configured to fail closed: then it makes the result unhealthy. A malformed query
// template is returned as an error before any query is sent.
func (g *Gate) Evaluate(ctx context.Context, deployment *v2pb.Deployment) (Result, error) {
	if g == nil {
		return Result{Healthy: true, AllQueriesAnswered: true}, nil
	}
	scope := scopeFor(deployment)
	if scope.Model == "" {
		return Result{Healthy: true, AllQueriesAnswered: true}, nil
	}
	exprs, err := g.renderAll(scope)
	if err != nil {
		return Result{}, err
	}
	answers := g.queryAll(ctx, exprs)

	result := Result{Healthy: true, Model: scope.Model, AllQueriesAnswered: true}
	for i, query := range g.queries {
		log := g.logger.With(
			zap.String("query", query.Name),
			zap.String("namespace", deployment.GetNamespace()),
			zap.String("deployment", deployment.GetName()),
			zap.String("model", scope.Model))
		samples, err := answers[i].samples, answers[i].err
		if err != nil {
			result.AllQueriesAnswered = false
			if g.failClosed {
				result.fail(fmt.Sprintf("metric gate %q could not be evaluated and the gate fails closed: %v",
					query.Name, err))
				continue
			}
			log.Warn("metric gate query failed; skipping it because the gate fails open", zap.Error(err))
			continue
		}
		if !slices.ContainsFunc(samples, isNumber) {
			log.Debug("metric gate query returned no samples",
				zap.String("expr", exprs[i]), zap.Int("nan_samples", len(samples)))
			result.NoData = append(result.NoData, query.Name)
			continue
		}
		if sample, breached := firstBreach(samples, query); breached {
			result.fail(fmt.Sprintf("metric gate %q breached for model %s: %g %s %g",
				query.Name, scope.Model, sample, comparisonOf(query), query.Threshold))
		}
	}
	return result, nil
}

// renderAll renders every query's expression. Rendering them all before any is sent makes a
// malformed template an error whatever the other queries return.
func (g *Gate) renderAll(scope QueryScope) ([]string, error) {
	exprs := make([]string, len(g.queries))
	for i, query := range g.queries {
		expr, err := render(query, scope)
		if err != nil {
			return nil, err
		}
		exprs[i] = expr
	}
	return exprs, nil
}

// answer is what Prometheus returned for one expression.
type answer struct {
	samples []float64
	err     error
}

// queryAll sends every expression to Prometheus at the same time, so that an evaluation
// takes about one timeout however many queries are configured. The caller acts on the
// verdict under the same deadline. answers[i] belongs to exprs[i].
func (g *Gate) queryAll(ctx context.Context, exprs []string) []answer {
	answers := make([]answer, len(exprs))
	var wg sync.WaitGroup
	for i, expr := range exprs {
		wg.Go(func() {
			// A panic here would end the process: the reconciler recovers panics only on
			// its own goroutine.
			defer func() {
				if r := recover(); r != nil {
					answers[i] = answer{err: fmt.Errorf("metric gate query panicked: %v", r)}
				}
			}()
			samples, err := g.query(ctx, expr)
			answers[i] = answer{samples: samples, err: err}
		})
	}
	wg.Wait()
	return answers
}

func firstBreach(samples []float64, query maconfig.MetricGateQuery) (float64, bool) {
	for _, sample := range samples {
		if breaches(sample, query) {
			return sample, true
		}
	}
	return 0, false
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

// isNumber reports whether a sample can be compared against a threshold. NaN cannot: it is
// what zero divided by zero yields, for example a failure ratio before any traffic.
func isNumber(sample float64) bool {
	return !math.IsNaN(sample)
}

func breaches(sample float64, query maconfig.MetricGateQuery) bool {
	if !isNumber(sample) {
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
