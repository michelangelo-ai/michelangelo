package utils

import (
	"context"

	"github.com/michelangelo-ai/michelangelo/go/logging"
	"go.uber.org/zap"
)

const (
	// AuditLogHPTopic is the heatpipe topic to send Audit Log
	AuditLogHPTopic = "hp-michelangelo-apiserver-audit-log"
	// AuditLogTopicVersion is the version of the audit log schema
	AuditLogTopicVersion = 1
)

type auditLog struct {
	logger *zap.Logger
}

// NewAuditLogEmitter instantiate a auditlog interface
func NewAuditLogEmitter(logger *zap.Logger) logging.AuditLog {
	return &auditLog{
		logger: logger,
	}
}

// Emit emit audit log through heatpipe
func (a *auditLog) Emit(ctx context.Context, event *logging.AuditLogEvent) {
	// pass
}
