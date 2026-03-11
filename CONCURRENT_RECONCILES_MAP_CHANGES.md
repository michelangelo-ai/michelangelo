# ConcurrentReconcilesMap Implementation - OSS Migration

This document describes the changes made to replicate Uber's internal `ConcurrentReconcilesMap` feature in OSS Michelangelo.

## Overview

This feature allows per-controller concurrency configuration for ingester reconcilers, enabling fine-grained performance tuning for different CRD types.

## Changes Made

### 1. Base Configuration (`go/base/config/ingester_config.go`)

**Added:**
- `ConcurrentReconcilesMap map[string]int` - Map of CRD kind to concurrency limit
- `RequeuePeriodMap map[string]time.Duration` - Map of CRD kind to requeue period
- `GetControllerConfig(crdKind string) ingester.Config` - Helper method to get per-controller config

**Backwards Compatibility:**
- Legacy `ConcurrentReconciles` and `RequeuePeriod` fields are retained
- They serve as defaults when map values are not specified
- Existing configs continue to work without modification

### 2. Ingester Module (`go/components/ingester/module.go`)

**Changed:**
- `registerParams.Config` → `registerParams.IngesterConfig` (changed type)
- Registration loop now calls `GetControllerConfig(gvk.Kind)` to get per-controller settings
- Added logging to show configured concurrency per controller

**Added Import:**
- `baseconfig "github.com/michelangelo-ai/michelangelo/go/base/config"`

### 3. Provider Function (`go/cmd/controllermgr/ingester_providers.go`)

**Changed:**
- Return type: `ingester.Config` → `baseconfig.IngesterConfig`
- Returns raw config instead of converting it (conversion happens per-controller now)

### 4. YAML Configuration

**New Structure:**
```yaml
ingester:
  # Global defaults (backwards compatible)
  concurrentReconciles: 2
  requeuePeriod: 30s

  # Per-controller overrides (new feature)
  concurrentReconcilesMap:
    PipelineRun: 10
    Deployment: 3
    Pipeline: 3
    Revision: 3

  # Optional per-controller requeue periods
  requeuePeriodMap:
    Deployment: 60s
```

## How It Works

### Configuration Resolution

For each CRD kind (e.g., "PipelineRun"):

1. **Check map first:** If `concurrentReconcilesMap["PipelineRun"]` exists, use that value
2. **Fallback to global:** Otherwise, use `concurrentReconciles` value
3. **Default to 1:** If neither is set, controller defaults to 1 (in SetupWithManager)

### Example Scenarios

**Scenario 1: Legacy config (still works)**
```yaml
ingester:
  concurrentReconciles: 2
  requeuePeriod: 30s
```
Result: All controllers use concurrency=2

**Scenario 2: Mixed config (recommended)**
```yaml
ingester:
  concurrentReconciles: 2           # Default for most
  concurrentReconcilesMap:
    PipelineRun: 10                 # Override for high-traffic
```
Result: PipelineRun uses 10, all others use 2

**Scenario 3: Map-only config**
```yaml
ingester:
  concurrentReconcilesMap:
    PipelineRun: 10
    Deployment: 3
    Model: 5
```
Result: Specified controllers use map values, others default to 0 → 1 (controller default)

## Testing

### Verify Configuration Loading

```bash
# Check logs when controller starts
kubectl logs -n michelangelo-system michelangelo-controllermgr-xxx | grep "Ingester controller registered"

# Should show lines like:
# Ingester controller registered successfully  kind=PipelineRun concurrentReconciles=10
# Ingester controller registered successfully  kind=Deployment concurrentReconciles=3
# Ingester controller registered successfully  kind=Model concurrentReconciles=2
```

### Verify Runtime Behavior

```bash
# Monitor reconciliation concurrency
kubectl get pods -n michelangelo-system -w

# Check metrics (if enabled)
curl http://michelangelo-controllermgr:8091/metrics | grep ingester
```

### Unit Test Example

Add to `go/base/config/ingester_config_test.go`:

```go
func TestGetControllerConfig(t *testing.T) {
    tests := []struct {
        name           string
        config         IngesterConfig
        crdKind        string
        expectedConcur int
        expectedReq    time.Duration
    }{
        {
            name: "map override",
            config: IngesterConfig{
                ConcurrentReconciles: 2,
                RequeuePeriod:        30 * time.Second,
                ConcurrentReconcilesMap: map[string]int{
                    "PipelineRun": 10,
                },
            },
            crdKind:        "PipelineRun",
            expectedConcur: 10,
            expectedReq:    30 * time.Second,
        },
        {
            name: "fallback to default",
            config: IngesterConfig{
                ConcurrentReconciles: 2,
                RequeuePeriod:        30 * time.Second,
                ConcurrentReconcilesMap: map[string]int{
                    "PipelineRun": 10,
                },
            },
            crdKind:        "Model",
            expectedConcur: 2,
            expectedReq:    30 * time.Second,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := tt.config.GetControllerConfig(tt.crdKind)
            assert.Equal(t, tt.expectedConcur, result.ConcurrentReconciles)
            assert.Equal(t, tt.expectedReq, result.RequeuePeriod)
        })
    }
}
```

## Migration Guide

### For Existing Deployments

**Option 1: No changes needed**
- Keep current config as-is
- All controllers continue using global values

**Option 2: Gradual migration**
```yaml
ingester:
  concurrentReconciles: 2  # Keep as default
  concurrentReconcilesMap:
    PipelineRun: 10        # Start by overriding high-traffic CRDs
```

**Option 3: Full migration**
```yaml
ingester:
  # Set defaults to 0 (controllers will use internal default of 1)
  concurrentReconciles: 0

  # Explicitly configure all controllers
  concurrentReconcilesMap:
    PipelineRun: 10
    Deployment: 3
    Pipeline: 3
    Revision: 3
    Model: 5
    # ... etc
```

### Recommended Values (Based on Uber Internal)

```yaml
concurrentReconcilesMap:
  PipelineRun: 10        # High-throughput workloads
  Deployment: 3          # Moderate deployment traffic
  Pipeline: 3            # Pipeline definition changes
  Revision: 3            # Version management
  InferenceServer: 3     # Server lifecycle
  Model: 2               # Model registry
  ModelFamily: 2         # Model organization
  Project: 1             # Low-frequency admin
  Cluster: 1             # Infrastructure changes
  RayCluster: 1          # Compute cluster lifecycle
  RayJob: 2              # Job submissions
  SparkJob: 2            # Job submissions
  TriggerRun: 5          # Automated runs
```

## Performance Tuning

### When to Increase Concurrency

✅ Increase if:
- Many CRDs of this type are created/updated frequently
- Reconciliation is fast and CPU-light
- You observe reconciliation lag (check metrics)

❌ Don't increase if:
- Reconciliation involves heavy I/O or external API calls
- Controllers have shared resource contention
- Current concurrency handles load fine

### Monitoring

Key metrics to watch:
- `workqueue_depth` - Pending reconciliations
- `workqueue_adds_total` - Reconciliation requests
- `ingester_latency` - Time to sync to MySQL
- Controller error logs

## Rollback Plan

If issues occur:

1. **Quick rollback:** Remove map config, keep only legacy fields
```yaml
ingester:
  concurrentReconciles: 2
  requeuePeriod: 30s
  # Remove concurrentReconcilesMap
```

2. **Partial rollback:** Reduce specific controller concurrency
```yaml
concurrentReconcilesMap:
  PipelineRun: 5  # Reduced from 10
```

3. **Full rollback:** Revert code changes (all changes are backwards compatible)

## Future Enhancements

Potential improvements:
- [ ] Add validation for map values (must be > 0)
- [ ] Add metrics per-controller for concurrency utilization
- [ ] Support dynamic config reload without restart
- [ ] Add CLI command to show current controller configs
- [ ] Add per-controller worker pool metrics

## References

- Internal implementation: `/home/user/Uber/go-code/src/code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/ingester/controller.go`
- Kubernetes controller-runtime: https://pkg.go.dev/sigs.k8s.io/controller-runtime
- Uber Go Style Guide: Follow `MixedCaps` for map keys (e.g., "PipelineRun" not "pipeline_run")
