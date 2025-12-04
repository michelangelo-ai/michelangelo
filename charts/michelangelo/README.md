# Michelangelo Helm Chart

This Helm chart deploys the Michelangelo ML platform to Kubernetes clusters (GKE or local).

## Prerequisites

- Kubernetes 1.24+
- Helm 3.8+
- kubectl configured with cluster access
- For GKE: `gcloud` CLI configured
- GitHub Personal Access Token (PAT) with `read:packages` scope for pulling container images from GHCR

### Image Pull Secret (Required for GKE)

The container images are hosted on GitHub Container Registry (GHCR) which requires authentication. Before deploying to GKE, create an image pull secret:

```bash
# Create namespace first
kubectl create namespace michelangelo

# Create the secret (replace YOUR_GITHUB_USERNAME, use your CR_PAT token)
kubectl create secret docker-registry ghcr-secret \
  --namespace michelangelo \
  --docker-server=ghcr.io \
  --docker-username=YOUR_GITHUB_USERNAME \
  --docker-password=$CR_PAT
```

This secret is referenced in `values-gke.yaml` and allows GKE nodes to pull private images from GHCR.

> **Note:** For local k3d clusters, the `CR_PAT` is passed during cluster creation and handles authentication automatically.

## Quick Start

### Deploy to Local Cluster (k3d)

```bash
# Install the chart with default values (uses MinIO, MySQL)
helm install michelangelo ./charts/michelangelo \
  --create-namespace \
  --namespace michelangelo
```

### Deploy to GKE

1. **Set up GCP prerequisites:**

```bash
# Set your project
export PROJECT_ID=your-project-id
export REGION=us-central1
export CLUSTER_NAME=michelangelo-cluster

# Create GKE cluster
gcloud container clusters create $CLUSTER_NAME \
  --region $REGION \
  --num-nodes 3 \
  --machine-type n1-standard-4 \
  --enable-autoscaling \
  --min-nodes 3 \
  --max-nodes 10 \
  --enable-workload-identity

# Get cluster credentials
gcloud container clusters get-credentials $CLUSTER_NAME --region $REGION
```

2. **Set up Workload Identity:**

```bash
# Create GCP service account
gcloud iam service-accounts create michelangelo \
  --display-name="Michelangelo Platform"

# Grant permissions (GCS, Cloud SQL, etc.)
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:michelangelo@${PROJECT_ID}.iam.gserviceaccount.com" \
  --role="roles/storage.admin"

# Bind Kubernetes SA to GCP SA
gcloud iam service-accounts add-iam-policy-binding \
  michelangelo@${PROJECT_ID}.iam.gserviceaccount.com \
  --role roles/iam.workloadIdentityUser \
  --member "serviceAccount:${PROJECT_ID}.svc.id.goog[michelangelo/michelangelo]"
```

3. **Create GCS buckets:**

```bash
# Create buckets with project ID prefix for uniqueness (mirrors MinIO sandbox buckets)
gsutil mb -l $REGION gs://${PROJECT_ID}-default
gsutil mb -l $REGION gs://${PROJECT_ID}-deploy-models
gsutil mb -l $REGION gs://${PROJECT_ID}-log-viewer
gsutil mb -l $REGION gs://${PROJECT_ID}-logs
```

4. **Create image pull secret for GHCR:**

```bash
# Create namespace first
kubectl create namespace michelangelo

# Create secret for pulling images from GitHub Container Registry
kubectl create secret docker-registry ghcr-secret \
  --namespace michelangelo \
  --docker-server=ghcr.io \
  --docker-username=YOUR_GITHUB_USERNAME \
  --docker-password=YOUR_GITHUB_PAT \
  --docker-email=YOUR_EMAIL
```

5. **Update GKE values file:**

Edit `values-gke.yaml` and update:
- `gcp.projectID` with your GCP project ID
- `storage.s3.buckets` with your bucket names (prefixed with project ID)
- Domain names and certificate settings (if using ingress)

6. **Install the chart:**

```bash
# Install Michelangelo
helm install michelangelo ./charts/michelangelo \
  -f charts/michelangelo/values-gke.yaml \
  --namespace michelangelo
```

## Configuration

### Key Configuration Options

| Parameter | Description | Default |
|-----------|-------------|---------|
| `cloud` | Cloud provider (gcp or local) | `local` |
| `image.registry` | Container registry | `ghcr.io/michelangelo-ai` |
| `components.apiserver.enabled` | Enable API server | `true` |
| `components.controllermgr.enabled` | Enable controller manager | `true` |
| `components.worker.enabled` | Enable workflow worker | `true` |
| `workflow.engine` | Workflow engine (cadence, temporal) | `cadence` |
| `storage.s3.type` | Storage type (minio, gcs) | `minio` |
| `mysql.enabled` | Deploy MySQL in-cluster | `true` |
| `minio.enabled` | Deploy MinIO in-cluster | `true` |

### Component Configuration

#### API Server
```yaml
apiserver:
  replicaCount: 2
  resources:
    requests:
      cpu: 1000m
      memory: 2Gi
  autoscaling:
    enabled: true
    minReplicas: 2
    maxReplicas: 10
```

#### Controller Manager
```yaml
controllermgr:
  replicaCount: 2
  config:
    leaderElection: true  # Enable for HA
```

#### Storage (GKE)
```yaml
storage:
  storageClass: standard-rwo
  s3:
    type: gcs
    endpoint: https://storage.googleapis.com
    region: us-central1
```

## Upgrade

```bash
# Upgrade existing installation
helm upgrade michelangelo ./charts/michelangelo \
  -f charts/michelangelo/values-gke.yaml \
  --namespace michelangelo

# Upgrade with new image tag
helm upgrade michelangelo ./charts/michelangelo \
  --set image.tag=v1.2.3 \
  --namespace michelangelo
```

## Uninstall

```bash
helm uninstall michelangelo --namespace michelangelo

# Delete namespace (optional)
kubectl delete namespace michelangelo
```

## Accessing Services

### Local Deployment

```bash
# API Server
kubectl port-forward -n michelangelo svc/michelangelo-apiserver 14566:14566

# UI
kubectl port-forward -n michelangelo svc/michelangelo-ui 8090:8090

# Cadence Web
kubectl port-forward -n michelangelo svc/cadence-web 8088:8088
```

### GKE Deployment

Services are exposed via LoadBalancer or Ingress. Get external IPs:

```bash
kubectl get svc -n michelangelo
kubectl get ingress -n michelangelo
```

## Monitoring

### Prometheus

```bash
kubectl port-forward -n michelangelo svc/prometheus 9092:9092
# Access at http://localhost:9092
```

### Grafana

```bash
kubectl port-forward -n michelangelo svc/grafana 3000:3000
# Access at http://localhost:3000
# Default credentials: admin/admin
```

## Troubleshooting

### Check pod status
```bash
kubectl get pods -n michelangelo
kubectl describe pod <pod-name> -n michelangelo
kubectl logs <pod-name> -n michelangelo
```

### Check storage connectivity (GCS)
```bash
kubectl exec -it -n michelangelo deploy/michelangelo-worker -- \
  gsutil ls gs://michelangelo-models
```

### Database connection issues
```bash
# Test MySQL connection
kubectl exec -it -n michelangelo deploy/mysql -- \
  mysql -u root -proot -e "SHOW DATABASES;"
```

### Workflow engine issues
```bash
# Check Cadence
kubectl logs -n michelangelo deploy/cadence
kubectl port-forward -n michelangelo svc/cadence-web 8088:8088
```

## Advanced Configuration

### Using External Databases (Cloud SQL)

```yaml
mysql:
  enabled: false
  external:
    enabled: true
    host: cloudsql-proxy
    port: 3306
    database: michelangelo
```

### GPU Support

```yaml
nodeSelector:
  cloud.google.com/gke-accelerator: nvidia-tesla-t4

worker:
  resources:
    limits:
      nvidia.com/gpu: 1
```

## Contributing

See the main [Michelangelo repository](https://github.com/michelangelo-ai/michelangelo) for contribution guidelines.

## License

See LICENSE file in the repository root.
