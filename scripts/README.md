# Ingester Database Schema

## Files

- **`ingester_schema.sql`** - Complete MySQL schema for all 13 CRDs (39 tables)
- **`ingester_schema_job.yaml`** - Kubernetes Job to initialize schema
- **`init_ingester_db.sh`** - Shell script to initialize schema

## Usage

### Kubernetes (Recommended)

```bash
# Create ConfigMap from SQL file
kubectl create configmap ingester-schema-sql \
  --from-file=ingester_schema.sql=scripts/ingester_schema.sql

# Run the Job
kubectl apply -f scripts/ingester_schema_job.yaml
```

### Shell Script

```bash
export MYSQL_HOST=localhost
export MYSQL_PASSWORD=root
./scripts/init_ingester_db.sh
```

### Direct Execution

```bash
mysql -h localhost -u root -proot < scripts/ingester_schema.sql
```

## What Gets Created

- 39 tables total (13 CRDs × 3 tables each)
- Each CRD has: main table, labels table, annotations table
- Supports: Model, ModelFamily, Pipeline, PipelineRun, Deployment, InferenceServer, Project, Revision, Cluster, RayCluster, RayJob, SparkJob, TriggerRun
