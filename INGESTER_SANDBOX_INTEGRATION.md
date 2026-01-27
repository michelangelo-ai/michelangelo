# Michelangelo Ingester - Integrated Sandbox Guide

## Overview

The Michelangelo Ingester is now **fully integrated** into the sandbox creation process. When you create a sandbox with `ma sandbox create`, the MySQL database is automatically set up with all the necessary tables for the ingester controller.

## ✅ What's Automated

When you run `ma sandbox create`, the following happens automatically:

1. **MySQL Pod** is created with the `michelangelo` database
2. **MySQL Ingester Schema ConfigMap** is created with all CRD tables
3. **MySQL Ingester Init Job** runs automatically to create tables:
   - model, model_labels, model_annotations
   - pipeline, pipeline_labels, pipeline_annotations
   - pipeline_run, pipeline_run_labels, pipeline_run_annotations
   - dataset, dataset_labels, dataset_annotations
   - deployment, deployment_labels, deployment_annotations

## Quick Start

### Step 1: Create Sandbox with Ingester Support

```bash
# Standard sandbox creation - ingester tables are automatically set up
ma sandbox create

# The sandbox will now have MySQL with ingester tables ready!
```

### Step 2: Verify MySQL Schema

```bash
# Port forward to access MySQL (if not already exposed)
kubectl port-forward pod/mysql 3306:3306

# Check tables were created
mysql -h localhost -P 3306 -u root -proot michelangelo -e "SHOW TABLES;"
```

**Expected output:**
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

### Step 3: Test the Ingester

Once the sandbox is running and the controllermgr is built with ingester support:

```bash
# Create a test model
kubectl apply -f - <<EOF
apiVersion: ai.michelangelo/v2beta1
kind: Model
metadata:
  name: test-model
  namespace: default
  labels:
    team: ml-platform
spec:
  algorithm: XGBOOST
EOF

# Verify in MySQL (after ~30 seconds for ingester to sync)
mysql -h localhost -P 3306 -u root -proot michelangelo <<SQL
SELECT namespace, name, algorithm, create_time
FROM model
WHERE name = 'test-model';
SQL
```

## Architecture

```
ma sandbox create
    ↓
Creates MySQL Pod
    ↓
Creates mysql-ingester ConfigMap (with schema SQL)
    ↓
Runs mysql-ingester-init Job
    ↓
Job waits for MySQL to be ready
    ↓
Job executes schema SQL
    ↓
Tables created automatically!
    ↓
Sandbox ready with ingester support ✅
```

## Configuration

### MySQL Connection Details

The sandbox MySQL is accessible via:
- **Host**: `mysql` (within cluster) or `localhost` (via NodePort)
- **Port**: `3306` (internal) or `30001` (NodePort)
- **User**: `root`
- **Password**: `root`
- **Database**: `michelangelo`

### Ingester Configuration

To enable the ingester in your controllermgr, set the following in your config:

```yaml
# config/sandbox.yaml
metadataStorage:
  enableMetadataStorage: true

mysql:
  enabled: true
  host: "mysql"  # Use service name within cluster
  port: 3306
  user: "root"
  password: "root"
  database: "michelangelo"

ingester:
  concurrentReconciles: 2
  requeuePeriod: "30s"
```

## Troubleshooting

### Check Init Job Status

```bash
# Check if init job completed
kubectl get job mysql-ingester-init

# Check init job logs
kubectl logs job/mysql-ingester-init
```

**Expected logs:**
```
Waiting for MySQL to be ready...
MySQL is ready!
Initializing ingester schema...
Schema initialization complete!
```

### Re-run Schema Initialization

If you need to re-initialize the schema:

```bash
# Delete the job
kubectl delete job mysql-ingester-init

# Delete the config map
kubectl delete configmap mysql-ingester-schema

# Recreate
kubectl apply -f python/michelangelo/cli/sandbox/resources/mysql.yaml
```

### Verify Tables Directly

```bash
# Connect to MySQL pod
kubectl exec -it mysql -- mysql -u root -proot michelangelo

# In MySQL shell
mysql> SHOW TABLES;
mysql> DESCRIBE model;
mysql> SELECT COUNT(*) FROM model;
```

## Adding More CRD Tables

To add support for additional CRDs, edit:
`python/michelangelo/cli/sandbox/resources/mysql.yaml`

Add the SQL for your new CRD following the pattern:

```sql
CREATE TABLE IF NOT EXISTS my_crd (
    uid VARCHAR(255) NOT NULL,
    group_ver VARCHAR(255) NOT NULL,
    namespace VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    res_version BIGINT UNSIGNED NOT NULL,
    create_time DATETIME NOT NULL,
    update_time DATETIME,
    delete_time DATETIME,
    proto MEDIUMBLOB,
    json JSON,
    -- Add CRD-specific fields here
    PRIMARY KEY (uid),
    KEY my_crd_namespace_name (namespace, name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS my_crd_labels (...);
CREATE TABLE IF NOT EXISTS my_crd_annotations (...);
```

Then recreate the sandbox or rerun the init job.

## Next Steps

1. ✅ **Sandbox Created** - MySQL tables are automatically set up
2. Build controllermgr with ingester support
3. Deploy controllermgr to sandbox
4. Test create/update/delete flows
5. Query MySQL to verify ingestion

## Integration with Controllermgr

The ingester controller should be configured to connect to the sandbox MySQL:

```go
// In controllermgr startup
if metadataStorageEnabled {
    mysqlConfig := mysql.Config{
        Host:     "mysql",    // K8s service name
        Port:     3306,
        User:     "root",
        Password: "root",
        Database: "michelangelo",
    }

    metadataStorage, err := mysql.NewMetadataStorage(mysqlConfig)
    // ...
}
```

## Benefits of Integration

✅ **No Manual Steps** - Schema setup is fully automated
✅ **Consistent** - Same schema every time
✅ **Version Controlled** - Schema in source control
✅ **Easy Updates** - Update ConfigMap to change schema
✅ **Sandbox Lifecycle** - Schema creation tied to sandbox creation

## File Locations

- **Schema SQL**: `/python/michelangelo/cli/sandbox/resources/mysql.yaml`
- **Sandbox Script**: `/python/michelangelo/cli/sandbox/sandbox.py`
- **Original standalone scripts** (deprecated):
  - `/scripts/setup_mysql_sandbox.sh` - No longer needed
  - `/scripts/mysql_schema.sql` - No longer needed

---

**The ingester schema is now part of the standard sandbox setup! 🎉**
