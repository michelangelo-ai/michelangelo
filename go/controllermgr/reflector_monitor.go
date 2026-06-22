package controllermgr

import (
	"regexp"
	"strings"

	"github.com/go-logr/logr"

	"github.com/michelangelo-ai/michelangelo/go/kubeproto/metrics"
)

const (
	errTypeDurationOverflow = "duration_overflow"
	errTypeSchemaMismatch   = "schema_mismatch"
	errTypeTypeMismatch     = "type_mismatch"
	errTypeDecodeError      = "decode_error"
	errTypeStoreError       = "store_error"
	errTypeListFailure      = "list_failure"
	errTypeWatchFailure     = "watch_failure"
)

type errorClass struct {
	errorType string
	crdType   string
	blocking  bool
}

var crdTypeRe = regexp.MustCompile(`(?:failed to list|Failed to watch)\s+(\*?\w+\.\w+)`)

// alertingSink wraps a logr.LogSink to intercept reflector errors from
// controller-runtime and increment the cr_unmarshal_errors_total metric.
//
// When a CR stored in etcd contains invalid data (e.g. a proto Duration
// that overflows Go's time.Duration), the reflector's List() or Watch()
// calls fail and the controller-manager may enter a crash loop. This sink
// detects those errors at both warning and error severity levels and emits
// classified metrics so oncall can set up alerts.
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

// Info intercepts klog warning messages. klog.Warningf maps to
// logr.Info(level=0, msg). Level > 0 messages are regular info logs
// and are passed through without inspection.
func (s *alertingSink) Info(level int, msg string, keysAndValues ...interface{}) {
	if level == 0 && isReflectorFailure(msg) {
		ec := classifyMessage(msg, nil, keysAndValues)
		blocking := "false"
		if ec.blocking {
			blocking = "true"
		}
		metrics.IncCRUnmarshalError(ec.crdType, "unknown", ec.errorType, blocking)
	}
	s.inner.Info(level, msg, keysAndValues...)
}

func (s *alertingSink) Error(err error, msg string, keysAndValues ...interface{}) {
	if isReflectorFailure(msg) || (err != nil && isReflectorFailure(err.Error())) {
		ec := classifyMessage(msg, err, keysAndValues)
		blocking := "false"
		if ec.blocking {
			blocking = "true"
		}
		metrics.IncCRUnmarshalError(ec.crdType, "unknown", ec.errorType, blocking)
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

// classifyMessage inspects both the message and error to determine the
// error type, the affected CRD, and whether the error blocks the
// reconciler loop (prevents cache sync).
func classifyMessage(msg string, err error, keysAndValues []interface{}) errorClass {
	combined := msg
	if err != nil {
		combined = msg + " " + err.Error()
	}

	crdType := extractKVString(keysAndValues, "type")
	if crdType == "unknown" {
		crdType = extractCRDTypeFromMessage(combined)
	}

	if strings.Contains(combined, "invalid duration") || strings.Contains(combined, "bad Duration") {
		return errorClass{errTypeDurationOverflow, crdType, true}
	}

	if strings.Contains(combined, "proto:") || strings.Contains(combined, "unknown field") ||
		strings.Contains(combined, "wireType") {
		return errorClass{errTypeSchemaMismatch, crdType, true}
	}

	if strings.Contains(combined, "expected type") || strings.Contains(combined, "expected gvk") {
		return errorClass{errTypeTypeMismatch, crdType, false}
	}

	if strings.Contains(combined, "unable to understand watch event") {
		return errorClass{errTypeDecodeError, crdType, false}
	}

	if strings.Contains(combined, "unable to add") || strings.Contains(combined, "unable to update") ||
		strings.Contains(combined, "unable to delete") {
		return errorClass{errTypeStoreError, crdType, false}
	}

	if strings.Contains(combined, "failed to list") {
		return errorClass{errTypeListFailure, crdType, true}
	}

	return errorClass{errTypeWatchFailure, crdType, false}
}

func extractCRDTypeFromMessage(msg string) string {
	matches := crdTypeRe.FindStringSubmatch(msg)
	if len(matches) >= 2 {
		return matches[1]
	}
	return "unknown"
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
