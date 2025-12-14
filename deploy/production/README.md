# Michelangelo Production Deployment

This directory contains production-ready Kubernetes configurations for deploying Michelangelo using Kustomize.

## 🏗️ Architecture Overview

Michelangelo consists of the following core components:

- **API Server** - Core gRPC API service for pipeline management
- **Controller Manager** - Kubernetes controller for managing Michelangelo resources
- **Worker** - Workflow execution workers (Temporal/Cadence)
- **UI** - Web interface with Envoy proxy

## 📁 Configuration Structure

```
deploy/production/
├── README.md                    # This file
├── base/                       # Base Kubernetes manifests
│   ├── kustomization.yaml
│   ├── namespace.yaml
│   ├── rbac/
│   ├── apiserver/
│   ├── controllermgr/
│   ├── worker/
│   └── ui/
└── environments/               # Environment-specific configurations
    ├── dev/                   # Development environment
    ├── staging/               # Staging environment
    └── prod/                  # Production environment
```

## 🚀 Quick Start

### Prerequisites

Before deploying Michelangelo, ensure you have:

- **Kubernetes cluster** (>= 1.24)
- **kubectl** installed and configured
- **kustomize** (or use `kubectl kustomize`)
- **External Temporal/Cadence** workflow engine
- **Object storage** (S3-compatible)
- **Database** (MySQL/PostgreSQL) for Temporal/Cadence

### Deploy to Production

```bash
# Clone the repository
git clone https://github.com/michelangelo-ai/michelangelo.git
cd michelangelo

# Deploy to production environment
kubectl apply -k deploy/production/environments/prod
```

### Deploy to Other Environments

```bash
# Deploy to staging
kubectl apply -k deploy/production/environments/staging

# Deploy to development
kubectl apply -k deploy/production/environments/dev
```

## ⚙️ Configuration

### Environment-Specific Settings

Each environment (`dev`, `staging`, `prod`) can have its own:

- Resource limits and requests
- Number of replicas
- Storage configuration
- Database connection details
- Image tags

### Required Secrets

Before deployment, create the following secrets in your target namespace:

```bash
# Database credentials
kubectl create secret generic michelangelo-database \
  --from-literal=host=your-db-host \
  --from-literal=port=3306 \
  --from-literal=database=michelangelo \
  --from-literal=username=michelangelo \
  --from-literal=password=your-password

# Storage credentials
kubectl create secret generic michelangelo-storage \
  --from-literal=endpoint=s3.amazonaws.com \
  --from-literal=region=us-west-2 \
  --from-literal=bucket=your-bucket \
  --from-literal=access_key_id=your-access-key \
  --from-literal=secret_access_key=your-secret-key

# Temporal/Cadence connection
kubectl create secret generic michelangelo-temporal \
  --from-literal=host=temporal.yourdomain.com:7233 \
  --from-literal=domain=michelangelo
```

## 🔧 Customization

### Update Image Versions

Edit `deploy/production/environments/{env}/kustomization.yaml`:

```yaml
images:
  - name: ghcr.io/michelangelo-ai/apiserver
    newTag: v1.2.3
  - name: ghcr.io/michelangelo-ai/controller-manager
    newTag: v1.2.3
  - name: ghcr.io/michelangelo-ai/worker
    newTag: v1.2.3
  - name: ghcr.io/michelangelo-ai/ui
    newTag: v1.2.3
```

### Scale Components

Adjust replica counts in environment kustomization files:

```yaml
replicas:
  - name: michelangelo-apiserver
    count: 3
  - name: michelangelo-worker
    count: 5
```

## 🔍 Verification

After deployment, verify all components are running:

```bash
# Check pod status
kubectl get pods -n michelangelo

# Check services
kubectl get services -n michelangelo

# Test API server health
kubectl port-forward -n michelangelo svc/michelangelo-apiserver 14566:14566
curl http://localhost:14566/health
```

## 🆘 Support

For deployment issues:
- Check the [troubleshooting guide](docs/troubleshooting.md)
- Review [known issues](https://github.com/michelangelo-ai/michelangelo/issues)
- Join the [community discussions](https://github.com/michelangelo-ai/michelangelo/discussions)