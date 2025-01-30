package logging

import (
	"context"

	"go.uber.org/fx"
)

// AuditLog is the client interface for audit log
type AuditLog interface {
	Emit(ctx context.Context, event *AuditLogEvent)
}

type DummyAuditLog struct{}

func (d *DummyAuditLog) Emit(_ context.Context, _ *AuditLogEvent) {
}

var DummyAuditLogModule = fx.Options(
	fx.Provide(func() AuditLog {
		return &DummyAuditLog{}
	},
	),
)
