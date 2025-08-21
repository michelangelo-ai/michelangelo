package controllermgr

import (
	"time"

	"github.com/uber-go/tally"
	"go.uber.org/zap"
)

// Metrics holds all CRD sync related metrics
type Metrics struct {
	scope                  tally.Scope
	schemaMismatchCounter  tally.Counter
	schemaCheckCounter     tally.Counter
	comparisonDuration     tally.Timer
	crdNotFoundCounter     tally.Counter
}

// NewMetrics creates a new Metrics instance
func NewMetrics(scope tally.Scope) *Metrics {
	return &Metrics{
		scope: scope,
		schemaMismatchCounter: scope.Tagged(map[string]string{
			"component": "crd_sync",
		}).Counter("schema_mismatch_total"),
		schemaCheckCounter: scope.Tagged(map[string]string{
			"component": "crd_sync",
		}).Counter("schema_checks_total"),
		comparisonDuration: scope.Tagged(map[string]string{
			"component": "crd_sync",
		}).Timer("comparison_duration"),
		crdNotFoundCounter: scope.Tagged(map[string]string{
			"component": "crd_sync",
		}).Counter("crd_not_found_total"),
	}
}

// RecordSchemaMismatch records when a schema mismatch is detected
func (m *Metrics) RecordSchemaMismatch(crdName string) {
	m.scope.Tagged(map[string]string{
		"component": "crd_sync",
		"crd_name":  crdName,
	}).Counter("schema_mismatch_total").Inc(1)
}

// RecordSchemaCheck records each schema comparison performed
func (m *Metrics) RecordSchemaCheck(crdName string, matched bool) {
	tags := map[string]string{
		"component": "crd_sync",
		"crd_name":  crdName,
		"matched":   "false",
	}
	if matched {
		tags["matched"] = "true"
	}
	m.scope.Tagged(tags).Counter("schema_checks_total").Inc(1)
}

// RecordComparisonDuration records how long the comparison took
func (m *Metrics) RecordComparisonDuration(duration time.Duration) {
	m.comparisonDuration.Record(duration)
}

// RecordCRDNotFound records when a local CRD is not found on the server
func (m *Metrics) RecordCRDNotFound(crdName string) {
	m.scope.Tagged(map[string]string{
		"component": "crd_sync",
		"crd_name":  crdName,
	}).Counter("crd_not_found_total").Inc(1)
}

// MetricsLogger wraps zap.Logger to emit both logs and metrics
type MetricsLogger struct {
	logger  *zap.Logger
	metrics *Metrics
}

// NewMetricsLogger creates a logger that emits both logs and metrics
func NewMetricsLogger(logger *zap.Logger, metrics *Metrics) *MetricsLogger {
	return &MetricsLogger{
		logger:  logger,
		metrics: metrics,
	}
}

// LogSchemaMismatch logs and records metrics for schema mismatches
func (ml *MetricsLogger) LogSchemaMismatch(crdName string) {
	ml.logger.Info("Schema mismatch detected", zap.String("name", crdName))
	if ml.metrics != nil {
		ml.metrics.RecordSchemaMismatch(crdName)
	}
}

// LogSchemaMatch logs and records metrics for schema matches
func (ml *MetricsLogger) LogSchemaMatch(crdName string) {
	ml.logger.Debug("Schemas match", zap.String("name", crdName))
	if ml.metrics != nil {
		ml.metrics.RecordSchemaCheck(crdName, true)
	}
}

// LogCRDNotFound logs and records metrics when CRD is not found on server
func (ml *MetricsLogger) LogCRDNotFound(crdName string) {
	ml.logger.Info("CRD not found on server", zap.String("name", crdName))
	if ml.metrics != nil {
		ml.metrics.RecordCRDNotFound(crdName)
	}
}

// LogError logs errors without metrics (errors are handled separately)
func (ml *MetricsLogger) LogError(msg string, fields ...zap.Field) {
	ml.logger.Error(msg, fields...)
}

// LogInfo logs info messages
func (ml *MetricsLogger) LogInfo(msg string, fields ...zap.Field) {
	ml.logger.Info(msg, fields...)
}

// LogDebug logs debug messages  
func (ml *MetricsLogger) LogDebug(msg string, fields ...zap.Field) {
	ml.logger.Debug(msg, fields...)
}