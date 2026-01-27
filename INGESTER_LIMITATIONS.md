# Michelangelo Ingester - Limitations and Missing Pieces

## 🔴 Critical Missing Pieces

### 1. **Ingester Controller Not Wired to Controllermgr**
**Status**: Implementation exists but not integrated

**What's Missing:**
- Ingester module not imported in main controllermgr
- No Fx module wiring in `go/controllermgr/module.go`
- Configuration not loaded

**What Needs to Be Done:**
```go
// In go/controllermgr/module.go
import (
    "github.com/michelangelo-ai/michelangelo/go/components/ingester"
    "github.com/michelangelo-ai/michelangelo/go/storage/mysql"
    "github.com/michelangelo-ai/michelangelo/go/storage/minio"
)

var Module = fx.Options(
    // ... existing modules
    ingester.Module,  // Add this
    fx.Provide(newMetadataStorage),  // Provide MySQL storage
    fx.Provide(newBlobStorage),      // Provide MinIO storage
)
```

**Impact**: Controller won't run until this is wired up

---

### 2. **MySQL Storage Implementation Incomplete**

**Missing Functions:**
- ❌ `DirectUpdate()` - Optimistic concurrency control for metadata-only updates
- ❌ `DeleteCollection()` - Bulk delete operations
- ❌ `QueryByTemplateID()` - Predefined query templates
- ❌ `Backfill()` - Bulk import of existing K8s objects to MySQL
- ⚠️ `List()` - Basic implementation exists but label selectors not fully working
- ⚠️ `createObjectFromTypeMeta()` - Needs K8s scheme integration

**What Works:**
- ✅ `Upsert()` - Insert/update objects
- ✅ `GetByName()` - Retrieve by namespace/name
- ✅ `GetByID()` - Retrieve by UID
- ✅ `Delete()` - Soft delete
- ✅ Connection pooling
- ✅ Transactions

**Impact**: Some advanced features won't work

---

### 3. **Schema Only Covers 5 CRDs**

**Covered CRDs:**
- ✅ Model
- ✅ Pipeline
- ✅ PipelineRun
- ✅ Dataset
- ✅ Deployment

**Missing CRDs** (exist in internal but not in OSS schema):
- ❌ FeatureGroup
- ❌ FeaturePackage
- ❌ Feature
- ❌ Endpoint
- ❌ Agent
- ❌ GenAIService
- ❌ InferenceServer
- ❌ Cluster
- ❌ Storage
- ❌ Dashboard
- ❌ Alert
- ❌ ~40+ more CRDs

**Impact**: Only 5 CRDs will be ingested to MySQL

---

### 4. **Indexed Fields Limited**

**Current Indexed Fields per CRD:**

**Model:**
- algorithm
- training_framework
- owner

**Pipeline:**
- pipeline_type
- owner

**PipelineRun:**
- status

**Dataset:**
- dataset_type
- owner

**Deployment:**
- deployment_type
- owner
- status

**Missing Common Indexed Fields:**
- Created by user
- Last modified by user
- Project ID
- Tags
- Most custom fields from CRD specs

**Impact**: Cannot efficiently query by non-indexed fields

---

### 5. **No Blob Storage Integration**

**Status**: MinIO code exists but not connected

**What's Missing:**
- No CRDs implement `ObjectWithBlobFields` interface
- No logic to determine which fields should go to blob storage
- Ingester doesn't call blob storage upload/download
- No blob field clearing before MySQL storage

**Code Exists But Unused:**
```go
// In ingester controller
if r.BlobStorage != nil {
    if blobObj, ok := object.(storage.ObjectWithBlobFields); ok && blobObj.HasBlobFields() {
        // TODO: Upload blob fields to MinIO
    }
}
```

**Impact**: Large objects stored entirely in MySQL (16MB proto limit)

---

### 6. **No Scheme Integration**

**Problem**: `createObjectFromTypeMeta()` not implemented

**Current Code:**
```go
func createObjectFromTypeMeta(typeMeta *metav1.TypeMeta) (runtime.Object, error) {
    // TODO: Use scheme to create object dynamically
    return nil, fmt.Errorf("createObjectFromTypeMeta not fully implemented - needs scheme integration")
}
```

**Impact**: `List()` operations may not work correctly when deserializing objects

---

### 7. **No Tests**

**Missing Test Coverage:**
- ❌ No unit tests for MySQL storage
- ❌ No unit tests for MinIO storage
- ❌ No unit tests for ingester controller
- ❌ No integration tests
- ❌ No end-to-end tests
- ❌ No load/performance tests

**Impact**: Unknown bugs, no regression prevention, unclear performance characteristics

---

### 8. **No Metrics or Monitoring**

**Missing Observability:**
- ❌ No Prometheus metrics
- ❌ No ingestion rate tracking
- ❌ No error rate tracking
- ❌ No latency measurements
- ❌ No MySQL connection pool metrics
- ❌ No reconciliation lag metrics
- ❌ No success/failure counters

**Impact**: Cannot monitor health or performance in production

---

### 9. **No Health Checks**

**Missing:**
- ❌ No readiness probe for ingester
- ❌ No liveness probe
- ❌ No MySQL connectivity checks
- ❌ No MinIO connectivity checks
- ❌ No startup checks

**Impact**: K8s can't detect unhealthy ingester, may send traffic to dead controllers

---

### 10. **Configuration Management Issues**

**Problems:**
- ⚠️ Credentials in plain text (config files)
- ❌ No secrets management (Vault, K8s Secrets)
- ❌ No environment-specific configs
- ❌ No config validation
- ❌ Hard to change config without rebuild

**Current Approach:**
```yaml
mysql:
  password: "root"  # ⚠️ Plain text!
```

**Should Be:**
```yaml
mysql:
  passwordSecret: "mysql-password"  # Reference to K8s Secret
```

**Impact**: Security risk, inflexible configuration

---

## ⚠️ Significant Limitations

### 11. **Label Selector Queries Not Fully Implemented**

**Current State:**
```go
// In List() function
if listOptions != nil && listOptions.LabelSelector != "" {
    // TODO: Implement proper label selector parsing and SQL generation
    // For now, this is a simplified version
}
```

**What's Missing:**
- No parsing of complex label selectors (e.g., `environment in (prod, staging), tier!=frontend`)
- No support for label operators (==, !=, in, notin, exists, doesNotExist)
- No combination with field selectors

**Impact**: Advanced label-based queries won't work

---

### 12. **No Resource Version Cache**

**Missing:**
- Resource version caching for optimistic concurrency
- Conflict detection for concurrent updates
- Version history tracking

**Impact**:
- Race conditions possible with concurrent updates
- No protection against overwriting newer data with older data

---

### 13. **No Schema Migration Strategy**

**Problems:**
- Schema is fixed in ConfigMap
- No migration scripts
- No version tracking
- No rollback capability
- Breaking changes require manual intervention

**Example Scenario:**
```
1. Add new indexed field to Model CRD
2. Need to update MySQL schema
3. No automated way to add column
4. Need manual ALTER TABLE
5. Risk of downtime or data loss
```

**Impact**: Schema evolution is manual and error-prone

---

### 14. **No Batch Operations**

**Missing:**
- Batch upsert (sync 100 objects in one transaction)
- Batch delete
- Batch query
- Streaming results for large result sets

**Current:**
- Objects synced one at a time
- Each sync is a separate transaction

**Impact**:
- Slow for bulk operations
- High overhead for mass imports
- Backfill would be very slow

---

### 15. **No Caching Layer**

**Missing:**
- No in-memory cache for frequently accessed objects
- No query result caching
- No distributed cache (Redis)
- Every query hits MySQL

**Impact**:
- Higher MySQL load
- Higher latency for reads
- No benefit from repeated queries

---

### 16. **Soft Delete Never Hard Deletes**

**Problem:**
- Objects are soft deleted (delete_time set)
- No cleanup job to actually remove old deleted objects
- MySQL grows indefinitely

**Current Behavior:**
```sql
-- Deletion just sets delete_time
UPDATE model SET delete_time = NOW() WHERE uid = 'xxx';

-- But row never actually deleted
-- Database size grows forever
```

**Impact**: MySQL database will grow without bounds over time

---

### 17. **No High Availability**

**Missing:**
- No leader election (multiple ingesters would conflict)
- No failover mechanism
- Single point of failure
- No active-passive setup

**Current:**
- Only one ingester instance can run safely
- If it crashes, ingestion stops until restart

**Impact**: Not production-ready, no fault tolerance

---

### 18. **Limited Error Handling**

**Problems:**
- Simple retry logic (fixed 30s requeue)
- No exponential backoff
- No circuit breakers
- No dead letter queue for persistent failures
- Errors just logged and retried forever

**Impact**: Failed objects keep retrying indefinitely, wasting resources

---

### 19. **No Graceful Shutdown**

**Missing:**
- No signal handling for shutdown
- MySQL connections abruptly closed
- In-flight reconciliations may be interrupted
- No connection draining

**Impact**: Potential data loss or corruption during shutdown/restart

---

### 20. **Performance Not Optimized**

**Issues:**
- No connection pooling tuning guidance
- No query optimization
- No index tuning
- No prepared statements
- No query profiling
- Labels/annotations deleted and reinserted on every update (even if unchanged)

**Impact**: May not scale to high throughput scenarios

---

## 🟡 Minor Issues and Gaps

### 21. **Limited Documentation**

**Missing:**
- No API documentation
- No architecture decision records (ADRs)
- No troubleshooting runbooks
- No capacity planning guide
- No disaster recovery procedures

---

### 22. **No Alerting**

**Missing:**
- No PagerDuty/alert manager integration
- No SLO definitions
- No error rate thresholds
- No latency alerts

---

### 23. **No Audit Logging**

**Missing:**
- No audit trail of who/what/when for changes
- No compliance logging
- No change history beyond soft delete

---

### 24. **No Rate Limiting**

**Missing:**
- No protection against thundering herd
- No backpressure mechanism
- No request throttling

---

### 25. **No Data Retention Policies**

**Missing:**
- No automatic cleanup of old data
- No archival strategy
- No compliance with data retention requirements

---

### 26. **MinIO Not Configured in Sandbox by Default**

**Current State:**
- MinIO exists in sandbox
- But ingester config doesn't include MinIO by default
- Need to manually configure

**In sandbox config should add:**
```yaml
minio:
  enabled: true
  endpoint: "minio:9000"
  accessKeyID: "minioadmin"
  secretAccessKey: "minioadmin"
  bucketName: "michelangelo-metadata"
```

---

### 27. **No Support for Immutable Objects in Practice**

**Code Exists:**
```go
if isImmutable(object) {
    return r.handleImmutableObject(ctx, log, object)
}
```

**But Missing:**
- No CRDs actually set ImmutableAnnotation
- No documentation on when to use
- No API support for marking objects immutable

**Impact**: ETCD savings feature not usable

---

### 28. **Concurrent Reconciles Not Tuned**

**Current:**
```go
concurrentReconciles := r.Config.ConcurrentReconciles
if concurrentReconciles <= 0 {
    concurrentReconciles = 1  // Default to 1
}
```

**Issues:**
- Default of 1 is very conservative
- No guidance on tuning
- No auto-scaling based on load

---

### 29. **MySQL Schema Not Optimized**

**Missing Optimizations:**
- No covering indexes
- No partitioning for large tables
- No table compression
- No index hints in queries

---

### 30. **No Disaster Recovery**

**Missing:**
- No backup strategy
- No point-in-time recovery
- No cross-region replication
- No backup testing

---

## 📊 Summary by Category

| Category | Implemented | Missing | Partially Done |
|----------|-------------|---------|----------------|
| **Core Functionality** | 60% | 40% | - |
| **Storage** | 70% | 30% | - |
| **Observability** | 0% | 100% | - |
| **Testing** | 0% | 100% | - |
| **Security** | 20% | 80% | - |
| **Operations** | 30% | 70% | - |
| **Performance** | 40% | 60% | - |
| **Documentation** | 70% | 30% | - |

---

## 🎯 Priority Roadmap

### **P0 - Must Have for Basic Functionality**
1. Wire ingester into controllermgr
2. Add scheme integration for List() operations
3. Add basic metrics (ingestion rate, errors)
4. Test in sandbox end-to-end
5. Add health checks

### **P1 - Needed for Production**
1. Implement all remaining MySQL operations
2. Add comprehensive test suite
3. Secrets management integration
4. High availability / leader election
5. Monitoring and alerting
6. Schema migration strategy

### **P2 - Nice to Have**
1. Blob storage integration
2. Performance optimization
3. Caching layer
4. More CRD coverage
5. Advanced query features

### **P3 - Future Enhancements**
1. Multi-region support
2. Advanced analytics
3. Query optimization
4. Auto-tuning

---

## ✅ What's Actually Working

To be fair, here's what **is** working:

- ✅ MySQL schema automatically created in sandbox
- ✅ Basic CRUD operations (Upsert, Get, Delete)
- ✅ Ingester controller logic (reconciliation)
- ✅ Finalizer handling
- ✅ Grace period support
- ✅ Soft delete
- ✅ Labels and annotations storage
- ✅ Connection pooling
- ✅ Sandbox integration
- ✅ Configuration structure
- ✅ Documentation (guides, examples)

**Bottom Line**: The foundation is solid, but production readiness requires significant additional work.
