// Package notification provides the pipeline run notification workflow.
package notification

import (
	"github.com/cadence-workflow/starlark-worker/workflow"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"go.uber.org/zap"
)

// RecordingSinkCall holds the arguments passed to a single RecordingSink.Notify call.
type RecordingSinkCall struct {
	// Notif is the Notification proto passed to Notify.
	Notif *v2pb.Notification
	// Msg is the pre-rendered Message passed to Notify.
	Msg Message
}

// RecordingSink is a Sink implementation that records every Notify call.
// Use it in tests to assert which notifications were delivered and with what content
// without requiring a real Cadence/Temporal workflow engine.
type RecordingSink struct {
	// Calls accumulates every invocation of Notify in order.
	Calls []RecordingSinkCall
}

// Notify records the call and returns nil.
func (r *RecordingSink) Notify(_ workflow.Context, _ *zap.Logger, notif *v2pb.Notification, msg Message) error {
	r.Calls = append(r.Calls, RecordingSinkCall{Notif: notif, Msg: msg})
	return nil
}

// FailingSink is a Sink implementation that always returns the configured error.
// Use it alongside RecordingSink to verify that errors.Join fan-out behaviour
// delivers to remaining sinks even when one sink fails.
type FailingSink struct {
	// Err is the error returned by every Notify call.
	Err error
}

// Notify always returns f.Err.
func (f *FailingSink) Notify(_ workflow.Context, _ *zap.Logger, _ *v2pb.Notification, _ Message) error {
	return f.Err
}
