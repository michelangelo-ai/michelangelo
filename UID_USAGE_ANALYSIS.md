# UID Usage Analysis in Michelangelo Controllers

This document comprehensively lists **every place** where Kubernetes UID is used in Michelangelo, along with the specific use case for each.

---

## Summary

**Total UID Usage Points: 8 major categories across the codebase**

Kubernetes UID (`metav1.ObjectMeta.UID`) is a **globally unique, immutable identifier** assigned by Kubernetes to every object. Unlike `namespace/name`, which can be reused after deletion, UID is **permanent** and critical for:
- Tracking object identity across lifecycle changes
- Preventing naming conflicts after delete/recreate cycles
- Establishing foreign key relationships in relational storage
- Generating collision-free object storage paths

---

## 1. MySQL Metadata Storage - Primary Key

### Location
- `go/storage/mysql/mysql.go:352`
- `scripts/mysql_schema.sql:34` (and 12 other CRD tables)

### Implementation
```go
// In Upsert()
columns := []string{"uid", "group_ver", "namespace", "name", ...}
values := []interface{}{
    string(metaObj.GetUID()),  // PRIMARY KEY
    groupVer,
    metaObj.GetNamespace(),
    metaObj.GetName(),
    // ...
}
```

```sql
CREATE TABLE `model` (
    `uid` VARCHAR(255) NOT NULL COMMENT 'Kubernetes UID',
    -- ... other columns
    PRIMARY KEY (`uid`),  -- UID is the PRIMARY KEY
    KEY `model_namespace_name` (`namespace`, `name`),
)
```

### Use Case
**Primary Key for MySQL Tables**
- UID is the **primary key** for all 13 CRD main tables (`model`, `pipeline`, `pipeline_run`, `deployment`, etc.)
- Ensures **uniqueness** even if objects with the same `namespace/name` are deleted and recreated
- **Prevents race conditions**: If a Model named `prod-model` is deleted and recreated rapidly, the old and new versions have different UIDs, preventing data corruption
- **Example**:
  - Model `default/fraud-model` created → UID: `abc123`
  - Model deleted, then recreated with same name → UID: `xyz789`
  - MySQL correctly stores both versions separately in audit history

**Why Not Use namespace/name as Primary Key?**
- namespace/name can be **reused** after deletion
- UID is **immutable** and never reused across the entire Kubernetes cluster
- Allows tracking full object lifecycle including deletion/recreation cycles

---

## 2. MySQL Labels & Annotations Tables - Foreign Key

### Location
- `go/storage/mysql/mysql.go:135` (labels)
- `go/storage/mysql/mysql.go:141` (annotations)
- `go/storage/mysql/mysql.go:398` (upsertLabels)
- `go/storage/mysql/mysql.go:418` (upsertAnnotations)
- `scripts/mysql_schema.sql` (56 occurrences of `obj_uid`)

### Implementation
```go
// In Upsert() - after inserting main table
err = m.upsertLabels(ctx, tx, tableName, string(metaObj.GetUID()), metaObj.GetLabels())
err = m.upsertAnnotations(ctx, tx, tableName, string(metaObj.GetUID()), metaObj.GetAnnotations())

// In upsertLabels()
func (m *mysqlMetadataStorage) upsertLabels(ctx, tx, tableName string, uid string, labels map[string]string) {
    deleteQuery := fmt.Sprintf("DELETE FROM %s_labels WHERE obj_uid = ?", tableName)
    tx.ExecContext(ctx, deleteQuery, uid)

    insertQuery := fmt.Sprintf("INSERT INTO %s_labels (obj_uid, `key`, `value`) VALUES (?, ?, ?)", tableName)
    for key, value := range labels {
        tx.ExecContext(ctx, insertQuery, uid, key, value)
    }
}
```

```sql
-- For EVERY CRD (Model, Pipeline, etc.), there are 2 related tables:

CREATE TABLE `model_labels` (
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,  -- Foreign key to model.uid
    `key`     VARCHAR(255) NOT NULL,
    `value`   VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY `model_labels_uid` (`obj_uid`),  -- Index on FK
    KEY `model_labels_value` (`key`, `value`)
);

CREATE TABLE `model_annotations` (
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,  -- Foreign key to model.uid
    `key`     VARCHAR(255) NOT NULL,
    `value`   TEXT,
    PRIMARY KEY (`id`),
    KEY `model_annotations_uid` (`obj_uid`)  -- Index on FK
);
```

### Use Case
**Foreign Key Relationship for Labels & Annotations**
- Each CRD has **2 companion tables** for labels and annotations
- `obj_uid` is the **foreign key** linking back to the main table's `uid` primary key
- Total: **13 CRDs × 2 tables = 26 companion tables**, all using `obj_uid`
- Enables **efficient queries** like:
  - "Find all Models with label `env=prod`"
  - "Get all annotations for Pipeline UID `abc123`"
- **Referential integrity**: Labels/annotations are tied to a specific object version by UID

**Example Query Enabled by UID FK:**
```sql
-- Find all Models owned by "alice@uber.com" via labels
SELECT m.*
FROM model m
JOIN model_labels ml ON m.uid = ml.obj_uid
WHERE ml.key = 'owner' AND ml.value = 'alice@uber.com';
```

**Total UID Foreign Key Columns: 56** (from `grep -c obj_uid`)
- 13 CRDs × 2 tables (labels + annotations) × 2 references each (DELETE + INSERT)

---

## 3. MySQL GetByID - Direct Object Retrieval

### Location
- `go/storage/mysql/mysql.go:184-210`
- `go/storage/metadata_storage.go:39-40` (interface definition)

### Implementation
```go
// GetByID retrieves an object by its UID
func (m *mysqlMetadataStorage) GetByID(ctx context.Context, uid string, object runtime.Object) error {
    tableName := getTableName(object)

    query := fmt.Sprintf(`
        SELECT proto
        FROM %s
        WHERE uid = ? AND delete_time IS NULL  -- Query by UID PRIMARY KEY
        LIMIT 1
    `, tableName)

    var protoBytes []byte
    err := m.db.QueryRowContext(ctx, query, uid).Scan(&protoBytes)
    if err == sql.ErrNoRows {
        return fmt.Errorf("object not found with uid: %s", uid)
    }

    // Deserialize and return
    proto.Unmarshal(protoBytes, object)
}
```

### Use Case
**Fast Direct Lookup by UID**
- Retrieves an object when you **only know the UID**, not namespace/name
- Uses **PRIMARY KEY index** → extremely fast O(1) lookup
- Critical for:
  1. **Blob storage deletion** (see #4 below) - needs to fetch full object by UID before deleting blobs
  2. **Cross-references** - when one object references another by UID
  3. **API queries** - `GET /api/v1/objects/{uid}`

**Performance Benefit:**
- `WHERE uid = ?` uses PRIMARY KEY index → **instant lookup**
- `WHERE namespace = ? AND name = ?` uses secondary index → slightly slower

**When is this used?**
- During **finalizer cleanup** in API handlers when deleting objects with blob fields

---

## 4. API Handler - Blob Storage Deletion

### Location
- `go/api/handler/metadata_handler.go:124`
- `go/api/handler/metadata_handler.go:133`

### Implementation
```go
func deleteObjectFromStorage(ctx, object, metadataStorage, blobHandler) error {
    if blobHandler.IsObjectInteresting(object) {
        // Fetch full object by UID to get blob metadata
        getErr := metadataStorage.GetByID(ctx, string(object.GetUID()), object)

        // Delete from metadata storage
        if err := metadataStorage.Delete(ctx, typeMeta, object.GetNamespace(), object.GetName()); err != nil {
            return err
        }

        // Delete blobs from MinIO/S3
        if getErr == nil {
            if err := blobHandler.DeleteFromBlobStorage(ctx, object); err != nil {
                log.Error(err, "Failed to delete blobs", "uid", object.GetUID())
            }
        }
    }
}
```

### Use Case
**Two-Phase Deletion with Blob Cleanup**
1. **Phase 1**: Get full object by UID (includes blob field metadata like URIs)
2. **Phase 2**: Delete from MySQL + delete blob files from MinIO

**Why UID is needed here:**
- When API receives a delete request, it might **only have namespace/name**
- Need to fetch the **full object** (with blob URIs) before deletion
- GetByID uses UID for this fetch
- UID is logged for **audit trail** of blob deletion failures

**Example Flow:**
```
DELETE /api/v1/models/default/large-model
  ↓
1. Get UID from object: "abc123"
2. GetByID("abc123") → fetches full Model with blob_uri: "s3://bucket/model.tar.gz"
3. Delete from MySQL (by namespace/name)
4. Delete "s3://bucket/model.tar.gz" from MinIO
5. Log: "Failed to delete blob, uid: abc123" (for debugging)
```

---

## 5. MinIO Blob Storage - Object Key/Path Generation

### Location
- `go/storage/minio/minio.go:197-205`

### Implementation
```go
// getObjectKey generates the storage key for an object
func (m *minioBlobStorage) getObjectKey(object runtime.Object) string {
    metaObj, _ := meta.Accessor(object)
    gvk := object.GetObjectKind().GroupVersionKind()

    // Format: <group>/<version>/<kind>/<namespace>/<name>/<uid>
    return fmt.Sprintf("%s/%s/%s/%s/%s/%s",
        gvk.Group,       // michelangelo.api
        gvk.Version,     // v2
        gvk.Kind,        // Model
        metaObj.GetNamespace(),  // default
        metaObj.GetName(),       // fraud-detector
        metaObj.GetUID(),        // abc-123-def-456  ← UID for uniqueness
    )
}
```

### Use Case
**Collision-Free Blob Storage Paths**
- Generates **unique paths** in MinIO/S3 for blob objects
- **Prevents overwrite conflicts** when objects are deleted and recreated

**Example Paths:**
```
Without UID (DANGEROUS):
  s3://bucket/michelangelo.api/v2/Model/default/fraud-detector/

  Problem: If Model is deleted and recreated, new version overwrites old blobs!

With UID (SAFE):
  s3://bucket/michelangelo.api/v2/Model/default/fraud-detector/abc-123/model.tar.gz
  s3://bucket/michelangelo.api/v2/Model/default/fraud-detector/xyz-789/model.tar.gz

  ✅ Old version (abc-123) and new version (xyz-789) never conflict
```

**Real-World Scenario:**
1. User creates Model `prod-classifier` → UID: `aaa111`
   - Blobs saved to: `s3://.../prod-classifier/aaa111/weights.pkl`
2. User deletes Model
3. User recreates Model with same name → UID: `bbb222`
   - Blobs saved to: `s3://.../prod-classifier/bbb222/weights.pkl`
4. **No conflict!** Both versions coexist in blob storage

**Additional Benefits:**
- **Audit/Compliance**: Can retain old model blobs even after K8s object is deleted
- **Rollback**: Can restore old model by pointing to old UID path
- **Deduplication**: Same UID → same path → no duplicate uploads

---

## 6. Spark Job Integration - Application ID Tracking

### Location
- `go/components/spark/job/client/client.go:95`
- `go/components/spark/job/client/client.go:131`

### Implementation
```go
// CreateJob - when creating SparkApplication CRD
func (c *Client) CreateJob(ctx, job) error {
    result, err := c.client.Create(ctx, sparkApp)

    // Store SparkApplication UID as the ApplicationId in our custom status
    job.Status.ApplicationId = string(result.UID)
    job.Status.JobUrl = result.Status.DriverInfo.WebUIIngressAddress
}

// GetJobStatus - when polling job status
func (c *Client) GetJobStatus(ctx, job) error {
    result, err := c.client.Get(ctx, name, namespace)

    // Retrieve ApplicationId from UID
    job.Status.ApplicationId = string(result.UID)
    job.Status.JobUrl = result.Status.DriverInfo.WebUIIngressAddress
    job.Status.State = result.Status.AppState.State
}
```

### Use Case
**Cross-System ID Mapping (Michelangelo ↔ Spark Operator)**
- Michelangelo's `Job` CRD wraps the Spark Operator's `SparkApplication` CRD
- Need a **stable, unique identifier** to correlate the two objects
- Uses SparkApplication's UID as the `ApplicationId` in Michelangelo Job status

**Why UID, not name?**
- SparkApplication names are long and unwieldy (e.g., `spark-app-20240205-123456-default`)
- UID is **compact, unique, and immutable**
- Enables tracking across multiple Spark retries/recreations

**Example:**
```yaml
# Michelangelo Job CRD
kind: Job
metadata:
  name: training-job-1
  uid: michelangelo-job-uid-111
status:
  applicationId: "spark-app-uid-222"  ← SparkApplication UID stored here
  jobUrl: "http://spark-ui:4040"
```

**Use Cases:**
1. **Status Sync**: Query SparkApplication by UID to update Job status
2. **Cleanup**: Delete SparkApplication by UID when Job is deleted
3. **Debugging**: Link Spark logs to Michelangelo Job via UID mapping

---

## 7. Ingester Controller - Finalizer Management

### Location
- `go/components/ingester/controller.go:116` (finalizer check)
- `go/components/ingester/controller.go:151` (finalizer removal)
- `go/components/ingester/controller.go:178` (finalizer removal)
- `go/components/ingester/controller.go:210` (finalizer removal)

### Implementation
```go
func (r *Reconciler) Reconcile(ctx, req) (ctrl.Result, error) {
    // Check if finalizer is present
    if !ctrlutil.ContainsFinalizer(object, api.IngesterFinalizer) {
        log.Info("Finalizer not present, nothing to do")
        return ctrl.Result{}, nil
    }

    // ... sync object to MySQL (using UID as primary key) ...

    // Remove finalizer after successful sync
    ctrlutil.RemoveFinalizer(object, api.IngesterFinalizer)
    return ctrl.Result{}, r.Update(ctx, object)
}
```

### Use Case
**Finalizer Lifecycle Tied to UID**
- Finalizers are **part of ObjectMeta** (same level as UID)
- Ingester adds `michelangelo/Ingester` finalizer when object is created
- Before object is deleted from ETCD, ingester must:
  1. **Sync object to MySQL** (using UID as key)
  2. **Remove finalizer** to allow K8s deletion

**UID's Role:**
- When finalizer is removed, object still has the **same UID**
- MySQL record with this UID is marked with `delete_time` (soft delete)
- Ensures **referential integrity** between K8s deletion event and MySQL state

**Timeline:**
```
1. Object created → UID: abc123, Finalizer: [michelangelo/Ingester]
2. User deletes object → K8s marks deletionTimestamp
3. Ingester reconciles:
   - Upserts to MySQL with UID: abc123, delete_time: NOW()
   - Removes finalizer
4. K8s deletes from ETCD → but MySQL still has record with UID: abc123
```

**Why UID matters:**
- Finalizer removal is **not instant** (async reconciliation)
- UID ensures MySQL record is updated for the **correct object version**
- Prevents race: If object is deleted and recreated during finalizer removal, UIDs differ

---

## 8. MySQL Schema Templates - Code Generation

### Location
- `go/kubeproto/templates/mysql_main_table_columns.tmpl`
- `go/kubeproto/templates/mysql_main_table_indices.tmpl`
- `go/cmd/kubeproto/protoc-gen-sql/test/object_expected_output.sql`
- `go/cmd/kubeproto/protoc-gen-sql/test/index_expected_output.sql`

### Implementation
```sql
-- Template: mysql_main_table_columns.tmpl
CREATE TABLE `{{.TableName}}` (
    `uid` VARCHAR(255) NOT NULL,  ← Hardcoded in template
    `group_ver` VARCHAR(255) NOT NULL,
    `namespace` VARCHAR(255) NOT NULL,
    `name` VARCHAR(255) NOT NULL,
    -- ... custom indexed fields from protobuf annotations ...
    `proto` MEDIUMBLOB,
    `json` JSON,
    PRIMARY KEY (`uid`),  ← UID as PRIMARY KEY
    KEY `{{.TableName}}_namespace_name` (`namespace`, `name`),
)
```

### Use Case
**Automated Schema Generation from Protobuf**
- `protoc-gen-sql` plugin generates MySQL schema from `.proto` files
- **UID is always the first column** and **always the primary key**
- Ensures consistency across all 13 CRD tables

**Code Generation Flow:**
```
1. Define CRD in protobuf:
   message Model { ... }

2. Run: bazel build //proto/api/v2:v2_kube_proto_sql

3. Generated SQL:
   CREATE TABLE `model` (
       `uid` VARCHAR(255) NOT NULL,  ← Auto-generated
       -- ... fields from protobuf with [(michelangelo.api.index) = true] ...
       PRIMARY KEY (`uid`),
   )
```

**Why UID is in the template:**
- **Standard Kubernetes pattern**: Every K8s object has UID
- **Enforces best practice**: Always use UID as primary key for object tables
- **Prevents developer mistakes**: Can't accidentally use namespace/name as PK

**Test Coverage:**
- `object_expected_output.sql`: Validates UID column generation
- `index_expected_output.sql`: Validates PRIMARY KEY on UID

---

## Additional UID Properties (Not Directly Used, but Important)

### 9. UID Format and Generation
- **Type**: `types.UID` (string alias in `k8s.io/apimachinery/pkg/types`)
- **Format**: UUID v4 (e.g., `abc12345-def6-7890-ghij-klmnopqrstuv`)
- **Generation**: By Kubernetes API server during object creation
- **Immutability**: Never changes throughout object's lifecycle
- **Uniqueness**: Globally unique across entire cluster and all time

### 10. UID vs Other Identifiers

| Identifier | Mutable? | Unique Across Deletion? | Use Case |
|------------|----------|-------------------------|----------|
| **UID** | ❌ No | ✅ Yes (permanent) | Primary keys, foreign keys, blob paths |
| **Name** | ❌ No | ❌ No (can be reused) | Human-readable identifier |
| **Namespace + Name** | ❌ No | ❌ No (can be reused) | K8s object lookup |
| **ResourceVersion** | ✅ Yes | ❌ No | Optimistic concurrency control |
| **Generation** | ✅ Yes | ❌ No | Spec change tracking |

**Example Illustrating Reuse Problem:**
```yaml
# Create Model
apiVersion: michelangelo.api/v2
kind: Model
metadata:
  name: fraud-detector
  namespace: default
  uid: aaa-111  ← Unique UID
  resourceVersion: "1000"

# Delete Model → UID aaa-111 is gone forever

# Recreate with same name
apiVersion: michelangelo.api/v2
kind: Model
metadata:
  name: fraud-detector  ← Same name
  namespace: default    ← Same namespace
  uid: bbb-222          ← DIFFERENT UID!
  resourceVersion: "2000"
```

**Result:**
- MySQL has **two separate records** (one with UID `aaa-111` marked deleted, one with UID `bbb-222` active)
- MinIO has **two separate blob paths** (`s3://.../aaa-111/` and `s3://.../bbb-222/`)
- Spark ApplicationId mapping is **correct** (old job mapped to `aaa-111`, new job to `bbb-222`)

---

## Summary Table: All UID Usage Points

| # | Location | Purpose | Why UID (not name)? |
|---|----------|---------|---------------------|
| 1 | MySQL main tables (13 tables) | PRIMARY KEY | Uniqueness across delete/recreate cycles |
| 2 | MySQL labels/annotations (26 tables × 2 ops) | FOREIGN KEY (`obj_uid`) | Link labels to correct object version |
| 3 | `mysql.go:GetByID()` | Direct object retrieval | Fast O(1) lookup by primary key |
| 4 | `metadata_handler.go:deleteObjectFromStorage()` | Blob cleanup before deletion | Need full object with blob URIs |
| 5 | `minio.go:getObjectKey()` | Blob storage path | Prevent blob path collisions |
| 6 | `spark/job/client.go` | ApplicationId tracking | Stable ID for cross-system correlation |
| 7 | `ingester/controller.go` | Finalizer management | Ensure correct object version synced |
| 8 | `protoc-gen-sql` templates | Schema generation | Enforce UID-as-PK pattern |

**Total UID References in Codebase:**
- **Direct UID access**: ~15 locations in Go code
- **obj_uid foreign keys**: 56 SQL operations (26 tables × 2 queries)
- **PRIMARY KEY on uid**: 13 CRD tables
- **Schema templates**: 4 template/test files

---

## Key Design Decisions: Why UID Everywhere?

### Decision 1: UID as MySQL Primary Key (Not namespace/name)
**Rationale:**
- Kubernetes allows deleting and recreating objects with the same namespace/name
- Using namespace/name as PK would cause **primary key conflicts**
- UID ensures **each object version** gets a separate row in MySQL

**Alternative Considered:** Composite key `(namespace, name, resource_version)`
**Rejected Because:** Resource version can be reset; UID is permanent

### Decision 2: UID in Blob Storage Paths
**Rationale:**
- S3/MinIO doesn't support versioning by default
- Including UID in path **automatically versions** blob objects
- Simplifies cleanup: Delete by UID → no orphaned blobs

**Alternative Considered:** Include timestamp in path
**Rejected Because:** Timestamps can collide; UID is guaranteed unique

### Decision 3: UID for Foreign Keys (Not name)
**Rationale:**
- Labels/annotations must link to a **specific object version**
- If object is recreated, old labels should not appear on new version
- UID provides **referential integrity**

**Alternative Considered:** Delete labels/annotations when main object is deleted
**Rejected Because:** Breaks audit trail; can't query deleted object's labels

---

## Potential Issues if UID is Not Used

### Issue 1: MySQL Primary Key Conflicts
**Scenario:**
```yaml
1. Create Model default/classifier → UID: aaa
2. Insert into MySQL: namespace='default', name='classifier', uid='aaa'
3. Delete Model
4. Recreate Model default/classifier → UID: bbb
5. Insert into MySQL: namespace='default', name='classifier', uid='bbb'
```

**If PK was namespace/name:**
- ❌ **Primary key violation** on step 5 (unless old row is hard-deleted)
- ❌ Loses audit trail (can't keep both versions)

**With PK as UID:**
- ✅ Both rows coexist (old with `delete_time`, new without)
- ✅ Full audit trail preserved

### Issue 2: Blob Storage Overwrites
**Scenario:**
```
1. Model default/fraud-model → UID: aaa → Save blob to s3://bucket/default/fraud-model/model.pkl
2. Delete Model
3. Recreate Model → UID: bbb → Save blob to s3://bucket/default/fraud-model/model.pkl
```

**Without UID in path:**
- ❌ New blob **overwrites** old blob
- ❌ Can't rollback to previous model version

**With UID in path:**
- ✅ Old: `s3://bucket/default/fraud-model/aaa/model.pkl`
- ✅ New: `s3://bucket/default/fraud-model/bbb/model.pkl`
- ✅ Both versions retained

### Issue 3: Finalizer Race Conditions
**Scenario:**
```
1. Object created → UID: aaa, Finalizer: [ingester]
2. User deletes object
3. Ingester starts syncing to MySQL
4. K8s recreates object with same name → UID: bbb
5. Ingester completes sync → Which UID does it use?
```

**Without UID tracking:**
- ❌ Might sync new object (bbb) but remove finalizer from old object (aaa)
- ❌ Finalizer stuck, deletion blocked

**With UID tracking:**
- ✅ Ingester syncs UID aaa, removes finalizer from aaa
- ✅ New object (bbb) gets its own finalizer lifecycle

---

## Recommendations for Future Development

### 1. Add Foreign Key Constraints
**Current State:** `obj_uid` in labels/annotations tables is just a VARCHAR, no FK constraint

**Recommendation:**
```sql
ALTER TABLE model_labels
ADD CONSTRAINT fk_model_labels_uid
FOREIGN KEY (obj_uid) REFERENCES model(uid)
ON DELETE CASCADE;
```

**Benefits:**
- **Referential integrity** enforced by database
- **Cascading deletes** automatically clean up labels/annotations
- **Query optimization** via FK indexes

### 2. Add UID Index to JSON Column
**Current State:** `json` column stores full object, but UID is not indexed

**Recommendation:**
```sql
ALTER TABLE model
ADD INDEX idx_model_json_uid ((JSON_UNQUOTE(JSON_EXTRACT(json, '$.metadata.uid'))));
```

**Benefits:**
- Query objects by UID even when `proto` column is not available
- Faster JSON-based queries

### 3. Add UID to Audit Logs
**Current State:** Logs use namespace/name for object references

**Recommendation:**
```go
log.Info("Object synced",
    "namespace", obj.GetNamespace(),
    "name", obj.GetName(),
    "uid", obj.GetUID(),  // Add this
)
```

**Benefits:**
- **Unambiguous log correlation** across delete/recreate cycles
- **Easier debugging** of finalizer issues

### 4. Expose UID in API Responses
**Current State:** REST API returns namespace/name, but not UID

**Recommendation:**
```json
GET /api/v1/models/default/fraud-detector
{
  "metadata": {
    "name": "fraud-detector",
    "namespace": "default",
    "uid": "abc-123-def-456",  // Add this
    "resourceVersion": "1000"
  }
}
```

**Benefits:**
- Clients can use UID for direct lookups (`GET /api/v1/objects/{uid}`)
- Avoids ambiguity if object is recreated during API call

---

## Conclusion

**UID is the cornerstone of Michelangelo's metadata storage architecture.**

It's used in **8 critical places**:
1. MySQL primary keys (13 tables)
2. MySQL foreign keys (26 tables, 56 operations)
3. Direct object retrieval
4. Blob storage deletion
5. Blob storage path generation
6. Spark job correlation
7. Finalizer lifecycle management
8. Code generation templates

**Why UID?**
- **Immutable**: Never changes, unlike resourceVersion
- **Unique**: Globally unique across all time, unlike namespace/name
- **Permanent**: Never reused, even after deletion
- **Fast**: Enables O(1) primary key lookups in MySQL

**Without UID, Michelangelo would have:**
- ❌ Primary key conflicts on object recreation
- ❌ Blob storage overwrites
- ❌ Broken audit trails
- ❌ Finalizer race conditions
- ❌ Label/annotation mismatches

**UID is not just an identifier—it's the foundation of object identity in a distributed system.**
