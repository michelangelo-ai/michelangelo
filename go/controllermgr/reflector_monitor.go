package controllermgr

import (
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	toolscache "k8s.io/client-go/tools/cache"

	"github.com/michelangelo-ai/michelangelo/go/kubeproto/metrics"
)

const (
	errTypeDurationOverflow = "duration_overflow"
	errTypeSchemaMismatch   = "schema_mismatch"
	errTypeEnumMismatch     = "enum_mismatch"
	errTypeListFailure      = "list_failure"
	errTypeWatchFailure     = "watch_failure"
)

// crdTypeRe extracts the CRD type (e.g. "*v2.Deployment") from reflector
// error messages like "failed to list *v2.Deployment: bad Duration: ...".
var crdTypeRe = regexp.MustCompile(`(?:failed to list|Failed to watch)\s+(\*?\w+\.\w+)`)

// durationErrRe matches duration parse failures regardless of the exact wording
// used by the originating library. Two known forms:
//   - "bad Duration" — protobuf JSON decoder (google.golang.org/protobuf)
//   - "invalid duration" — Go standard library time.ParseDuration
//
// Case-insensitive so minor wording changes in either library do not silently
// break classification.
//
// Duration overflow is a known failure mode caused by proto field values that
// exceed Go's time.Duration range (~292 years). When this occurs the reflector's
// List() call fails, the informer cache never syncs, and the controller-manager
// enters a crash loop until the offending CR is corrected or the binary is
// updated. Classification as duration_overflow (rather than the generic
// list_failure that "failed to list" would produce) is intentional — it gives
// oncall a specific actionable signal via the cr_reflector_errors_total metric.
// See: https://github.com/michelangelo-ai/michelangelo/issues/1318
var durationErrRe = regexp.MustCompile(`(?i)\b(bad|invalid)\s+duration\b`)

// NewWatchErrorHandler returns a client-go WatchErrorHandler that classifies
// reflector errors and emits the cr_unmarshal_errors_total metric.
//
// It replaces the default handler but preserves its behavior for benign
// cases: io.EOF (normal watch close) and expired/gone errors are passed
// through to the default handler without emitting a metric. Only real
// failures (duration overflow, schema mismatch, list/watch errors) are
// classified and counted.
//
// The CRD type is extracted from the error string because
// Reflector.typeDescription is unexported. The error format is
// "failed to list *v2.Deployment: <cause>" per client-go reflector.go:562.
//
// NOTE: The WatchErrorHandler signature changes to WatchErrorHandlerWithContext
// in controller-runtime v0.21+ / client-go v0.32+. Update on k8s dep bump.
func NewWatchErrorHandler(logger logr.Logger) toolscache.WatchErrorHandler {
	return func(r *toolscache.Reflector, err error) {
		if err == nil {
			return
		}

		// Delegate benign errors to the default handler without emitting metrics.
		// These are normal watch lifecycle events, not failures.
		if err == io.EOF || err == io.ErrUnexpectedEOF || apierrors.IsResourceExpired(err) || apierrors.IsGone(err) {
			if r != nil {
				toolscache.DefaultWatchErrorHandler(r, err)
			}
			return
		}

		ec := classifyError(err.Error())
		logger.Error(err, "reflector error detected",
			"crd_type", ec.crdType,
			"error_type", ec.errorType,
			"blocking", ec.blocking,
		)
		metrics.IncCRUnmarshalError(ec.crdType, ec.errorType, strconv.FormatBool(ec.blocking))
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
// load-bearing — a "failed to list" message containing a duration error
// must classify as duration_overflow, not list_failure.
func classifyError(errMsg string) errorClass {
	crdType := extractCRDTypeFromMessage(errMsg)

	if durationErrRe.MatchString(errMsg) {
		return errorClass{errTypeDurationOverflow, crdType, true}
	}
	if strings.Contains(errMsg, "proto:") || strings.Contains(errMsg, "unknown field") ||
		strings.Contains(errMsg, "wireType") {
		return errorClass{errTypeSchemaMismatch, crdType, true}
	}
	// Enum version skew: CRD has a string enum value the binary doesn't know.
	// Two error formats depending on the deserialization path:
	// - apiserver/reflector: "unknown value \"X\" for enum pkg.EnumType"
	// - generated UnmarshalJSON: "invalid DataType: X"
	if strings.Contains(errMsg, "unknown value") && strings.Contains(errMsg, "for enum") ||
		strings.Contains(errMsg, "invalid DataType") {
		return errorClass{errTypeEnumMismatch, crdType, true}
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
