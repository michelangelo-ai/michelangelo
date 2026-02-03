# Ingester Production Readiness - Gaps & Requirements

## Overview

This document outlines what's needed to make the Michelangelo ingester production-ready. The core functionality is complete and tested, but several gaps need to be addressed before production deployment.

---

## 🔴 Critical Gaps (Must Fix - P0)

### 1. Testing (Effort: 2-3 weeks)

**What's Missing:**
- ❌ No unit tests for ingester controller
- ❌ No unit tests for MySQL storage  
- ❌ No unit tests for MinIO storage
- ❌ No integration tests
- ❌ No load tests
- ❌ No chaos/failure tests

**Impact**: Unknown behavior under failures, race conditions, edge cases

**Required Tests:**
```
Unit Tests:
├── MySQL storage
│   ├── Upsert success/failure
│   ├── GetByName/GetByID
│   ├── List with filters
│   ├── Delete (soft delete)
│   └── Label/annotation sync
├── Ingester controller
│   ├── Reconcile create/update/delete
│   ├── Finalizer handling
│   ├── Immutable object handling
│   └── Error handling/retry
└── MinIO storage
    ├── Upload/download
    ├── Tag management
    └── Bucket operations

Integration Tests:
├── End-to-end: K8s → Ingester → MySQL
├── Label sync verification
├── Finalizer lifecycle
└── Concurrent operations

Load Tests:
├── 1000+ objects
├── Concurrent creates
├── Update storms
└── Delete cascades

Chaos Tests:
├── MySQL connection failures
├── MySQL restarts
├── Network partitions
└── Resource exhaustion
```

### 2. Table Naming Inconsistency (Effort: 1 day)

**Problem:**
```go
// Code: go/storage/mysql/mysql.go:446
func getTableName(object runtime.Object) string {
    gvk := object.GetObjectKind().GroupVersionKind()
    return strings.ToLower(gvk.Kind)  // ← Returns "pipelinerun"
}

// Schema: scripts/ingester_schema.sql
CREATE TABLE `pipeline_run` ...  // ← Has underscore!
```

**Impact**: PipelineRun objects can't sync (table name mismatch)

**Solution**:
```go
// Option A: Use snake_case conversion
import "github.com/michelangelo-ai/michelangelo/go/api/utils"

func getTableName(object runtime.Object) string {
    gvk := object.GetObjectKind().GroupVersionKind()
    return utils.ToSnakeCase(gvk.Kind)  // ← Returns "pipeline_run"
}

// Option B: Table name mapping
var tableNameOverrides = map[string]string{
    "pipelinerun": "pipeline_run",
}
```

### 3. Schema Auto-Generation (Effort: 1 week)

**Problem:**
- Schema is hand-written (error-prone)
- Can get out of sync with protobuf
- Manual updates when adding indexed fields

**Impact**: Schema drift, missing columns, sync failures

**Solution:**
```bash
# 1. Build protoc-gen-sql integration
bazel run //go/cmd/kubeproto/protoc-gen-sql -- \
  --proto_path=proto/api/v2 \
  --output=scripts/ingester_schema.sql \
  model.proto pipeline.proto ...

# 2. Add CI validation
# In .github/workflows:
- name: Validate Schema
  run: |
    ./scripts/generate_schema.sh > /tmp/generated.sql
    diff scripts/ingester_schema.sql /tmp/generated.sql
    # Fail if schema is out of sync

# 3. Add pre-commit hook
scripts/validate_schema.sh
```

### 4. Security Hardening (Effort: 1-2 weeks)

**Issues:**
- ❌ Credentials in plain text
- ❌ No TLS for MySQL
- ❌ No TLS for MinIO
- ❌ No encryption at rest
- ❌ No audit logging

**Required:**

**Secrets Management:**
```yaml
# Use Kubernetes Secrets
apiVersion: v1
kind: Secret
metadata:
  name: ingester-mysql-creds
type: Opaque
stringData:
  host: prod-mysql.example.com
  username: ingester_user
  password: secure_random_password_here
  database: michelangelo
```

**TLS Configuration:**
```yaml
mysql:
  host: prod-mysql.example.com
  port: 3306
  tls:
    enabled: true
    caFile: /certs/ca.pem
    certFile: /certs/client-cert.pem
    keyFile: /certs/client-key.pem
    skipVerify: false  # Verify certificate

minio:
  endpoint: prod-minio.example.com:9000
  useSSL: true
  tlsConfig:
    insecureSkipVerify: false
```

**Vault Integration:**
```go
// Use HashiCorp Vault for secrets
import "github.com/hashicorp/vault/api"

func getMySQLCredsFromVault() (MySQLConfig, error) {
    client, _ := vault.NewClient(...)
    secret, _ := client.Logical().Read("secret/data/ingester/mysql")
    // Extract credentials
}
```

### 5. High Availability (Effort: 1 week)

**Problem:**
- Single controllermgr instance
- No failover
- Leader election not enabled

**Impact**: Single point of failure, potential data corruption with multiple replicas

**Solution:**
```yaml
# Enable leader election in controllermgr config
controllermgr:
  leaderElection: true  # ← Currently false!
  leaderElectionID: ingester.michelangelo.uber.com
  leaseDuration: 15s
  renewDeadline: 10s
  retryPeriod: 2s
```

```go
// In go/cmd/controllermgr/main.go
mgr, err := ctrl.NewManager(cfg, ctrl.Options{
    LeaderElection: true,
    LeaderElectionID: "ingester.michelangelo.uber.com",
    // ... other options
})
```

**Testing:**
- Deploy 3 replicas
- Kill leader, verify new leader elected
- Ensure no duplicate writes during transition

### 6. Metrics & Monitoring (Effort: 3-4 days)

**Required Metrics:**

```go
// Add to go/components/ingester/controller.go
import "github.com/prometheus/client_golang/prometheus"

var (
    objectsSyncedTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "ingester_objects_synced_total",
            Help: "Total objects synced to metadata storage",
        },
        []string{"kind", "status"},  // status: success|error
    )
    
    syncDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "ingester_sync_duration_seconds",
            Help: "Time taken to sync object",
            Buckets: prometheus.DefBuckets,
        },
        []string{"kind"},
    )
    
    errorsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "ingester_errors_total",
            Help: "Total sync errors",
        },
        []string{"kind", "error_type"},
    )
    
    queueDepth = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "ingester_queue_depth",
            Help: "Number of objects waiting to be reconciled",
        },
        []string{"kind"},
    )
)

func init() {
    prometheus.MustRegister(objectsSyncedTotal, syncDuration, errorsTotal, queueDepth)
}
```

**Alerting Rules:**
```yaml
# Prometheus rules
groups:
  - name: ingester
    rules:
      - alert: IngesterHighErrorRate
        expr: rate(ingester_errors_total[5m]) > 0.1
        annotations:
          summary: "Ingester error rate is high"
      
      - alert: IngesterSyncLatency
        expr: histogram_quantile(0.99, ingester_sync_duration_seconds) > 5
        annotations:
          summary: "Ingester sync latency is high"
      
      - alert: IngesterQueueBacklog
        expr: ingester_queue_depth > 1000
        annotations:
          summary: "Ingester has large backlog"
```

---

## 🟡 High Priority Gaps (P1)

### 7. Schema Migrations (Effort: 1 week)

**Use Flyway or golang-migrate:**

```
migrations/
├── V001__initial_schema.sql
├── V002__add_model_revision_id.sql
├── V003__add_project_tier.sql
└── ...

# Apply migrations
flyway -url=jdbc:mysql://prod-mysql/michelangelo -user=ingester migrate
```

### 8. Error Handling & Retry (Effort: 3-4 days)

**Improvements:**
```go
// Exponential backoff
import "k8s.io/apimachinery/pkg/util/wait"

backoff := wait.Backoff{
    Duration: 1 * time.Second,
    Factor: 2.0,
    Steps: 5,
    Cap: 30 * time.Second,
}

// Circuit breaker
import "github.com/sony/gobreaker"

cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
    Name: "mysql",
    MaxRequests: 3,
    Interval: 60 * time.Second,
    Timeout: 120 * time.Second,
})
```

### 9. Observability (Effort: 1 week)

- Add correlation IDs to logs
- OpenTelemetry integration
- Distributed tracing
- SLO/SLI definitions

### 10. Blob Storage Integration (Effort: 1 week)

**Complete the implementation:**
```go
// Implement on CRDs that need blob storage
func (m *Model) HasBlobFields() bool {
    return m.Spec.TrainedModelArtifact != ""
}

func (m *Model) GetBlobFields() map[string][]byte {
    return map[string][]byte{
        "trained_model": m.Spec.TrainedModelArtifact,
    }
}

// Wire up in ingester
if blobObj.HasBlobFields() {
    err := r.BlobStorage.UploadToBlobStorage(ctx, object)
    // ...
}
```

### 11. Backfill Functionality (Effort: 3-4 days)

```go
// Add Backfill command
func Backfill(ctx context.Context, crdType string) error {
    // 1. List all objects of type from K8s
    objects := listAllObjects(crdType)
    
    // 2. Batch insert to MySQL
    for batch := range objects.Batch(100) {
        metadataStorage.BatchUpsert(ctx, batch)
    }
}
```

---

## 🟢 Medium Priority Gaps (P2)

### 12-16. Query Optimization, Field Selectors, Graceful Shutdown, etc.

See detailed breakdown above.

---

## 📊 Recommended Roadmap

### Phase 1: MVP (3 weeks)
- Week 1: Testing framework + critical tests
- Week 2: Security (Secrets, TLS) + Table naming fix
- Week 3: Basic metrics + documentation

### Phase 2: Alpha (4 additional weeks)
- Week 4-5: Schema auto-gen + migrations
- Week 6: HA/Leader election  
- Week 7: Observability + error handling

### Phase 3: GA (8-12 additional weeks)
- Weeks 8-9: Complete test coverage
- Weeks 10-11: Performance optimization
- Weeks 12-15: Multi-region, advanced features
- Weeks 16-19: Hardening, documentation, runbooks

---

## ✅ What's Already Production-Ready

- ✅ Core sync logic (tested with 3 models)
- ✅ MySQL connection pooling
- ✅ Transaction-based operations
- ✅ Finalizer handling
- ✅ Soft delete support
- ✅ Label/annotation separation
- ✅ Idempotent schema
- ✅ Complete CRD coverage (schema for all 13 CRDs)
- ✅ Single canonical SQL file
- ✅ Kubernetes Job deployment
- ✅ Configuration system

---

## 🎯 Minimum Bar for Production

**Must Have (P0):**
1. ✅ Testing (at least critical paths)
2. ✅ Security (Secrets + TLS)
3. ✅ Basic metrics
4. ✅ Table naming fix
5. ✅ Documentation

**Timeline**: ~3 weeks of focused work

**After this**: Feature can go to production with monitoring and gradual rollout.
