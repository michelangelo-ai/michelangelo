package controllermgr

import (
	"strings"

	"github.com/go-logr/logr"

	"github.com/michelangelo-ai/michelangelo/go/kubeproto/metrics"
)

// alertingSink wraps a logr.LogSink to intercept reflector errors from
// controller-runtime and increment the cr_unmarshal_errors_total metric.
//
// When a CR stored in etcd contains a proto Duration value that overflows
// Go's time.Duration (int64 nanoseconds, max ~292 years), the reflector's
// List() call fails on deserialization and the controller-manager enters a
// crash loop. This sink detects those errors and emits a metric so oncall
// can set up alerts.
type alertingSink struct {
	inner logr.LogSink
}

// NewAlertingLogger wraps a logr.Logger so that reflector errors from
// controller-runtime are counted in cr_unmarshal_errors_total.
func NewAlertingLogger(base logr.Logger) logr.Logger {
	return logr.New(&alertingSink{inner: base.GetSink()})
}

func (s *alertingSink) Init(info logr.RuntimeInfo) {
	s.inner.Init(info)
}

func (s *alertingSink) Enabled(level int) bool {
	return s.inner.Enabled(level)
}

func (s *alertingSink) Info(level int, msg string, keysAndValues ...interface{}) {
	s.inner.Info(level, msg, keysAndValues...)
}

func (s *alertingSink) Error(err error, msg string, keysAndValues ...interface{}) {
	if isReflectorFailure(msg) {
		errorType := classifyError(err)
		crdType := extractKVString(keysAndValues, "type")
		metrics.IncCRUnmarshalError(crdType, "unknown", errorType)
	}
	s.inner.Error(err, msg, keysAndValues...)
}

func (s *alertingSink) WithValues(keysAndValues ...interface{}) logr.LogSink {
	return &alertingSink{inner: s.inner.WithValues(keysAndValues...)}
}

func (s *alertingSink) WithName(name string) logr.LogSink {
	return &alertingSink{inner: s.inner.WithName(name)}
}

func isReflectorFailure(msg string) bool {
	return strings.Contains(msg, "Failed to watch") ||
		strings.Contains(msg, "failed to list")
}

func classifyError(err error) string {
	if err == nil {
		return "reflector_failure"
	}
	errMsg := err.Error()
	if strings.Contains(errMsg, "invalid duration") || strings.Contains(errMsg, "Duration") {
		return "duration_overflow"
	}
	return "reflector_failure"
}

func extractKVString(kv []interface{}, key string) string {
	for i := 0; i+1 < len(kv); i += 2 {
		if k, ok := kv[i].(string); ok && k == key {
			if v, ok := kv[i+1].(string); ok {
				return v
			}
		}
	}
	return "unknown"
}
