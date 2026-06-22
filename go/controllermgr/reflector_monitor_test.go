package controllermgr

import (
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
)

func TestClassifyMessage(t *testing.T) {
	tests := []struct {
		name      string
		msg       string
		err       error
		kv        []interface{}
		wantType  string
		wantCRD   string
		wantBlock bool
	}{
		{
			name:      "duration_overflow_invalid_duration",
			msg:       "failed to list *v2.Deployment",
			err:       errors.New(`bad Duration: time: invalid duration "35999996400s"`),
			wantType:  errTypeDurationOverflow,
			wantCRD:   "*v2.Deployment",
			wantBlock: true,
		},
		{
			name:      "duration_overflow_bad_Duration",
			msg:       "failed to list *v2.Deployment: bad Duration in field X",
			err:       nil,
			wantType:  errTypeDurationOverflow,
			wantCRD:   "*v2.Deployment",
			wantBlock: true,
		},
		{
			name:      "schema_mismatch_proto",
			msg:       "failed to list *v2.Pipeline",
			err:       errors.New("proto: wrong wireType = 2 for field Spec"),
			wantType:  errTypeSchemaMismatch,
			wantCRD:   "*v2.Pipeline",
			wantBlock: true,
		},
		{
			name:      "schema_mismatch_unknown_field",
			msg:       "failed to list *v2.PipelineRun",
			err:       errors.New("unknown field in proto message"),
			wantType:  errTypeSchemaMismatch,
			wantCRD:   "*v2.PipelineRun",
			wantBlock: true,
		},
		{
			name:      "schema_mismatch_wireType",
			msg:       "Failed to watch *v2.TriggerRun",
			err:       errors.New("wireType mismatch for field status"),
			wantType:  errTypeSchemaMismatch,
			wantCRD:   "*v2.TriggerRun",
			wantBlock: true,
		},
		{
			name:      "type_mismatch_expected_type",
			msg:       "Unhandled Error",
			err:       errors.New("expected type *v2.Deployment, but watch event object had type *v2.Pipeline"),
			wantType:  errTypeTypeMismatch,
			wantCRD:   "unknown",
			wantBlock: false,
		},
		{
			name:      "type_mismatch_expected_gvk",
			msg:       "Unhandled Error",
			err:       errors.New("expected gvk michelangelo.api/v2 Deployment, but got Pipeline"),
			wantType:  errTypeTypeMismatch,
			wantCRD:   "unknown",
			wantBlock: false,
		},
		{
			name:      "decode_error",
			msg:       "Unhandled Error",
			err:       errors.New("unable to understand watch event {raw data}"),
			wantType:  errTypeDecodeError,
			wantCRD:   "unknown",
			wantBlock: false,
		},
		{
			name:      "store_error_add",
			msg:       "Unhandled Error",
			err:       errors.New("unable to add watch event object to store: key conflict"),
			wantType:  errTypeStoreError,
			wantCRD:   "unknown",
			wantBlock: false,
		},
		{
			name:      "store_error_update",
			msg:       "Unhandled Error",
			err:       errors.New("unable to update watch event object in store"),
			wantType:  errTypeStoreError,
			wantCRD:   "unknown",
			wantBlock: false,
		},
		{
			name:      "store_error_delete",
			msg:       "Unhandled Error",
			err:       errors.New("unable to delete watch event object from store"),
			wantType:  errTypeStoreError,
			wantCRD:   "unknown",
			wantBlock: false,
		},
		{
			name:      "list_failure_generic",
			msg:       "failed to list *v2.InferenceServer: connection refused",
			err:       nil,
			wantType:  errTypeListFailure,
			wantCRD:   "*v2.InferenceServer",
			wantBlock: true,
		},
		{
			name:      "watch_failure_generic",
			msg:       "Unhandled Error",
			err:       errors.New("Failed to watch *v2.Deployment: connection reset"),
			wantType:  errTypeWatchFailure,
			wantCRD:   "*v2.Deployment",
			wantBlock: false,
		},
		{
			name:      "nil_error_list_failure",
			msg:       "failed to list *v2.SparkJob: timeout",
			err:       nil,
			wantType:  errTypeListFailure,
			wantCRD:   "*v2.SparkJob",
			wantBlock: true,
		},
		{
			name:      "crd_from_kv_takes_precedence",
			msg:       "Failed to watch",
			err:       errors.New("connection refused"),
			kv:        []interface{}{"type", "Deployment"},
			wantType:  errTypeWatchFailure,
			wantCRD:   "Deployment",
			wantBlock: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kv := tt.kv
			if kv == nil {
				kv = []interface{}{}
			}
			got := classifyMessage(tt.msg, tt.err, kv)
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

func TestIsReflectorFailure(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"Failed to watch", true},
		{"failed to list", true},
		{"Failed to watch *v2.Deployment", true},
		{"reflector.go: failed to list *v2.Pipeline: timeout", true},
		{"reconcile failed", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			got := isReflectorFailure(tt.msg)
			if got != tt.want {
				t.Errorf("isReflectorFailure(%q) = %v, want %v", tt.msg, got, tt.want)
			}
		})
	}
}

func TestExtractKVString(t *testing.T) {
	tests := []struct {
		name string
		kv   []interface{}
		key  string
		want string
	}{
		{"found", []interface{}{"type", "Deployment", "ns", "default"}, "type", "Deployment"},
		{"not found", []interface{}{"ns", "default"}, "type", "unknown"},
		{"empty", nil, "type", "unknown"},
		{"non-string value", []interface{}{"type", 42}, "type", "unknown"},
		{"odd length", []interface{}{"type"}, "type", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractKVString(tt.kv, tt.key)
			if got != tt.want {
				t.Errorf("extractKVString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAlertingSink_Error_ReflectorFailure(t *testing.T) {
	base := testr.New(t)
	logger := NewAlertingLogger(base)

	logger.Error(
		errors.New(`failed to list *v2beta1pb.Deployment: bad Duration: time: invalid duration "35999996400s"`),
		"Unhandled Error",
	)
}

func TestAlertingSink_Error_NonReflector(t *testing.T) {
	base := testr.New(t)
	logger := NewAlertingLogger(base)

	logger.Error(errors.New("some other error"), "reconcile failed")
}

func TestAlertingSink_Info_Level0_ReflectorWarning(t *testing.T) {
	base := testr.New(t)
	logger := NewAlertingLogger(base)

	logger.Info(`reflector.go:243: failed to list *v2.Deployment: bad Duration: time: invalid duration "35999996400s"`)
}

func TestAlertingSink_Info_Level0_NonReflector(t *testing.T) {
	base := testr.New(t)
	logger := NewAlertingLogger(base)

	logger.Info("starting controller")
}

func TestAlertingSink_Info_HighLevel_Passthrough(t *testing.T) {
	base := testr.New(t)
	logger := NewAlertingLogger(base).V(1)

	logger.Info("failed to list *v2.Deployment: should not trigger metric at level 1")
}

func TestAlertingSink_WithValues(t *testing.T) {
	base := testr.New(t)
	logger := NewAlertingLogger(base)

	derived := logger.WithValues("key", "value")
	derived.Error(
		errors.New(`time: invalid duration "35999996400s"`),
		"failed to list",
	)
}

func TestAlertingSink_WithName(t *testing.T) {
	base := testr.New(t)
	logger := NewAlertingLogger(base)

	named := logger.WithName("test")
	named.Error(
		errors.New(`time: invalid duration "35999996400s"`),
		"Failed to watch",
		"type", "Deployment",
	)
}

func TestAlertingSink_Enabled(t *testing.T) {
	base := testr.New(t)
	logger := NewAlertingLogger(base)
	sink := logger.GetSink()
	if !sink.Enabled(0) {
		t.Error("expected Enabled(0) = true for testr logger")
	}
}

var _ logr.LogSink = (*alertingSink)(nil)
