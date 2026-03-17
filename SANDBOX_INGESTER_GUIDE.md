# Michelangelo Ingester - Sandbox Setup and Testing Guide

## Overview

This guide walks you through setting up and testing the Michelangelo Ingester controller in a sandbox environment. The ingester syncs CRD objects from Kubernetes/ETCD to MySQL (metadata storage) and MinIO (blob storage), enabling fast search and reducing ETCD load.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                      Michelangelo API Server                     │
│  (Adds IngesterFinalizer to objects during creation)             │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Kubernetes / ETCD                             │
│  (CRD objects with finalizers)                                   │
└──────┬──────────────────────────────────────────────────────────┘
       │
       │ Watch Events
       ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Ingester Controller                           │
│  - Watches all configured CRD types                              │
│  - Syncs create/update events to MySQL                           │
│  - Handles deletion with finalizers                              │
│  - Manages immutable objects                                     │
└──────┬───────────────────────────────┬──────────────────────────┘
       │                               │
       ▼                               ▼
┌──────────────────┐          ┌──────────────────┐
│  MySQL Database  │          │  MinIO (S3)      │
│  (Metadata)      │          │  (Blob Storage)  │
└──────────────────┘          └──────────────────┘
```

---

## Prerequisites

### 1. Software Requirements

- **Kubernetes cluster** (minikube, kind, or any K8s cluster)
- **MySQL 5.7+** or **MySQL 8.0**
- **MinIO** (for blob storage)
- **Go 1.20+** (for building)
- **kubectl** (for interacting with K8s)
- **mactl** (Michelangelo CLI tool)

### 2. Environment Setup

```bash
# Start MySQL (using Docker)
docker run --name mysql-michelangelo \
  -e MYSQL_ROOT_PASSWORD=rootpass \
  -e MYSQL_DATABASE=michelangelo \
  -p 3306:3306 \
  -d mysql:8.0

# Start MinIO (using Docker)
docker run --name minio-michelangelo \
  -e MINIO_ROOT_USER=minioadmin \
  -e MINIO_ROOT_PASSWORD=minioadmin \
  -p 9000:9000 \
  -p 9001:9001 \
  -d minio/minio server /data --console-address ":9001"

# Start Kubernetes (if using minikube)
minikube start
```

---

## Step 1: Database Setup

### Run MySQL Setup Script

```bash
cd /home/user/Uber/michelangelo

# Set MySQL credentials
export MYSQL_HOST=localhost
export MYSQL_PORT=3306
export MYSQL_USER=root
export MYSQL_PASSWORD=rootpass
export MYSQL_DATABASE=michelangelo

# Run the setup script
./scripts/init_ingester_db.sh

# Or use Kubernetes Job:
kubectl apply -f scripts/ingester-schema-init-job.yaml
```

### Verify Tables

```bash
mysql -h localhost -P 3306 -u root -prootpass michelangelo -e "SHOW TABLES;"
```

**Expected Output:**
```
+------------------------+
| Tables_in_michelangelo |
+------------------------+
| dataset                |
| dataset_annotations    |
| dataset_labels         |
| deployment             |
| deployment_annotations |
| deployment_labels      |
| model                  |
| model_annotations      |
| model_labels           |
| pipeline               |
| pipeline_annotations   |
| pipeline_labels        |
| pipeline_run           |
| pipeline_run_annotations|
| pipeline_run_labels    |
+------------------------+
```

---

## Step 2: Configure Michelangelo

### Edit Configuration

Create or update `config/sandbox_ingester.yaml`:

```yaml
mysql:
  enabled: true
  host: "localhost"
  port: 3306
  user: "root"
  password: "rootpass"
  database: "michelangelo"
  maxOpenConns: 25
  maxIdleConns: 5
  connMaxLifetime: "5m"

minio:
  enabled: true
  endpoint: "localhost:9000"
  accessKeyID: "minioadmin"
  secretAccessKey: "minioadmin"
  useSSL: false
  bucketName: "michelangelo-blobs"

ingester:
  concurrentReconciles: 2
  requeuePeriod: "30s"

metadataStorage:
  enableMetadataStorage: true
  deletionDelay: "10s"
  enableResourceVersionCache: false
```

---

## Step 3: Build and Run

### Build the Controller Manager

```bash
cd /home/user/Uber/michelangelo
bazel build //go/controllermgr/...
```

### Run the Controller Manager

```bash
# Set kubeconfig
export KUBECONFIG=~/.kube/config

# Run the controller manager
./bazel-bin/go/controllermgr/controllermgr_/controllermgr \
  --config=config/sandbox_ingester.yaml
```

---

## Step 4: Test the Ingester

### Test 1: Create a Model

```bash
# Create a test model
cat <<EOF | kubectl apply -f -
apiVersion: ai.michelangelo/v2beta1
kind: Model
metadata:
  name: test-model-001
  namespace: default
  labels:
    team: ml-platform
    environment: sandbox
spec:
  algorithm: XGBOOST
  trainingFramework: SCIKIT_LEARN
  description: "Test model for ingester validation"
EOF
```

### Verify in Kubernetes

```bash
kubectl get model test-model-001 -n default -o yaml
```

**Check for finalizer:**
```yaml
metadata:
  finalizers:
  - michelangelo/Ingester
```

### Verify in MySQL

```bash
mysql -h localhost -u root -prootpass michelangelo <<EOF
SELECT uid, namespace, name, algorithm, training_framework, create_time
FROM model
WHERE name = 'test-model-001';
EOF
```

**Expected Output:**
```
+------------------+-----------+-----------------+---------+-------------------+---------------------+
| uid              | namespace | name            | algorithm| training_framework| create_time        |
+------------------+-----------+-----------------+---------+-------------------+---------------------+
| abc-123-xyz...   | default   | test-model-001  | XGBOOST | SCIKIT_LEARN      | 2026-01-21 10:30:00|
+------------------+-----------+-----------------+---------+-------------------+---------------------+
```

### Verify Labels

```bash
mysql -h localhost -u root -prootpass michelangelo <<EOF
SELECT * FROM model_labels WHERE obj_uid = (
  SELECT uid FROM model WHERE name = 'test-model-001'
);
EOF
```

---

### Test 2: Update the Model

```bash
kubectl label model test-model-001 version=v2.0 -n default
```

### Verify Update in MySQL

```bash
mysql -h localhost -u root -prootpass michelangelo <<EOF
SELECT update_time FROM model WHERE name = 'test-model-001';
SELECT * FROM model_labels WHERE obj_uid = (
  SELECT uid FROM model WHERE name = 'test-model-001'
);
EOF
```

---

### Test 3: Delete the Model

```bash
kubectl delete model test-model-001 -n default
```

### Monitor Deletion Process

```bash
# Check K8s status (should show Terminating initially)
kubectl get model test-model-001 -n default

# After grace period (~10 seconds), check MySQL
mysql -h localhost -u root -prootpass michelangelo <<EOF
SELECT name, delete_time FROM model WHERE name = 'test-model-001';
EOF
```

**Expected:** `delete_time` should be set (soft delete)

---

### Test 4: List Models from Metadata Storage

```bash
# Use mactl to list models (should query MySQL)
mactl list models -n default

# Or use API directly
curl -X GET http://localhost:8080/api/v2beta1/namespaces/default/models
```

---

## Step 5: Monitor and Debug

### Check Controller Logs

```bash
# Controller logs show reconciliation events
grep "Reconciling object" /var/log/controllermgr.log
grep "Syncing object to metadata storage" /var/log/controllermgr.log
```

### Check MySQL Connections

```bash
mysql -h localhost -u root -prootpass -e "SHOW PROCESSLIST;"
```

### Check MinIO Buckets

```bash
# Access MinIO console
# Open browser: http://localhost:9001
# Login: minioadmin / minioadmin
```

---

## Limitations and Known Issues

### 1. **Partial Implementation**

| Feature | Status | Notes |
|---------|--------|-------|
| MySQL Metadata Storage | ✅ Implemented | Core CRUD operations work |
| MinIO Blob Storage | ✅ Implemented | Basic upload/download implemented |
| Ingester Controller | ✅ Implemented | Watches 5 CRD types: Model, Pipeline, PipelineRun, Dataset, Deployment |
| Finalizer Logic | ✅ Implemented | Graceful deletion with grace period |
| Immutable Objects | ✅ Implemented | Moves objects from ETCD to MySQL only |
| Label Selector Queries | ⚠️ Partial | Simple label queries work, complex selectors need more work |
| Field Selector Queries | ❌ Not Implemented | Need to add indexed field query support |
| Direct Updates | ❌ Not Implemented | Optimistic concurrency control for metadata-only objects |
| Backfill | ❌ Not Implemented | Bulk import of existing objects to MySQL |
| Query Templates | ❌ Not Implemented | Predefined query templates |
| Delete Collection | ❌ Not Implemented | Bulk delete operations |

### 2. **CRD Coverage**

**Currently Watched CRDs:**
- Model
- Pipeline
- PipelineRun
- Dataset
- Deployment

**Not Yet Watched** (but tables exist in schema):
- FeatureGroup
- FeaturePackage
- Endpoint
- Agent
- GenAIService
- etc.

**To Add More CRDs:** Edit `go/components/ingester/module.go` and add to `crdObjects` list.

### 3. **Schema Limitations**

- **Limited Indexed Fields**: Only a few fields per CRD are indexed
  - To add more: Update `scripts/complete_ingester_schema.sql` and add fields to the table
  - All indexed fields are auto-generated from protobuf `GetIndexedKeyValuePairs()` method

- **Fixed Schema**: Schema must be updated manually when CRD spec changes
  - No automatic schema migration
  - Need to write DB migration scripts for schema changes

### 4. **Blob Storage Limitations**

- **No Automatic Upload**: Blob storage upload is not fully integrated
  - Need to implement `ObjectWithBlobFields` interface on CRDs
  - Need to mark which fields should be stored in blob storage

- **No Compression**: Blob data is stored as-is without compression
  - Large models/datasets may consume significant storage

### 5. **Performance Considerations**

- **No Connection Pooling Tuning**: Default connection pool settings used
  - May need tuning for high-throughput scenarios
  - Configure `maxOpenConns` and `maxIdleConns` based on load

- **No Batch Operations**: Objects are synced one at a time
  - May be slow for bulk operations
  - Consider implementing batch upsert for better performance

- **No Caching**: Every query hits MySQL directly
  - No resource version cache
  - May add latency for read-heavy workloads

### 6. **Security Considerations**

- **Credentials in Config File**: MySQL/MinIO credentials in plain text
  - **Sandbox only!**
  - Production should use secrets management (Vault, K8s Secrets)

- **No TLS/SSL**: MySQL and MinIO connections are unencrypted
  - Enable `useSSL: true` for production

- **No Authentication**: API server doesn't validate MySQL queries
  - No row-level security
  - All namespaces accessible to all users

### 7. **Operational Limitations**

- **No Metrics**: Controller doesn't emit metrics
  - Can't monitor ingestion rate, lag, errors
  - Should add Prometheus metrics

- **No Alerts**: No alerting on failures
  - Manual monitoring required

- **No Health Checks**: No readiness/liveness probes
  - Can't detect unhealthy ingester automatically

- **No Graceful Shutdown**: MySQL connections may be abruptly closed
  - Should implement graceful shutdown with connection draining

### 8. **Testing Limitations**

- **No Unit Tests**: Code has no unit tests yet
  - Should add tests for MySQL storage, ingester reconciliation, blob storage

- **No Integration Tests**: No automated end-to-end tests
  - Manual testing required for now

- **No Load Tests**: Unknown behavior under high load
  - Should test with 1000s of objects

### 9. **Dependency on Internal Protos**

- **Uses v2beta1 Protos**: Assumes CRDs implement `proto.Message`
  - All CRDs must be protobuf-based
  - Can't work with CRDs that are YAML-only

- **No Scheme Integration**: `createObjectFromTypeMeta` not fully implemented
  - List operations may not work correctly
  - Need to connect to K8s scheme registry

### 10. **Error Handling**

- **Limited Retry Logic**: Failed reconciliations requeue after 30s
  - No exponential backoff
  - No dead letter queue for persistent failures

- **No Transaction Rollback**: If MySQL upsert succeeds but K8s update fails, state is inconsistent
  - Need proper transaction management across systems

### 11. **Production Readiness**

⚠️ **NOT PRODUCTION READY** - This is a sandbox implementation for testing and development only.

**Missing for Production:**
- [ ] Comprehensive unit and integration tests
- [ ] Performance benchmarking and optimization
- [ ] Metrics and monitoring
- [ ] Alerting and SLOs
- [ ] Security hardening (TLS, secrets management)
- [ ] High availability (leader election, failover)
- [ ] Documentation and runbooks
- [ ] Schema migration strategy
- [ ] Backup and disaster recovery
- [ ] Capacity planning

---

## Troubleshooting

### Issue: Objects Not Syncing to MySQL

**Symptoms:** Objects created in K8s but not appearing in MySQL

**Debug Steps:**
1. Check controller logs for errors
2. Verify MySQL connection: `mysql -h localhost -u root -prootpass michelangelo`
3. Check if metadata storage is enabled in config
4. Verify finalizer is added to object: `kubectl get model <name> -o yaml | grep finalizers`

### Issue: Objects Stuck in Terminating State

**Symptoms:** `kubectl delete` doesn't remove object

**Cause:** Finalizer not removed by ingester

**Fix:**
1. Check ingester controller logs
2. Verify MySQL delete succeeded
3. Manually remove finalizer: `kubectl patch model <name> -p '{"metadata":{"finalizers":[]}}' --type=merge`

### Issue: MySQL Connection Errors

**Symptoms:** `failed to open database connection` in logs

**Fix:**
1. Verify MySQL is running: `docker ps | grep mysql`
2. Test connection: `mysql -h localhost -u root -prootpass`
3. Check firewall rules
4. Verify credentials in config file

### Issue: MinIO Upload Failures

**Symptoms:** `failed to upload to blob storage` in logs

**Fix:**
1. Verify MinIO is running: `docker ps | grep minio`
2. Check MinIO console: http://localhost:9001
3. Verify bucket exists
4. Check MinIO credentials

---

## Next Steps

1. **Add More CRDs**: Update `ingester/module.go` to watch additional CRD types
2. **Implement Blob Storage Integration**: Add `ObjectWithBlobFields` interface to CRDs
3. **Add Metrics**: Instrument controller with Prometheus metrics
4. **Write Tests**: Add unit and integration tests
5. **Optimize Queries**: Add indexes and optimize SQL queries
6. **Production Hardening**: Implement missing production features

---

## Useful Commands

```bash
# Check all models in MySQL
mysql -h localhost -u root -prootpass michelangelo -e "SELECT namespace, name, create_time FROM model ORDER BY create_time DESC LIMIT 10;"

# Count objects by type
mysql -h localhost -u root -prootpass michelangelo -e "
SELECT 'models' as type, COUNT(*) as count FROM model WHERE delete_time IS NULL
UNION ALL
SELECT 'pipelines', COUNT(*) FROM pipeline WHERE delete_time IS NULL
UNION ALL
SELECT 'pipeline_runs', COUNT(*) FROM pipeline_run WHERE delete_time IS NULL;
"

# Find objects with specific label
mysql -h localhost -u root -prootpass michelangelo -e "
SELECT m.namespace, m.name, ml.key, ml.value
FROM model m
JOIN model_labels ml ON m.uid = ml.obj_uid
WHERE ml.key = 'team' AND ml.value = 'ml-platform';
"

# Check ingester finalizers in K8s
kubectl get models -A -o json | jq '.items[] | select(.metadata.finalizers[] == "michelangelo/Ingester") | {name: .metadata.name, namespace: .metadata.namespace}'
```

---

## Support

For issues and questions:
- Check controller logs
- Verify MySQL and MinIO are running
- Review this guide's troubleshooting section
- Check the implementation code for TODOs
