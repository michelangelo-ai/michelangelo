package controllermgr

import (
	"context"
	"reflect"
	"time"

	"github.com/uber-go/tally"
	"go.uber.org/zap"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MetricsDemo demonstrates how metrics are triggered in the CRD sync process
func MetricsDemo() {
	// Example: How metrics are triggered during schema comparison

	// 1. Initialize metrics scope (this would normally come from the application)
	scope := tally.NewTestScope("crd_sync", map[string]string{})
	
	// 2. Create metrics instance
	metrics := NewMetrics(scope)
	
	// 3. Create logger with metrics
	logger := zap.NewNop() // In real usage, this comes from the application
	metricsLogger := NewMetricsLogger(logger, metrics)

	// 4. Simulate schema comparison scenarios

	// Scenario 1: Schema mismatch detected
	simulateSchemaMismatch(metricsLogger)
	
	// Scenario 2: Schema matches
	simulateSchemaMatch(metricsLogger)
	
	// Scenario 3: CRD not found on server
	simulateCRDNotFound(metricsLogger)
	
	// Scenario 4: Performance timing
	simulatePerformanceTiming(metricsLogger)

	// 5. Print metrics that would be emitted
	printMetricsSnapshot(scope)
}

func simulateSchemaMismatch(metricsLogger *MetricsLogger) {
	// This is called when reflect.DeepEqual(serverCRD.Spec.Versions, localCRD.Spec.Versions) returns false
	metricsLogger.LogSchemaMismatch("projects.example.com")
	
	// Metrics emitted:
	// - Counter: crd_sync.schema_mismatch_total{component="crd_sync", crd_name="projects.example.com"} +1
}

func simulateSchemaMatch(metricsLogger *MetricsLogger) {
	// This is called when schemas are identical
	metricsLogger.LogSchemaMatch("deployments.example.com")
	
	// Metrics emitted:
	// - Counter: crd_sync.schema_checks_total{component="crd_sync", crd_name="deployments.example.com", matched="true"} +1
}

func simulateCRDNotFound(metricsLogger *MetricsLogger) {
	// This is called when local CRD is not found on server
	metricsLogger.LogCRDNotFound("missing.example.com")
	
	// Metrics emitted:
	// - Counter: crd_sync.crd_not_found_total{component="crd_sync", crd_name="missing.example.com"} +1
}

func simulatePerformanceTiming(metricsLogger *MetricsLogger) {
	// This is how duration is measured in performSchemaComparison()
	startTime := time.Now()
	
	// Simulate some work
	time.Sleep(100 * time.Millisecond)
	
	if metricsLogger.metrics != nil {
		duration := time.Since(startTime)
		metricsLogger.metrics.RecordComparisonDuration(duration)
	}
	
	// Metrics emitted:
	// - Timer: crd_sync.comparison_duration{component="crd_sync"} = 100ms
}

func printMetricsSnapshot(scope tally.TestScope) {
	// This shows what metrics would be available for monitoring systems
	snapshot := scope.Snapshot()
	
	println("=== METRICS EMITTED ===")
	
	// Counters
	for name, counter := range snapshot.Counters() {
		println("Counter:", name, "=", counter.Value())
	}
	
	// Timers
	for name, timer := range snapshot.Timers() {
		println("Timer:", name, "=", timer.Values())
	}
}

// RealWorldExample shows how this integrates with actual comparison logic
func RealWorldExample(ctx context.Context, metricsLogger *MetricsLogger) {
	// This is exactly how it works in the real crd_sync.go

	// Example CRDs with different schemas
	localCRD := &apiextv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "projects.example.com"},
		Spec: apiextv1.CustomResourceDefinitionSpec{
			Versions: []apiextv1.CustomResourceDefinitionVersion{
				{Name: "v1", Served: true, Storage: true},
			},
		},
	}

	serverCRD := &apiextv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "projects.example.com"},
		Spec: apiextv1.CustomResourceDefinitionSpec{
			Versions: []apiextv1.CustomResourceDefinitionVersion{
				{Name: "v1", Served: true, Storage: false}, // Different!
				{Name: "v2", Served: true, Storage: true},  // Additional version!
			},
		},
	}

	// This is the actual comparison logic from crd_sync.go:155-159
	name := "projects.example.com"
	
	// Compare schemas and emit metrics/logs for any mismatches
	if hasChange := !reflect.DeepEqual(serverCRD.Spec.Versions, localCRD.Spec.Versions); hasChange {
		metricsLogger.LogSchemaMismatch(name)
		// ☝️ This triggers: Counter crd_sync.schema_mismatch_total{crd_name="projects.example.com"} +1
	} else {
		metricsLogger.LogSchemaMatch(name)
		// ☝️ This triggers: Counter crd_sync.schema_checks_total{crd_name="projects.example.com", matched="true"} +1
	}
}

/*
METRICS EXPORTED TO MONITORING SYSTEMS:

1. Schema Mismatch Counter:
   Name: crd_sync.schema_mismatch_total
   Labels: {component="crd_sync", crd_name="<crd_name>"}
   Description: Number of times schema mismatches were detected

2. Schema Check Counter:
   Name: crd_sync.schema_checks_total  
   Labels: {component="crd_sync", crd_name="<crd_name>", matched="true|false"}
   Description: Total number of schema comparisons performed

3. CRD Not Found Counter:
   Name: crd_sync.crd_not_found_total
   Labels: {component="crd_sync", crd_name="<crd_name>"}
   Description: Number of times local CRDs were not found on server

4. Comparison Duration Timer:
   Name: crd_sync.comparison_duration
   Labels: {component="crd_sync"}
   Description: Time taken to perform schema comparison

ALERTING EXAMPLES:

1. Schema Drift Alert:
   rate(crd_sync_schema_mismatch_total[5m]) > 0
   
2. CRD Deployment Issues:
   rate(crd_sync_crd_not_found_total[5m]) > 0

3. Performance Degradation:
   histogram_quantile(0.95, crd_sync_comparison_duration) > 10s

4. Service Health:
   rate(crd_sync_schema_checks_total[5m]) == 0  # No checks happening
*/