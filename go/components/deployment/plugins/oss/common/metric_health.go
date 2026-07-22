package common

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"go.uber.org/zap"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// MetricHealthChecker evaluates a list of MetricHealthRule entries against a
// Prometheus endpoint. It is stateless and safe to construct on every
// HealthCheckGate call.
type MetricHealthChecker struct {
	queryAPI promv1.API
	logger   *zap.Logger
}

// NewMetricHealthChecker creates a checker that queries the given Prometheus URL.
// Returns an error only if the URL is syntactically invalid; network failures
// are deferred to IsHealthy and treated as fail-open.
func NewMetricHealthChecker(prometheusURL string, logger *zap.Logger) (*MetricHealthChecker, error) {
	client, err := api.NewClient(api.Config{Address: prometheusURL})
	if err != nil {
		return nil, fmt.Errorf("create prometheus client for %s: %w", prometheusURL, err)
	}
	return &MetricHealthChecker{
		queryAPI: promv1.NewAPI(client),
		logger:   logger,
	}, nil
}

// IsHealthy evaluates each rule in order. Returns (false, ruleName) on the
// first breached rule, (true, "") when all rules pass, or (true, "") with a
// warning log when Prometheus is unreachable (fail-open).
func (c *MetricHealthChecker) IsHealthy(ctx context.Context, rules []*v2pb.MetricHealthRule) (bool, string) {
	for _, rule := range rules {
		if rule == nil || rule.GetQuery() == "" {
			continue
		}
		healthy, err := c.evalRule(ctx, rule)
		if err != nil {
			// Fail-open: do not roll back just because Prometheus is unavailable.
			c.logger.Warn("metric health rule query failed — treating as healthy (fail-open)",
				zap.String("rule", rule.GetName()),
				zap.String("query", rule.GetQuery()),
				zap.Error(err))
			continue
		}
		if !healthy {
			c.logger.Warn("metric health rule breached — signalling unhealthy",
				zap.String("rule", rule.GetName()),
				zap.String("query", rule.GetQuery()),
				zap.Float64("threshold", rule.GetThreshold()),
				zap.String("op", rule.GetOp()))
			return false, rule.GetName()
		}
	}
	return true, ""
}

// evalRule runs a single instant PromQL query and compares it to the threshold.
// Returns true when the metric does NOT breach the threshold (i.e. is healthy).
func (c *MetricHealthChecker) evalRule(ctx context.Context, rule *v2pb.MetricHealthRule) (bool, error) {
	queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	result, warnings, err := c.queryAPI.Query(queryCtx, rule.GetQuery(), time.Now())
	if err != nil {
		return true, fmt.Errorf("prometheus query %q: %w", rule.GetQuery(), err)
	}
	for _, w := range warnings {
		c.logger.Warn("prometheus query warning",
			zap.String("rule", rule.GetName()),
			zap.String("warning", w))
	}

	value, ok := extractScalar(result)
	if !ok {
		// No data returned — treat as healthy (metric may not exist yet during early rollout).
		c.logger.Debug("prometheus query returned no data — treating as healthy",
			zap.String("rule", rule.GetName()),
			zap.String("query", rule.GetQuery()))
		return true, nil
	}

	return !thresholdBreached(value, rule.GetOp(), rule.GetThreshold()), nil
}

// extractScalar pulls the first numeric value from a Prometheus result,
// supporting both scalar and instant-vector results.
func extractScalar(result model.Value) (float64, bool) {
	switch v := result.(type) {
	case model.Vector:
		if len(v) == 0 {
			return 0, false
		}
		return float64(v[0].Value), true
	case *model.Scalar:
		if v == nil {
			return 0, false
		}
		return float64(v.Value), true
	default:
		return 0, false
	}
}

// thresholdBreached returns true when op(value, threshold) is true — i.e. the
// rule has fired and the deployment should be considered unhealthy.
func thresholdBreached(value float64, op v2pb.MetricHealthRule_ComparisonOp, threshold float64) bool {
	switch op {
	case v2pb.MetricHealthRule_GT:
		return value > threshold
	case v2pb.MetricHealthRule_LT:
		return value < threshold
	case v2pb.MetricHealthRule_GTE:
		return value >= threshold
	case v2pb.MetricHealthRule_LTE:
		return value <= threshold
	default:
		return false
	}
}