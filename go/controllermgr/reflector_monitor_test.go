package controllermgr

import (
	"errors"
	"io"
	"testing"

	"github.com/go-logr/logr/testr"
	"github.com/prometheus/client_golang/prometheus"
)

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name      string
		errMsg    string
		wantType  string
		wantCRD   string
		wantBlock bool
	}{
		{
			name:      "duration_overflow_invalid_duration",
			errMsg:    `failed to list *v2.Deployment: bad Duration: time: invalid duration "35999996400s"`,
			wantType:  errTypeDurationOverflow,
			wantCRD:   "*v2.Deployment",
			wantBlock: true,
		},
		{
			name:      "duration_overflow_bad_Duration",
			errMsg:    "failed to list *v2.Deployment: bad Duration in field X",
			wantType:  errTypeDurationOverflow,
			wantCRD:   "*v2.Deployment",
			wantBlock: true,
		},
		{
			name:      "schema_mismatch_proto",
			errMsg:    "failed to list *v2.Pipeline: proto: wrong wireType = 2 for field Spec",
			wantType:  errTypeSchemaMismatch,
			wantCRD:   "*v2.Pipeline",
			wantBlock: true,
		},
		{
			name:      "schema_mismatch_unknown_field",
			errMsg:    "failed to list *v2.PipelineRun: unknown field in proto message",
			wantType:  errTypeSchemaMismatch,
			wantCRD:   "*v2.PipelineRun",
			wantBlock: true,
		},
		{
			name:      "schema_mismatch_wireType",
			errMsg:    "failed to list *v2.TriggerRun: wireType mismatch for field status",
			wantType:  errTypeSchemaMismatch,
			wantCRD:   "*v2.TriggerRun",
			wantBlock: true,
		},
		{
			name:      "list_failure_generic",
			errMsg:    "failed to list *v2.InferenceServer: connection refused",
			wantType:  errTypeListFailure,
			wantCRD:   "*v2.InferenceServer",
			wantBlock: true,
		},
		{
			name:      "list_failure_timeout",
			errMsg:    "failed to list *v2.SparkJob: timeout",
			wantType:  errTypeListFailure,
			wantCRD:   "*v2.SparkJob",
			wantBlock: true,
		},
		{
			name:      "enum_mismatch_unknown_value",
			errMsg:    `failed to list *v2.Deployment: unknown value "TARGET_TYPE_STREAMING" for enum michelangelo.api.v2.TargetType`,
			wantType:  errTypeEnumMismatch,
			wantCRD:   "*v2.Deployment",
			wantBlock: true,
		},
		{
			name:      "enum_mismatch_invalid_datatype",
			errMsg:    `failed to list *v2.Pipeline: invalid DataType: PIPELINE_TYPE_CUSTOM`,
			wantType:  errTypeEnumMismatch,
			wantCRD:   "*v2.Pipeline",
			wantBlock: true,
		},
		{
			name:      "watch_failure_generic",
			errMsg:    "connection reset by peer",
			wantType:  errTypeWatchFailure,
			wantCRD:   "unknown",
			wantBlock: false,
		},
		{
			name:      "empty_message",
			errMsg:    "",
			wantType:  errTypeWatchFailure,
			wantCRD:   "unknown",
			wantBlock: false,
		},
		{
			name:      "ordering_duration_beats_list",
			errMsg:    `failed to list *v2.Deployment: bad Duration: time: invalid duration "999s"`,
			wantType:  errTypeDurationOverflow,
			wantCRD:   "*v2.Deployment",
			wantBlock: true,
		},
		{
			name:      "watch_failure_with_crd_type",
			errMsg:    "Failed to watch *v2.Deployment: connection reset",
			wantType:  errTypeWatchFailure,
			wantCRD:   "*v2.Deployment",
			wantBlock: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyError(tt.errMsg)
			if got.errorType != tt.wantType {
				t.Errorf("errorType = %q, want %q", got.errorType, tt.wantType)
			}
			if got.crdType != tt.wantCRD {
				t.Errorf("crdType = %q, want %q", got.crdType, tt.wantCRD)
			}
			if got.blocking != tt.wantBlock {
				t.Errorf("blocking = %v, want %v", got.blocking, tt.wantBlock)
			}
		})
	}
}

func TestExtractCRDTypeFromMessage(t *testing.T) {
	tests := []struct {
		msg  string
		want string
	}{
		{"failed to list *v2.Deployment: bad Duration", "*v2.Deployment"},
		{"Failed to watch *v2.Pipeline: connection reset", "*v2.Pipeline"},
		{"failed to list *v2beta1pb.InferenceServer: timeout", "*v2beta1pb.InferenceServer"},
		{"failed to list v2.SparkJob: error", "v2.SparkJob"},
		{"some random message", "unknown"},
		{"", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			got := extractCRDTypeFromMessage(tt.msg)
			if got != tt.want {
				t.Errorf("extractCRDTypeFromMessage(%q) = %q, want %q", tt.msg, got, tt.want)
			}
		})
	}
}

// TestMetricCollectorRegisters verifies the cr_reflector_errors_total counter
// is well-formed and can be registered in an isolated registry.
func TestMetricCollectorRegisters(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cr_reflector_errors_total",
			Help: "Total number of reflector errors when listing or watching CRs",
		},
		[]string{"crd_type", "error_type", "blocking"},
	)
	if err := reg.Register(c); err != nil {
		t.Fatalf("failed to register metric: %v", err)
	}
	c.WithLabelValues("*v2.Deployment", "duration_overflow", "true").Inc()
}

func TestWatchErrorHandler_DurationOverflow(t *testing.T) {
	base := testr.New(t)
	handler := NewWatchErrorHandler(base)

	handler(nil, errors.New(`failed to list *v2.Deployment: bad Duration: time: invalid duration "35999996400s"`))
}

func TestWatchErrorHandler_NilError(t *testing.T) {
	base := testr.New(t)
	handler := NewWatchErrorHandler(base)

	handler(nil, nil)
}

func TestWatchErrorHandler_SchemaError(t *testing.T) {
	base := testr.New(t)
	handler := NewWatchErrorHandler(base)

	handler(nil, errors.New("failed to list *v2.Pipeline: proto: wrong wireType = 2"))
}

func TestWatchErrorHandler_GenericListFailure(t *testing.T) {
	base := testr.New(t)
	handler := NewWatchErrorHandler(base)

	handler(nil, errors.New("failed to list *v2.RayJob: connection refused"))
}

func TestWatchErrorHandler_WatchFailure(t *testing.T) {
	base := testr.New(t)
	handler := NewWatchErrorHandler(base)

	handler(nil, errors.New("connection reset by peer"))
}

// TestWatchErrorHandler_EOF verifies that io.EOF (normal watch close) is
// delegated to the default handler without emitting a metric.
func TestWatchErrorHandler_EOF(t *testing.T) {
	base := testr.New(t)
	handler := NewWatchErrorHandler(base)

	handler(nil, io.EOF)
}

// TestWatchErrorHandler_UnexpectedEOF verifies that io.ErrUnexpectedEOF
// is delegated to the default handler without emitting a metric.
func TestWatchErrorHandler_UnexpectedEOF(t *testing.T) {
	base := testr.New(t)
	handler := NewWatchErrorHandler(base)

	handler(nil, io.ErrUnexpectedEOF)
}
