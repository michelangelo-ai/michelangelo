# Ingester in Sandbox - Automatic Setup

## ✅ What Happens Automatically

When you run `ma sandbox create`, the ingester is **automatically enabled and configured**:

### 1. MySQL Database Created
- **File**: `mysql.yaml`
- **What it does**: Creates MySQL pod and service
- **Database**: `michelangelo` database is auto-created

### 2. Ingester Schema Initialized
- **File**: `mysql-ingester.yaml`
- **What it does**:
  - Creates a ConfigMap with complete auto-generated schema from protobuf
  - Runs a Job that waits for MySQL and applies the schema
  - Schema includes ALL indexed columns from proto annotations

### 3. Controller Manager Configured
- **File**: `michelangelo-controllermgr.yaml`
- **Config already has**:
  ```yaml
  metadataStorage:
    enableMetadataStorage: true  # ✅ Ingester enabled
  mysql:
    host: mysql
    database: michelangelo
    # ... credentials
  ```

### 4. Ingester Controllers Started
- Controller manager automatically registers ingester controllers for:
  - Model
  - ModelFamily
  - Pipeline
  - PipelineRun
  - Deployment
  - InferenceServer
  - Project
  - Revision
  - Cluster
  - RayCluster
  - RayJob
  - SparkJob
  - TriggerRun

## 🎯 No Manual Steps Needed!

Just run:
```bash
export CR_PAT=ghp_your_token_here
ma sandbox create
```

And the ingester will be:
- ✅ Enabled
- ✅ Schema initialized
- ✅ Watching all CRD objects
- ✅ Syncing to MySQL automatically

## 🧪 How to Verify It's Working

### 1. Check Schema Initialization Job
```bash
kubectl get job mysql-ingester-schema-init
kubectl logs job/mysql-ingester-schema-init
```

Expected output: `"Schema applied successfully!"`

### 2. Check Controller Manager Logs
```bash
kubectl logs -l app=michelangelo-controllermgr --tail=50 | grep ingester
```

Expected output:
```
INFO Setting up ingester controllers
INFO Ingester controller registered successfully {"kind": "Model"}
INFO Ingester controller registered successfully {"kind": "Pipeline"}
...
```

### 3. Create a Test Object
```bash
kubectl apply -f - <<EOF
apiVersion: michelangelo.api/v2
kind: Pipeline
metadata:
  name: test-pipeline
  namespace: default
spec:
  owner: "test@example.com"
  pipelineType: "training"
EOF
```

### 4. Verify MySQL Data
```bash
kubectl port-forward pod/mysql 3306:3306 &
mysql -h 127.0.0.1 -u root -proot -D michelangelo -e "
  SELECT namespace, name, owner, pipeline_type 
  FROM pipeline 
  WHERE name='test-pipeline';"
```

Expected: Row with your test pipeline data!

### 5. Check Controller Logs for Success
```bash
kubectl logs -l app=michelangelo-controllermgr | grep "test-pipeline"
```

Expected: `"Successfully synced object to metadata storage"`

## 📋 What Gets Stored in MySQL

For each CRD object, the ingester stores:
- **Main table**: Full object data (protobuf + JSON) + all indexed fields
- **Labels table**: Kubernetes labels as key-value pairs
- **Annotations table**: Kubernetes annotations as key-value pairs

Example for Model:
- `model` table: uid, namespace, name, algorithm, training_framework, owner, etc.
- `model_labels` table: All labels attached to the model
- `model_annotations` table: All annotations attached to the model

## 🔄 Ingester Behavior

The ingester automatically:
1. **On Create/Update**: Syncs object to MySQL
2. **On Delete**: 
   - Waits for grace period (10s)
   - Soft-deletes from MySQL (sets `delete_time`)
   - Removes finalizer
3. **On Immutable Annotation**: Removes object from ETCD, keeps in MySQL only
4. **On Deleting Annotation**: Immediate deletion from both MySQL and ETCD

## 🎉 Bottom Line

**The ingester just works in sandbox!** No configuration, no manual steps, no separate scripts.

Just create the sandbox and start using it.
