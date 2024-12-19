package logging

import (
	"context"
)

// AuditLog is the client interface for audit log
type AuditLog interface {
	Emit(ctx context.Context, event *AuditLogEvent)
}
