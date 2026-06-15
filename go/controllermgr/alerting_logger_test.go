package controllermgr

import (
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
)

func TestAlertingSink_Error_ReflectorFailure(t *testing.T) {
	base := testr.New(t)
	logger := NewAlertingLogger(base)

	// Should not panic and should pass through to inner sink.
	logger.Error(
		errors.New(`failed to list *v2beta1pb.Deployment: bad Duration: time: invalid duration "35999996400s"`),
		"Failed to watch",
		"type", "Deployment",
	)
}

func TestAlertingSink_Error_NonReflector(t *testing.T) {
	base := testr.New(t)
	logger := NewAlertingLogger(base)

	// Non-reflector errors should pass through without incrementing metrics.
	logger.Error(errors.New("some other error"), "reconcile failed")
}

func TestAlertingSink_Info(t *testing.T) {
	base := testr.New(t)
	logger := NewAlertingLogger(base)

	// Info messages should pass through.
	logger.Info("some info message")
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

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil error", nil, "reflector_failure"},
		{"duration overflow", errors.New(`time: invalid duration "35999996400s"`), "duration_overflow"},
		{"Duration in message", errors.New("bad Duration field"), "duration_overflow"},
		{"generic error", errors.New("connection refused"), "reflector_failure"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyError(tt.err)
			if got != tt.want {
				t.Errorf("classifyError() = %q, want %q", got, tt.want)
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
		{"Failed to watch *v2beta1pb.Deployment", true},
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

func TestAlertingSink_Enabled(t *testing.T) {
	base := testr.New(t)
	logger := NewAlertingLogger(base)
	sink := logger.GetSink()
	if !sink.Enabled(0) {
		t.Error("expected Enabled(0) = true for testr logger")
	}
}

// Verify the wrapper implements logr.LogSink.
var _ logr.LogSink = (*alertingSink)(nil)
