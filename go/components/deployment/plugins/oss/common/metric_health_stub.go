package common

import (
	"context"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"go.uber.org/zap"
)

// MetricHealthChecker evaluates PromQL-based health rules.
// This stub always returns healthy; the full implementation requires
// the prometheus/common module which is not yet registered in the Bazel workspace.
type MetricHealthChecker struct{}

// NewMetricHealthChecker creates a MetricHealthChecker for the given Prometheus URL.
func NewMetricHealthChecker(prometheusURL string, logger *zap.Logger) (*MetricHealthChecker, error) {
	return &MetricHealthChecker{}, nil
}

// IsHealthy always returns (true, "") in this stub implementation.
func (c *MetricHealthChecker) IsHealthy(_ context.Context, _ []*v2pb.MetricHealthRule) (bool, string) {
	return true, ""
}
