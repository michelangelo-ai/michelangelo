# Ingester Implementation - Quick Checklist

## 🔴 Critical Blockers (Must Fix to Run)

- [ ] **Wire ingester module into controllermgr**
  - Location: `go/controllermgr/module.go`
  - Add: `ingester.Module` to fx.Options
  - Add: MySQL and MinIO provider functions

- [ ] **Add scheme integration for object creation**
  - Location: `go/storage/mysql/mysql.go`
  - Fix: `createObjectFromTypeMeta()` function
  - Connect to K8s scheme registry

- [ ] **Add ingester config to sandbox controllermgr**
  - Create controllermgr deployment YAML with ingester enabled
  - Set `enableMetadataStorage: true`
  - Configure MySQL connection details

## ⚠️ High Priority (Needed Soon)

- [ ] **Add basic metrics**
  - Ingestion rate counter
  - Error counter
  - Reconciliation duration histogram

- [ ] **Add health checks**
  - MySQL connectivity check
  - Readiness probe
  - Liveness probe

- [ ] **Write basic tests**
  - Unit test for MySQL Upsert
  - Unit test for ingester reconciliation
  - Integration test for full flow

- [ ] **Implement label selector queries**
  - Parse label selector syntax
  - Generate SQL WHERE clauses
  - Test complex selectors

- [ ] **Add secrets management**
  - Use K8s Secrets for MySQL password
  - Use K8s Secrets for MinIO credentials
  - Remove plain text passwords

## 🟡 Medium Priority (Nice to Have)

- [ ] **Complete MySQL implementation**
  - DirectUpdate() with optimistic locking
  - DeleteCollection()
  - QueryByTemplateID()
  - Backfill()

- [ ] **Add more CRD coverage**
  - FeatureGroup
  - Endpoint
  - InferenceServer
  - (20+ more)

- [ ] **Integrate blob storage**
  - Define ObjectWithBlobFields for CRDs
  - Wire up MinIO upload in ingester
  - Add blob field clearing logic

- [ ] **Add schema migration**
  - Version tracking
  - Migration scripts
  - Rollback capability

## 🟢 Low Priority (Future)

- [ ] **Performance optimization**
  - Batch operations
  - Query caching
  - Connection pool tuning

- [ ] **High availability**
  - Leader election
  - Multiple replicas
  - Failover testing

- [ ] **Advanced features**
  - Audit logging
  - Data retention policies
  - Multi-region support

## 📝 Current Status

| Component | Status |
|-----------|--------|
| MySQL Schema | ✅ **Done** (in sandbox) |
| MySQL Storage Code | 🟡 **70% done** (basic ops work) |
| MinIO Storage Code | 🟡 **80% done** (not integrated) |
| Ingester Controller | ✅ **Done** (logic complete) |
| Wiring to controllermgr | ❌ **Not started** |
| Tests | ❌ **None** |
| Metrics | ❌ **None** |
| Documentation | ✅ **Good** |

## 🚀 Next Steps to Get It Working

### Step 1: Wire Everything Together (1-2 hours)

```go
// go/controllermgr/module.go
import (
    "github.com/michelangelo-ai/michelangelo/go/components/ingester"
    mysqlstorage "github.com/michelangelo-ai/michelangelo/go/storage/mysql"
    miniostorage "github.com/michelangelo-ai/michelangelo/go/storage/minio"
)

var Module = fx.Options(
    // ... existing ...
    ingester.Module,
    fx.Provide(provideMetadataStorage),
    fx.Provide(provideBlobStorage),
    fx.Provide(provideIngesterConfig),
)

func provideMetadataStorage(config Config) (storage.MetadataStorage, error) {
    if !config.MySQL.Enabled {
        return nil, nil
    }
    return mysqlstorage.NewMetadataStorage(config.MySQL.ToMySQLConfig())
}

func provideBlobStorage(config Config) (storage.BlobStorage, error) {
    if !config.MinIO.Enabled {
        return nil, nil
    }
    return miniostorage.NewBlobStorage(config.MinIO.ToMinIOConfig())
}

func provideIngesterConfig(config Config) ingester.Config {
    return config.Ingester.ToIngesterConfig()
}
```

### Step 2: Fix Scheme Integration (30 minutes)

```go
// go/storage/mysql/mysql.go
import "k8s.io/apimachinery/pkg/runtime"

type mysqlMetadataStorage struct {
    db     *sql.DB
    config Config
    scheme *runtime.Scheme  // Add this
}

func NewMetadataStorage(config Config, scheme *runtime.Scheme) (storage.MetadataStorage, error) {
    // ...
    return &mysqlMetadataStorage{
        db:     db,
        config: config,
        scheme: scheme,  // Store scheme
    }, nil
}

func (m *mysqlMetadataStorage) createObjectFromTypeMeta(typeMeta *metav1.TypeMeta) (runtime.Object, error) {
    gvk := schema.GroupVersionKind{
        Group:   strings.Split(typeMeta.APIVersion, "/")[0],
        Version: strings.Split(typeMeta.APIVersion, "/")[1],
        Kind:    typeMeta.Kind,
    }
    return m.scheme.New(gvk)
}
```

### Step 3: Test in Sandbox (30 minutes)

```bash
# Build
bazel build //go/controllermgr/...

# Update controllermgr deployment with ingester config
# Restart controllermgr

# Test create
kubectl apply -f test-model.yaml

# Verify in MySQL
mysql -h localhost -u root -proot michelangelo -e "SELECT * FROM model;"
```

### Step 4: Add Basic Metrics (1 hour)

```go
// go/components/ingester/controller.go
import "github.com/prometheus/client_golang/prometheus"

var (
    ingestionsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "ingester_ingestions_total",
            Help: "Total number of ingestions",
        },
        []string{"kind", "result"},
    )
)

func init() {
    prometheus.MustRegister(ingestionsTotal)
}

func (r *Reconciler) handleSync(...) {
    err := r.MetadataStorage.Upsert(...)
    if err != nil {
        ingestionsTotal.WithLabelValues(kind, "error").Inc()
    } else {
        ingestionsTotal.WithLabelValues(kind, "success").Inc()
    }
}
```

## ⏱️ Estimated Time to Production Ready

- **Minimal Viable** (sandbox demo): **2-4 hours**
- **Alpha** (basic production): **1-2 weeks**
- **Beta** (full features): **4-6 weeks**
- **GA** (production hardened): **2-3 months**

## 📚 Reference Documents

- Main implementation: `INGESTER_IMPLEMENTATION_SUMMARY.md`
- Sandbox integration: `INGESTER_SANDBOX_INTEGRATION.md`
- Testing guide: `SANDBOX_INGESTER_GUIDE.md`
- Limitations: `INGESTER_LIMITATIONS.md`
- Architecture details: `ingester_detailed_architecture.md`
- Finalizers guide: `finalizer_implementation_guide.md`
