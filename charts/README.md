# Michelangelo Helm Charts

This directory contains Helm charts for deploying Michelangelo ML Platform to Kubernetes.

## Available Charts

- **michelangelo**: Main chart for deploying the complete Michelangelo platform

## Quick Start

### Local Development (k3d)

```bash
# Install with default values (includes MinIO, MySQL, Cadence)
helm install michelangelo ./michelangelo \
  --create-namespace \
  --namespace michelangelo
```

### Google Kubernetes Engine (GKE)

**Prerequisites:** The container images are hosted on GitHub Container Registry (GHCR) which is private. You need a GitHub Personal Access Token (PAT) with `read:packages` scope.

```bash
# 1. Create namespace
kubectl create namespace michelangelo

# 2. Create image pull secret for GHCR authentication
#    Replace YOUR_GITHUB_USERNAME and use your CR_PAT token
kubectl create secret docker-registry ghcr-secret \
  --namespace michelangelo \
  --docker-server=ghcr.io \
  --docker-username=YOUR_GITHUB_USERNAME \
  --docker-password=$CR_PAT

# 3. Deploy using the automated script
./michelangelo/deploy-gke.sh

# Or deploy manually
helm install michelangelo ./michelangelo \
  -f michelangelo/values-gke.yaml \
  --namespace michelangelo
```

> **Note:** The `ghcr-secret` is referenced in `values-gke.yaml` via `imagePullSecrets`. This tells Kubernetes pods to use this secret when pulling images from the private GHCR registry.

## Chart Structure

```
michelangelo/
├── Chart.yaml                  # Chart metadata
├── values.yaml                 # Default configuration (local)
├── values-gke.yaml            # GKE-specific overrides
├── templates/                  # Kubernetes manifests
│   ├── _helpers.tpl           # Template helpers
│   ├── namespace.yaml
│   ├── serviceaccount.yaml
│   ├── configmap.yaml
│   ├── apiserver/             # API server resources
│   ├── controllermgr/         # Controller manager resources
│   ├── worker/                # Workflow worker resources
│   ├── inference/             # Inference server resources
│   └── infrastructure/        # MySQL, MinIO, Cadence, etc.
├── scripts/                    # Helper scripts
│   └── sync-models.sh         # Model synchronization script
├── deploy-gke.sh              # Automated GKE deployment
└── README.md
```

## Configuration

See the chart's README for detailed configuration options:

- [Michelangelo Chart README](./michelangelo/README.md)

## Key Features

### GKE Production Support

- Uses GCS for object storage
- Workload Identity for secure authentication
- Cloud SQL integration
- High Availability with multiple replicas
- LoadBalancer services with GCP annotations
- Persistent storage with GCP Persistent Disks
- Pod topology spread across zones

### Observability

- Prometheus for metrics collection
- Grafana for visualization

### Scalability

- Horizontal Pod Autoscaling
- GPU node support
- KubeRay and Spark operator integration

## Development

### Testing the Chart Locally

```bash
# Lint the chart
helm lint ./michelangelo

# Render templates without installing
helm template michelangelo ./michelangelo

# Dry-run installation
helm install michelangelo ./michelangelo --dry-run --debug

# Install to local k3d cluster
k3d cluster create test-cluster
helm install michelangelo ./michelangelo
```

## Support

For issues or questions:

- GitHub Issues: https://github.com/michelangelo-ai/michelangelo/issues

## License

See LICENSE file in the repository root.
