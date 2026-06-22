package controllermgr

import (
	"regexp"
	"strings"

	"github.com/go-logr/logr"
	toolscache "k8s.io/client-go/tools/cache"

	"github.com/michelangelo-ai/michelangelo/go/kubeproto/metrics"
)

const (
	errTypeDurationOverflow = "duration_overflow"
	errTypeSchemaMismatch   = "schema_mismatch"
	errTypeListFailure      = "list_failure"
	errTypeWatchFailure     = "watch_failure"
)

// crdTypeRe extracts the CRD type (e.g. "*v2.Deployment") from reflector
// error messages like "failed to list *v2.Deployment: bad Duration: ...".
var crdTypeRe = regexp.MustCompile(`(?:failed to list|Failed to watch)\s+(\*?\w+\.\w+)`)

// NewWatchErrorHandler returns a client-go WatchErrorHandler that classifies
// reflector errors and emits the cr_unmarshal_errors_total metric.
//
// It fires whenever ListAndWatch() returns an error — covering all blocking
// failures (duration overflow, schema mismatch, list failures) that prevent
// the informer cache from syncing and block the reconciler loop.
//
// The CRD type is extracted from the error string because
// Reflector.typeDescription is unexported. The error format is
// "failed to list *v2.Deployment: <cause>" per client-go reflector.go:562.
func NewWatchErrorHandler(logger logr.Logger) toolscache.WatchErrorHandler {
	return func(r *toolscache.Reflector, err error) {
		if err == nil {
			return
		}
		ec := classifyError(err.Error())
		logger.Error(err, "reflector error detected",
			"crd_type", ec.crdType,
			"error_type", ec.errorType,
			"blocking", ec.blocking,
		)
		blocking := "false"
		if ec.blocking {
			blocking = "true"
		}
		metrics.IncCRUnmarshalError(ec.crdType, "unknown", ec.errorType, blocking)
	}
}

type errorClass struct {
	errorType string
	crdType   string
	blocking  bool
}

// classifyError inspects an error message to determine the error type,
// the affected CRD, and whether the error blocks the reconciler loop.
//
// Classification is ordered most-specific-first: duration and schema errors
// are checked before generic list/watch failures. This ordering is
// load-bearing — a "failed to list" message containing "invalid duration"
// must classify as duration_overflow, not list_failure.
func classifyError(errMsg string) errorClass {
	crdType := extractCRDTypeFromMessage(errMsg)

	if strings.Contains(errMsg, "invalid duration") || strings.Contains(errMsg, "bad Duration") {
		return errorClass{errTypeDurationOverflow, crdType, true}
	}
	if strings.Contains(errMsg, "proto:") || strings.Contains(errMsg, "unknown field") ||
		strings.Contains(errMsg, "wireType") {
		return errorClass{errTypeSchemaMismatch, crdType, true}
	}
	if strings.Contains(errMsg, "failed to list") {
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
