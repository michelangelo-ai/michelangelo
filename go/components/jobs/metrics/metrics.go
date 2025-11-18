package metrics

import (
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/uber-go/tally"
)

// ControllerMetrics is responsible to hold Job related metrics
type ControllerMetrics struct {
	MetricsScope tally.Scope
}

// NewControllerMetrics is responsible for creating a new ControllerMetrics object for given Job Controller
func NewControllerMetrics(scope tally.Scope, controllerType string) *ControllerMetrics {
	scope = scope.SubScope(controllerType).Tagged(map[string]string{
		constants.ControllerTag: controllerType,
	})

	return &ControllerMetrics{
		MetricsScope: scope,
	}
}

// IncJobFailure is responsible for inc constants.JobFailedCountMetricName counter with given reason as tag
func (m ControllerMetrics) IncJobFailure(reason string) {
	m.MetricsScope.Tagged(map[string]string{constants.FailureReasonKey: reason}).
		Counter(constants.JobFailedCountMetricName).Inc(1)
}
